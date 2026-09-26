package protocol

import "slices"

// A card's diff (docs/backend-checklist.md B2.9, docs/backend-inventory.md N15, section 4.1). It
// is what the card's Diff tab draws: the changed files with their counts, and, when a person opens
// one file, that file's hunks.
//
// The whole list is never sent with hunks in it. A card can change thousands of files and a
// generated file can carry thousands of lines, so the list route answers with counts alone and one
// file's hunks come from their own route (FileHunks), which the screen asks for when the file is
// opened. A file whose hunks are large is marked Large in the list, and the screen keeps it
// collapsed until it is asked for.
//
// This is not the chat's diff summary (ChatDiffSummary, history.go). That one is a stored fact
// about one turn - how many files and lines that turn changed - and the chat keeps carrying it.
// What is here is the card's live diff: its worktree against the branch it started from. The two
// answer different questions and can disagree (a turn's summary never grows, the live diff does).

// DiffFileStatus says what happened to one file of a card's diff. The words are Git's own, so a
// status a person reads elsewhere in Git means the same here.
type DiffFileStatus string

const (
	// DiffFileStatusAdded is a file the card created.
	DiffFileStatusAdded DiffFileStatus = "added"
	// DiffFileStatusModified is a file the card changed.
	DiffFileStatusModified DiffFileStatus = "modified"
	// DiffFileStatusDeleted is a file the card removed.
	DiffFileStatusDeleted DiffFileStatus = "deleted"
	// DiffFileStatusRenamed is a file the card moved, possibly changing it as well. Its old path
	// is in ChangedFile.OldPath.
	DiffFileStatusRenamed DiffFileStatus = "renamed"
)

// DiffFileStatusValues lists every status a changed file can have.
func DiffFileStatusValues() []DiffFileStatus {
	return []DiffFileStatus{
		DiffFileStatusAdded, DiffFileStatusModified, DiffFileStatusDeleted, DiffFileStatusRenamed,
	}
}

// Valid reports whether s is a status of a changed file.
func (s DiffFileStatus) Valid() bool { return slices.Contains(DiffFileStatusValues(), s) }

// DiffLineKind says which side of a change one line of a hunk is on.
type DiffLineKind string

const (
	// DiffLineKindContext is a line both sides have, drawn unchanged.
	DiffLineKindContext DiffLineKind = "context"
	// DiffLineKindAdded is a line only the card's side has, drawn with a plus.
	DiffLineKindAdded DiffLineKind = "added"
	// DiffLineKindRemoved is a line only the base side has, drawn with a minus.
	DiffLineKindRemoved DiffLineKind = "removed"
)

// DiffLineKindValues lists every kind of line a hunk can carry.
func DiffLineKindValues() []DiffLineKind {
	return []DiffLineKind{DiffLineKindContext, DiffLineKindAdded, DiffLineKindRemoved}
}

// Valid reports whether k is a kind of diff line.
func (k DiffLineKind) Valid() bool { return slices.Contains(DiffLineKindValues(), k) }

// CardDiff is the answer to GET /v1/cards/{id}/diff: every file the card changed, with its counts
// and without any hunks. The files are in path order, so two answers of the same diff read the
// same, and the list is bounded: Files holds at most the daemon's cap and Truncated says when the
// rest was left out. FileCount, Additions, and Deletions describe the whole diff, not only the
// files that came back, so a screen can say "showing 500 of 1,203 files" and still show the right
// totals.
type CardDiff struct {
	// CardID is the card whose worktree this is.
	CardID string `json:"cardId"`
	// Base is the branch the card's work was compared against: its project's default branch. The
	// comparison starts at the merge base, so work that landed on the default branch after the
	// card started is not drawn as the card's own change.
	Base string `json:"base"`
	// Branch is the card's own branch. It is empty for a card that never started.
	Branch string `json:"branch"`
	// Files are the changed files, in path order, with their counts and no hunks. Never null.
	Files []ChangedFile `json:"files"`
	// FileCount is how many files the whole diff has, including any that Truncated left out.
	FileCount int `json:"fileCount"`
	// Additions is how many lines the whole diff added.
	Additions int `json:"additions"`
	// Deletions is how many lines the whole diff removed.
	Deletions int `json:"deletions"`
	// Truncated is true when Files was cut short. A client that shows the whole list then shows
	// the cap instead of FileCount.
	Truncated bool `json:"truncated"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// ChangedFile is one file of a card's diff, as the list draws it: its path, what happened to it,
// and how many lines it gained and lost. It carries no hunks, so a diff of thousands of files is
// still a small answer.
type ChangedFile struct {
	// Path is the file's path, relative to the repository, with forward slashes on every
	// platform.
	Path string `json:"path"`
	// OldPath is the path the file had before a rename, and empty for every other status.
	OldPath string `json:"oldPath"`
	// Status is what happened to the file.
	Status DiffFileStatus `json:"status"`
	// Additions is how many lines the file gained.
	Additions int `json:"additions"`
	// Deletions is how many lines the file lost.
	Deletions int `json:"deletions"`
	// Binary is true when the file is not text, so it has counts of nothing and no hunks.
	Binary bool `json:"binary"`
	// Large is true when the file has more changed lines than the daemon draws in one answer. The
	// screen keeps such a file collapsed until a person asks for it, and the hunks route bounds
	// what it sends even then.
	Large bool `json:"large"`
}

// FileHunks is the answer to GET /v1/cards/{id}/diff/{path}: one changed file's hunks, loaded when
// the screen opens it (N15). Hunks is bounded: a file with more than the daemon's cap of lines
// comes back cut, with Truncated set, so one very large file cannot stall the screen.
type FileHunks struct {
	// Path is the file these hunks belong to.
	Path string `json:"path"`
	// Status is what happened to the file, so the header of an opened file is drawn from the same
	// words as the list.
	Status DiffFileStatus `json:"status"`
	// Hunks are the file's hunks, in the order Git gives them. Never null.
	Hunks []DiffHunk `json:"hunks"`
	// Truncated is true when the hunks were cut short at the daemon's line cap.
	Truncated bool `json:"truncated"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// DiffHunk is one hunk of a file: the line the view draws above the lines, and the lines.
type DiffHunk struct {
	// Header is the hunk's own header, such as "@@ -41,18 +41,35 @@ type Client struct". It
	// carries the ranges and, when Git wrote one, the enclosing line's text.
	Header string `json:"header"`
	// Lines are the hunk's lines, in order. Never null.
	Lines []DiffLine `json:"lines"`
}

// DiffLine is one line of a hunk. Kind says which side it is on; the other side's line number is
// zero, so a client draws one number for a context line and one for an added or removed line
// without guessing which column it belongs in.
type DiffLine struct {
	// Kind is which side of the change this line belongs to.
	Kind DiffLineKind `json:"kind"`
	// OldLine is the line's number in the base file, counting from 1, or 0 for an added line.
	OldLine int `json:"oldLine"`
	// NewLine is the line's number in the card's file, counting from 1, or 0 for a removed line.
	NewLine int `json:"newLine"`
	// Text is the line itself, without its newline and without the sign a client draws.
	Text string `json:"text"`
}
