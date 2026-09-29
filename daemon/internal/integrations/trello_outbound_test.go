package integrations_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The Marshal-to-Trello half of the move sync (B8.2, trello_outbound.go): a card that reaches
// done, and is linked to a Trello card, is moved to a list on the board that reads as done.

// eventually waits for a check to pass, so a test follows the sync's own goroutine instead of
// guessing how long it takes.
func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// outboundFixture is a connections service linked to a project and a fake Trello board with two
// lists, one of them named "Done", plus the moves the fake board received. The board answers on the
// httptest server's own goroutine, so `moved` is behind a mutex: the test's assertions and the
// handler both touch it, from different goroutines, while the sync's own watcher is running.
type outboundFixture struct {
	*fixture
	bus  *events.Bus
	sync *integrations.OutboundSync

	mu    sync.Mutex
	moved []string
}

// record adds a move the fake board received, for the handler goroutine.
func (of *outboundFixture) record(list string) {
	of.mu.Lock()
	defer of.mu.Unlock()
	of.moved = append(of.moved, list)
}

// movedList reads the moves the fake board has received so far, for the test goroutine.
func (of *outboundFixture) movedList() []string {
	of.mu.Lock()
	defer of.mu.Unlock()
	return append([]string(nil), of.moved...)
}

func newOutboundFixture(t *testing.T) *outboundFixture {
	t.Helper()
	of := &outboundFixture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/lists"):
			_, _ = w.Write([]byte(`[{"id":"list-doing","name":"Doing"},{"id":"list-done","name":"Done"}]`))
		case strings.HasPrefix(r.URL.Path, "/cards/") && r.Method == http.MethodPut:
			of.record(r.URL.Query().Get("idList"))
			_, _ = w.Write([]byte(`{"id":"trello-card-9","idList":"list-done"}`))
		default:
			_, _ = w.Write([]byte(`{"id":"board-1","name":"Board","url":"https://trello.com/b/board-1"}`))
		}
	}))
	t.Cleanup(server.Close)
	of.fixture = newFixture(t, func(o *integrations.Options) {
		o.TrelloBaseURL = server.URL
		o.Now = func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) }
	})
	save := trelloSave()
	save.ProjectID = "small-repo"
	if err := of.svc.SaveTrello(context.Background(), save); err != nil {
		t.Fatalf("SaveTrello: %v", err)
	}
	bus, err := events.New()
	if err != nil {
		t.Fatalf("make the event bus: %v", err)
	}
	t.Cleanup(bus.Close)
	sync, err := integrations.NewOutboundSync(of.svc, bus)
	if err != nil {
		t.Fatalf("NewOutboundSync: %v", err)
	}
	t.Cleanup(func() { _ = sync.Close() })
	of.bus, of.sync = bus, sync
	return of
}

func TestOutboundSyncMovesALinkedCardToTrellosDoneList(t *testing.T) {
	of := newOutboundFixture(t)
	seedCardRow(t, of.fixture, "marshal-card-1")
	if err := of.svc.LinkExternal(context.Background(), "marshal-card-1", trello.ID, "trello-card-9"); err != nil {
		t.Fatalf("LinkExternal: %v", err)
	}
	if err := of.sync.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	of.bus.Publish(string(protocol.ProjectTopic("p-marshal-card-1")), string(protocol.EventTypeCardMoved),
		protocol.CardMovedEventData{
			Card: protocol.Card{ID: "marshal-card-1", State: protocol.CardStateDone},
			From: protocol.CardStateReview,
		}, true)

	eventually(t, "the Trello card to move", func() bool { return len(of.movedList()) == 1 })
	if moved := of.movedList(); moved[0] != "list-done" {
		t.Errorf("moved to list %q, want list-done", moved[0])
	}
}

func TestOutboundSyncIgnoresAnUnlinkedCard(t *testing.T) {
	of := newOutboundFixture(t)
	seedCardRow(t, of.fixture, "marshal-card-2")
	if err := of.sync.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	of.bus.Publish(string(protocol.ProjectTopic("p-marshal-card-2")), string(protocol.EventTypeCardMoved),
		protocol.CardMovedEventData{
			Card: protocol.Card{ID: "marshal-card-2", State: protocol.CardStateDone},
			From: protocol.CardStateReview,
		}, true)

	// There is no positive signal to wait for, so this proves the negative by giving the
	// subscriber's goroutine time to have acted if it were going to.
	time.Sleep(50 * time.Millisecond)
	if moved := of.movedList(); len(moved) != 0 {
		t.Errorf("moved = %v, want no move for an unlinked card", moved)
	}
}

func TestOutboundSyncIgnoresAMoveThatIsNotIntoDone(t *testing.T) {
	of := newOutboundFixture(t)
	seedCardRow(t, of.fixture, "marshal-card-3")
	if err := of.svc.LinkExternal(context.Background(), "marshal-card-3", trello.ID, "trello-card-3"); err != nil {
		t.Fatalf("LinkExternal: %v", err)
	}
	if err := of.sync.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	of.bus.Publish(string(protocol.ProjectTopic("p-marshal-card-3")), string(protocol.EventTypeCardMoved),
		protocol.CardMovedEventData{
			Card: protocol.Card{ID: "marshal-card-3", State: protocol.CardStateReview},
			From: protocol.CardStateWorking,
		}, true)

	time.Sleep(50 * time.Millisecond)
	if moved := of.movedList(); len(moved) != 0 {
		t.Errorf("moved = %v, want no move into a state that is not done", moved)
	}
}
