package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Reading a diff. Every diff of a card is read here, in three parts, so the rest of the daemon
// never runs Git itself:
//
//   - DiffFiles lists what changed, with each file's status and its line counts, and no content.
//     It compares the branch the caller names with the working tree, so uncommitted work counts,
//     and it adds the files Git does not track yet, because an agent writes a new file far more
//     often than it commits one.
//   - DiffHunks returns one file's hunks, for the screen that opened that file.
//   - Both are bounded. A diff is a user's repository and can be enormous: a generated file alone
//     can carry hundreds of thousands of lines. Every command here has a byte cap and every answer
//     has a line cap, and what was left out is reported instead of silently dropped.

const (
	// DefaultDiffBytes is how much of one Git diff command's output is read before the rest is
	// dropped. It is a backstop, not the usual limit: the caps that shape an answer are the
	// caller's.
	DefaultDiffBytes = 4 << 20
	// untrackedCountBytes is how much of an untracked file is read to count its lines. A file
	// larger than this is counted up to the cap; the caller marks it large either way.
	untrackedCountBytes = 4 << 20
	// binarySniffBytes is how much of a file is looked at for a NUL byte, which is how Git itself
	// decides that a file is not text.
	binarySniffBytes = 8000
	// hunkHeaderPrefix starts a hunk of a unified diff.
	hunkHeaderPrefix = "@@"
	// The header lines that say what happened to a file. Every other changed file changed in place.
	newFileMode     = "new file mode "
	deletedFileMode = "deleted file mode "
	renameFrom      = "rename from "
	// noNewlineMarker starts the line Git writes when a file does not end with a newline.
	noNewlineMarker = `\ No newline at end of file`
)

// DiffStatus says what happened to one file of a diff. It is this package's own small list, so
// gitx stays a leaf that depends on nothing; the wire's list is protocol.DiffFileStatus.
type DiffStatus string

const (
	// DiffStatusAdded is a file that is in the working tree and not in the base revision.
	DiffStatusAdded DiffStatus = "added"
	// DiffStatusModified is a file that changed.
	DiffStatusModified DiffStatus = "modified"
	// DiffStatusDeleted is a file that is in the base revision and not in the working tree.
	DiffStatusDeleted DiffStatus = "deleted"
	// DiffStatusRenamed is a file that moved. OldPath holds where it was.
	DiffStatusRenamed DiffStatus = "renamed"
)

// DiffFile is one file of a diff, with its counts and no content.
type DiffFile struct {
	// Path is the file's path relative to the repository, with forward slashes.
	Path string
	// OldPath is where the file was before a rename, and empty otherwise.
	OldPath string
	// Status is what happened to the file.
	Status DiffStatus
	// Additions and Deletions are the line counts Git reports. Both are zero for a binary file.
	Additions int
	Deletions int
	// Binary is true for a file Git does not read as text.
	Binary bool
}

// DiffLineKind says which side of a change one line of a hunk is on.
type DiffLineKind string

const (
	// DiffLineContext is a line both sides have.
	DiffLineContext DiffLineKind = "context"
	// DiffLineAdded is a line only the working tree has.
	DiffLineAdded DiffLineKind = "added"
	// DiffLineRemoved is a line only the base revision has.
	DiffLineRemoved DiffLineKind = "removed"
)

// DiffLine is one line of a hunk. The side a line is not on has a line number of zero.
type DiffLine struct {
	Kind    DiffLineKind
	OldLine int
	NewLine int
	Text    string
}

// DiffHunk is one hunk of a file, with the header line the view draws above it.
type DiffHunk struct {
	Header string
	Lines  []DiffLine
}

// DiffFileHunks is one file's hunks and what happened to the file.
type DiffFileHunks struct {
	// Status is what happened to the file, read from the diff's own header.
	Status DiffStatus
	// Hunks are the file's hunks, in the order Git gives them. Never nil.
	Hunks []DiffHunk
	// Truncated is true when the byte cap or the caller's line cap cut the answer short.
	Truncated bool
}

// DiffFiles lists what changed between base and the working tree at dir, in path order, with each
// file's status and counts. dir must be the top folder of the working tree. maxBytes caps how much
// of each Git command's output is read; zero means DefaultDiffBytes. The second return value says
// whether that cap cut the list short.
//
// The comparison starts at the merge base of base and the checked out commit, and ends at the
// working tree (Git's --merge-base). So work that landed on the base branch after the card
// started is not drawn as the card's change, and work that is staged or merely edited is drawn.
// Files Git does not track yet are added as DiffStatusAdded, because an agent writes a new file
// long before it commits one. Ignored files are never listed.
func (g *Git) DiffFiles(ctx context.Context, dir, base string, maxBytes int) ([]DiffFile, bool, error) {
	if err := checkRevision(base); err != nil {
		return nil, false, err
	}
	files, truncated, err := g.diffNameStatus(ctx, dir, base, maxBytes)
	if err != nil {
		return nil, false, err
	}
	counts, countsCut, err := g.diffCounts(ctx, dir, base, maxBytes)
	if err != nil {
		return nil, false, err
	}
	truncated = truncated || countsCut
	for i := range files {
		if count, ok := counts[files[i].Path]; ok {
			files[i].Additions, files[i].Deletions, files[i].Binary = count.Additions, count.Deletions, count.Binary
		}
	}
	untracked, err := g.untrackedFiles(ctx, dir)
	if err != nil {
		return nil, false, err
	}
	files = append(files, untracked...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, truncated, nil
}

// diffNameStatus reads what happened to each file: `git diff --name-status`.
func (g *Git) diffNameStatus(ctx context.Context, dir, base string, maxBytes int) ([]DiffFile, bool, error) {
	out, truncated, err := g.runCapped(ctx, dir, maxBytes,
		"--no-optional-locks", "diff", "--name-status", "-z", "-M", "--no-color", "--no-ext-diff",
		"--merge-base", base)
	if err != nil {
		return nil, false, fmt.Errorf("list the changed files: %w", err)
	}
	files := []DiffFile{}
	fields := strings.Split(completeRecords(out, truncated), "\x00")
	for i := 0; i < len(fields); {
		code := fields[i]
		i++
		if code == "" {
			continue
		}
		status, renamed := diffStatusOf(code)
		if i >= len(fields) || fields[i] == "" {
			break // the byte cap cut the last record in half
		}
		first := filepath.ToSlash(fields[i])
		i++
		file := DiffFile{Status: status, Path: first}
		if renamed && i < len(fields) {
			// A rename is written as the old path and then the new one.
			file.OldPath, file.Path = first, filepath.ToSlash(fields[i])
			i++
		}
		files = append(files, file)
	}
	return files, truncated, nil
}

// completeRecords drops the record a byte cap left half-written, so half a path never reaches a
// caller. Git's NUL-separated records make it exact: everything up to the last NUL is whole.
func completeRecords(out string, truncated bool) string {
	if !truncated || strings.HasSuffix(out, "\x00") {
		return out
	}
	cut := strings.LastIndex(out, "\x00")
	if cut < 0 {
		return ""
	}
	return out[:cut+1]
}

// diffStatusOf turns Git's status letter into one of this package's statuses.
func diffStatusOf(code string) (status DiffStatus, renamed bool) {
	switch code[0] {
	case 'A':
		return DiffStatusAdded, false
	case 'D':
		return DiffStatusDeleted, false
	case 'R':
		return DiffStatusRenamed, true
	default:
		// M is a change, T a change of file type, and U an unresolved merge; all are drawn as a
		// change. C (a copy) only appears when a person's own Git config asks for one, and a copy
		// is a new file.
		return DiffStatusModified, false
	}
}

// diffCount is one file's line counts.
type diffCount struct {
	Additions int
	Deletions int
	Binary    bool
}

// diffCounts reads the line counts of each changed file: `git diff --numstat`.
func (g *Git) diffCounts(ctx context.Context, dir, base string, maxBytes int) (map[string]diffCount, bool, error) {
	out, truncated, err := g.runCapped(ctx, dir, maxBytes,
		"--no-optional-locks", "diff", "--numstat", "-z", "-M", "--no-color", "--no-ext-diff",
		"--merge-base", base)
	if err != nil {
		return nil, false, fmt.Errorf("count the changed lines: %w", err)
	}
	counts := map[string]diffCount{}
	fields := strings.Split(completeRecords(out, truncated), "\x00")
	for i := 0; i < len(fields); {
		record := fields[i]
		i++
		parts := strings.Split(record, "\t")
		if len(parts) < 3 {
			continue
		}
		path := strings.Join(parts[2:], "\t")
		if path == "" {
			// A rename writes the counts, an empty path, and then the old and the new path as
			// their own fields. The counts belong to the new path.
			if i+1 >= len(fields) {
				break
			}
			path, i = fields[i+1], i+2
		}
		counts[filepath.ToSlash(path)] = diffCount{
			Additions: diffNumber(parts[0]), Deletions: diffNumber(parts[1]), Binary: parts[0] == "-",
		}
	}
	return counts, truncated, nil
}

// diffNumber reads one of numstat's two numbers. A binary file writes a dash instead.
func diffNumber(text string) int {
	number, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || number < 0 {
		return 0
	}
	return number
}

// untrackedFiles lists the files Git does not track, with the line count of each, and calls them
// added. Ignored files are left out by Git itself.
func (g *Git) untrackedFiles(ctx context.Context, dir string) ([]DiffFile, error) {
	out, err := g.Run(ctx, dir, "--no-optional-locks", "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("list the new files: %w", err)
	}
	files := []DiffFile{}
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		record := fields[i]
		if len(record) < 4 {
			continue
		}
		code := record[:2]
		if strings.ContainsAny(code, "RC") {
			i++ // a rename or a copy carries the original path as the next field
			continue
		}
		if code != "??" {
			continue
		}
		path := filepath.ToSlash(record[3:])
		file := DiffFile{Path: path, Status: DiffStatusAdded}
		file.Additions, file.Binary = countFileLines(filepath.Join(dir, filepath.FromSlash(path)))
		files = append(files, file)
	}
	return files, nil
}

// countFileLines counts the lines of a file that Git does not track, and says whether it looks
// binary. It reads at most untrackedCountBytes, so a very large file is counted up to that cap;
// the caller marks such a file large, and the screen keeps it collapsed.
func countFileLines(path string) (lines int, binary bool) {
	data, _, err := readFileCapped(path, untrackedCountBytes)
	if err != nil {
		return 0, false
	}
	if looksBinary(data) {
		return 0, true
	}
	return dataLines(data), false
}

// looksBinary reports whether data holds a NUL byte near its start, the way Git decides that a file
// is not text.
func looksBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), binarySniffBytes)], 0) >= 0
}

// dataLines counts the lines in a file's contents: one per newline, plus a last line with no
// newline of its own.
func dataLines(data []byte) int {
	lines := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}

// DiffHunks returns one file's hunks, read from base to the working tree at dir. maxLines caps how
// many lines the answer carries and maxBytes caps how much of the command's output is read; zero
// means DefaultDiffBytes for the bytes and no line cap. The second return value is false when the
// file is not part of the diff at all, which is how a caller tells a file that changed from a path
// that names nothing.
//
// The path is passed to Git as a literal pathspec, so a file whose name looks like a pattern or a
// magic word is read as the name it is and can never name another file.
func (g *Git) DiffHunks(ctx context.Context, dir, base, path string, maxLines, maxBytes int) (DiffFileHunks, bool, error) {
	if err := checkRevision(base); err != nil {
		return DiffFileHunks{}, false, err
	}
	if err := checkDiffPath(path); err != nil {
		return DiffFileHunks{}, false, err
	}
	out, cut, err := g.runCapped(ctx, dir, maxBytes,
		"--no-optional-locks", "diff", "--no-color", "--no-ext-diff", "--no-textconv",
		"--unified=3", "-M", "--merge-base", base, "--", ":(literal)"+path)
	if err != nil {
		return DiffFileHunks{}, false, fmt.Errorf("read the diff of %s: %w", path, err)
	}
	if strings.TrimSpace(out) == "" {
		// Git says nothing about a file that did not change. It says nothing about a new file
		// either, because a new file is not in the base revision: those are read from disk.
		return g.untrackedHunks(ctx, dir, path, maxLines, maxBytes)
	}
	file := parseUnifiedDiff(out)
	file.Hunks, file.Truncated = cutHunks(file.Hunks, maxLines)
	file.Truncated = file.Truncated || cut
	return file, true, nil
}

// untrackedHunks builds the hunks of a file Git does not track yet: every line is an added line,
// the way a diff against an empty file would draw it. It answers found=false when the path is not
// an untracked file either, so an unchanged path and a path that is not there read the same to a
// caller.
func (g *Git) untrackedHunks(ctx context.Context, dir, path string, maxLines, maxBytes int) (DiffFileHunks, bool, error) {
	status, err := g.Run(ctx, dir, "--no-optional-locks", "status", "--porcelain", "-z",
		"--untracked-files=all", "--", ":(literal)"+path)
	if err != nil {
		return DiffFileHunks{}, false, fmt.Errorf("look for %s: %w", path, err)
	}
	if !strings.HasPrefix(status, "?? ") {
		return DiffFileHunks{}, false, nil
	}
	data, cut, err := readFileCapped(filepath.Join(dir, filepath.FromSlash(path)), maxBytes)
	if err != nil {
		return DiffFileHunks{}, false, fmt.Errorf("read the new file %s: %w", path, err)
	}
	if looksBinary(data) {
		return DiffFileHunks{Status: DiffStatusAdded, Hunks: []DiffHunk{}}, true, nil
	}
	hunks, lineCut := cutHunks(addedHunks(data), maxLines)
	return DiffFileHunks{Status: DiffStatusAdded, Hunks: hunks, Truncated: cut || lineCut}, true, nil
}

// addedHunks is a new file as one hunk in which every line is added.
func addedHunks(data []byte) []DiffHunk {
	if len(data) == 0 {
		return []DiffHunk{}
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	hunk := DiffHunk{Header: fmt.Sprintf("@@ -0,0 +1,%d @@", len(lines)), Lines: make([]DiffLine, 0, len(lines))}
	for i, text := range lines {
		hunk.Lines = append(hunk.Lines, DiffLine{Kind: DiffLineAdded, NewLine: i + 1, Text: text})
	}
	return []DiffHunk{hunk}
}

// parseUnifiedDiff reads the hunks of a unified diff into structs, with the status of the file
// taken from the diff's own header.
func parseUnifiedDiff(out string) DiffFileHunks {
	file := DiffFileHunks{Status: DiffStatusModified, Hunks: []DiffHunk{}}
	inside := -1
	oldLine, newLine := 0, 0
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, newFileMode):
			file.Status = DiffStatusAdded
		case strings.HasPrefix(line, deletedFileMode):
			file.Status = DiffStatusDeleted
		case strings.HasPrefix(line, renameFrom):
			file.Status = DiffStatusRenamed
		case strings.HasPrefix(line, hunkHeaderPrefix):
			start, ok := hunkHeaderOf(line)
			if !ok {
				inside = -1
				continue
			}
			file.Hunks = append(file.Hunks, DiffHunk{Header: line, Lines: []DiffLine{}})
			inside = len(file.Hunks) - 1
			oldLine, newLine = start.oldLine, start.newLine
		case inside < 0, strings.HasPrefix(line, noNewlineMarker):
			// The file's own header, or the note about a missing final newline: neither is a line.
			continue
		default:
			text, kind, numbers := splitDiffLine(line, oldLine, newLine)
			if kind == "" {
				continue
			}
			file.Hunks[inside].Lines = append(file.Hunks[inside].Lines,
				DiffLine{Kind: kind, OldLine: numbers.oldLine, NewLine: numbers.newLine, Text: text})
			oldLine, newLine = numbers.nextOld, numbers.nextNew
		}
	}
	return file
}

// hunkStart is where a hunk's two sides begin: the numbers in its header.
type hunkStart struct {
	oldLine int
	newLine int
}

// hunkHeaderOf reads the two ranges of a hunk header, such as `@@ -41,18 +41,35 @@ type Client`.
func hunkHeaderOf(line string) (hunkStart, bool) {
	end := strings.Index(line[len(hunkHeaderPrefix):], hunkHeaderPrefix)
	if end < 0 {
		return hunkStart{}, false
	}
	ranges := strings.Fields(line[len(hunkHeaderPrefix) : len(hunkHeaderPrefix)+end])
	if len(ranges) != 2 {
		return hunkStart{}, false
	}
	oldLine, ok := hunkNumber(ranges[0])
	if !ok {
		return hunkStart{}, false
	}
	newLine, ok := hunkNumber(ranges[1])
	if !ok {
		return hunkStart{}, false
	}
	return hunkStart{oldLine: oldLine, newLine: newLine}, true
}

// hunkNumber reads one number of a hunk header's range, such as the "41" of "-41,18".
func hunkNumber(field string) (int, bool) {
	number, _, _ := strings.Cut(strings.TrimLeft(field, "-+"), ",")
	value, err := strconv.Atoi(strings.TrimSpace(number))
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

// lineNumbers says which numbers a line gets, and what each side's counter becomes after it.
type lineNumbers struct {
	oldLine int
	newLine int
	nextOld int
	nextNew int
}

// splitDiffLine reads one line of a hunk: its text without the sign, which side it is on, and the
// numbers to draw beside it. kind is empty for a line that is not part of a hunk.
func splitDiffLine(line string, oldLine, newLine int) (text string, kind DiffLineKind, numbers lineNumbers) {
	if line == "" {
		return "", "", lineNumbers{}
	}
	switch line[0] {
	case ' ':
		return line[1:], DiffLineContext,
			lineNumbers{oldLine: oldLine, newLine: newLine, nextOld: oldLine + 1, nextNew: newLine + 1}
	case '-':
		return line[1:], DiffLineRemoved,
			lineNumbers{oldLine: oldLine, newLine: 0, nextOld: oldLine + 1, nextNew: newLine}
	case '+':
		return line[1:], DiffLineAdded,
			lineNumbers{oldLine: 0, newLine: newLine, nextOld: oldLine, nextNew: newLine + 1}
	default:
		return "", "", lineNumbers{}
	}
}

// cutHunks keeps at most maxLines lines of a file's hunks, and says whether it cut any. A cap of
// zero or less keeps everything.
func cutHunks(hunks []DiffHunk, maxLines int) ([]DiffHunk, bool) {
	if maxLines <= 0 {
		return hunks, false
	}
	left := maxLines
	kept := make([]DiffHunk, 0, len(hunks))
	for _, hunk := range hunks {
		if left <= 0 {
			return kept, true
		}
		if len(hunk.Lines) > left {
			hunk.Lines = hunk.Lines[:left]
			return append(kept, hunk), true
		}
		left -= len(hunk.Lines)
		kept = append(kept, hunk)
	}
	return kept, false
}

// checkDiffPath refuses a path that cannot name a file inside a repository. A path that is empty,
// is absolute, or walks upwards is refused; the caller answers not found for it rather than asking
// Git about a file outside the worktree.
func checkDiffPath(path string) error {
	if path == "" || strings.ContainsRune(path, 0) || strings.HasPrefix(path, "/") ||
		strings.HasPrefix(path, `\`) || filepath.IsAbs(path) {
		return newOpError(ErrBadPath, path, nil)
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return newOpError(ErrBadPath, path, nil)
		}
	}
	return nil
}

// readFileCapped reads at most maxBytes of a file and says whether there was more. A cap of zero
// or less means DefaultDiffBytes.
func readFileCapped(path string, maxBytes int) ([]byte, bool, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultDiffBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > maxBytes {
		return data[:maxBytes], true, nil
	}
	return data, false, nil
}

// runCapped runs a Git command and keeps at most maxBytes of its output, so one enormous diff
// cannot be held in memory in full. It returns what it read and whether there was more. A command
// that still has output is stopped: every command here only reads, so there is nothing to finish.
func (g *Git) runCapped(ctx context.Context, dir string, maxBytes int, args ...string) (string, bool, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultDiffBytes
	}
	cmd := exec.CommandContext(ctx, g.bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), g.env...)
	cmd.WaitDelay = killWaitDelay
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", false, fmt.Errorf("read the output of git %s: %w", strings.Join(args, " "), err)
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", false, errors.New("Git is not installed, or is not on the PATH")
		}
		return "", false, &Error{Args: args, Stderr: stderr.String(), Err: err}
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, int64(maxBytes)+1))
	truncated := len(data) > maxBytes
	if truncated {
		data = data[:maxBytes]
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil && !truncated {
		return "", false, &Error{Args: args, Stderr: stderr.String(), Err: fmt.Errorf("%w: %w", ctxErr, waitErr)}
	}
	if waitErr != nil && !truncated {
		return "", false, &Error{Args: args, Stderr: stderr.String(), Err: waitErr}
	}
	if readErr != nil && !truncated {
		return "", false, fmt.Errorf("read the output of git %s: %w", strings.Join(args, " "), readErr)
	}
	return string(data), truncated, nil
}
