package memory_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Memory is driven over a real database and a real vault folder in a temporary directory, because
// the two things it owns are the file and the row and its whole job is to keep them in step. Nothing
// else is real: the cards and their projects are a fake behind the memory.Cards interface, so no
// test needs the projects service, a socket, a program, or a network.

var testNow = time.Date(2026, time.September, 27, 9, 30, 0, 0, time.UTC)

const (
	projectID = "small-repo"
	otherProj = "other-repo"

	cardID   = "01M3C107JB041061050R3GG28A" // #7, this project
	sibling  = "01M3C107JB041061050R3GG28B" // #8, this project
	third    = "01M3C107JB041061050R3GG28C" // #9, this project
	awayCard = "01M3C107JB041061050R3GG28D" // #1, the other project

	notePath = "small-repo/cards/7-add-a-health-check.md"
)

// clock is a clock the test moves by hand, so a claim's or a row's time is a fact the test chose
// rather than whatever the machine's clock said while the test ran.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

// fakeCards is what memory needs from the projects service: one card, and the project it is in.
type fakeCards struct {
	cards    map[string]protocol.Card
	projects map[string]protocol.Project
}

func (f *fakeCards) Card(_ context.Context, id string) (protocol.Card, error) {
	card, ok := f.cards[id]
	if !ok {
		return protocol.Card{}, protocol.NotFound("card")
	}
	return card, nil
}

func (f *fakeCards) Cards(_ context.Context, projectID string) ([]protocol.Card, error) {
	var out []protocol.Card
	for _, card := range f.cards {
		if card.ProjectID == projectID {
			out = append(out, card)
		}
	}
	slices.SortFunc(out, func(a, b protocol.Card) int { return a.Number - b.Number })
	return out, nil
}

func (f *fakeCards) Get(_ context.Context, id string) (protocol.Project, error) {
	project, ok := f.projects[id]
	if !ok {
		return protocol.Project{}, protocol.NotFound("project")
	}
	return project, nil
}

// fixture is a service over an empty database and an empty vault, with four cards seeded and a clock
// the test drives. cards is the fake the service was built over, kept so a test can take a card away
// and watch the reads that name a card by it.
type fixture struct {
	svc   *memory.Service
	store *store.Store
	cards *fakeCards
	root  string
	clock *clock
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	seedProject(t, st, projectID)
	seedProject(t, st, otherProj)
	seeded := []struct {
		id      string
		project string
		number  int64
		title   string
	}{
		{cardID, projectID, 7, "Add a health check"},
		{sibling, projectID, 8, "Fix the retry loop"},
		{third, projectID, 9, "Trim the bundle"},
		{awayCard, otherProj, 1, "Elsewhere"},
	}
	for _, c := range seeded {
		seedCard(t, st, c.id, c.project, c.number, c.title)
	}

	cards := &fakeCards{
		cards:    map[string]protocol.Card{},
		projects: map[string]protocol.Project{},
	}
	for _, c := range seeded {
		cards.cards[c.id] = protocol.Card{
			ID: c.id, ProjectID: c.project, Number: int(c.number), Key: c.project + "#" + strconv.Itoa(int(c.number)),
			Title: c.title,
		}
	}
	for _, id := range []string{projectID, otherProj} {
		cards.projects[id] = protocol.Project{ID: id, Name: id}
	}

	clk := &clock{t: testNow}
	svc, err := memory.New(
		memory.Deps{Store: st, Cards: cards, Root: filepath.Join(t.TempDir(), "vault")},
		memory.WithClock(clk.now),
	)
	if err != nil {
		t.Fatalf("build the memory service: %v", err)
	}
	return &fixture{svc: svc, store: st, cards: cards, root: svc.Root(), clock: clk}
}

// openStore opens a real database in a temporary directory.
func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(),
		filepath.Join(t.TempDir(), "marshal.db"), store.WithLogger(nil))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seedProject adds a project and its board. Nothing else about it matters to memory.
func seedProject(t *testing.T, st *store.Store, id string) {
	t.Helper()
	err := st.Write(context.Background(), func(q *db.Queries) error {
		now := testNow.UnixMilli()
		if err := q.CreateProject(context.Background(), db.CreateProjectParams{
			ID: id, Name: id, RepoPath: "/code/" + id, DefaultBranch: "main",
			PackagesJSON: "[]", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return err
		}
		return q.CreateBoard(context.Background(), db.CreateBoardParams{
			ID: "board-" + id, ProjectID: id, ColumnsJSON: `["backlog","working","needs","done"]`,
		})
	})
	if err != nil {
		t.Fatalf("seed the project %s: %v", id, err)
	}
}

// seedCard adds a card, which is the row `file_claims`' foreign key points at and `notes.card_id`
// names without a foreign key of its own (migration 0019's header comment): the card itself is a
// fake, but its row has to be real or SaveNote's own read of it (cardAndProject) fails.
func seedCard(t *testing.T, st *store.Store, id, project string, number int64, title string) {
	t.Helper()
	err := st.Write(context.Background(), func(q *db.Queries) error {
		now := testNow.UnixMilli()
		return q.CreateCard(context.Background(), db.CreateCardParams{
			ID: id, ProjectID: project, Number: number, BoardID: "board-" + project,
			Title: title, State: string(protocol.CardStateWorking), AgentKind: "claude",
			PermissionMode: "auto-edits", CreatedAt: now, UpdatedAt: now,
		})
	})
	if err != nil {
		t.Fatalf("seed the card %s: %v", id, err)
	}
}

func assertInvalidArgument(t *testing.T, err error) {
	t.Helper()
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("want an invalid_argument error, got %v", err)
	}
	if perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Fatalf("want invalid_argument, got %s (%s)", perr.Code, perr.Message)
	}
}

func TestNewRequiresEveryPart(t *testing.T) {
	st := openStore(t)
	cards := &fakeCards{}
	for _, tc := range []struct {
		name string
		deps memory.Deps
	}{
		{"no store", memory.Deps{Cards: cards, Root: t.TempDir()}},
		{"no cards", memory.Deps{Store: st, Root: t.TempDir()}},
		{"no vault root", memory.Deps{Store: st, Cards: cards}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := memory.New(tc.deps); err == nil {
				t.Fatal("memory.New built a service without every part")
			}
		})
	}
}

func TestNoteOfACardNothingWasSavedForIsThePlaceholderAndWritesNoFile(t *testing.T) {
	f := newFixture(t)
	note, err := f.svc.Note(context.Background(), cardID)
	if err != nil {
		t.Fatalf("read the note: %v", err)
	}
	if note.UpdatedAt != nil {
		t.Fatalf("a note nothing has been saved for has no save time, got %v", note.UpdatedAt)
	}
	if note.Author != protocol.NoteAuthorPerson {
		t.Fatalf("the placeholder's author = %q, want person", note.Author)
	}
	if note.Path != notePath {
		t.Fatalf("the note's path = %q, want %q", note.Path, notePath)
	}
	if !strings.Contains(note.Body, "Add a health check") {
		t.Fatalf("the placeholder note does not name the card:\n%s", note.Body)
	}
	// Reading a note must not create it: the file appears when something is first saved.
	if _, err := os.Stat(filepath.Join(f.root, note.Path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reading a note wrote a file (stat: %v)", err)
	}
}

func TestSaveNoteWritesTheFileAndTheRow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	body := "# Notes\n\nA health check on the daemon.\n"

	saved, err := f.svc.SaveNote(ctx, cardID, body, protocol.NoteAuthorPerson)
	if err != nil {
		t.Fatalf("save the note: %v", err)
	}
	if saved.UpdatedAt == nil {
		t.Fatal("a saved note has a save time")
	}
	if age := time.Since(time.Time(*saved.UpdatedAt)); age < -time.Minute || age > time.Minute {
		t.Fatalf("the saved note's time is not now: %v", age)
	}

	full := filepath.Join(f.root, saved.Path)
	content, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read the note file: %v", err)
	}
	if string(content) != body {
		t.Fatalf("the note file holds %q, want %q", content, body)
	}
	info, err := os.Stat(full)
	if err != nil {
		t.Fatalf("look at the note file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("the note file's mode = %v, want 0600", got)
	}

	row, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: cardID})
	if err != nil {
		t.Fatalf("read the note row: %v", err)
	}
	if row.Body != body {
		t.Fatalf("the row holds %q, want %q", row.Body, body)
	}
	if row.Author != string(protocol.NoteAuthorPerson) {
		t.Fatalf("the row's author = %q, want person", row.Author)
	}
	if row.UpdatedAt != testNow.UnixMilli() {
		t.Fatalf("the row's time = %d, want %d", row.UpdatedAt, testNow.UnixMilli())
	}
}

func TestSavingACardTwiceKeepsOneNoteAndItsBirthday(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveNote(ctx, cardID, "first", protocol.NoteAuthorPerson); err != nil {
		t.Fatalf("save the note: %v", err)
	}
	first, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: cardID})
	if err != nil {
		t.Fatalf("read the note row: %v", err)
	}

	f.clock.advance(time.Hour)
	if _, err := f.svc.SaveNote(ctx, cardID, "second", protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save the note again: %v", err)
	}
	second, err := f.store.Queries().GetNote(ctx, db.GetNoteParams{ProjectID: projectID, CardID: cardID})
	if err != nil {
		t.Fatalf("read the note row again: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("one note per card: the id changed from %q to %q", first.ID, second.ID)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Fatalf("the note's birthday moved from %d to %d", first.CreatedAt, second.CreatedAt)
	}
	if second.Body != "second" {
		t.Fatalf("the row holds %q, want the new text", second.Body)
	}
	if second.UpdatedAt != f.clock.t.UnixMilli() {
		t.Fatalf("the row's time = %d, want %d", second.UpdatedAt, f.clock.t.UnixMilli())
	}

	// The index follows the edit: the old text is no longer found and the new text is.
	found, err := f.svc.SearchNotes(ctx, projectID, "second", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 1 || found[0].Body != "second" {
		t.Fatalf("search found %d notes for the new text, want 1", len(found))
	}
	gone, err := f.svc.SearchNotes(ctx, projectID, "first", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(gone) != 0 {
		t.Fatalf("the replaced text is still searchable: %d matches", len(gone))
	}
}

func TestReadPrefersTheFileOverTheRow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveNote(ctx, cardID, "the daemon wrote this", protocol.NoteAuthorPerson); err != nil {
		t.Fatalf("save the note: %v", err)
	}

	// A person edits the file by hand, the way Obsidian does.
	full := filepath.Join(f.root, notePath)
	if err := os.WriteFile(full, []byte("the person edited this in Obsidian\n"), 0o600); err != nil {
		t.Fatalf("edit the note file: %v", err)
	}

	note, err := f.svc.Note(ctx, cardID)
	if err != nil {
		t.Fatalf("read the note: %v", err)
	}
	if !strings.Contains(note.Body, "Obsidian") {
		t.Fatalf("a read did not take the file's text:\n%s", note.Body)
	}
	// The row is the index and not a second copy: it still holds the daemon's text, and search -
	// which reads the index - does too, until the vault watcher (task 7.8) catches up. This is the
	// seam the watcher closes, written down here rather than left implicit.
	found, err := f.svc.SearchNotes(ctx, projectID, "daemon", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("search reads the index, which has not seen the edit yet: %d matches, want 1", len(found))
	}
}

func TestReadFallsBackToTheRowWhenTheFileIsGone(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	body := "the row is the only copy for a moment"
	if _, err := f.svc.SaveNote(ctx, cardID, body, protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save the note: %v", err)
	}
	if err := os.Remove(filepath.Join(f.root, notePath)); err != nil {
		t.Fatalf("remove the note file: %v", err)
	}

	note, err := f.svc.Note(ctx, cardID)
	if err != nil {
		t.Fatalf("read the note: %v", err)
	}
	if note.Body != body {
		t.Fatalf("the note = %q, want the row's text %q", note.Body, body)
	}
	if note.Author != protocol.NoteAuthorAgent {
		t.Fatalf("the note's author = %q, want agent", note.Author)
	}
	if note.UpdatedAt == nil || !time.Time(*note.UpdatedAt).Equal(testNow) {
		t.Fatalf("the note's time = %v, want the row's %v", note.UpdatedAt, testNow)
	}
}

func TestSaveNoteRefusesAnAuthorItDoesNotKnow(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.SaveNote(context.Background(), cardID, "x", protocol.NoteAuthor("robot"))
	assertInvalidArgument(t, err)
	// The author is checked first, so a refused save wrote nothing.
	if _, err := os.Stat(filepath.Join(f.root, notePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused save wrote a file (stat: %v)", err)
	}
}

func TestNoteForACardThatIsNotThereIsNotFound(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Note(context.Background(), "no-such-card")
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeNotFound {
		t.Fatalf("want not_found from the projects service, got %v", err)
	}
}

func TestSearchFindsASavedNoteByAPrefixOfAWord(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SaveNote(ctx, cardID, "Add a health check to the daemon", protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save the note: %v", err)
	}
	// A note of another project with the same words, which this project's search must not answer.
	if _, err := f.svc.SaveNote(ctx, awayCard, "a health check elsewhere", protocol.NoteAuthorAgent); err != nil {
		t.Fatalf("save the other project's note: %v", err)
	}

	found, err := f.svc.SearchNotes(ctx, projectID, "heal", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 1 || found[0].CardID != cardID {
		t.Fatalf("a prefix search found %d notes, want this project's one", len(found))
	}
	if found[0].Path != notePath {
		t.Fatalf("a search answer's path = %q, want %q", found[0].Path, notePath)
	}

	for _, tc := range []struct {
		name  string
		query string
	}{
		{"a word nobody wrote", "pineapple"},
		{"no words at all", "   ---  "},
		{"nothing at all", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := f.svc.SearchNotes(ctx, projectID, tc.query, 10)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("search(%q) answered %d notes, want none", tc.query, len(got))
			}
		})
	}
}

func TestSearchAnswersAtMostTheLimitItIsGiven(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, id := range []string{cardID, sibling, third} {
		if _, err := f.svc.SaveNote(ctx, id, "the widget report", protocol.NoteAuthorAgent); err != nil {
			t.Fatalf("save the note of %s: %v", id, err)
		}
	}
	found, err := f.svc.SearchNotes(ctx, projectID, "widget", 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("search with a limit of 2 answered %d notes", len(found))
	}
	// A limit of zero means "the caller did not say", which is the default and finds all three.
	all, err := f.svc.SearchNotes(ctx, projectID, "widget", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("search with no limit answered %d notes, want 3", len(all))
	}
}

func TestClaimRecordsAPathAndClaimingItAgainDoesNotMoveIt(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	res, err := f.svc.Claim(ctx, cardID, []string{"internal/api/routes.go"})
	if err != nil {
		t.Fatalf("claim a path: %v", err)
	}
	if len(res.Claims) != 1 || res.Claims[0].PathOrPackage != "internal/api/routes.go" {
		t.Fatalf("the claim recorded %v", res.Claims)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("the first card on a path has no conflict, got %v", res.Conflicts)
	}
	if !res.Claims[0].ClaimedAt.Equal(testNow) {
		t.Fatalf("the claim's time = %v, want %v", res.Claims[0].ClaimedAt, testNow)
	}

	f.clock.advance(time.Hour)
	again, err := f.svc.Claim(ctx, cardID, []string{"internal/api/routes.go"})
	if err != nil {
		t.Fatalf("claim the same path again: %v", err)
	}
	if len(again.Claims) != 1 {
		t.Fatalf("claiming the same path again made %d claims, want 1", len(again.Claims))
	}
	if !again.Claims[0].ClaimedAt.Equal(testNow) {
		t.Fatalf("claiming again moved the claim to %v, want %v", again.Claims[0].ClaimedAt, testNow)
	}
	mine, err := f.svc.Claims(ctx, cardID)
	if err != nil {
		t.Fatalf("read the claims: %v", err)
	}
	if len(mine) != 1 {
		t.Fatalf("the card holds %d claims, want 1", len(mine))
	}
}

func TestClaimWarnsWhenAnotherCardAlreadyHoldsThePath(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Claim(ctx, cardID, []string{"internal/store/store.go"}); err != nil {
		t.Fatalf("the first card's claim: %v", err)
	}
	f.clock.advance(time.Minute)

	res, err := f.svc.Claim(ctx, sibling, []string{"internal/store/store.go"})
	if err != nil {
		t.Fatalf("the second card's claim: %v", err)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("the second card got %d conflicts, want the first card's one", len(res.Conflicts))
	}
	if res.Conflicts[0].CardID != cardID {
		t.Fatalf("the conflict names %q, want %q", res.Conflicts[0].CardID, cardID)
	}
	if !res.Conflicts[0].ClaimedAt.Equal(testNow) {
		t.Fatalf("the conflict's time = %v, want the first card's %v", res.Conflicts[0].ClaimedAt, testNow)
	}
	// A claim is not a lock: the second card holds it too, which is what the warning is about.
	if len(res.Claims) != 1 {
		t.Fatalf("the second card recorded %d claims, want 1", len(res.Claims))
	}
}

func TestClaimRejectsPathsThatAreNotInTheRepository(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{"nothing named", nil},
		{"a blank path", []string{"   "}},
		{"an absolute path", []string{"/etc/passwd"}},
		{"a path above the repository", []string{"../secrets"}},
		{"a path climbing through the repository", []string{"internal/../../etc/passwd"}},
		{"a path too long to be real", []string{strings.Repeat("a", 600)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.Claim(ctx, cardID, tc.paths)
			assertInvalidArgument(t, err)
		})
	}
	mine, err := f.svc.Claims(ctx, cardID)
	if err != nil {
		t.Fatalf("read the claims: %v", err)
	}
	if len(mine) != 0 {
		t.Fatalf("a refused claim wrote %d rows", len(mine))
	}
}

func TestReleaseGivesUpTheNamedPathsAndEverythingWhenNoneAreNamed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Claim(ctx, cardID, []string{"a.go", "b.go"}); err != nil {
		t.Fatalf("claim two paths: %v", err)
	}
	if err := f.svc.Release(ctx, cardID, []string{"a.go"}); err != nil {
		t.Fatalf("release one path: %v", err)
	}
	mine, err := f.svc.Claims(ctx, cardID)
	if err != nil {
		t.Fatalf("read the claims: %v", err)
	}
	if len(mine) != 1 || mine[0].PathOrPackage != "b.go" {
		t.Fatalf("after releasing a.go the card holds %v", mine)
	}
	// Releasing something the card does not hold is not an error.
	if err := f.svc.Release(ctx, cardID, []string{"never.go"}); err != nil {
		t.Fatalf("releasing a path the card does not hold: %v", err)
	}
	// Naming nothing releases everything.
	if err := f.svc.Release(ctx, cardID, nil); err != nil {
		t.Fatalf("release everything: %v", err)
	}
	mine, err = f.svc.Claims(ctx, cardID)
	if err != nil {
		t.Fatalf("read the claims: %v", err)
	}
	if len(mine) != 0 {
		t.Fatalf("after releasing everything the card holds %v", mine)
	}
}

func TestProjectClaimsListsWhatEveryCardHolds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.Claim(ctx, cardID, []string{"one.go"}); err != nil {
		t.Fatalf("the first card's claim: %v", err)
	}
	f.clock.advance(time.Minute)
	if _, err := f.svc.Claim(ctx, sibling, []string{"two.go"}); err != nil {
		t.Fatalf("the second card's claim: %v", err)
	}

	all, err := f.svc.ProjectClaims(ctx, projectID)
	if err != nil {
		t.Fatalf("read the project's claims: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("the project holds %d claims, want 2", len(all))
	}
	if all[0].PathOrPackage != "one.go" || all[1].PathOrPackage != "two.go" {
		t.Fatalf("the project's claims = %v, want them oldest first", all)
	}
	// The other project's card holds nothing, so its list is empty and not this one's.
	other, err := f.svc.ProjectClaims(ctx, otherProj)
	if err != nil {
		t.Fatalf("read the other project's claims: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("the other project holds %d claims, want none", len(other))
	}
}
