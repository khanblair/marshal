package projects_test

import (
	"context"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Card dependencies: the edges of a plan (docs/architecture.md section 10's card_links row,
// migration 0020; build-plan task 7.4). An edge is made in one way only - a card is created naming
// the cards it waits for - so these tests are mostly about what that create refuses, and about the
// two readings of the edges: one card's, and one project's.

// keyOf is a card's key as a caller would write it, from a project and a number.
func keyOf(projectID string, number int) protocol.CardKey {
	return protocol.CardKey{ProjectID: projectID, Number: number}
}

func keysOf(t *testing.T, projectID string, cards ...protocol.Card) []protocol.CardKey {
	t.Helper()
	out := make([]protocol.CardKey, 0, len(cards))
	for _, card := range cards {
		out = append(out, keyOf(projectID, card.Number))
	}
	return out
}

// sameKeys fails unless two lists of keys hold the same keys in the same order.
func sameKeys(t *testing.T, what string, got, want []protocol.CardKey) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

func TestCreateCardRecordsWhatItWaitsFor(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	first := e.card(t, project.ID, "Set up the pipeline")
	second, err := e.svc.CreateCard(context.Background(), project.ID,
		protocol.CreateCardRequest{Title: "Add a health check"},
		projects.WithDependsOn(keysOf(t, project.ID, first)))
	if err != nil {
		t.Fatal(err)
	}
	deps, err := e.svc.CardDependencies(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	sameKeys(t, "what the card waits for", deps, keysOf(t, project.ID, first))
}

func TestCardDependenciesAreInNumberOrder(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	first := e.card(t, project.ID, "One")
	second := e.card(t, project.ID, "Two")
	third := e.card(t, project.ID, "Three")
	// Named out of order on purpose: the answer is in number order whatever order the keys came in,
	// so a plan reads back the way it was numbered.
	fourth, err := e.svc.CreateCard(context.Background(), project.ID,
		protocol.CreateCardRequest{Title: "Fourth"},
		projects.WithDependsOn(keysOf(t, project.ID, third, first, second)))
	if err != nil {
		t.Fatal(err)
	}
	deps, err := e.svc.CardDependencies(context.Background(), fourth.ID)
	if err != nil {
		t.Fatal(err)
	}
	sameKeys(t, "what the card waits for", deps, keysOf(t, project.ID, first, second, third))
}

func TestACardWithNoDependenciesAnswersAnEmptyList(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	card := e.card(t, project.ID, "On its own")
	deps, err := e.svc.CardDependencies(context.Background(), card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deps == nil || len(deps) != 0 {
		t.Errorf("a card that waits for nothing answers %v (nil: %v), want an empty list", deps, deps == nil)
	}
}

func TestRepeatingADependencyIsTheSameEdge(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	first := e.card(t, project.ID, "One")
	key := keyOf(project.ID, first.Number)
	second, err := e.svc.CreateCard(context.Background(), project.ID,
		protocol.CreateCardRequest{Title: "Two"}, projects.WithDependsOn([]protocol.CardKey{key, key}))
	if err != nil {
		t.Fatalf("naming the same card twice should be the same edge, got %v", err)
	}
	deps, err := e.svc.CardDependencies(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	sameKeys(t, "what the card waits for", deps, []protocol.CardKey{key})
}

// A dependency that cannot be honoured refuses the whole create: no card, no edges, and the
// project's number counter is not used up. That is what makes a plan built one card at a time safe
// to retry - a refused card leaves nothing behind.
func TestADependencyThatCannotBeHonouredRefusesTheCreate(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	other := e.folder(t, "small-repo", projects.WithID("other"))
	there := e.card(t, other.ID, "In the other project")
	first := e.card(t, project.ID, "One")

	tests := []struct {
		name    string
		key     protocol.CardKey
		code    protocol.ErrorCode
		phrase  string
		details map[string]string
	}{
		{
			name: "a card in another project", key: keyOf(other.ID, there.Number),
			code: protocol.ErrorCodeInvalidArgument, phrase: "not in this project",
			details: map[string]string{"dependsOn": keyOf(other.ID, there.Number).String()},
		},
		{
			// A number no card will ever have, so the shared fixture cannot grow a card into it.
			name: "a number no card has", key: keyOf(project.ID, 999),
			code: protocol.ErrorCodeNotFound, phrase: "cannot find that card",
			details: map[string]string{"id": keyOf(project.ID, 999).String()},
		},
		{
			name: "a number below one", key: keyOf(project.ID, 0),
			code: protocol.ErrorCodeInvalidArgument, phrase: "1 or more",
			details: map[string]string{"dependsOn": keyOf(project.ID, 0).String()},
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			card, err := e.svc.CreateCard(context.Background(), project.ID,
				protocol.CreateCardRequest{Title: "A plan"}, projects.WithDependsOn([]protocol.CardKey{test.key}))
			if err == nil {
				t.Fatalf("creating a card that waits for %s worked and made %s", test.key, card.Key)
			}
			perr := wantCode(t, err, test.code)
			if !strings.Contains(perr.Message, test.phrase) {
				t.Errorf("the refusal says %q, which does not say %q", perr.Message, test.phrase)
			}
			for name, want := range test.details {
				if perr.Details[name] != want {
					t.Errorf("the refusal names %s = %q, want %q", name, perr.Details[name], want)
				}
			}
			// Nothing of the refused card is left: the board does not hold it, and the number it
			// would have taken is still free, so the cards made after these attempts are numbered
			// as if the attempts had never happened.
			next := e.card(t, project.ID, "Afterwards")
			if want := first.Number + 1 + i; next.Number != want {
				t.Errorf("the card after %d refused creates is number %d, want %d", i+1, next.Number, want)
			}
			cards, err := e.svc.Cards(context.Background(), project.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(cards) != i+2 {
				t.Errorf("the project holds %d cards (%v), want %d", len(cards), titlesOf(cards), i+2)
			}
			for _, card := range cards {
				if card.Title == "A plan" {
					t.Errorf("the refused card %s was made anyway", card.Key)
				}
			}
		})
	}
}

// titlesOf names cards for a failure message.
func titlesOf(cards []protocol.Card) []string {
	out := make([]string, 0, len(cards))
	for _, card := range cards {
		out = append(out, card.Key+" "+card.Title)
	}
	return out
}

func TestProjectDependenciesGroupsEveryEdge(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	other := e.folder(t, "small-repo", projects.WithID("other"))
	one := e.card(t, project.ID, "One")
	two := e.card(t, project.ID, "Two")
	three, err := e.svc.CreateCard(context.Background(), project.ID,
		protocol.CreateCardRequest{Title: "Three"},
		projects.WithDependsOn(keysOf(t, project.ID, two, one)))
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := e.card(t, other.ID, "Elsewhere")

	deps, err := e.svc.ProjectDependencies(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 {
		t.Fatalf("the project's edges are %v, want only the card that waits", deps)
	}
	sameKeys(t, "the project's edges for the waiting card", deps[three.ID], keysOf(t, project.ID, one, two))
	if _, ok := deps[elsewhere.ID]; ok {
		t.Errorf("another project's card %s is in this project's edges", elsewhere.Key)
	}
	if _, ok := deps[one.ID]; ok {
		t.Errorf("a card that waits for nothing is in the edges of %s", one.Key)
	}
}

func TestCardDependenciesOfACardThatIsNotThere(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.CardDependencies(context.Background(), "card_does_not_exist")
	wantCode(t, err, protocol.ErrorCodeNotFound)
}

// Deleting a card takes its edges with it, on both sides: what it waited for no longer counts it as
// a waiter, and it no longer appears as waiting for anything.
func TestDeletingACardRemovesItsEdges(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	first := e.card(t, project.ID, "One")
	second, err := e.svc.CreateCard(context.Background(), project.ID,
		protocol.CreateCardRequest{Title: "Two"}, projects.WithDependsOn(keysOf(t, project.ID, first)))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteCard(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	deps, err := e.svc.CardDependencies(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 0 {
		t.Errorf("after deleting what it waited for, card %s still waits for %v", second.Key, deps)
	}
	if all := projectEdges(t, e, project.ID); len(all) != 0 {
		t.Errorf("the project still has the edges %v", all)
	}
	if err := e.svc.DeleteCard(context.Background(), second.ID); err != nil {
		t.Fatal(err)
	}
	if all := projectEdges(t, e, project.ID); len(all) != 0 {
		t.Errorf("deleting the waiting card left the edges %v", all)
	}
}

// projectEdges is every edge in a project, for a test that only cares whether any are left.
func projectEdges(t *testing.T, e *env, projectID string) map[string][]protocol.CardKey {
	t.Helper()
	all, err := e.svc.ProjectDependencies(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	return all
}
