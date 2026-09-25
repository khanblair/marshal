package projects_test

import (
	"context"
	"database/sql"
	"testing"

	// The driver that the store uses, so the test can open the same database file on its own.
	_ "modernc.org/sqlite"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// failInserts makes every insert into a table fail, like a full disk or a broken constraint
// would, by way of a trigger made on a second connection to the same file. It returns the function
// that removes the trigger again.
func (e *env) failInserts(t *testing.T, table string) (allow func()) {
	t.Helper()
	conn, err := sql.Open("sqlite", e.dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	name := "fail_" + table
	if _, err := conn.Exec("CREATE TRIGGER " + name + " BEFORE INSERT ON " + table +
		" BEGIN SELECT RAISE(ABORT, 'the test made this insert fail'); END"); err != nil {
		t.Fatalf("make the trigger: %v", err)
	}
	return func() {
		t.Helper()
		if _, err := conn.Exec("DROP TRIGGER " + name); err != nil {
			t.Fatalf("drop the trigger: %v", err)
		}
	}
}

func TestAFailedCreateLeavesNothingBehind(t *testing.T) {
	e := newEnv(t)
	allow := e.failInserts(t, "boards")
	path := testutil.Fixture(t, "small-repo")

	_, err := e.svc.Create(context.Background(), folderRequest(path))
	if err == nil {
		t.Fatal("Create succeeded although its board could not be written")
	}
	var perr *protocol.Error
	if errorsAs(err, &perr) {
		t.Errorf("the failure was turned into a message for a person: %v", perr)
	}
	list, err := e.svc.List(context.Background())
	if err != nil || len(list.Projects) != 0 {
		t.Errorf("List = %+v, %v; the project row must have been rolled back with the board", list, err)
	}
	if _, err := e.store.Queries().GetProjectByRepoPath(context.Background(), path); !sqlNoRows(err) {
		t.Errorf("a row for the folder is left behind: %v", err)
	}
	e.noEvent(t)

	allow()
	project, err := e.svc.Create(context.Background(), folderRequest(path))
	if err != nil || project.ID != "small-repo" {
		t.Fatalf("Create after the failure = %+v, %v; the id and the folder must be free again", project, err)
	}
	board, err := e.svc.Board(context.Background(), project.ID)
	if err != nil || len(board.Columns) == 0 {
		t.Errorf("Board = %+v, %v", board, err)
	}
	e.nextType(t, protocol.EventTypeProjectCreated, protocol.HomeTopic)
}

func TestAFailedCardInsertGivesItsNumberBack(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	first := e.card(t, project.ID, "first")
	e.drainEvents()
	allow := e.failInserts(t, "cards")
	for range 3 {
		if _, err := e.svc.CreateCard(context.Background(), project.ID, protocol.CreateCardRequest{Title: "fails"}); err == nil {
			t.Fatal("CreateCard succeeded although the insert failed")
		}
	}
	e.noEvent(t)
	allow()
	second := e.card(t, project.ID, "second")
	if first.Number != 1 || second.Number != 2 {
		t.Errorf("numbers %d and %d; failed creates must not use up numbers", first.Number, second.Number)
	}
}

func TestAFailedStateChangeIsNotPublished(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "x")
	e.drainEvents()
	if err := e.store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetState(context.Background(), card.ID, protocol.CardStateWorking); err == nil {
		t.Fatal("SetState succeeded on a closed store")
	}
	e.noEvent(t)
}
