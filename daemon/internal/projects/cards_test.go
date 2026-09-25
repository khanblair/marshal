package projects_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestCreateCardFillsInTheDefaults(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	e.drainEvents()
	card, err := e.svc.CreateCard(context.Background(), project.ID, protocol.CreateCardRequest{Title: "  Add a health check  "})
	if err != nil {
		t.Fatal(err)
	}
	if card.Number != 1 || card.Key != "small-repo#1" || card.Title != "Add a health check" || card.ProjectID != project.ID {
		t.Errorf("card = %+v", card)
	}
	if card.State != protocol.CardStateBacklog || card.Agent != protocol.AgentKindClaude ||
		card.PermissionMode != protocol.PermissionModeAutoEdits || card.Thinking != nil || card.Model != "" || card.Branch != "" {
		t.Errorf("card defaults = %+v", card)
	}
	if !protocol.ValidID(card.ID) || card.CreatedAt != card.UpdatedAt {
		t.Errorf("id %q, created %v, updated %v", card.ID, card.CreatedAt, card.UpdatedAt)
	}
	ev := e.nextType(t, protocol.EventTypeCardCreated, protocol.ProjectTopic(project.ID))
	if data, ok := ev.Data.(protocol.CardEventData); !ok || data.Card != card || !ev.Critical {
		t.Errorf("event = %+v (critical %v), want the card and a critical event", ev.Data, ev.Critical)
	}
	again, err := e.svc.Card(context.Background(), card.ID)
	if err != nil || again != card {
		t.Errorf("Card = %+v, %v; want %+v", again, err, card)
	}
}

func TestCreateCardWithEveryField(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card, err := e.svc.CreateCard(context.Background(), project.ID, protocol.CreateCardRequest{
		Title: "Plan it", Body: "Long text", Agent: protocol.AgentKindGemini, Model: "gemini-2.5-pro",
		Thinking: protocol.ThinkingModeHigh, PermissionMode: protocol.PermissionModePlan,
	}, projects.WithCreatedBy("user-1"))
	if err != nil {
		t.Fatal(err)
	}
	if card.Body != "Long text" || card.Agent != protocol.AgentKindGemini || card.Model != "gemini-2.5-pro" ||
		card.Thinking == nil || *card.Thinking != protocol.ThinkingModeHigh || card.PermissionMode != protocol.PermissionModePlan {
		t.Errorf("card = %+v", card)
	}
	row, err := e.store.Queries().GetCard(context.Background(), card.ID)
	if err != nil || row.CreatedBy != "user-1" {
		t.Errorf("created_by = %q, %v; want user-1", row.CreatedBy, err)
	}
}

func TestCreateCardRefusesWhatIsNotAllowed(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	e.drainEvents()
	tests := []struct {
		name string
		in   protocol.CreateCardRequest
	}{
		{"no title", protocol.CreateCardRequest{Title: "   "}},
		{"a title that is too long", protocol.CreateCardRequest{Title: strings.Repeat("t", 201)}},
		{"a description that is too long", protocol.CreateCardRequest{Title: "x", Body: strings.Repeat("b", 100*1024+1)}},
		{"a model name that is too long", protocol.CreateCardRequest{Title: "x", Model: strings.Repeat("m", 101)}},
		{"an unknown agent", protocol.CreateCardRequest{Title: "x", Agent: "gpt"}},
		{"an unknown thinking setting", protocol.CreateCardRequest{Title: "x", Thinking: "Extra high"}},
		{"an unknown permission mode", protocol.CreateCardRequest{Title: "x", PermissionMode: "Bypass permissions"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.CreateCard(context.Background(), project.ID, tc.in)
			_ = wantCode(t, err, protocol.ErrorCodeInvalidArgument)
		})
	}
	e.noEvent(t)
	first := e.card(t, project.ID, "after the refusals")
	if first.Number != 1 {
		t.Errorf("number = %d; refused cards must not use numbers", first.Number)
	}
}

func TestCreateCardInAnUnknownProject(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CreateCard(context.Background(), "nope", protocol.CreateCardRequest{Title: "x"})
	perr := wantCode(t, err, protocol.ErrorCodeNotFound)
	if perr.Details["id"] != "nope" {
		t.Errorf("details = %v", perr.Details)
	}
	e.noEvent(t)
}

func TestCardNumbersAreNeverRepeatedOrSkippedUnderConcurrency(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	const creators = 20
	cards := make([]protocol.Card, creators)
	errs := make([]error, creators)
	var wg sync.WaitGroup
	for i := range creators {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cards[i], errs[i] = e.svc.CreateCard(context.Background(), project.ID, protocol.CreateCardRequest{Title: "card"})
		}()
	}
	wg.Wait()
	numbers := make([]int, 0, creators)
	ids := map[string]bool{}
	for i, card := range cards {
		if errs[i] != nil {
			t.Fatalf("creator %d: %v", i, errs[i])
		}
		numbers = append(numbers, card.Number)
		ids[card.ID] = true
	}
	slices.Sort(numbers)
	for i, n := range numbers {
		if n != i+1 {
			t.Fatalf("numbers = %v, want 1 to %d with no repeat and no gap", numbers, creators)
		}
	}
	if len(ids) != creators {
		t.Errorf("%d different ids for %d cards", len(ids), creators)
	}
	stored, err := e.svc.Cards(context.Background(), project.ID)
	if err != nil || len(stored) != creators || stored[0].Number != 1 || stored[creators-1].Number != creators {
		t.Errorf("Cards = %d cards, %v; want %d in number order", len(stored), err, creators)
	}
}

func TestEachProjectCountsItsOwnCards(t *testing.T) {
	e := newEnv(t)
	api := e.folder(t, "small-repo", projects.WithID("api"))
	web := e.folder(t, "small-repo", projects.WithID("web"))
	const number = 12
	byProject := map[string]map[int]protocol.Card{"api": {}, "web": {}}
	for _, project := range []protocol.Project{api, web} {
		for i := 1; i <= number; i++ {
			card := e.card(t, project.ID, "card")
			byProject[project.ID][card.Number] = card
		}
	}
	apiCard, err := e.svc.CardByKey(context.Background(), protocol.CardKey{ProjectID: "api", Number: number})
	if err != nil {
		t.Fatal(err)
	}
	webCard, err := e.svc.CardByKey(context.Background(), protocol.CardKey{ProjectID: "web", Number: number})
	if err != nil {
		t.Fatal(err)
	}
	if apiCard.Number != number || webCard.Number != number || apiCard.ID == webCard.ID || apiCard.Key == webCard.Key {
		t.Errorf("api %+v and web %+v: both are card 12 with different ids and keys", apiCard, webCard)
	}
	if apiCard.Key != "api#12" || webCard.Key != "web#12" || apiCard.ProjectID != "api" || webCard.ProjectID != "web" {
		t.Errorf("keys %q and %q, projects %q and %q", apiCard.Key, webCard.Key, apiCard.ProjectID, webCard.ProjectID)
	}
	if apiCard != byProject["api"][number] || webCard != byProject["web"][number] {
		t.Error("CardByKey did not return the card that was made with that number")
	}
	key, err := protocol.ParseCardKey(apiCard.Key)
	if err != nil || key != (protocol.CardKey{ProjectID: "api", Number: number}) {
		t.Errorf("ParseCardKey(%q) = %+v, %v", apiCard.Key, key, err)
	}
	_, err = e.svc.CardByKey(context.Background(), protocol.CardKey{ProjectID: "api", Number: number + 1})
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
	_, err = e.svc.Card(context.Background(), "01M3C107JB041061050R3GG28A")
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
}

func TestBoardListsTheColumnsAndTheCardsInOrder(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	first := e.card(t, project.ID, "first")
	second := e.card(t, project.ID, "second")
	if _, err := e.svc.SetState(context.Background(), second.ID, protocol.CardStateMerging); err != nil {
		t.Fatal(err)
	}
	board, err := e.svc.Board(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []protocol.CardState{"backlog", "planning", "working", "needs", "review", "ready", "done"}
	if !slices.Equal(board.Columns, want) || board.ProjectID != project.ID {
		t.Errorf("columns = %v, want %v", board.Columns, want)
	}
	if len(board.Cards) != 2 || board.Cards[0].ID != first.ID || board.Cards[1].State != protocol.CardStateMerging {
		t.Errorf("cards = %+v; want both in number order, the merging one included", board.Cards)
	}
	if board.ServerTime.Time().IsZero() {
		t.Error("the board has no server time")
	}
	_, err = e.svc.Board(context.Background(), "nope")
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
	_, err = e.svc.Cards(context.Background(), "nope")
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
}

func TestSetStatePublishesTheMove(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "move me")
	e.drainEvents()
	moved, err := e.svc.SetState(context.Background(), card.ID, protocol.CardStateWorking)
	if err != nil {
		t.Fatal(err)
	}
	if moved.State != protocol.CardStateWorking || !moved.UpdatedAt.Time().After(card.UpdatedAt.Time()) {
		t.Errorf("moved = %+v; want the new state and a later update time", moved)
	}
	ev := e.nextType(t, protocol.EventTypeCardMoved, protocol.ProjectTopic(project.ID))
	data, ok := ev.Data.(protocol.CardMovedEventData)
	if !ok || data.Card != moved || data.From != protocol.CardStateBacklog || !ev.Critical {
		t.Errorf("event = %+v (critical %v)", ev.Data, ev.Critical)
	}
	e.noEvent(t) // working is not "needs", so the project's badge did not change
	got, err := e.svc.Card(context.Background(), card.ID)
	if err != nil || got != moved {
		t.Errorf("Card = %+v, %v", got, err)
	}
}

func TestSetStateRefusesWhatIsNotAState(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "x")
	e.drainEvents()
	for _, state := range []protocol.CardState{"", "Needs you", "needs-you", "archived"} {
		_, err := e.svc.SetState(context.Background(), card.ID, state)
		_ = wantCode(t, err, protocol.ErrorCodeInvalidArgument)
	}
	_, err := e.svc.SetState(context.Background(), "01M3C107JB041061050R3GG28A", protocol.CardStateDone)
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
	e.noEvent(t)
}

func TestSetStateToTheSameStateDoesNothing(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "x")
	e.drainEvents()
	got, err := e.svc.SetState(context.Background(), card.ID, protocol.CardStateBacklog)
	if err != nil || got != card {
		t.Errorf("SetState = %+v, %v; want the card unchanged", got, err)
	}
	e.noEvent(t)
}

type awakeCounts map[string]int

func (a awakeCounts) AwakeCards(_ context.Context, projectID string) (int, error) {
	return a[projectID], nil
}

func TestBadgesComeFromTheCards(t *testing.T) {
	e := newEnv(t, projects.WithAwakeCounter(awakeCounts{"small-repo": 3}))
	project := e.folder(t, "small-repo")
	other := e.folder(t, "small-repo", projects.WithID("other"))
	cards := []protocol.Card{e.card(t, project.ID, "a"), e.card(t, project.ID, "b"), e.card(t, project.ID, "c")}
	e.drainEvents()
	for _, card := range cards[:2] {
		if _, err := e.svc.SetState(context.Background(), card.ID, protocol.CardStateNeeds); err != nil {
			t.Fatal(err)
		}
		e.nextType(t, protocol.EventTypeCardMoved, protocol.ProjectTopic(project.ID))
		e.nextType(t, protocol.EventTypeProjectUpdated, protocol.HomeTopic)
	}
	check := func(want int) {
		t.Helper()
		got, err := e.svc.Get(context.Background(), project.ID)
		if err != nil || got.Badges.Needs != want || got.Badges.Awake != 3 {
			t.Errorf("Get badges = %+v, %v; want needs %d and awake 3", got.Badges, err, want)
		}
		list, err := e.svc.List(context.Background())
		if err != nil || len(list.Projects) != 2 {
			t.Fatalf("List = %+v, %v", list, err)
		}
		if list.Projects[0].Badges != got.Badges || list.Projects[1].ID != other.ID || list.Projects[1].Badges != (protocol.ProjectBadges{}) {
			t.Errorf("List badges = %+v and %+v; the second project has no cards and no awake agents", list.Projects[0].Badges, list.Projects[1].Badges)
		}
	}
	check(2)
	// Moving out of "needs" changes the badge again, and the announcement carries the new count.
	if _, err := e.svc.SetState(context.Background(), cards[0].ID, protocol.CardStateReview); err != nil {
		t.Fatal(err)
	}
	e.nextType(t, protocol.EventTypeCardMoved, protocol.ProjectTopic(project.ID))
	ev := e.nextType(t, protocol.EventTypeProjectUpdated, protocol.HomeTopic)
	if data, ok := ev.Data.(protocol.ProjectEventData); !ok || data.Project.Badges.Needs != 1 {
		t.Errorf("event = %+v, want a badge of 1", ev.Data)
	}
	check(1)
	// A move that never touches "needs" does not change the badge, so only the move is published.
	if _, err := e.svc.SetState(context.Background(), cards[2].ID, protocol.CardStateWorking); err != nil {
		t.Fatal(err)
	}
	e.nextType(t, protocol.EventTypeCardMoved, protocol.ProjectTopic(project.ID))
	e.noEvent(t)
}

type failingAwake struct{}

func (failingAwake) AwakeCards(context.Context, string) (int, error) {
	return 0, context.DeadlineExceeded
}

func TestAFailingAwakeCounterFailsTheList(t *testing.T) {
	e := newEnv(t, projects.WithAwakeCounter(failingAwake{}))
	project := e.folder(t, "small-repo")
	if _, err := e.svc.List(context.Background()); err == nil {
		t.Error("List hid a failing awake counter")
	}
	if _, err := e.svc.Get(context.Background(), project.ID); err == nil {
		t.Error("Get hid a failing awake counter")
	}
}
