package projects_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func errorsAs(err error, target **protocol.Error) bool { return errors.As(err, target) }

func sqlNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// The events of one life of a project, in the order they happened, with the topic and the payload
// each one carries. A client that follows the home topic and the project topic sees exactly this.
func TestEventsAreInOrderWithTheirPayloads(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	project := e.folder(t, "small-repo")
	created := e.nextType(t, protocol.EventTypeProjectCreated, protocol.HomeTopic)
	if data, ok := created.Data.(protocol.ProjectEventData); !ok || data.Project.ID != project.ID || data.Project.Name != project.Name {
		t.Errorf("project.created data = %+v", created.Data)
	}
	// The event is published after the commit, so a reader that hears it can already read the row.
	if _, err := e.svc.Get(ctx, project.ID); err != nil {
		t.Errorf("Get after project.created: %v", err)
	}

	renamed, err := e.svc.Update(ctx, project.ID, protocol.UpdateProjectRequest{Name: str("Renamed")})
	if err != nil {
		t.Fatal(err)
	}
	updated := e.nextType(t, protocol.EventTypeProjectUpdated, protocol.HomeTopic)
	if data, ok := updated.Data.(protocol.ProjectEventData); !ok || !equalProjects(data.Project, renamed) {
		t.Errorf("project.updated data = %+v, want %+v", updated.Data, renamed)
	}

	card := e.card(t, project.ID, "A card")
	cardCreated := e.nextType(t, protocol.EventTypeCardCreated, protocol.ProjectTopic(project.ID))
	if data, ok := cardCreated.Data.(protocol.CardEventData); !ok || data.Card != card {
		t.Errorf("card.created data = %+v", cardCreated.Data)
	}

	moved, err := e.svc.SetState(ctx, card.ID, protocol.CardStateNeeds)
	if err != nil {
		t.Fatal(err)
	}
	cardMoved := e.nextType(t, protocol.EventTypeCardMoved, protocol.ProjectTopic(project.ID))
	if data, ok := cardMoved.Data.(protocol.CardMovedEventData); !ok || data.Card != moved || data.From != protocol.CardStateBacklog {
		t.Errorf("card.moved data = %+v", cardMoved.Data)
	}
	badge := e.nextType(t, protocol.EventTypeProjectUpdated, protocol.HomeTopic)
	if data, ok := badge.Data.(protocol.ProjectEventData); !ok || data.Project.Badges.Needs != 1 {
		t.Errorf("the badge event = %+v, want needs 1", badge.Data)
	}

	path := t.TempDir() + "/worktrees"
	_ = path
	worktree, branch := e.startCard(t, project, card)
	_ = worktree
	cardUpdated := e.nextType(t, protocol.EventTypeCardUpdated, protocol.ProjectTopic(project.ID))
	if data, ok := cardUpdated.Data.(protocol.CardEventData); !ok || data.Card.Branch != branch || data.Card.State != protocol.CardStateNeeds {
		t.Errorf("card.updated data = %+v, want the branch %s", cardUpdated.Data, branch)
	}

	if err := e.svc.Remove(ctx, project.ID, protocol.RemoveProjectRequest{}); err != nil {
		t.Fatal(err)
	}
	removed := e.nextType(t, protocol.EventTypeProjectRemoved, protocol.HomeTopic)
	if data, ok := removed.Data.(protocol.ProjectRemovedEventData); !ok || data.ProjectID != project.ID {
		t.Errorf("project.removed data = %+v", removed.Data)
	}
	e.noEvent(t)

	if created.Seq >= updated.Seq || updated.Seq >= cardCreated.Seq || cardCreated.Seq >= cardMoved.Seq ||
		cardMoved.Seq >= badge.Seq || badge.Seq >= cardUpdated.Seq || cardUpdated.Seq >= removed.Seq {
		t.Error("the sequence numbers do not follow the order of the changes")
	}
}

// The payloads are what the wire types say, so the app can read them without a second request.
func TestEventPayloadsEncodeAsTheWireTypes(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "monorepo")
	ev := e.nextType(t, protocol.EventTypeProjectCreated, protocol.HomeTopic)
	wire := protocol.Event{Seq: ev.Seq, Topic: protocol.Topic(ev.Topic), Type: protocol.EventType(ev.Type), At: protocol.NewTimestamp(ev.At)}
	data, err := json.Marshal(ev.Data)
	if err != nil {
		t.Fatal(err)
	}
	wire.Data = data
	batch, err := json.Marshal(protocol.EventBatch{Epoch: e.bus.Epoch(), Events: []protocol.Event{wire}})
	if err != nil {
		t.Fatalf("the event does not encode: %v", err)
	}
	if len(batch) == 0 || project.ID == "" {
		t.Fatal("empty batch")
	}
}
