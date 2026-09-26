package fixture_test

import (
	"context"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/fixture"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The fixture writes the prototype's 29 cards: 11 in api, 9 in web, 9 in mobile, with the
// prototype's own numbers and titles. This is what makes the screens look the same against the
// daemon, and what the end-to-end specs open.
func TestLoadPrototypeWritesThePrototypesCards(t *testing.T) {
	e := newEnv(t)
	if err := e.load(t); err != nil {
		t.Fatalf("LoadPrototype: %v", err)
	}
	want := map[string]int{"api": 11, "web": 9, "mobile": 9}
	for id, count := range want {
		board, err := e.svc.Board(context.Background(), id)
		if err != nil {
			t.Fatalf("Board(%s): %v", id, err)
		}
		if len(board.Cards) != count {
			t.Errorf("project %s has %d cards, want %d", id, len(board.Cards), count)
		}
	}
	card, err := e.svc.CardByKey(context.Background(), protocol.CardKey{ProjectID: "api", Number: 41})
	if err != nil {
		t.Fatalf("card api#41: %v", err)
	}
	if card.Title != "Fix token refresh on login" {
		t.Errorf("api#41 is %q, want the prototype's title", card.Title)
	}
	if card.State != protocol.CardStateWorking || card.DoingNow != "Running auth tests" {
		t.Errorf("api#41 = %s / %q, want working and its doing line", card.State, card.DoingNow)
	}
	if card.ContextUsed != 46 {
		t.Errorf("api#41 context = %d, want 46", card.ContextUsed)
	}
	if card.PlannedStart == nil || card.PlannedEnd == nil {
		t.Error("api#41 has no planned dates, and the Timeline draws them")
	}
}

// The fields the screens show come through: roles, packages, labels, pull requests, CI states, and
// the needs-you reasons.
func TestLoadPrototypeWritesTheCardFields(t *testing.T) {
	e := newEnv(t)
	if err := e.load(t); err != nil {
		t.Fatalf("LoadPrototype: %v", err)
	}
	ctx := context.Background()

	// A card with a plan waiting for review.
	waiting, err := e.svc.CardByKey(ctx, protocol.CardKey{ProjectID: "api", Number: 43})
	if err != nil {
		t.Fatalf("api#43: %v", err)
	}
	if waiting.State != protocol.CardStateNeeds || waiting.NeedsReason == nil {
		t.Fatalf("api#43 = %s / %+v, want it waiting on a person", waiting.State, waiting.NeedsReason)
	}
	if waiting.NeedsReason.Kind != protocol.NeedsReasonKindPlanReady {
		t.Errorf("reason = %s, want plan-ready", waiting.NeedsReason.Kind)
	}
	if waiting.NeedsReason.Text != "Plan ready for review" {
		t.Errorf("reason text = %q, want the prototype's sentence", waiting.NeedsReason.Text)
	}
	// Labels came through, and the project has them as a managed list.
	if len(waiting.Labels) == 0 || waiting.Labels[0].Name != "feature" {
		t.Errorf("api#43 labels = %+v, want the prototype's", waiting.Labels)
	}
	labels, err := e.svc.Labels(ctx, "api")
	if err != nil {
		t.Fatalf("Labels: %v", err)
	}
	if len(labels.Labels) < 4 {
		t.Errorf("the api project has %d labels, want the prototype's set", len(labels.Labels))
	}
	for _, label := range labels.Labels {
		if !label.Color.Valid() {
			t.Errorf("label %q has color %q, which is not one of the fixed set", label.Name, label.Color)
		}
	}

	// A card with a pull request and a CI state, and one with a package and a role.
	review, err := e.svc.CardByKey(ctx, protocol.CardKey{ProjectID: "api", Number: 39})
	if err != nil {
		t.Fatalf("api#39: %v", err)
	}
	if review.PullRequest == nil || review.PullRequest.Number == 0 {
		t.Errorf("api#39 has no pull request: %+v", review.PullRequest)
	}
	packaged, err := e.svc.CardByKey(ctx, protocol.CardKey{ProjectID: "mobile", Number: 209})
	if err != nil {
		t.Fatalf("mobile#209: %v", err)
	}
	if packaged.Package == "" || packaged.Role == "" {
		t.Errorf("mobile#209 = package %q role %q, want the prototype's", packaged.Package, packaged.Role)
	}
	// The three agents the prototype shows.
	kinds := map[protocol.AgentKind]bool{}
	for _, id := range []string{"api", "web", "mobile"} {
		cards, err := e.svc.Cards(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, card := range cards {
			kinds[card.Agent] = true
		}
	}
	for _, kind := range []protocol.AgentKind{protocol.AgentKindClaude, protocol.AgentKindCodex, protocol.AgentKindGemini} {
		if !kinds[kind] {
			t.Errorf("no fixture card uses the %s agent", kind)
		}
	}
}

// Loading twice changes nothing: the fixture is safe on every start.
func TestLoadPrototypeWithCardsIsSafeToRepeat(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i := 1; i <= 2; i++ {
		if err := e.load(t); err != nil {
			t.Fatalf("LoadPrototype (run %d): %v", i, err)
		}
	}
	board, err := e.svc.Board(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Cards) != 11 {
		t.Errorf("api has %d cards after two loads, want 11", len(board.Cards))
	}
	labels, err := e.svc.Labels(ctx, "api")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(labels.Labels))
	for _, label := range labels.Labels {
		names = append(names, label.Name)
	}
	for _, name := range names {
		if strings.Count(strings.Join(names, ","), name) > 1 {
			t.Errorf("the label %q was made twice", name)
		}
	}
}

// A card a person adds after the fixture gets the number after the highest one the fixture wrote,
// so no two cards share a number.
func TestTheCardCounterIsPastTheFixturesNumbers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.load(t); err != nil {
		t.Fatalf("LoadPrototype: %v", err)
	}
	card, err := e.svc.CreateCard(ctx, "api", projects.CardInput{Title: "A new card"})
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if card.Number != 47 {
		t.Errorf("the new card is %s, want api#47 (46 is the highest the fixture wrote)", card.Key)
	}
}

// fakeSessions records the sessions the fixture asked for, so a fixture test can check which cards
// got one and in which state without a session manager.
type fakeSessions struct {
	seen map[string]protocol.SessionState
}

func (f *fakeSessions) SeedSession(_ context.Context, cardID string, state protocol.SessionState) error {
	if f.seen == nil {
		f.seen = map[string]protocol.SessionState{}
	}
	f.seen[cardID] = state
	return nil
}

// The fixture seeds a session for every card that has started, so the screens that draw sessions
// have something to draw: Home's awake list and the Agents view. A card in the backlog or done has
// none, because it was never started or it is merged and closed.
func TestLoadPrototypeSeedsTheSessionsTheScreensDraw(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sessions := &fakeSessions{}
	if err := e.load(t, fixture.WithSessions(sessions)); err != nil {
		t.Fatalf("LoadPrototype: %v", err)
	}
	// Every card with a session is a card that has started, with the state its column is drawn
	// with: working for a card whose agent is on a turn, awake for one that finished and waits.
	want := map[protocol.CardState]protocol.SessionState{
		protocol.CardStateWorking:  protocol.SessionStateWorking,
		protocol.CardStatePlanning: protocol.SessionStateWorking,
		protocol.CardStateMerging:  protocol.SessionStateWorking,
		protocol.CardStateNeeds:    protocol.SessionStateAwake,
		protocol.CardStateReview:   protocol.SessionStateAwake,
		protocol.CardStateReady:    protocol.SessionStateAwake,
	}
	started := 0
	for _, id := range []string{"api", "web", "mobile"} {
		board, err := e.svc.Board(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, card := range board.Cards {
			wanted, ok := want[card.State]
			if !ok {
				if _, seeded := sessions.seen[card.ID]; seeded {
					t.Errorf("%s is in %s and should have no session", card.Key, card.State)
				}
				continue
			}
			started++
			if got := sessions.seen[card.ID]; got != wanted {
				t.Errorf("%s is in %s and its session is %q, want %q", card.Key, card.State, got, wanted)
			}
		}
	}
	if started != len(sessions.seen) {
		t.Errorf("the fixture seeded %d sessions, want %d (one per started card)", len(sessions.seen), started)
	}
	if started != 20 {
		t.Errorf("the prototype has %d started cards, want 20", started)
	}
}

// Loading the fixture twice asks for the same sessions again, and the session manager keeps one row
// per card (SeedSession leaves a row that is already there alone).
func TestLoadPrototypeAsksForTheSameSessionsTwice(t *testing.T) {
	e := newEnv(t)
	sessions := &fakeSessions{}
	for i := 1; i <= 2; i++ {
		if err := e.load(t, fixture.WithSessions(sessions)); err != nil {
			t.Fatalf("LoadPrototype (run %d): %v", i, err)
		}
	}
	if len(sessions.seen) != 20 {
		t.Errorf("the fixture asked for %d sessions, want 20", len(sessions.seen))
	}
}

// A fixture loaded without a session seeder still writes its cards: a test that does not care about
// sessions does not have to build one.
func TestLoadPrototypeWithoutASessionSeeder(t *testing.T) {
	e := newEnv(t)
	if err := e.load(t); err != nil {
		t.Fatalf("LoadPrototype: %v", err)
	}
	board, err := e.svc.Board(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Cards) != 11 {
		t.Errorf("api has %d cards, want 11", len(board.Cards))
	}
}
