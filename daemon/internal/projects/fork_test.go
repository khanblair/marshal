package projects_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A fork is a new card in the backlog with a copy of the card it came from, the labels included,
// and it starts from that card's latest commit.
func TestForkCardCopiesTheCard(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	source := e.card(t, project.ID, "Fix token refresh")
	label, err := e.svc.CreateLabel(context.Background(), project.ID,
		protocol.CreateLabelRequest{Name: "backend", Color: protocol.LabelColorBlue})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	agent, mode, thinking, role, pkg := protocol.AgentKindGemini, protocol.PermissionModePlan,
		protocol.ThinkingModeHigh, "Implementer", "packages/api"
	ids := []string{label.ID}
	source, err = e.svc.UpdateCard(context.Background(), source.ID, protocol.UpdateCardRequest{
		Body:           ptrOf("Serve GET /health."),
		Agent:          &agent,
		Thinking:       &thinking,
		Role:           &role,
		Package:        &pkg,
		PermissionMode: &mode,
		Labels:         &ids,
	})
	if err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	// A card that never started has nothing to fork from.
	if _, err := e.svc.ForkCard(context.Background(), source.ID); err == nil {
		t.Fatal("a card that never started was forked")
	}
	e.startCard(t, project, source)
	e.drainEvents()

	fork, err := e.svc.ForkCard(context.Background(), source.ID)
	if err != nil {
		t.Fatalf("ForkCard: %v", err)
	}
	if fork.ID == source.ID || fork.Number == source.Number || fork.Key == source.Key {
		t.Errorf("the fork is the same card: %+v", fork)
	}
	if fork.Title != "Fix token refresh (fork)" {
		t.Errorf("title = %q", fork.Title)
	}
	if fork.State != protocol.CardStateBacklog {
		t.Errorf("state = %s, want backlog", fork.State)
	}
	if fork.Body != source.Body || fork.Agent != source.Agent || fork.Model != source.Model ||
		fork.PermissionMode != source.PermissionMode || fork.Role != source.Role || fork.Package != source.Package {
		t.Errorf("the fork lost a setting: %+v", fork)
	}
	if fork.Thinking == nil || source.Thinking == nil || *fork.Thinking != *source.Thinking {
		t.Errorf("thinking = %v, want %v", fork.Thinking, source.Thinking)
	}
	if len(fork.Labels) != 1 || fork.Labels[0].ID != label.ID {
		t.Errorf("labels = %+v, want the source card's label", fork.Labels)
	}
	if fork.DoingNow != "Starting from the latest checkpoint" {
		t.Errorf("doing now = %q", fork.DoingNow)
	}
	if fork.Branch != "" {
		t.Errorf("branch = %q: the branch is made when the fork starts", fork.Branch)
	}
	// A fork never carries the source's pull request, CI state, or dates.
	if fork.PullRequest != nil || fork.CI != nil || fork.ActualStart != nil || fork.ActualEnd != nil {
		t.Errorf("the fork copied state it should not: %+v", fork)
	}
	// Both cards are published: the new one, and the source is not touched.
	ev := e.nextType(t, protocol.EventTypeCardCreated, protocol.ProjectTopic(project.ID))
	if data, ok := ev.Data.(protocol.CardEventData); !ok || data.Card.ID != fork.ID || !ev.Critical {
		t.Errorf("event = %+v (critical %v), want the fork", ev.Data, ev.Critical)
	}
}

// The card a fork's branch starts from is the card it came from, and a source that is gone falls
// back to the default branch rather than failing.
func TestForkBaseIsTheSourceBranch(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	ctx := context.Background()
	source := e.card(t, project.ID, "Source")
	_, branch := e.startCard(t, project, source)

	fork, err := e.svc.ForkCard(ctx, source.ID)
	if err != nil {
		t.Fatalf("ForkCard: %v", err)
	}
	if got := e.svc.ForkBase(ctx, project, fork.ID); got != branch {
		t.Errorf("ForkBase = %q, want %q", got, branch)
	}
	// A card that is not a fork starts from the project's default branch.
	plain := e.card(t, project.ID, "Plain")
	if got := e.svc.ForkBase(ctx, project, plain.ID); got != "" {
		t.Errorf("ForkBase of a card that is not a fork = %q, want empty", got)
	}
	// A fork without its source card in the store, and one whose branch was deleted, both fall
	// back to the default branch instead of failing the start.
	if err := e.svc.DeleteCard(ctx, source.ID); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if got := e.svc.ForkBase(ctx, project, fork.ID); got != "" {
		t.Errorf("ForkBase after the source was deleted = %q, want empty", got)
	}
}

// The source card is not changed by a fork.
func TestForkCardLeavesTheSourceAlone(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	ctx := context.Background()
	source := e.card(t, project.ID, "Source")
	e.startCard(t, project, source)
	before, err := e.svc.Card(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ForkCard(ctx, source.ID); err != nil {
		t.Fatalf("ForkCard: %v", err)
	}
	after, err := e.svc.Card(ctx, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("the source changed: %+v, want %+v", after, before)
	}
}

// ptrOf is for the request fields that are pointers.
func ptrOf[T any](v T) *T { return &v }
