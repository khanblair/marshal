package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A project's lessons: what an agent learned worth remembering next time, written to the vault as
// its own markdown file the same way a card's note is (docs/architecture.md section 12; docs/
// backend-checklist.md B7.4, B7.6; build-plan task 7.6). The owner settled this as file-only and
// indexed the same way notes already are, rather than a table of its own - see migration 0019's
// header comment - so this file mirrors notes.go on purpose: a lesson's read prefers its file the
// same way a note's does, a save writes the file first and the row second, and a search reads the
// index rather than walking the vault. What differs is the key: a card note is found by its card, a
// lesson is found by its slug, because a lesson belongs to a project and not to any one card.
//
// Nothing in docs/architecture.md section 11.4's tool table gives an agent its own way to write a
// lesson over MCP today - only search_memory, which reads them. Until that is settled, a lesson is
// written by a person through the lessons screen (build-plan task 7.13), with SaveLesson taking
// whichever author the caller is, so the day an internal MCP tool calls it the answer already
// carries "agent" correctly.

// lessonTitleMax caps a lesson's title. A person types a heading, not an essay, and the cap keeps a
// pasted paragraph from becoming a file name by way of noteSlug.
const lessonTitleMax = 200

// Lesson returns one project's lesson by its slug. Unlike Note, there is no placeholder to fall back
// to: a card always exists to write a placeholder note for, but a slug nobody has saved a lesson
// under names nothing, so that is answered as protocol.NotFound rather than invented content.
func (s *Service) Lesson(ctx context.Context, projectID, slug string) (protocol.Lesson, error) {
	if _, err := s.cards.Get(ctx, projectID); err != nil {
		return protocol.Lesson{}, fmt.Errorf("read the project of the lesson: %w", err)
	}
	rel := lessonRelPath(projectID, slug)
	row, rowErr := s.store.Queries().GetLessonBySlug(ctx, db.GetLessonBySlugParams{ProjectID: projectID, Slug: slug})
	if rowErr != nil && !store.IsNotFound(rowErr) {
		return protocol.Lesson{}, fmt.Errorf("read lesson %s of project %s: %w", slug, projectID, rowErr)
	}
	body, modified, found, err := s.readNoteFile(rel)
	if err != nil {
		return protocol.Lesson{}, err
	}
	switch {
	case found:
		// The file is there, so it is what the lesson says, the same rule Note follows and for the
		// same reason: it is the file a person edits in Obsidian.
		title, author := "", protocol.NoteAuthorPerson
		if rowErr == nil {
			title, author = row.Title, authorOf(row)
		}
		return protocol.NewLesson(projectID, slug, title, rel, body, author, modified), nil
	case rowErr == nil:
		// The row is there and the file is not: a vault moved or emptied by hand. The row still
		// knows what the lesson said, so it answers, and the next save writes the file again.
		return protocol.NewLesson(projectID, slug, row.Title, rel, row.Body, authorOf(row),
			time.UnixMilli(row.UpdatedAt).UTC()), nil
	default:
		return protocol.Lesson{}, protocol.NotFound("lesson")
	}
}

// SaveLesson writes a project's lesson: the file in the vault, then the row that indexes it, in that
// order for the same reason SaveNote gives. The slug is made from the title with noteSlug, the same
// slugging a card's note file name uses, so a lesson titled "CI is flaky on Windows" is found at
// `lessons/ci-is-flaky-on-windows.md` whether it is saved through this call or the file is read by
// hand.
//
// A second save of the same title updates the same lesson rather than making a new one: the
// conflict target is (project_id, slug) among lessons (migration 0019), so retitling a lesson to a
// title that slugs the same way is an edit, and retitling it to a different one moves it to a new
// slug and leaves the old file behind, the same trade a card's own rename makes with noteFileName.
func (s *Service) SaveLesson(ctx context.Context, projectID, title, body string, author protocol.NoteAuthor) (protocol.Lesson, error) {
	if !author.Valid() {
		return protocol.Lesson{}, protocol.InvalidArgument("A lesson is written by a person or by an agent.").
			With("author", string(author))
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return protocol.Lesson{}, protocol.InvalidArgument("A lesson needs a title.")
	}
	if len(title) > lessonTitleMax {
		title = title[:lessonTitleMax]
	}
	if _, err := s.cards.Get(ctx, projectID); err != nil {
		return protocol.Lesson{}, fmt.Errorf("read the project of the lesson: %w", err)
	}
	slug := noteSlug(title)
	rel := lessonRelPath(projectID, slug)
	if err := s.writeNoteFile(rel, body); err != nil {
		return protocol.Lesson{}, err
	}
	id, err := s.newID()
	if err != nil {
		return protocol.Lesson{}, fmt.Errorf("make a lesson id: %w", err)
	}
	now := s.now().UnixMilli()
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertLesson(ctx, db.UpsertLessonParams{
			ID: id, ProjectID: projectID, Slug: slug, Title: title, Author: string(author),
			Body: body, CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		return protocol.Lesson{}, fmt.Errorf("save lesson %s of project %s: %w", slug, projectID, err)
	}
	return s.Lesson(ctx, projectID, slug)
}

// ListLessons returns a project's lessons, newest first, for the lessons screen (build-plan task
// 7.13). It reads the index and not the files, the same trade ListNotes makes for the memory
// screens: a project with many lessons is one query rather than a walk that reads every one, and a
// lesson edited in Obsidian shows the list's stale copy until the watcher (task 7.8) catches up.
func (s *Service) ListLessons(ctx context.Context, projectID string) ([]protocol.Lesson, error) {
	if _, err := s.cards.Get(ctx, projectID); err != nil {
		return nil, fmt.Errorf("read the project of the lessons list: %w", err)
	}
	rows, err := s.store.Queries().ListLessons(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list the lessons of project %s: %w", projectID, err)
	}
	out := make([]protocol.Lesson, 0, len(rows))
	for _, row := range rows {
		out = append(out, lessonOf(row))
	}
	return out, nil
}

// SearchLessons returns a project's lessons whose title or body matches a person's or an agent's
// words, best match first. Same rule as SearchNotes: a query with no words in it matches nothing.
func (s *Service) SearchLessons(ctx context.Context, projectID, query string, limit int) ([]protocol.Lesson, error) {
	match := matchExpression(query)
	if match == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	limit = min(limit, MaxSearchLimit)
	rows, err := s.store.SearchLessons(ctx, projectID, match, limit)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Lesson, 0, len(rows))
	for _, row := range rows {
		out = append(out, lessonOf(row))
	}
	return out, nil
}

// DeleteLesson removes a lesson outright: its file in the vault and the row that indexes it. Unlike
// the vault watcher, which never deletes a file a person removed by hand (watch.go's header
// comment), this is the lessons screen's own delete action, an action a person or an agent took on
// purpose, so both halves go rather than one being left as a row an empty file cannot back up.
func (s *Service) DeleteLesson(ctx context.Context, projectID, slug string) error {
	rel := lessonRelPath(projectID, slug)
	if err := s.removeVaultFile(rel); err != nil {
		return err
	}
	err := s.store.Write(ctx, func(q *db.Queries) error {
		return q.DeleteLesson(ctx, db.DeleteLessonParams{ProjectID: projectID, Slug: slug})
	})
	if err != nil {
		return fmt.Errorf("delete lesson %s of project %s: %w", slug, projectID, err)
	}
	return nil
}

// lessonOf turns an indexed row into the wire shape. It never fails: a row's own fields are
// everything a listed or searched lesson needs, unlike Lesson's single read, which prefers the file.
func lessonOf(row db.Note) protocol.Lesson {
	return protocol.NewLesson(row.ProjectID, row.Slug, row.Title, lessonRelPath(row.ProjectID, row.Slug),
		row.Body, authorOf(row), time.UnixMilli(row.UpdatedAt).UTC())
}
