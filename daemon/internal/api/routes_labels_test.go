package api_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A project's labels through HTTP: create, list, rename, recolor, refuse a duplicate, delete, and
// know that a label that is gone is not found.
func TestLabelsThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")

	created := decode[protocol.Label](t, st.do(http.MethodPost, "/v1/projects/"+project.ID+"/labels",
		protocol.CreateLabelRequest{Name: "backend", Color: protocol.LabelColorBlue}).want(t, http.StatusCreated))
	if created.Name != "backend" || created.Color != protocol.LabelColorBlue || created.ProjectID != project.ID {
		t.Errorf("label = %+v", created)
	}
	listed := decode[protocol.LabelSnapshot](t, st.do(http.MethodGet, "/v1/projects/"+project.ID+"/labels", nil).want(t, http.StatusOK))
	if len(listed.Labels) != 1 || listed.Labels[0].ID != created.ID || listed.ProjectID != project.ID {
		t.Errorf("labels = %+v", listed)
	}

	name, color := "platform", protocol.LabelColorPurple
	updated := decode[protocol.Label](t, st.do(http.MethodPatch, "/v1/labels/"+created.ID,
		protocol.UpdateLabelRequest{Name: &name, Color: &color}).want(t, http.StatusOK))
	if updated.Name != "platform" || updated.Color != protocol.LabelColorPurple {
		t.Errorf("label = %+v", updated)
	}

	// The same name twice is a conflict, not a second label.
	st.do(http.MethodPost, "/v1/projects/"+project.ID+"/labels", protocol.CreateLabelRequest{Name: "platform"}).
		apiError(t, http.StatusConflict, protocol.ErrorCodeConflict)
	// A project that is not there is not found.
	st.do(http.MethodGet, "/v1/projects/no-such-project/labels", nil).
		apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	// A body that is not a label is refused.
	st.do(http.MethodPost, "/v1/projects/"+project.ID+"/labels", map[string]string{"colour": "blue"}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)

	st.do(http.MethodDelete, "/v1/labels/"+created.ID, nil).want(t, http.StatusNoContent)
	st.do(http.MethodDelete, "/v1/labels/"+created.ID, nil).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	empty := decode[protocol.LabelSnapshot](t, st.do(http.MethodGet, "/v1/projects/"+project.ID+"/labels", nil).want(t, http.StatusOK))
	if len(empty.Labels) != 0 {
		t.Errorf("labels after the delete = %+v", empty.Labels)
	}
}

// A card carries its project's labels through HTTP, and the answer names them.
func TestCardLabelsThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Carries a label")
	label := decode[protocol.Label](t, st.do(http.MethodPost, "/v1/projects/"+project.ID+"/labels",
		protocol.CreateLabelRequest{Name: "urgent", Color: protocol.LabelColorRed}).want(t, http.StatusCreated))

	ids := []string{label.ID}
	tagged := decode[protocol.Card](t, st.do(http.MethodPatch, "/v1/cards/"+card.ID,
		protocol.UpdateCardRequest{Labels: &ids}).want(t, http.StatusOK))
	if len(tagged.Labels) != 1 || !reflect.DeepEqual(tagged.Labels[0], label) {
		t.Errorf("labels = %+v, want %+v", tagged.Labels, label)
	}
	// A label that is not this project's is refused.
	other, _ := st.addProject("monorepo")
	theirs := decode[protocol.Label](t, st.do(http.MethodPost, "/v1/projects/"+other.ID+"/labels",
		protocol.CreateLabelRequest{Name: "theirs"}).want(t, http.StatusCreated))
	bad := []string{theirs.ID}
	st.do(http.MethodPatch, "/v1/cards/"+card.ID, protocol.UpdateCardRequest{Labels: &bad}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
}
