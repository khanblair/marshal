package fixture_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The fixture gives each project the saved views the prototype's menu shows, apart from "All cards",
// which is the app's own view and is never stored.
func TestLoadPrototypeWritesThePrototypesSavedViews(t *testing.T) {
	e := newEnv(t)
	if err := e.load(t); err != nil {
		t.Fatalf("LoadPrototype: %v", err)
	}
	want := map[string][]string{
		"api":    {"Needs me", "Claude Code by role"},
		"web":    {"UI work", "By agent"},
		"mobile": {"By package", "api-client only"},
	}
	for id, names := range want {
		got, err := e.svc.SavedViews(context.Background(), id)
		if err != nil {
			t.Fatalf("SavedViews(%s): %v", id, err)
		}
		if len(got.Views) != len(names) {
			t.Fatalf("project %s has %d saved views, want %d", id, len(got.Views), len(names))
		}
		for i, view := range got.Views {
			if view.Name != names[i] {
				t.Errorf("project %s view %d is %q, want %q", id, i, view.Name, names[i])
			}
		}
	}
	got, err := e.svc.SavedViews(context.Background(), "api")
	if err != nil {
		t.Fatal(err)
	}
	needs := got.Views[0]
	if needs.Swimlane != protocol.SwimlaneNone || len(needs.Filters) != 1 ||
		needs.Filters[0].Key != protocol.FilterKeyStatus || needs.Filters[0].Value != "needs" {
		t.Errorf("api's Needs me = %+v, want the status filter for needs and no grouping", needs)
	}
	if got.Views[1].Swimlane != protocol.SwimlaneRole {
		t.Errorf("api's Claude Code by role groups by %q, want role", got.Views[1].Swimlane)
	}
}

// Loading twice does not duplicate them, and a view a person deleted does not come back while the
// project still has another one.
func TestLoadPrototypeSavedViewsAreSafeToRepeat(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for i := 1; i <= 2; i++ {
		if err := e.load(t); err != nil {
			t.Fatalf("LoadPrototype (run %d): %v", i, err)
		}
	}
	got, err := e.svc.SavedViews(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Views) != 2 {
		t.Fatalf("web has %d saved views after two loads, want 2", len(got.Views))
	}
	if err := e.svc.DeleteSavedView(ctx, got.Views[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := e.load(t); err != nil {
		t.Fatalf("LoadPrototype (after a delete): %v", err)
	}
	after, err := e.svc.SavedViews(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Views) != 1 || after.Views[0].Name != "By agent" {
		t.Errorf("web has %+v after a delete and a load, want only By agent", after.Views)
	}
}
