package projects

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// A card cannot wait for itself. No caller can produce that today - a card is created before its
// edges are written, and a new card's number is one no card has held - so the check is reached here
// directly, handing insertCardLinks a card and its own key. It is kept because the check is what
// makes "no cycle is possible" true of the function itself rather than only of its caller.
func TestInsertCardLinksRefusesACardWaitingForItself(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})
	bus, err := events.New(events.WithEpoch("test-epoch"))
	if err != nil {
		t.Fatalf("make the bus: %v", err)
	}
	t.Cleanup(bus.Close)
	svc, err := New(Deps{Store: st, Bus: bus, Git: testutil.Git(), DataDir: filepath.Join(dir, "data")}, WithLocalClones(true))
	if err != nil {
		t.Fatalf("make the service: %v", err)
	}
	project, err := svc.Create(ctx, CreateInput{Source: protocol.ProjectSourceFolder, Path: testutil.Fixture(t, "small-repo")})
	if err != nil {
		t.Fatalf("create a project: %v", err)
	}
	card, err := svc.CreateCard(ctx, project.ID, protocol.CreateCardRequest{Title: "One"})
	if err != nil {
		t.Fatalf("create a card: %v", err)
	}
	key := protocol.CardKey{ProjectID: project.ID, Number: card.Number}

	err = insertCardLinks(ctx, st.Queries(), project.ID, card.ID, []protocol.CardKey{key})
	if err == nil {
		t.Fatal("a card was allowed to wait for itself")
	}
	var perr *protocol.Error
	if !errors.As(err, &perr) {
		t.Fatalf("the refusal is %v, want a protocol error", err)
	}
	if perr.Code != protocol.ErrorCodeInvalidArgument || !strings.Contains(perr.Message, "cannot depend on itself") {
		t.Errorf("the refusal is %s %q, want invalid_argument saying a card cannot depend on itself", perr.Code, perr.Message)
	}
	if perr.Details["dependsOn"] != key.String() {
		t.Errorf("the refusal names dependsOn %q, want %q", perr.Details["dependsOn"], key.String())
	}
	// Nothing was written: the card does not wait for itself in the store either.
	numbers, err := st.Queries().ListCardDependencyNumbers(ctx, card.ID)
	if err != nil {
		t.Fatalf("read what the card waits for: %v", err)
	}
	if len(numbers) != 0 {
		t.Errorf("the card still waits for %v", numbers)
	}
}
