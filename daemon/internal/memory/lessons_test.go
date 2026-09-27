package memory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Lessons (task 7.6): a project's own file-backed knowledge, saved and read the same way a card's
// note is (see notes_test.go's neighbour, service_test.go) - but keyed by slug rather than by card,
// because a lesson belongs to the project and not to any one card. These tests reuse newFixture from
// service_test.go: same package, same store, same vault.

const lessonSlug = "ci-is-flaky-on-windows"
const lessonPath = "small-repo/lessons/" + lessonSlug + ".md"

func TestLessonNotFoundForASlugNothingWasSavedUnder(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Lesson(context.Background(), projectID, "nothing-here")
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeNotFound {
		t.Fatalf("Lesson of an unknown slug = %v, want not_found", err)
	}
}

func TestSaveLessonWritesTheFileAndTheRowThenReadsItBack(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	body := "# CI is flaky on Windows\n\nRetry the flaky step once before failing the run.\n"

	saved, err := f.svc.SaveLesson(ctx, projectID, "CI is flaky on Windows", body, protocol.NoteAuthorAgent)
	if err != nil {
		t.Fatalf("save the lesson: %v", err)
	}
	if saved.Slug != lessonSlug {
		t.Fatalf("the slug = %q, want %q", saved.Slug, lessonSlug)
	}
	if saved.Path != lessonPath {
		t.Fatalf("the path = %q, want %q", saved.Path, lessonPath)
	}
	if saved.Author != protocol.NoteAuthorAgent {
		t.Fatalf("the author = %q, want agent", saved.Author)
	}
	if age := time.Since(saved.UpdatedAt.Time()); age < -time.Minute || age > time.Minute {
		t.Fatalf("the saved lesson's time is not now: %v", age)
	}

	full := filepath.Join(f.root, saved.Path)
	content, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read the lesson file: %v", err)
	}
	if string(content) != body {
		t.Fatalf("the lesson file holds %q, want %q", content, body)
	}

	read, err := f.svc.Lesson(ctx, projectID, lessonSlug)
	if err != nil {
		t.Fatalf("read the lesson back: %v", err)
	}
	if read.Body != body || read.Title != "CI is flaky on Windows" {
		t.Fatalf("the read-back lesson = %+v, want the saved title and body", read)
	}
}

func TestSaveLessonRefusesAnEmptyTitle(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.SaveLesson(context.Background(), projectID, "   ", "body", protocol.NoteAuthorPerson)
	assertInvalidArgument(t, err)
}

func TestSaveLessonRefusesAnAuthorItDoesNotKnow(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.SaveLesson(context.Background(), projectID, "A lesson", "body", protocol.NoteAuthor("robot"))
	assertInvalidArgument(t, err)
}

// A second save under a title that slugs the same way is an edit, the same rule UpsertNote follows
// for a card's note: the id and the birthday stay, and only the row's body, author, and time move.
func TestSavingALessonTwiceUnderTheSameTitleKeepsOneRow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveLesson(ctx, projectID, "CI is flaky", "first", protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save the lesson: %v", err)
	}
	f.clock.advance(time.Hour)
	if _, err := f.svc.SaveLesson(ctx, projectID, "CI is flaky", "second", protocol.NoteAuthorPerson); err != nil {
		t.Fatalf("save the lesson again: %v", err)
	}
	list, err := f.svc.ListLessons(ctx, projectID)
	if err != nil {
		t.Fatalf("list the lessons: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("saving one title twice left %d lessons, want 1", len(list))
	}
	if list[0].Body != "second" || list[0].Author != protocol.NoteAuthorPerson {
		t.Fatalf("the lesson = %+v, want the second save's body and author", list[0])
	}
}

// Retitling a lesson slugs to a new file: the old one is left behind (the same trade a card's own
// rename makes with its note file, noteFileName's own doc comment), and the project now has two
// rows until someone tidies the old one up.
func TestRetitlingALessonMovesItToANewSlug(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveLesson(ctx, projectID, "CI is flaky", "body", protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save the lesson: %v", err)
	}
	moved, err := f.svc.SaveLesson(ctx, projectID, "CI is flaky on Windows only", "body", protocol.NoteAuthorAgent)
	if err != nil {
		t.Fatalf("retitle the lesson: %v", err)
	}
	if moved.Slug == "ci-is-flaky" {
		t.Fatalf("the retitled lesson kept the old slug %q", moved.Slug)
	}
	if _, err := f.svc.Lesson(ctx, projectID, "ci-is-flaky"); err != nil {
		t.Fatalf("the old slug's row should still be there after a retitle: %v", err)
	}
	if _, err := f.svc.Lesson(ctx, projectID, moved.Slug); err != nil {
		t.Fatalf("the new slug's row is missing: %v", err)
	}
}

// ListLessons must never answer a card's note: the two share one table (migration 0019), told apart
// by kind, and a leak would put a card's private note in front of every reader of a project's lessons.
func TestListLessonsExcludesCardNotes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveNote(ctx, cardID, "a card's own note", protocol.NoteAuthorPerson); err != nil {
		t.Fatalf("save a card note: %v", err)
	}
	if _, err := f.svc.SaveLesson(ctx, projectID, "A real lesson", "body", protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save a lesson: %v", err)
	}
	list, err := f.svc.ListLessons(ctx, projectID)
	if err != nil {
		t.Fatalf("list the lessons: %v", err)
	}
	if len(list) != 1 || list[0].Title != "A real lesson" {
		t.Fatalf("ListLessons answered %+v, want only the one real lesson", list)
	}
}

func TestListLessonsIsNewestFirst(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveLesson(ctx, projectID, "First", "body", protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save the first lesson: %v", err)
	}
	f.clock.advance(time.Hour)
	if _, err := f.svc.SaveLesson(ctx, projectID, "Second", "body", protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save the second lesson: %v", err)
	}
	list, err := f.svc.ListLessons(ctx, projectID)
	if err != nil {
		t.Fatalf("list the lessons: %v", err)
	}
	if len(list) != 2 || list[0].Title != "Second" || list[1].Title != "First" {
		t.Fatalf("ListLessons = %+v, want the newest first", list)
	}
}

func TestSearchLessonsFindsASavedLessonByAPrefixOfAWord(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveLesson(ctx, projectID, "CI is flaky", "Retry the flaky step.", protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save the lesson: %v", err)
	}
	got, err := f.svc.SearchLessons(ctx, projectID, "flak", 0)
	if err != nil {
		t.Fatalf("search the lessons: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "ci-is-flaky" {
		t.Fatalf("SearchLessons = %+v, want the one lesson about flakiness", got)
	}
}

// DeleteLesson removes both halves outright, unlike the vault watcher, which never deletes a file:
// this is the lessons screen's own action, taken on purpose (lessons.go's own doc comment).
func TestDeleteLessonRemovesTheFileAndTheRow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	saved, err := f.svc.SaveLesson(ctx, projectID, "Gone soon", "body", protocol.NoteAuthorAgent)
	if err != nil {
		t.Fatalf("save the lesson: %v", err)
	}
	if err := f.svc.DeleteLesson(ctx, projectID, saved.Slug); err != nil {
		t.Fatalf("delete the lesson: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.root, saved.Path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the lesson file is still there after delete")
	}
	if _, err := f.svc.Lesson(ctx, projectID, saved.Slug); err == nil {
		t.Fatal("the lesson row is still there after delete")
	}
}

// A lesson written by hand, straight into the vault - no row yet at all - is indexed by the vault
// watcher's sweep the same way a hand-written note is, and its title comes from its own heading
// when there is one to read (watch.go's lessonTitle).
func TestReindexPicksUpALessonWrittenByHandWithATitleFromItsHeading(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dir := filepath.Join(f.root, projectID, "lessons")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("make the lessons folder: %v", err)
	}
	body := "# Retries beat flakiness\n\nRetry once before giving up.\n"
	if err := os.WriteFile(filepath.Join(dir, "hand-written.md"), []byte(body), 0o600); err != nil {
		t.Fatalf("write a lesson by hand: %v", err)
	}
	report, err := f.svc.Reindex(ctx)
	if err != nil {
		t.Fatalf("reindex the vault: %v", err)
	}
	if report.Indexed != 1 {
		t.Fatalf("the pass indexed %d, want 1", report.Indexed)
	}
	got, err := f.svc.Lesson(ctx, projectID, "hand-written")
	if err != nil {
		t.Fatalf("read the hand-written lesson: %v", err)
	}
	if got.Title != "Retries beat flakiness" {
		t.Fatalf("the title = %q, want the heading's own words", got.Title)
	}
	if got.Author != protocol.NoteAuthorPerson {
		t.Fatalf("a hand-written lesson's author = %q, want person", got.Author)
	}
}

// A lesson file name that is not already the slug it would have to be found by - Obsidian's own
// default naming ("My Lesson.md"), or a name that merely contains ".." without naming a parent
// directory at all ("v1..v2.md", which notePath's guard reads as a path escape) - is skipped rather
// than indexed under a slug nothing could ever open again, and rather than letting one such file
// stop the sweep from reaching every lesson after it (the whole point of this test: before the fix,
// this Reindex call returned an error and indexed nothing at all, not even the valid file after it).
func TestReindexSkipsALessonFileWhoseNameIsNotAlreadyASlugWithoutStoppingTheSweep(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	dir := filepath.Join(f.root, projectID, "lessons")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("make the lessons folder: %v", err)
	}
	for name, body := range map[string]string{
		"My Lesson.md":  "# A lesson named the way Obsidian would default to\n",
		"v1..v2.md":     "# A name that merely contains two dots\n",
		"a-real-one.md": "# A properly named lesson\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	report, err := f.svc.Reindex(ctx)
	if err != nil {
		t.Fatalf("reindex the vault: %v (a badly named file must not stop the sweep)", err)
	}
	if report.Files != 3 {
		t.Fatalf("the pass looked at %d files, want all 3 counted", report.Files)
	}
	if report.Indexed != 1 {
		t.Fatalf("the pass indexed %d, want only the properly named one", report.Indexed)
	}
	if _, err := f.svc.Lesson(ctx, projectID, "a-real-one"); err != nil {
		t.Fatalf("the properly named lesson was not indexed: %v", err)
	}
	// Never indexed, so never in the list the lessons screen reads - a badly named file is invisible
	// there until it is renamed, rather than reachable under a slug the screen never showed anyone.
	list, err := f.svc.ListLessons(ctx, projectID)
	if err != nil {
		t.Fatalf("list the lessons: %v", err)
	}
	for _, lesson := range list {
		if lesson.Slug == "My Lesson" || lesson.Slug == "v1..v2" {
			t.Fatalf("a badly named lesson file was listed: %+v", lesson)
		}
	}
}
