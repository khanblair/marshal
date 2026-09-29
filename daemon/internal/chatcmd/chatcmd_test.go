package chatcmd_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/chatcmd"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Turning what a person types into a decision (B9.3, build-plan 9.5 and 9.6). Everything here is
// driven through two fakes, so nothing reaches a chat service or a session.

// asked is one call the fake approver was given.
type asked struct {
	id       string
	decision protocol.ApprovalDecision
	option   string
	actor    string
}

type fakeApprover struct {
	mu      sync.Mutex
	calls   []asked
	respond error
}

func (f *fakeApprover) Respond(_ context.Context, id string, decision protocol.ApprovalDecision, optionID, actor string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.respond != nil {
		return f.respond
	}
	f.calls = append(f.calls, asked{id: id, decision: decision, option: optionID, actor: actor})
	return nil
}

func (f *fakeApprover) all() []asked {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]asked(nil), f.calls...)
}

// fakeBot remembers what the service sent back. It never contacts a service; its receive loop is
// driven by the test calling the handler directly.
type fakeBot struct {
	mu    sync.Mutex
	sent  []string
	start chatbot.Handler
}

func (f *fakeBot) Kind() chatbot.Kind { return chatbot.KindTelegram }
func (f *fakeBot) Notify(_ context.Context, notice chatbot.Notice) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, notice.Title)
	return nil
}
func (f *fakeBot) Start(_ context.Context, handle chatbot.Handler) error {
	f.start = handle
	return nil
}
func (f *fakeBot) Test(context.Context) (protocol.TestResult, error) {
	return protocol.TestResult{}, nil
}
func (f *fakeBot) Close() error { return nil }

// hear drives one message through the receive loop and answers what the bot said back.
func hear(t *testing.T, approver *fakeApprover, text string) (string, []asked) {
	t.Helper()
	bot := &fakeBot{}
	svc, err := chatcmd.New(approver, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := svc.Run(context.Background(), bot); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if bot.start == nil {
		t.Fatal("the bot's receive loop was never started")
	}
	bot.start(context.Background(), chatbot.Incoming{ChatID: "1", Text: text})
	bot.mu.Lock()
	defer bot.mu.Unlock()
	var last string
	if len(bot.sent) > 0 {
		last = bot.sent[len(bot.sent)-1]
	}
	return last, approver.all()
}

func TestAnApprovalIsAnsweredWithTheIdAPersonSent(t *testing.T) {
	approver := &fakeApprover{}
	reply, calls := hear(t, approver, "approve 01H1234567890ABCDEFGHJKMNPQ")
	if len(calls) != 1 {
		t.Fatalf("the approver was asked %d times, want 1", len(calls))
	}
	if calls[0].id != "01H1234567890ABCDEFGHJKMNPQ" {
		t.Errorf("the approval id is %q", calls[0].id)
	}
	if calls[0].decision != protocol.ApprovalDecisionApproved {
		t.Errorf("the decision is %q, want approved", calls[0].decision)
	}
	if calls[0].actor != "person" {
		t.Errorf("the actor is %q: a decision from a chat is a person's", calls[0].actor)
	}
	if reply == "" {
		t.Error("nothing was said back, so a person cannot tell it landed")
	}
}

func TestARejectIsADenial(t *testing.T) {
	approver := &fakeApprover{}
	_, calls := hear(t, approver, "Reject 01H1234567890ABCDEFGHJKMNPQ")
	if len(calls) != 1 || calls[0].decision != protocol.ApprovalDecisionDenied {
		t.Fatalf("the approver was asked %+v, want one denial", calls)
	}
}

func TestAPressedButtonCarriesItsIdAndOption(t *testing.T) {
	approver := &fakeApprover{}
	_, calls := hear(t, approver, "approval:01H1234567890ABCDEFGHJKMNPQ:allow_always")
	if len(calls) != 1 {
		t.Fatalf("the approver was asked %d times, want 1", len(calls))
	}
	if calls[0].id != "01H1234567890ABCDEFGHJKMNPQ" || calls[0].option != "allow_always" {
		t.Errorf("the call was %+v, want the id and its option", calls[0])
	}
}

func TestTheOptionIsReadAfterTheIdWhenTyped(t *testing.T) {
	approver := &fakeApprover{}
	_, calls := hear(t, approver, "approve 01H1234567890ABCDEFGHJKMNPQ:allow_always")
	if len(calls) != 1 || calls[0].id != "01H1234567890ABCDEFGHJKMNPQ" || calls[0].option != "allow_always" {
		t.Fatalf("the call was %+v", calls)
	}
}

func TestAMessageMarshalDoesNotUnderstandGetsTheHelpText(t *testing.T) {
	approver := &fakeApprover{}
	reply, calls := hear(t, approver, "what is for lunch")
	if len(calls) != 0 {
		t.Fatalf("a message that named no approval decided one: %+v", calls)
	}
	if reply == "" {
		t.Error("nothing was said back, so a message that was ignored looks like a broken bot")
	}
}

func TestAVoiceNoteIsAskedForInWordsRatherThanGuessedAt(t *testing.T) {
	approver := &fakeApprover{}
	bot := &fakeBot{}
	svc, err := chatcmd.New(approver, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := svc.Run(context.Background(), bot); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// A voice note is handed over as a voice note, with no text to read.
	bot.start(context.Background(), chatbot.Incoming{ChatID: "1", Voice: true})
	bot.mu.Lock()
	if len(bot.sent) == 0 {
		bot.mu.Unlock()
		t.Fatal("a voice note was ignored rather than answered")
	}
	bot.mu.Unlock()
	if calls := approver.all(); len(calls) != 0 {
		t.Errorf("a voice note decided an approval: %+v", calls)
	}
}

func TestADecisionTheDaemonRefusesIsSaidInTheDaemonsOwnWords(t *testing.T) {
	approver := &fakeApprover{respond: protocol.NotFound("approval").With("id", "app_1")}
	reply, _ := hear(t, approver, "approve 01H1234567890ABCDEFGHJKMNPQ")
	if reply == "" {
		t.Fatal("a refused decision said nothing back")
	}
}

func TestAServiceWithNoApproverIsRefused(t *testing.T) {
	if _, err := chatcmd.New(nil, nil); err == nil {
		t.Fatal("a service that cannot act on what it reads was built anyway")
	}
}

// A failure with no protocol message is the daemon's own problem, not the person's: it is logged,
// and the person is told something they can act on rather than shown the internals.
func TestAnInternalFailureIsNotReadOutLoud(t *testing.T) {
	approver := &fakeApprover{respond: errors.New("the database is gone")}
	reply, _ := hear(t, approver, "approve 01H1234567890ABCDEFGHJKMNPQ")
	if reply == "" {
		t.Fatal("a failure said nothing back")
	}
	if reply == "the database is gone" {
		t.Error("an internal error was read out loud to a person")
	}
}
