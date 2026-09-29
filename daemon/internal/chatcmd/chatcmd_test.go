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

// Accepts is true for chat "1", the one the fake bot stands for.
func (f *fakeBot) Accepts(in chatbot.Incoming) bool { return in.ChatID == "1" }

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

// fakeCards is a set of projects and a record of the cards a chat asked for.
type fakeCards struct {
	mu       sync.Mutex
	projects []chatcmd.Project
	created  []protocol.CreateCardRequest
	inside   []string
	fail     error
}

func (f *fakeCards) Projects(context.Context) ([]chatcmd.Project, error) {
	return f.projects, nil
}

func (f *fakeCards) CreateCard(_ context.Context, projectID string, in protocol.CreateCardRequest) (protocol.Card, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return protocol.Card{}, f.fail
	}
	f.created = append(f.created, in)
	f.inside = append(f.inside, projectID)
	return protocol.Card{ID: "card-1", Title: in.Title}, nil
}

// hearWithCards drives one message through a service that can add cards.
func hearWithCards(t *testing.T, cards *fakeCards, text string) string {
	t.Helper()
	bot := &fakeBot{}
	svc, err := chatcmd.New(&fakeApprover{}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc.WithCards(cards)
	if err := svc.Run(context.Background(), bot); err != nil {
		t.Fatalf("Run: %v", err)
	}
	bot.start(context.Background(), chatbot.Incoming{ChatID: "1", Text: text})
	bot.mu.Lock()
	defer bot.mu.Unlock()
	if len(bot.sent) == 0 {
		return ""
	}
	return bot.sent[len(bot.sent)-1]
}

func twoProjects() *fakeCards {
	return &fakeCards{projects: []chatcmd.Project{{ID: "api", Name: "api-gateway"}, {ID: "web", Name: "web-dashboard"}}}
}

func TestANewMessageAddsACardInTheProjectItNames(t *testing.T) {
	cards := twoProjects()
	reply := hearWithCards(t, cards, "new web Fix the flaky login test")
	if len(cards.created) != 1 || cards.inside[0] != "web" || cards.created[0].Title != "Fix the flaky login test" {
		t.Fatalf("the cards made were %+v in %v, want one in web", cards.created, cards.inside)
	}
	if reply != `Added "Fix the flaky login test" to web-dashboard.` {
		t.Errorf("the reply is %q, want it to say where the card went", reply)
	}
}

func TestAProjectIsFoundByNameOrByTheStartOfIt(t *testing.T) {
	cards := twoProjects()
	hearWithCards(t, cards, "new api-gateway: Rotate the keys")
	hearWithCards(t, cards, "new web-d Add a chart")
	if len(cards.inside) != 2 || cards.inside[0] != "api" || cards.inside[1] != "web" {
		t.Errorf("the cards went to %v, want api then web", cards.inside)
	}
}

func TestWithOneProjectTheWholeMessageIsTheTitle(t *testing.T) {
	cards := &fakeCards{projects: []chatcmd.Project{{ID: "api", Name: "api-gateway"}}}
	reply := hearWithCards(t, cards, "new Fix the login")
	if len(cards.created) != 1 || cards.created[0].Title != "Fix the login" || cards.inside[0] != "api" {
		t.Fatalf("the cards made were %+v in %v, want the whole message as the title", cards.created, cards.inside)
	}
	if reply == "" {
		t.Error("nothing was said back")
	}
}

func TestAnUnknownProjectAddsNothingAndListsTheOnesThereAre(t *testing.T) {
	cards := twoProjects()
	reply := hearWithCards(t, cards, "new mobile Fix the icon")
	if len(cards.created) != 0 {
		t.Fatalf("a card was added to a project that does not exist: %+v", cards.created)
	}
	if reply != `Marshal has no project called "mobile". Choose one of: api-gateway, web-dashboard.` {
		t.Errorf("the reply is %q", reply)
	}
}

func TestTheDaemonsSentenceIsSaidWhenACardIsRefused(t *testing.T) {
	cards := twoProjects()
	cards.fail = protocol.InvalidArgument("A card needs a title.")
	if reply := hearWithCards(t, cards, "new web Something"); reply != "A card needs a title." {
		t.Errorf("the reply is %q, want the daemon's own sentence", reply)
	}
}

func TestAChatWithNoCardsSetUpSaysSoInsteadOfStayingSilent(t *testing.T) {
	reply, _ := hear(t, &fakeApprover{}, "new web Fix it")
	if reply != "Adding a card from a chat is not set up on this computer." {
		t.Errorf("the reply is %q", reply)
	}
}

// A message from a chat the connection is not for is not acted on and gets no answer at all.
func TestAMessageFromAnotherChatIsIgnoredWithoutAReply(t *testing.T) {
	approver := &fakeApprover{}
	svc, err := chatcmd.New(approver, nil)
	if err != nil {
		t.Fatal(err)
	}
	bot := &fakeBot{}
	if err := svc.Run(context.Background(), bot); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"approve 01H1234567890ABCDEFGHJKMNPQ", "new api Do a thing", "status", "hello"} {
		bot.start(context.Background(), chatbot.Incoming{ChatID: "999", Text: text})
	}
	bot.start(context.Background(), chatbot.Incoming{ChatID: "999", Voice: true})
	if len(approver.all()) != 0 {
		t.Fatalf("a stranger answered an approval: %+v", approver.all())
	}
	bot.mu.Lock()
	defer bot.mu.Unlock()
	if len(bot.sent) != 0 {
		t.Fatalf("a stranger got replies: %v", bot.sent)
	}
}
