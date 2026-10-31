package cardpanel_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/cardpanel"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A comment imported from another tool, such as Trello: it is stored as the person's, it is announced
// like any new comment, and it never wakes the card's agent.

// importEnv is the shared env with a service that has an event bus, so a test can see what was
// published.
func importEnv(t *testing.T) (*env, *cardpanel.Service, *events.Bus) {
	t.Helper()
	e := newEnv(t, "Go")
	bus, err := events.New()
	if err != nil {
		t.Fatalf("make the event bus: %v", err)
	}
	t.Cleanup(bus.Close)
	svc, err := cardpanel.New(cardpanel.Deps{
		Store: e.st, Bus: bus, Agent: e.agent, Now: func() time.Time { return testTime },
	})
	if err != nil {
		t.Fatalf("make the card panel: %v", err)
	}
	return e, svc, bus
}

func commentEvents(bus *events.Bus, topic protocol.Topic) []events.Event {
	missed, _ := bus.Since(bus.Epoch(), 0)
	var out []events.Event
	for _, ev := range missed {
		if ev.Topic == string(topic) && ev.Type == string(protocol.EventTypeCommentCreated) {
			out = append(out, ev)
		}
	}
	return out
}

func TestImportCommentStoresThePersonsCommentWithTheTimeItWasWritten(t *testing.T) {
	_, svc, bus := importEnv(t)
	ctx := context.Background()
	wrote := time.Date(2026, time.September, 1, 9, 30, 0, 0, time.UTC)

	id, err := svc.ImportComment(ctx, cardID, "Ada on Trello: ship it", "ada", wrote)
	if err != nil {
		t.Fatalf("ImportComment: %v", err)
	}

	list, err := svc.Comments(ctx, cardID)
	if err != nil || len(list.Comments) != 1 {
		t.Fatalf("Comments = %+v, %v, want the one imported comment", list, err)
	}
	got := list.Comments[0]
	if got.ID != id {
		t.Errorf("comment id = %q, want the id ImportComment answered, %q", got.ID, id)
	}
	if got.AuthorKind != protocol.AuthorKindPerson || got.AuthorID != "ada" {
		t.Errorf("author = %q %q, want the person ada", got.AuthorKind, got.AuthorID)
	}
	if got.Body != "Ada on Trello: ship it" {
		t.Errorf("body = %q, want the text as given", got.Body)
	}
	if !got.CreatedAt.Time().Equal(wrote) {
		t.Errorf("created at %v, want the time it was written, %v", got.CreatedAt.Time(), wrote)
	}
	if got.AgentReadAt != nil {
		t.Errorf("the agent read an imported comment at %v", got.AgentReadAt.Time())
	}
	seen := commentEvents(bus, protocol.CardTopic(cardID))
	if len(seen) != 1 {
		t.Errorf("published %d comment.created events on the card, want 1", len(seen))
	}
}

func TestImportCommentWithNoTimeUsesTheClock(t *testing.T) {
	_, svc, _ := importEnv(t)
	ctx := context.Background()

	if _, err := svc.ImportComment(ctx, cardID, "no date on it", "ada", time.Time{}); err != nil {
		t.Fatalf("ImportComment: %v", err)
	}

	list, _ := svc.Comments(ctx, cardID)
	if got := list.Comments[0].CreatedAt.Time(); !got.Equal(testTime) {
		t.Errorf("created at %v, want the clock's %v", got, testTime)
	}
}

func TestImportCommentCutsATooLongTextByCharactersNotBytes(t *testing.T) {
	_, svc, _ := importEnv(t)
	ctx := context.Background()
	// Each character is three bytes, so a cut by bytes would split one in half.
	long := strings.Repeat("日", protocol.MaxCommentChars+50)

	if _, err := svc.ImportComment(ctx, cardID, long, "ada", testTime); err != nil {
		t.Fatalf("ImportComment: %v", err)
	}

	list, _ := svc.Comments(ctx, cardID)
	body := list.Comments[0].Body
	if got := len([]rune(body)); got != protocol.MaxCommentChars {
		t.Errorf("stored %d characters, want the limit, %d", got, protocol.MaxCommentChars)
	}
	if strings.ContainsRune(body, '�') {
		t.Error("the cut split a character")
	}
}

func TestImportCommentRefusesTextThatIsEmpty(t *testing.T) {
	_, svc, bus := importEnv(t)
	ctx := context.Background()

	for _, body := range []string{"", "   \n\t "} {
		_, err := svc.ImportComment(ctx, cardID, body, "ada", testTime)
		var perr *protocol.Error
		if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeInvalidArgument {
			t.Errorf("ImportComment(%q) answered %v, want an invalid argument", body, err)
		}
	}

	list, _ := svc.Comments(ctx, cardID)
	if len(list.Comments) != 0 {
		t.Errorf("a refused import left %d comments", len(list.Comments))
	}
	if seen := commentEvents(bus, protocol.CardTopic(cardID)); len(seen) != 0 {
		t.Errorf("a refused import published %d events", len(seen))
	}
}

func TestImportCommentOnAnUnknownCardIsNotFound(t *testing.T) {
	_, svc, _ := importEnv(t)

	_, err := svc.ImportComment(context.Background(), "no-such-card", "text", "ada", testTime)

	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeNotFound {
		t.Errorf("answered %v, want not found", err)
	}
}

func TestImportCommentNeverReachesTheAgent(t *testing.T) {
	e, svc, _ := importEnv(t)
	ctx := context.Background()
	// Each of these would wake the agent if a person had posted it.
	for _, body := range []string{"@agent please look", "is this right?", "@agent is this right?"} {
		if _, err := svc.ImportComment(ctx, cardID, body, "ada", testTime); err != nil {
			t.Fatalf("ImportComment(%q): %v", body, err)
		}
	}
	before, _ := svc.Comments(ctx, cardID)
	for _, c := range before.Comments {
		if c.AgentReadAt != nil {
			t.Errorf("imported comment %q was marked read by the agent", c.Body)
		}
	}

	// A comment a person posts the same way does reach it, which proves the fake agent is wired and
	// that the imports above did not send anything first.
	if _, err := svc.Post(ctx, cardID, protocol.PostCommentRequest{Body: "@agent control"},
		cardpanel.Actor{UserID: "ada"}); err != nil {
		t.Fatalf("Post: %v", err)
	}
	waitFor(t, func() bool { return e.agent.count() >= 1 })
	time.Sleep(20 * time.Millisecond)

	e.agent.mu.Lock()
	defer e.agent.mu.Unlock()
	if len(e.agent.sent) != 1 || !strings.Contains(e.agent.sent[0], "control") {
		t.Errorf("the agent was sent %q, want only the posted comment", e.agent.sent)
	}
}
