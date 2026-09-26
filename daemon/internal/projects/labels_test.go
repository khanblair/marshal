package projects_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A label is created, listed, renamed, recolored, and deleted, and every change is published so a
// client's list is never stale.
func TestLabelsLifecycle(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	e.drainEvents()
	ctx := context.Background()

	created, err := e.svc.CreateLabel(ctx, project.ID, protocol.CreateLabelRequest{Name: "  backend  "})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if created.Name != "backend" || created.ProjectID != project.ID || created.ID == "" {
		t.Errorf("label = %+v", created)
	}
	if created.Color != protocol.LabelColorSlate {
		t.Errorf("color = %s, want the default slate", created.Color)
	}
	ev := e.nextType(t, protocol.EventTypeLabelUpdated, protocol.ProjectTopic(project.ID))
	if data, ok := ev.Data.(protocol.LabelUpdatedEventData); !ok || len(data.Labels) != 1 || !ev.Critical {
		t.Errorf("event = %+v (critical %v), want the project's one label and a critical event", ev.Data, ev.Critical)
	}

	snapshot, err := e.svc.Labels(ctx, project.ID)
	if err != nil || len(snapshot.Labels) != 1 || snapshot.ProjectID != project.ID || snapshot.ServerTime == (protocol.Timestamp{}) {
		t.Fatalf("Labels = %+v, %v", snapshot, err)
	}

	name, color := "platform", protocol.LabelColorPurple
	updated, err := e.svc.UpdateLabel(ctx, created.ID, protocol.UpdateLabelRequest{Name: &name, Color: &color})
	if err != nil {
		t.Fatalf("UpdateLabel: %v", err)
	}
	if updated.Name != "platform" || updated.Color != protocol.LabelColorPurple {
		t.Errorf("label = %+v", updated)
	}
	e.nextType(t, protocol.EventTypeLabelUpdated, protocol.ProjectTopic(project.ID))

	// An empty body answers with the label as it is, and publishes nothing.
	same, err := e.svc.UpdateLabel(ctx, created.ID, protocol.UpdateLabelRequest{})
	if err != nil || !reflect.DeepEqual(same, updated) {
		t.Errorf("UpdateLabel with nothing set = %+v, %v; want %+v", same, err, updated)
	}
	e.noEvent(t)

	if err := e.svc.DeleteLabel(ctx, created.ID); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}
	after, err := e.svc.Labels(ctx, project.ID)
	if err != nil || len(after.Labels) != 0 {
		t.Errorf("Labels after the delete = %+v, %v", after, err)
	}
	e.nextType(t, protocol.EventTypeLabelUpdated, protocol.ProjectTopic(project.ID))
	if _, err := e.svc.UpdateLabel(ctx, created.ID, protocol.UpdateLabelRequest{Name: &name}); err == nil {
		t.Error("a deleted label could still be edited")
	}
}

// A project cannot have two labels with one name, however it is spelled.
func TestLabelNamesAreUniqueInAProject(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	ctx := context.Background()
	if _, err := e.svc.CreateLabel(ctx, project.ID, protocol.CreateLabelRequest{Name: "backend"}); err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	for _, name := range []string{"backend", "Backend", "  BACKEND  "} {
		_, err := e.svc.CreateLabel(ctx, project.ID, protocol.CreateLabelRequest{Name: name})
		perr, ok := err.(*protocol.Error)
		if !ok || perr.Code != protocol.ErrorCodeConflict {
			t.Errorf("CreateLabel(%q) = %v, want a conflict", name, err)
		}
	}
	// Another project may use the same name: the list belongs to one project.
	other := e.folder(t, "monorepo")
	if _, err := e.svc.CreateLabel(ctx, other.ID, protocol.CreateLabelRequest{Name: "backend"}); err != nil {
		t.Errorf("CreateLabel in another project: %v", err)
	}
	// Renaming onto a name that is already taken is refused too.
	second, err := e.svc.CreateLabel(ctx, project.ID, protocol.CreateLabelRequest{Name: "urgent"})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	taken := "backend"
	if _, err := e.svc.UpdateLabel(ctx, second.ID, protocol.UpdateLabelRequest{Name: &taken}); err == nil {
		t.Error("a label was renamed onto a name another label has")
	}
}

// A label that is not allowed is refused with a plain sentence.
func TestLabelRefusals(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	ctx := context.Background()
	empty, long := "   ", strings.Repeat("x", 41)
	badColor := protocol.LabelColor("chartreuse")

	_, err := e.svc.CreateLabel(ctx, project.ID, protocol.CreateLabelRequest{Name: empty})
	perr, ok := err.(*protocol.Error)
	if !ok || perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Errorf("CreateLabel with no name = %v, want invalid_argument", err)
	}
	_, err = e.svc.CreateLabel(ctx, project.ID, protocol.CreateLabelRequest{Name: long})
	if perr, ok := err.(*protocol.Error); !ok || perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Errorf("CreateLabel with a long name = %v, want invalid_argument", err)
	}
	_, err = e.svc.CreateLabel(ctx, project.ID, protocol.CreateLabelRequest{Name: "ok", Color: badColor})
	if perr, ok := err.(*protocol.Error); !ok || perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Errorf("CreateLabel with a bad color = %v, want invalid_argument", err)
	}
	_, err = e.svc.CreateLabel(ctx, "no-such-project", protocol.CreateLabelRequest{Name: "ok"})
	if perr, ok := err.(*protocol.Error); !ok || perr.Code != protocol.ErrorCodeNotFound {
		t.Errorf("CreateLabel in an unknown project = %v, want not_found", err)
	}
	_, err = e.svc.Labels(ctx, "no-such-project")
	if perr, ok := err.(*protocol.Error); !ok || perr.Code != protocol.ErrorCodeNotFound {
		t.Errorf("Labels of an unknown project = %v, want not_found", err)
	}
	// A valid name and an id that names nothing is not found. A name that is not allowed is
	// refused before the store is touched, whatever the id is.
	good := "renamed"
	_, err = e.svc.UpdateLabel(ctx, "01M3C107JB041061050R3GG28A", protocol.UpdateLabelRequest{Name: &good})
	if perr, ok := err.(*protocol.Error); !ok || perr.Code != protocol.ErrorCodeNotFound {
		t.Errorf("UpdateLabel of an unknown label = %v, want not_found", err)
	}
	_, err = e.svc.UpdateLabel(ctx, "01M3C107JB041061050R3GG28A", protocol.UpdateLabelRequest{Name: &empty})
	if perr, ok := err.(*protocol.Error); !ok || perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Errorf("UpdateLabel with no name = %v, want invalid_argument", err)
	}
	if err := e.svc.DeleteLabel(ctx, "01M3C107JB041061050R3GG28A"); err == nil {
		t.Error("DeleteLabel of an unknown label succeeded")
	}
}

// A card carries its project's labels, and deleting a label takes it off the card and publishes
// the card as changed, so a screen that draws cards never shows a label that is gone.
func TestDeletingALabelTakesItOffCards(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	ctx := context.Background()
	card := e.card(t, project.ID, "Carries labels")
	label, err := e.svc.CreateLabel(ctx, project.ID, protocol.CreateLabelRequest{Name: "backend", Color: protocol.LabelColorBlue})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	ids := []string{label.ID}
	tagged, err := e.svc.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Labels: &ids})
	if err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	if len(tagged.Labels) != 1 || tagged.Labels[0].ID != label.ID {
		t.Fatalf("labels = %+v, want the one label", tagged.Labels)
	}
	e.drainEvents()

	if err := e.svc.DeleteLabel(ctx, label.ID); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}
	// The card is published as changed, with the label gone.
	var seen bool
	for _, typ := range []protocol.EventType{protocol.EventTypeLabelUpdated, protocol.EventTypeCardUpdated} {
		ev := e.nextType(t, typ, protocol.ProjectTopic(project.ID))
		if typ == protocol.EventTypeCardUpdated {
			data, ok := ev.Data.(protocol.CardEventData)
			if !ok || len(data.Card.Labels) != 0 {
				t.Errorf("card event = %+v, want the card with no labels", ev.Data)
			}
			seen = true
		}
	}
	if !seen {
		t.Error("the card that carried the label was not published as changed")
	}
	after, err := e.svc.Card(ctx, card.ID)
	if err != nil || len(after.Labels) != 0 {
		t.Errorf("the card still carries the label: %+v", after.Labels)
	}
}

// A card can only carry its own project's labels.
func TestACardCannotCarryAnotherProjectsLabel(t *testing.T) {
	e := newEnv(t)
	first := e.folder(t, "small-repo")
	second := e.folder(t, "monorepo")
	ctx := context.Background()
	card := e.card(t, first.ID, "Mine")
	other, err := e.svc.CreateLabel(ctx, second.ID, protocol.CreateLabelRequest{Name: "theirs"})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	ids := []string{other.ID}
	_, err = e.svc.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Labels: &ids})
	perr, ok := err.(*protocol.Error)
	if !ok || perr.Code != protocol.ErrorCodeInvalidArgument {
		t.Fatalf("UpdateCard with another project's label = %v, want invalid_argument", err)
	}
	// A repeated id is applied once, and the card keeps the labels it had when the list is
	// replaced with the same one.
	mine, err := e.svc.CreateLabel(ctx, first.ID, protocol.CreateLabelRequest{Name: "mine"})
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	same := []string{mine.ID, mine.ID}
	tagged, err := e.svc.UpdateCard(ctx, card.ID, protocol.UpdateCardRequest{Labels: &same})
	if err != nil || len(tagged.Labels) != 1 {
		t.Errorf("UpdateCard with a repeated id = %+v, %v; want one label", tagged.Labels, err)
	}
}
