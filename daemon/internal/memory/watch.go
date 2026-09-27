package memory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The vault watcher: how an edit a person makes in Obsidian reaches the index (docs/architecture.md
// section 12, docs/backend-checklist.md B7.4, build-plan tasks 7.6 and 7.8). A note's or a lesson's
// file is the truth about what it says, and its row in `notes` is the index search matches against,
// so an edit made outside Marshal is invisible to search until something brings the row up to date.
// That is this file, for both kinds it indexes.
//
// It is a sweep and not a file-system notification, for the reason the idle timer in
// internal/session/sleep.go is one: one pass that reads what is there is easier to reason about than
// a stream of events to debounce, there is no new dependency to add (fsnotify is in the module graph
// only as an indirect dependency of something else and is used by nothing), and a watcher that
// missed an event while the daemon was stopped is repaired by the first sweep after it starts
// rather than by a full re-index. The cost is that an edit is picked up within one interval, which
// is why the interval is short.
//
// The sweep compares each note file's own modification time with the time the row says it was last
// written. A file older than its row is left alone, so the common case - a note the daemon itself
// just wrote - costs one stat and no write, and a file the person edited is the only thing re-read.
// Nothing is ever deleted: a note file removed by hand leaves its row, which is what a read falls
// back to (see Note), because a note a person deleted in their file manager is a thing they can undo
// and a row Marshal deleted for them is not.

// vaultWatchInterval is how often the vault is swept for edits made outside Marshal. It is short
// against how long a person takes to edit and then search for what they wrote, and long against
// what one pass costs (a directory read and a stat per note), so a vault of a few thousand notes is
// not read constantly. The pass itself only reads the file system: it writes a row only for a file
// that is genuinely newer than it.
const vaultWatchInterval = 30 * time.Second

// ReindexReport is what one pass of the vault watcher did, so a caller can log it and a test can
// assert it. The counts are of the pass, not of the vault: Projects and Files are what was looked
// at, and Indexed is what was actually brought up to date - the number that is zero on an idle
// vault, which is the normal case.
type ReindexReport struct {
	// Projects is how many of the vault's folders were a project Marshal knows.
	Projects int
	// Files is how many note and lesson files were found under those projects.
	Files int
	// Indexed is how many of them were newer than their row (or had no row) and were re-indexed.
	Indexed int
}

// StartVaultWatch starts the goroutine that brings the index up to date after an edit made outside
// Marshal. cmd/marshald calls it once, after the memory module is built; it stops when ctx is done.
// A service whose watcher was never started never re-indexes anything on its own, which is every
// test that drives Reindex by hand, so a test is never racing a timer.
func (s *Service) StartVaultWatch(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(vaultWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// A pass that fails is logged by the caller's logger through the returned error
				// only if there is one; here there is nothing to hand it to, so a failure is
				// swallowed and the next tick tries again. The pass is idempotent, so trying again
				// is the whole recovery.
				_, _ = s.Reindex(ctx)
			}
		}
	}()
}

// Reindex is one pass of the vault watcher: walk the vault's project folders, and bring the index
// of every card note up to date with the file. It is exported because StartVaultWatch calls it on a
// ticker and a test drives it by hand rather than waiting out a wall-clock interval; calling it at
// any time is safe and idempotent, since it only writes a row that a file has made stale.
//
// A vault that is not there yet is not an error: a person who has never saved a note has no vault
// folder, and the first save makes it.
func (s *Service) Reindex(ctx context.Context) (ReindexReport, error) {
	var report ReindexReport
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("look inside the vault: %w", err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if !entry.IsDir() {
			continue
		}
		projectID := entry.Name()
		// A vault folder is a project's when the projects service knows that id. It cannot be
		// trusted from the name alone: the vault root also holds `briefs/`, and a person may leave
		// anything else in there.
		if _, err := s.cards.Get(ctx, projectID); err != nil {
			continue
		}
		indexed, files, err := s.reindexProject(ctx, projectID)
		if err != nil {
			return report, err
		}
		report.Projects++
		report.Files += files
		report.Indexed += indexed
	}
	return report, nil
}

// reindexProject is one project's pass: bring its card notes up to date, then its lessons. A folder
// that is not there is a project nothing of that kind has been saved for yet.
func (s *Service) reindexProject(ctx context.Context, projectID string) (indexed, files int, err error) {
	cardsIndexed, cardsFiles, err := s.reindexProjectCards(ctx, projectID)
	if err != nil {
		return 0, 0, err
	}
	lessonsIndexed, lessonsFiles, err := s.reindexProjectLessons(ctx, projectID)
	if err != nil {
		return cardsIndexed, cardsFiles, err
	}
	return cardsIndexed + lessonsIndexed, cardsFiles + lessonsFiles, nil
}

// reindexProjectCards is the card-note half of one project's pass: look at each note file under its
// `cards` folder and bring the row of the card its name points at up to date.
func (s *Service) reindexProjectCards(ctx context.Context, projectID string) (indexed, files int, err error) {
	cards, err := s.cards.Cards(ctx, projectID)
	if err != nil {
		return 0, 0, fmt.Errorf("read the cards of project %s: %w", projectID, err)
	}
	byNumber := make(map[int]protocol.Card, len(cards))
	for _, card := range cards {
		byNumber[card.Number] = card
	}
	dir := filepath.Join(s.root, projectID, cardsFolder)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("look inside %s: %w", dir, err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return indexed, files, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), noteExtension) {
			continue
		}
		files++
		number, ok := noteNumber(entry.Name())
		if !ok {
			continue
		}
		card, ok := byNumber[number]
		if !ok {
			// A file for a card that is not there: a card removed while its note file was left in
			// the vault. It is left alone rather than deleted, for the reason above.
			continue
		}
		changed, err := s.reindexNote(ctx, card)
		if err != nil {
			return indexed, files, err
		}
		if changed {
			indexed++
		}
	}
	return indexed, files, nil
}

// reindexProjectLessons is the lesson half of one project's pass: look at each file under its
// `lessons` folder and bring the row its slug points at up to date. Unlike a card note, a lesson's
// file name is already its slug (lessonRelPath), so there is no number to look up a card by - the
// file names itself.
func (s *Service) reindexProjectLessons(ctx context.Context, projectID string) (indexed, files int, err error) {
	dir := filepath.Join(s.root, projectID, lessonsFolder)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("look inside %s: %w", dir, err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return indexed, files, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), noteExtension) {
			continue
		}
		files++
		slug := strings.TrimSuffix(entry.Name(), noteExtension)
		// A lesson is found again by its slug alone (lessonRelPath has no number the way a card
		// note's name does), so its file name has to already be one: `noteSlug`'s own output, the
		// shape SaveLesson always writes. A name that is not - spaces, capitals, a punctuation run
		// `notePath`'s guard reads as a path escape ("v1..v2.md" contains ".." though it names no
		// parent directory at all) - is skipped rather than indexed under a slug the lessons screen
		// could never open, save, or delete again, and rather than letting one bad name stop the
		// sweep from reaching every lesson and every project after it. It still counts as a file
		// looked at, the same way a card note file with no number in its name does (reindexProject).
		if noteSlug(slug) != slug {
			continue
		}
		changed, err := s.reindexLesson(ctx, projectID, slug)
		if err != nil {
			return indexed, files, err
		}
		if changed {
			indexed++
		}
	}
	return indexed, files, nil
}

// reindexNote brings one card's note row up to date with its file, and reports whether it wrote
// anything. The file is read only when its own time is newer than the row's, so a note the daemon
// wrote is looked at and left alone.
func (s *Service) reindexNote(ctx context.Context, card protocol.Card) (bool, error) {
	rel := noteRelPath(card)
	row, err := s.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: card.ProjectID, CardID: card.ID})
	switch {
	case err == nil:
	case store.IsNotFound(err):
		row = db.Note{}
	default:
		return false, fmt.Errorf("read the note row of card %s: %w", card.ID, err)
	}
	// The file's own time is what says whether anything changed. It is read before the file itself
	// so a file that is not newer costs one stat, which is what keeps an idle sweep cheap.
	body, modified, found, err := s.readNoteFile(rel)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	fresh := modified.UnixMilli()
	if row.ID != "" && fresh <= row.UpdatedAt {
		return false, nil
	}
	author := protocol.NoteAuthorPerson
	id := ""
	created := fresh
	if row.ID != "" {
		// The row is kept and not replaced: a note keeps the id and the birthday it was first saved
		// with, so an edited note is the same note, and the author it had is who wrote it.
		id, created, author = row.ID, row.CreatedAt, authorOf(row)
	}
	if id == "" {
		id, err = s.newID()
		if err != nil {
			return false, err
		}
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertNote(ctx, db.UpsertNoteParams{
			ID: id, ProjectID: card.ProjectID, CardID: card.ID, Author: string(author),
			Body: body, CreatedAt: created, UpdatedAt: fresh,
		})
	})
	if err != nil {
		return false, fmt.Errorf("index the note of card %s: %w", card.ID, err)
	}
	return true, nil
}

// noteNumber reads the card number a note file's name starts with: the `41` of
// `41-refresh-the-token.md` (see noteFileName, which is the one place that name is built). It is how
// a file in the vault is tied back to a card without reading every note's body, and it is why the
// number is in the name at all.
func noteNumber(name string) (int, bool) {
	digits, _, _ := strings.Cut(name, "-")
	if digits == "" {
		return 0, false
	}
	number, err := strconv.Atoi(digits)
	if err != nil || number <= 0 {
		return 0, false
	}
	return number, true
}

// reindexLesson brings one lesson row up to date with its file, and reports whether it wrote
// anything. It follows reindexNote's rule exactly - the file's time against the row's - with one
// difference: a lesson has no card to read an author or a title from, so a lesson a person wrote
// straight into Obsidian, with no row yet, gets its title from the file itself (lessonTitle) and
// "person" as its author, the same default a fresh card note gets.
func (s *Service) reindexLesson(ctx context.Context, projectID, slug string) (bool, error) {
	rel := lessonRelPath(projectID, slug)
	row, err := s.store.Queries().GetLessonBySlug(ctx, db.GetLessonBySlugParams{ProjectID: projectID, Slug: slug})
	switch {
	case err == nil:
	case store.IsNotFound(err):
		row = db.Note{}
	default:
		return false, fmt.Errorf("read the lesson row %s of project %s: %w", slug, projectID, err)
	}
	body, modified, found, err := s.readNoteFile(rel)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	fresh := modified.UnixMilli()
	if row.ID != "" && fresh <= row.UpdatedAt {
		return false, nil
	}
	author := protocol.NoteAuthorPerson
	title := lessonTitle(slug, body)
	id := ""
	created := fresh
	if row.ID != "" {
		id, created, author, title = row.ID, row.CreatedAt, authorOf(row), row.Title
	}
	if id == "" {
		id, err = s.newID()
		if err != nil {
			return false, err
		}
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertLesson(ctx, db.UpsertLessonParams{
			ID: id, ProjectID: projectID, Slug: slug, Title: title, Author: string(author),
			Body: body, CreatedAt: created, UpdatedAt: fresh,
		})
	})
	if err != nil {
		return false, fmt.Errorf("index lesson %s of project %s: %w", slug, projectID, err)
	}
	return true, nil
}

// lessonTitle reads a lesson's title from its own file when there is no row to read it from - a
// file a person wrote by hand in Obsidian, with no save through SaveLesson ever behind it: the text
// of a leading `# ` heading if the file has one, or the slug turned back into words when it does
// not, so a hand-written file is read rather than refused.
func lessonTitle(slug, body string) string {
	line, _, _ := strings.Cut(body, "\n")
	if heading, ok := strings.CutPrefix(line, "# "); ok {
		if heading = strings.TrimSpace(heading); heading != "" {
			return heading
		}
	}
	words := strings.Split(slug, "-")
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}
