package memory

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The vault's shape on disk and the rules a note's path is made by (docs/architecture.md section
// 12).
//
//	vault/
//	  <project>/            the project's id (see the package comment for why it is the id)
//	    memory/             rules, architecture notes, decisions
//	    lessons/<slug>.md   one file per lesson (task 7.6)
//	    cards/<n>-<title>.md  one note per card
//	  briefs/               daily notes with morning and evening briefs
//
// This module writes `cards/` and, as of task 7.6, `lessons/`. The knowledge base (`memory/`) still
// has no writer, and `briefs/` belongs to Phase 8's scheduler. They are named here so the layout is
// written down in one place rather than three.

const (
	// cardsFolder is the folder of card notes inside a project's folder.
	cardsFolder = "cards"
	// lessonsFolder is the folder of a project's lessons inside its folder (task 7.6). The third
	// folder the package comment names, `memory` for the rules, notes, and decisions, still has no
	// constant, because nothing reads or writes one yet.
	lessonsFolder = "lessons"
	// noteExtension is what a note's file ends with. Markdown, because that is what the vault is
	// for: a person reads and edits these files, and Obsidian links between them.
	noteExtension = ".md"
	// noteSlugMax caps the readable part of a note's file name. File systems stop somewhere near
	// 255 bytes for a whole name; 60 leaves room for the number in front, the extension, and any
	// suffix, and keeps a name a person can read in a file picker.
	noteSlugMax = 60
	// noteSlugFallback is the readable part when a title has nothing in it a file name may keep,
	// such as a title written in a script this keeps no bytes from. The number in front is what
	// makes the name unique either way.
	noteSlugFallback = "note"
	// fileMode and dirMode keep the vault owner-only, the same as the database and for the same
	// reason: it is the owner's own notes, and no other user on the machine needs to read them.
	fileMode = 0o600
	dirMode  = 0o700
)

// noteRelPath is where a card's note lives, relative to the vault root, with forward slashes
// whatever the platform uses on disk: it is a path on the wire as well as on disk, and section 12's
// layout is written with forward slashes.
func noteRelPath(card protocol.Card) string {
	return path.Join(card.ProjectID, cardsFolder, noteFileName(card))
}

// noteFileName is the name of a card's note: the card's number, then its title, then `.md` - the
// shape the design's own `notePath` uses (`apps/web/src/views/card/card-note.ts`). The number in
// front is what makes the name unique inside the project and what makes it stable: it is the "7" a
// person reads in `#7`, so it is the same 7 in `7-add-a-health-check.md`, and it is what lets the
// watcher (task 7.8) say which card a file belongs to even after a person renames the file.
func noteFileName(card protocol.Card) string {
	return fmt.Sprintf("%d-%s%s", card.Number, noteSlug(card.Title), noteExtension)
}

// lessonRelPath is where a lesson lives, relative to the vault root, with forward slashes for the
// same reason noteRelPath uses them. Unlike a card note, a lesson has no number to make its name
// unique - a project's lessons are not numbered - so the slug alone is the file name, and it is what
// (project_id, slug) indexes in migration 0019.
func lessonRelPath(projectID, slug string) string {
	return path.Join(projectID, lessonsFolder, slug+noteExtension)
}

// noteSlug is the readable part of a note's file name: the title in lower case, words joined by one
// hyphen, cut to a length a file system is happy with. It keeps only the letters and digits of the
// ASCII range, which is the same rule the design's `notePath` applies, so the file this writes and
// the path the app draws agree.
func noteSlug(title string) string {
	var slug strings.Builder
	pendingHyphen := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if pendingHyphen && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			pendingHyphen = false
			slug.WriteRune(r)
		default:
			pendingHyphen = true
		}
	}
	out := slug.String()
	if len(out) > noteSlugMax {
		out = strings.TrimRight(out[:noteSlugMax], "-")
	}
	if out == "" {
		return noteSlugFallback
	}
	return out
}

// notePath is a note's path on disk. The relative path is checked to be inside the vault root before
// it is joined: every part of it is built from a project id and a title slug, both of which are
// already restricted, and this is the cheap second guard that keeps a future change to either of
// them from writing a note outside the vault.
func (s *Service) notePath(rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") {
		return "", fmt.Errorf("memory: %q is not a path inside the vault", rel)
	}
	full := filepath.Join(s.root, filepath.FromSlash(rel))
	inside, err := filepath.Rel(s.root, full)
	if err != nil {
		return "", fmt.Errorf("memory: %q is not inside the vault root: %w", rel, err)
	}
	// "." is the vault root itself, which is not *inside* it: a note is written below the root, and
	// a path that resolves to the root would be a write at the top of the person's vault.
	if inside == "." || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("memory: %q is not inside the vault root", rel)
	}
	return full, nil
}

// placeholderNote is the note a card starts with, for a card nothing has been saved for. It is
// built in memory and never written: reading a note must not create a file, so the file appears when
// something is first saved. The design's `ensureNote` does write the note as it draws the panel,
// which is fine for a mock and wrong for a daemon.
//
// It is deliberately thinner than the design's sample note, which also carries a Decisions list and
// a link to a lesson. Those are the prototype's sample content: the daemon has not decided anything
// about the card and knows of no lesson, and a note that says so would be Marshal making things up
// in the person's own file.
func placeholderNote(card protocol.Card, project protocol.Project) string {
	return fmt.Sprintf("# %s\n\nGoal: %s.\n\nLinks\n[[%s]]\n",
		card.Title, strings.ToLower(card.Title), project.ID)
}

// readNoteFile reads a note's file. It reports whether the file is there, and when it is, when it
// was last changed: a note edited in Obsidian has a file newer than its row until the watcher
// catches up, and the file's own time is the honest answer for how old the text is.
func (s *Service) readNoteFile(rel string) (body string, modified time.Time, found bool, err error) {
	full, err := s.notePath(rel)
	if err != nil {
		return "", time.Time{}, false, err
	}
	info, err := os.Stat(full)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", time.Time{}, false, nil
	case err != nil:
		return "", time.Time{}, false, fmt.Errorf("look at the note file: %w", err)
	case info.IsDir():
		return "", time.Time{}, false, fmt.Errorf("the note path %s is a folder", rel)
	}
	content, err := os.ReadFile(full)
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("read the note file: %w", err)
	}
	return string(content), info.ModTime().UTC(), true, nil
}

// writeNoteFile writes a note's file, making its folder. It writes a temporary file beside the
// target and renames it over it, so a reader - the person's own Obsidian, or the watcher - never
// sees half a note, and a crash leaves the previous note rather than a truncated one. The temporary
// file's name starts with a dot, which is how Obsidian is told to ignore it.
func (s *Service) writeNoteFile(rel string, body string) error {
	full, err := s.notePath(rel)
	if err != nil {
		return err
	}
	folder := filepath.Dir(full)
	if err := os.MkdirAll(folder, dirMode); err != nil {
		return fmt.Errorf("make the note's folder: %w", err)
	}
	temp, err := os.CreateTemp(folder, "."+filepath.Base(full)+".*.tmp")
	if err != nil {
		return fmt.Errorf("make a temporary note file: %w", err)
	}
	name := temp.Name()
	if _, err := temp.WriteString(body); err != nil {
		_ = temp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("write the note file: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close the note file: %w", err)
	}
	if err := os.Chmod(name, fileMode); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("set the note file's mode: %w", err)
	}
	if err := os.Rename(name, full); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("put the note file in place: %w", err)
	}
	return nil
}

// removeVaultFile deletes a file inside the vault, such as a lesson the lessons screen's own delete
// action asked to remove. It is the one place this package deletes a vault file outright rather than
// leaving it for a person to remove by hand, because it is only ever called for an action a person
// took on purpose in the screen built for exactly that - never by the watcher's sweep, which never
// deletes anything (watch.go's header comment says why). Deleting a file that is already gone is not
// an error: the row and the file can each go missing on their own, and a caller that removes both
// should not fail because one of them already had.
func (s *Service) removeVaultFile(rel string) error {
	full, err := s.notePath(rel)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove the vault file: %w", err)
	}
	return nil
}
