package projects

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A project's saved views (docs/backend-checklist.md B2.5, inventory N20): a name, the filter chips,
// and a swimlane, which the board's view chips restore in one click. They belong to the project,
// not to a person, like its labels. The view in use, and the filters as they were left, are a
// person's own and live in internal/accounts. Every change publishes saved_view.updated with the
// project's views as they now are.

const (
	// maxSavedViewChars is the longest saved view name. The name is the text of a chip.
	maxSavedViewChars = 60
	// maxSavedViews is the most saved views one project can hold, so the menu that lists them
	// stays a menu.
	maxSavedViews = 50
)

// SavedViews is what the API layer needs to manage a project's saved views.
type SavedViews interface {
	// SavedViews returns a project's saved views, the one saved longest ago first.
	SavedViews(ctx context.Context, projectID string) (protocol.SavedViewListSnapshot, error)
	// SavedView returns one saved view by its id.
	SavedView(ctx context.Context, id string) (protocol.SavedView, error)
	// CreateSavedView saves the filters and swimlane under a name, and says whether that made a
	// new view. A name the project already uses replaces that view and keeps its id.
	CreateSavedView(ctx context.Context, projectID string, in protocol.CreateSavedViewRequest) (protocol.SavedView, bool, error)
	// UpdateSavedView renames a view, changes its filters, or changes its swimlane.
	UpdateSavedView(ctx context.Context, id string, in protocol.UpdateSavedViewRequest) (protocol.SavedView, error)
	// DeleteSavedView removes a view.
	DeleteSavedView(ctx context.Context, id string) error
}

var _ SavedViews = (*Service)(nil)

// SavedViews returns a project's saved views in the order they were last saved.
func (s *Service) SavedViews(ctx context.Context, projectID string) (protocol.SavedViewListSnapshot, error) {
	var views []protocol.SavedView
	err := s.store.Read(ctx, func(q *db.Queries) error {
		if _, err := q.GetProject(ctx, projectID); err != nil {
			return notFound(fmt.Errorf("read project %s: %w", projectID, err), notFoundProject(projectID))
		}
		rows, err := q.ListSavedViewsByProject(ctx, projectID)
		if err != nil {
			return fmt.Errorf("list the saved views of project %s: %w", projectID, err)
		}
		if views, err = toSavedViews(rows); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return protocol.SavedViewListSnapshot{}, err
	}
	return protocol.SavedViewListSnapshot{
		ProjectID: projectID, Views: views, ServerTime: protocol.NewTimestamp(s.now()),
	}, nil
}

// SavedView returns one saved view by its id.
func (s *Service) SavedView(ctx context.Context, id string) (protocol.SavedView, error) {
	row, err := s.store.Queries().GetSavedView(ctx, id)
	if err != nil {
		return protocol.SavedView{}, notFound(fmt.Errorf("read saved view %s: %w", id, err), notFoundSavedView(id))
	}
	return toSavedView(row)
}

// CreateSavedView saves a project's filters and swimlane under a name. The name is matched without
// regard to case and spaces, like a label's, so "needs me" replaces "Needs me" rather than sitting
// beside it, and the view keeps its id and takes the new spelling and the new place at the end of
// the list. It is a new view only when the name was not in use.
func (s *Service) CreateSavedView(ctx context.Context, projectID string, in protocol.CreateSavedViewRequest) (protocol.SavedView, bool, error) {
	name := strings.TrimSpace(in.Name)
	if err := checkSavedView(name, in.Filters, in.Swimlane); err != nil {
		return protocol.SavedView{}, false, err
	}
	swimlane := in.Swimlane
	if swimlane == "" {
		swimlane = protocol.SwimlaneNone
	}
	filters, err := encodeFilters(in.Filters)
	if err != nil {
		return protocol.SavedView{}, false, err
	}
	id, err := s.newID()
	if err != nil {
		return protocol.SavedView{}, false, fmt.Errorf("make a saved view id: %w", err)
	}
	var (
		row     db.SavedView
		created bool
	)
	err = s.store.Write(ctx, func(q *db.Queries) error {
		var err error
		row, created, err = s.upsertSavedView(ctx, q, savedViewInput{
			id: id, projectID: projectID, name: name, filters: filters, swimlane: swimlane,
		})
		return err
	})
	if err != nil {
		return protocol.SavedView{}, false, err
	}
	s.log.Info("saved a view", "project_id", projectID, "saved_view_id", row.ID, "new", created)
	s.publishSavedViews(ctx, projectID)
	view, err := toSavedView(row)
	return view, created, err
}

// savedViewInput is a view to be saved: the id it gets when it is new, and its project, name, encoded
// filters, and swimlane.
type savedViewInput struct {
	id, projectID, name, filters string
	swimlane                     protocol.Swimlane
}

// upsertSavedView writes a view inside a transaction: over the project's view of the same name when
// there is one, and as a new view otherwise. It says which.
func (s *Service) upsertSavedView(ctx context.Context, q *db.Queries, in savedViewInput) (db.SavedView, bool, error) {
	if _, err := q.GetProject(ctx, in.projectID); err != nil {
		return db.SavedView{}, false, notFound(fmt.Errorf("read project %s: %w", in.projectID, err), notFoundProject(in.projectID))
	}
	existing, err := q.ListSavedViewsByProject(ctx, in.projectID)
	if err != nil {
		return db.SavedView{}, false, fmt.Errorf("list the saved views of project %s: %w", in.projectID, err)
	}
	now := s.now().UnixMilli()
	if same := viewNamed(existing, in.name, ""); same != nil {
		row := db.SavedView{
			ID: same.ID, ProjectID: in.projectID, Name: in.name, FiltersJSON: in.filters,
			Swimlane: string(in.swimlane), CreatedAt: same.CreatedAt, UpdatedAt: now,
		}
		return row, false, updateSavedViewRow(ctx, q, row)
	}
	if len(existing) >= maxSavedViews {
		return db.SavedView{}, false, protocol.Refused(fmt.Sprintf(
			"A project can have at most %d saved views. Delete one and try again.", maxSavedViews))
	}
	row := db.SavedView{
		ID: in.id, ProjectID: in.projectID, Name: in.name, FiltersJSON: in.filters,
		Swimlane: string(in.swimlane), CreatedAt: now, UpdatedAt: now,
	}
	if err := q.CreateSavedView(ctx, db.CreateSavedViewParams(row)); err != nil {
		return db.SavedView{}, false, fmt.Errorf("insert the saved view: %w", err)
	}
	return row, true, nil
}

// UpdateSavedView renames a view, changes its filters, or changes its swimlane. A body that sets
// nothing answers with the view as it is and publishes nothing. A change moves the view to the end
// of the list, as saving it again does.
func (s *Service) UpdateSavedView(ctx context.Context, id string, in protocol.UpdateSavedViewRequest) (protocol.SavedView, error) {
	if in.Name == nil && in.Filters == nil && in.Swimlane == nil {
		return s.SavedView(ctx, id)
	}
	if err := checkSavedViewUpdate(in); err != nil {
		return protocol.SavedView{}, err
	}
	var row db.SavedView
	err := s.store.Write(ctx, func(q *db.Queries) error {
		current, err := q.GetSavedView(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read saved view %s: %w", id, err), notFoundSavedView(id))
		}
		row = current
		if err := applySavedViewUpdate(ctx, q, &row, in); err != nil {
			return err
		}
		row.UpdatedAt = s.now().UnixMilli()
		return updateSavedViewRow(ctx, q, row)
	})
	if err != nil {
		return protocol.SavedView{}, err
	}
	s.log.Info("edited a saved view", "project_id", row.ProjectID, "saved_view_id", row.ID)
	s.publishSavedViews(ctx, row.ProjectID)
	return toSavedView(row)
}

// DeleteSavedView removes a view. A person whose filters were that view is left with the filters
// and no view in use: the link from their preferences is cleared by the database, and clients drop
// a view id that is missing from the list they are sent.
func (s *Service) DeleteSavedView(ctx context.Context, id string) error {
	var projectID string
	err := s.store.Write(ctx, func(q *db.Queries) error {
		row, err := q.GetSavedView(ctx, id)
		if err != nil {
			return notFound(fmt.Errorf("read saved view %s: %w", id, err), notFoundSavedView(id))
		}
		projectID = row.ProjectID
		if _, err := q.DeleteSavedView(ctx, id); err != nil {
			return fmt.Errorf("delete saved view %s: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.log.Info("deleted a saved view", "project_id", projectID, "saved_view_id", id)
	s.publishSavedViews(ctx, projectID)
	return nil
}

// applySavedViewUpdate sets the fields the request names on the row, and refuses a name that
// another view of the project already has.
func applySavedViewUpdate(ctx context.Context, q *db.Queries, row *db.SavedView, in protocol.UpdateSavedViewRequest) error {
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		existing, err := q.ListSavedViewsByProject(ctx, row.ProjectID)
		if err != nil {
			return fmt.Errorf("list the saved views of project %s: %w", row.ProjectID, err)
		}
		if viewNamed(existing, name, row.ID) != nil {
			return protocol.Conflict("This project already has a saved view with that name.").With("name", name)
		}
		row.Name = name
	}
	if in.Filters != nil {
		filters, err := encodeFilters(*in.Filters)
		if err != nil {
			return err
		}
		row.FiltersJSON = filters
	}
	if in.Swimlane != nil {
		row.Swimlane = string(*in.Swimlane)
	}
	return nil
}

// updateSavedViewRow writes a view's changeable fields, and answers not found when the view went
// away between the read and the write.
func updateSavedViewRow(ctx context.Context, q *db.Queries, row db.SavedView) error {
	changed, err := q.UpdateSavedView(ctx, db.UpdateSavedViewParams{
		Name: row.Name, FiltersJSON: row.FiltersJSON, Swimlane: row.Swimlane, UpdatedAt: row.UpdatedAt, ID: row.ID,
	})
	if err != nil {
		return fmt.Errorf("update saved view %s: %w", row.ID, err)
	}
	if changed == 0 {
		return notFoundSavedView(row.ID)
	}
	return nil
}

// viewNamed finds a view of the list that has this name, ignoring case, other than exceptID. It
// answers nil when there is none.
func viewNamed(views []db.SavedView, name, exceptID string) *db.SavedView {
	for i := range views {
		if views[i].ID != exceptID && strings.EqualFold(views[i].Name, name) {
			return &views[i]
		}
	}
	return nil
}

// publishSavedViews publishes saved_view.updated with the project's views as they now are. It is
// critical: a client that missed it would keep offering a view that is gone.
func (s *Service) publishSavedViews(ctx context.Context, projectID string) {
	snapshot, err := s.SavedViews(ctx, projectID)
	if err != nil {
		// The change is committed; the event says what the list is now. A read that fails here is
		// logged, and the clients reload on their next connection.
		s.log.Error("could not read the saved views to publish them", "project_id", projectID, "error", err)
		return
	}
	s.publish(protocol.ProjectTopic(projectID), protocol.EventTypeSavedViewUpdated,
		protocol.SavedViewUpdatedEventData{ProjectID: projectID, Views: snapshot.Views}, true)
}

// checkSavedView refuses a view that cannot be saved, before the store is touched.
func checkSavedView(name string, filters []protocol.Filter, swimlane protocol.Swimlane) error {
	if err := checkSavedViewName(name); err != nil {
		return err
	}
	if err := protocol.CheckFilters(filters); err != nil {
		return err
	}
	if swimlane != "" {
		return checkSwimlane(swimlane)
	}
	return nil
}

// checkSavedViewUpdate refuses an edit that is not allowed, before the store is touched.
func checkSavedViewUpdate(in protocol.UpdateSavedViewRequest) error {
	if in.Name != nil {
		if err := checkSavedViewName(strings.TrimSpace(*in.Name)); err != nil {
			return err
		}
	}
	if in.Filters != nil {
		if err := protocol.CheckFilters(*in.Filters); err != nil {
			return err
		}
	}
	if in.Swimlane != nil {
		return checkSwimlane(*in.Swimlane)
	}
	return nil
}

func checkSavedViewName(name string) error {
	switch {
	case name == "":
		return protocol.InvalidArgument("Give the view a name.")
	case utf8.RuneCountInString(name) > maxSavedViewChars:
		return protocol.InvalidArgument(fmt.Sprintf("Saved view names can have at most %d characters.", maxSavedViewChars))
	}
	return nil
}

func checkSwimlane(swimlane protocol.Swimlane) error {
	if !swimlane.Valid() {
		return protocol.InvalidArgument("That is not a swimlane Marshal knows. Group by role, agent, package, or label, or use none.").
			With("swimlane", string(swimlane))
	}
	return nil
}

// encodeFilters turns a view's filter chips into the JSON the table stores. A view with no filters
// stores an empty list, never null.
func encodeFilters(filters []protocol.Filter) (string, error) {
	if filters == nil {
		filters = []protocol.Filter{}
	}
	data, err := json.Marshal(filters)
	if err != nil {
		return "", fmt.Errorf("encode the filters: %w", err)
	}
	return string(data), nil
}

// toSavedView builds the wire saved view from a row.
func toSavedView(row db.SavedView) (protocol.SavedView, error) {
	filters := []protocol.Filter{}
	if err := json.Unmarshal([]byte(row.FiltersJSON), &filters); err != nil {
		return protocol.SavedView{}, fmt.Errorf("read the filters of saved view %s: %w", row.ID, err)
	}
	if filters == nil {
		filters = []protocol.Filter{}
	}
	return protocol.SavedView{
		ID: row.ID, ProjectID: row.ProjectID, Name: row.Name, Filters: filters,
		Swimlane:  protocol.Swimlane(row.Swimlane),
		CreatedAt: store.Timestamp(row.CreatedAt), UpdatedAt: store.Timestamp(row.UpdatedAt),
	}, nil
}

// toSavedViews builds the wire saved views of a project, in the order the rows came.
func toSavedViews(rows []db.SavedView) ([]protocol.SavedView, error) {
	views := make([]protocol.SavedView, 0, len(rows))
	for _, row := range rows {
		view, err := toSavedView(row)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func notFoundSavedView(id string) *protocol.Error {
	return protocol.NotFound("saved view").With("id", id)
}
