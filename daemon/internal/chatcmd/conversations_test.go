package chatcmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
)

type conversationBot struct {
	scope     string
	handler   chatbot.Handler
	sent      []chatbot.Notice
	addresses []chatbot.Incoming
	fail      bool
}

func (b *conversationBot) Scope() string                    { return b.scope }
func (b *conversationBot) Kind() chatbot.Kind               { return chatbot.KindTelegram }
func (b *conversationBot) Accepts(in chatbot.Incoming) bool { return in.ChatID == "owner" }
func (b *conversationBot) Close() error                     { return nil }
func (b *conversationBot) Test(context.Context) (protocol.TestResult, error) {
	return protocol.TestResult{}, nil
}
func (b *conversationBot) Start(_ context.Context, h chatbot.Handler) error {
	b.handler = h
	return nil
}
func (b *conversationBot) Notify(ctx context.Context, n chatbot.Notice) error {
	_, err := b.SendReply(ctx, n, chatbot.Incoming{})
	return err
}
func (b *conversationBot) SendReply(_ context.Context, n chatbot.Notice, to chatbot.Incoming) (string, error) {
	if b.fail {
		return "", errors.New("offline")
	}
	b.sent = append(b.sent, n)
	b.addresses = append(b.addresses, to)
	return fmt.Sprint(len(b.sent)), nil
}

type conversationControl struct {
	cards          map[string]protocol.Card
	text, revision string
	calls          []string
}

func (c *conversationControl) Respond(context.Context, string, protocol.ApprovalDecision, string, string) error {
	return nil
}
func (c *conversationControl) Card(_ context.Context, key protocol.CardKey) (protocol.Card, error) {
	return c.cards[key.String()], nil
}
func (c *conversationControl) Cards(context.Context) ([]protocol.Card, error) { return nil, nil }
func (c *conversationControl) Start(_ context.Context, id string) (protocol.Card, error) {
	c.calls = append(c.calls, "start "+id)
	return protocol.Card{ID: id}, nil
}
func (c *conversationControl) Stop(_ context.Context, id string) error {
	c.calls = append(c.calls, "stop "+id)
	return nil
}
func (c *conversationControl) Send(_ context.Context, id, text string) error {
	c.calls = append(c.calls, id+" "+text)
	return nil
}
func (c *conversationControl) Reply(context.Context, string) (string, error) { return c.text, nil }
func (c *conversationControl) Answer(context.Context, string) (string, string, error) {
	return c.text, c.revision, nil
}
func (c *conversationControl) SetAgent(context.Context, string, protocol.AgentKind) error { return nil }
func (c *conversationControl) Finish(context.Context, string) error                       { return nil }

func conversationFixture(t *testing.T) (*Service, *conversationBot, *conversationControl, *store.Store) {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	awake := protocol.SessionStateAwake
	c := &conversationControl{cards: map[string]protocol.Card{
		"web#1": {ID: "card-a", Key: "web#1", Session: &awake}, "web#2": {ID: "card-b", Key: "web#2", Session: &awake},
	}}
	s, err := New(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.WithControl(c).WithConversations(st)
	b := &conversationBot{scope: "test-scope"}
	// No polling goroutine: tests drive the same check deterministically.
	b.handler = func(ctx context.Context, in chatbot.Incoming) {
		if !b.Accepts(in) {
			return
		}
		state, err := s.conversation(ctx, b.Scope())
		if err != nil {
			t.Fatal(err)
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		s.handle(ctx, &linkedBot{ConversationBot: b, svc: s, state: state, in: in}, in)
	}
	return s, b, c, st
}

func TestRepliesStayWithTheirCardAndSurviveRestart(t *testing.T) {
	_, b, c, st := conversationFixture(t)
	b.handler(t.Context(), chatbot.Incoming{ChatID: "owner", MessageID: "in-a", Text: "status web#1"})
	first := fmt.Sprint(len(b.sent))
	b.handler(t.Context(), chatbot.Incoming{ChatID: "owner", MessageID: "in-b", Text: "status web#2"})
	second := fmt.Sprint(len(b.sent))
	restarted, _ := New(c, nil)
	restarted.WithControl(c).WithConversations(st)
	state, err := restarted.conversation(t.Context(), b.Scope())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ reply, text, want string }{{first, "fix the header", "card-a fix the header"}, {second, "fix the footer", "card-b fix the footer"}} {
		in := chatbot.Incoming{ChatID: "owner", MessageID: "follow", ReplyToID: tc.reply, Text: tc.text}
		restarted.handle(t.Context(), &linkedBot{ConversationBot: b, svc: restarted, state: state, in: in}, in)
		if got := c.calls[len(c.calls)-1]; got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
	before := len(c.calls)
	b.handler(t.Context(), chatbot.Incoming{ChatID: "stranger", ReplyToID: first, Text: "delete everything"})
	b.handler(t.Context(), chatbot.Incoming{ChatID: "owner", ReplyToID: "unknown", Text: "delete everything"})
	if len(c.calls) != before {
		t.Fatal("untrusted or unlinked text reached the agent")
	}
	other, _ := restarted.conversation(t.Context(), "different-connection")
	if len(other.Links) != 0 {
		t.Fatal("links leaked across connections")
	}
}

func TestThreadCannotSwitchCardsAndCommandsNeedNoRepeatedKey(t *testing.T) {
	_, b, c, _ := conversationFixture(t)
	in := chatbot.Incoming{ChatID: "owner", ThreadID: "task-thread", MessageID: "first", Text: "status web#1"}
	b.handler(t.Context(), in)
	in.Text = "message web#2 wrong card"
	b.handler(t.Context(), in)
	if len(c.calls) != 0 {
		t.Fatal("thread redirected another card")
	}
	in.Text = "stop"
	b.handler(t.Context(), in)
	if len(c.calls) != 1 || c.calls[0] != "stop card-a" {
		t.Fatalf("contextual command: %v", c.calls)
	}
	in.Text = "/typo"
	b.handler(t.Context(), in)
	if len(c.calls) != 1 {
		t.Fatal("unknown slash command became editing instructions")
	}
}

func TestAutomaticAnswerWaitsForIdleRetriesAndDoesNotRepeatAfterRestart(t *testing.T) {
	s, b, c, st := conversationFixture(t)
	b.handler(t.Context(), chatbot.Incoming{ChatID: "owner", MessageID: "command", Text: "start web#1"})
	state, _ := s.conversation(t.Context(), b.Scope())
	c.text = "Implemented the feature. Tests passed."
	c.revision = "turn-1"
	working := protocol.SessionStateWorking
	card := c.cards["web#1"]
	card.Session = &working
	c.cards["web#1"] = card
	before := len(b.sent)
	s.checkAnswers(t.Context(), b, state)
	if len(b.sent) != before {
		t.Fatal("sent partial answer while working")
	}
	awake := protocol.SessionStateAwake
	card.Session = &awake
	c.cards["web#1"] = card
	b.fail = true
	s.checkAnswers(t.Context(), b, state)
	if state.Cards[0].Revision == c.revision {
		t.Fatal("failed send marked delivered")
	}
	b.fail = false
	s.checkAnswers(t.Context(), b, state)
	if len(b.sent) != before+1 || !strings.Contains(b.sent[len(b.sent)-1].Title, c.text) {
		t.Fatal("automatic answer missing")
	}
	if b.addresses[len(b.addresses)-1].MessageID != "command" {
		t.Fatal("answer lost its conversation")
	}
	s.checkAnswers(t.Context(), b, state)
	restarted, _ := New(c, nil)
	restarted.WithControl(c).WithConversations(st)
	reloaded, _ := restarted.conversation(t.Context(), b.Scope())
	restarted.checkAnswers(t.Context(), b, reloaded)
	if len(b.sent) != before+1 {
		t.Fatal("delivered answer repeated")
	}
	c.revision = "turn-2" // Identical words in a different turn still deserve delivery.
	restarted.checkAnswers(t.Context(), b, reloaded)
	if len(b.sent) != before+2 {
		t.Fatal("second identical answer suppressed")
	}
}

func TestNormalApprovalNoticeKeepsButtonsAndReplyLink(t *testing.T) {
	s, b, _, _ := conversationFixture(t)
	b.handler(t.Context(), chatbot.Incoming{ChatID: "owner", ThreadID: "task-thread", MessageID: "command", Text: "status web#1"})
	notice := chatbot.Notice{CardID: "card-a", Title: "Run tests?", Actions: []chatbot.Action{{Label: "Allow once", Data: "approval:original-id:allow_once"}}}
	handled, err := s.Deliver(t.Context(), b, notice)
	if !handled || err != nil {
		t.Fatalf("route approval: %v %v", handled, err)
	}
	got := b.sent[len(b.sent)-1]
	if len(got.Actions) != 1 || got.Actions[0] != notice.Actions[0] {
		t.Fatal("approval options changed")
	}
	if b.addresses[len(b.addresses)-1].ThreadID != "task-thread" {
		t.Fatal("approval left the card thread")
	}
	handled, err = s.Deliver(t.Context(), b, chatbot.Notice{CardID: "unlinked", Title: "Other card"})
	if handled || err != nil {
		t.Fatal("unlinked notice should keep normal routing")
	}
}

func TestUnavailableLinkedConversationNeverFallsBackToMainChannel(t *testing.T) {
	s, b, _, _ := conversationFixture(t)
	b.handler(t.Context(), chatbot.Incoming{ChatID: "owner", MessageID: "command", Text: "status web#1"})
	state, _ := s.conversation(t.Context(), b.Scope())
	state.Cards[0].To.ChatID = "no-longer-accepted"
	before := len(b.sent)
	handled, err := s.Deliver(t.Context(), b, chatbot.Notice{CardID: "card-a", Title: "Private thread notice"})
	if !handled || err == nil || len(b.sent) != before {
		t.Fatal("unavailable linked card fell through to main-channel notification")
	}
}

func TestContextualMessageKeepsAllInstructions(t *testing.T) {
	_, b, c, _ := conversationFixture(t)
	in := chatbot.Incoming{ChatID: "owner", ThreadID: "thread", MessageID: "command", Text: "status web#1"}
	b.handler(t.Context(), in)
	in.Text = "message fix the header\nthen run tests"
	b.handler(t.Context(), in)
	if len(c.calls) != 1 || c.calls[0] != "card-a fix the header\nthen run tests" {
		t.Fatalf("lost contextual instruction: %v", c.calls)
	}
}
