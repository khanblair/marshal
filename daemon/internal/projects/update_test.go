package projects_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func str(s string) *string { return &s }

func flag(b bool) *bool { return &b }

func TestRenameChangesOnlyTheDisplayName(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	e.card(t, project.ID, "First")
	e.drainEvents()
	branchesBefore, err := e.git.Branches(context.Background(), project.Path, "")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := e.svc.Update(context.Background(), project.ID, protocol.UpdateProjectRequest{Name: str("  Dashboard  ")})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Dashboard" || renamed.ID != project.ID || renamed.Path != project.Path {
		t.Errorf("renamed = %+v; want a new name and the same id and folder", renamed)
	}
	if _, err := os.Stat(project.Path); err != nil {
		t.Errorf("the folder is gone: %v", err)
	}
	branchesAfter, err := e.git.Branches(context.Background(), project.Path, "")
	if err != nil || strings.Join(branchesAfter, ",") != strings.Join(branchesBefore, ",") {
		t.Errorf("branches %v then %v (%v); a rename must not touch Git", branchesBefore, branchesAfter, err)
	}
	ev := e.nextType(t, protocol.EventTypeProjectUpdated, protocol.HomeTopic)
	if data, ok := ev.Data.(protocol.ProjectEventData); !ok || data.Project.Name != "Dashboard" {
		t.Errorf("event data = %+v", ev.Data)
	}
}

func TestRenameRefusesAnEmptyNameAndKeepsTheOldOne(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	e.drainEvents()
	tests := []struct {
		name    string
		value   string
		message string
	}{
		{"empty", "", "Project names can't be empty. The old name is kept."},
		{"spaces", "   \t ", "Project names can't be empty. The old name is kept."},
		{"too long", strings.Repeat("x", 101), "Project names can have at most 100 characters."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.Update(context.Background(), project.ID, protocol.UpdateProjectRequest{Name: str(tc.value)})
			perr := wantCode(t, err, protocol.ErrorCodeInvalidArgument)
			if perr.Message != tc.message {
				t.Errorf("message = %q, want %q", perr.Message, tc.message)
			}
			got, err := e.svc.Get(context.Background(), project.ID)
			if err != nil || got.Name != project.Name {
				t.Errorf("name = %q, %v; want the old name %q", got.Name, err, project.Name)
			}
		})
	}
	e.noEvent(t)
}

func TestUpdateChangesTheFieldsThatAreSet(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	if _, err := e.git.Run(context.Background(), project.Path, "branch", "develop"); err != nil {
		t.Fatal(err)
	}
	e.drainEvents()
	updated, err := e.svc.Update(context.Background(), project.ID, protocol.UpdateProjectRequest{
		DevCommand: str("  npm run start  "), DefaultBranch: str("develop"), BypassLocked: flag(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.DevCommand != "npm run start" || updated.DefaultBranch != "develop" || !updated.BypassLocked || updated.Name != project.Name {
		t.Errorf("updated = %+v", updated)
	}
	cleared, err := e.svc.Update(context.Background(), project.ID, protocol.UpdateProjectRequest{
		DevCommand: str(""), BypassLocked: flag(false),
	})
	if err != nil || cleared.DevCommand != "" || cleared.BypassLocked || cleared.DefaultBranch != "develop" {
		t.Errorf("cleared = %+v, %v; want the command cleared, the lock off, and the branch kept", cleared, err)
	}
	e.nextType(t, protocol.EventTypeProjectUpdated, protocol.HomeTopic)
	e.nextType(t, protocol.EventTypeProjectUpdated, protocol.HomeTopic)
}

func TestUpdateRefusesValuesThatAreNotAllowed(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	e.drainEvents()
	tests := []struct {
		name string
		in   protocol.UpdateProjectRequest
	}{
		{"a branch that does not exist", protocol.UpdateProjectRequest{DefaultBranch: str("nope")}},
		{"a branch name that is not one", protocol.UpdateProjectRequest{DefaultBranch: str("bad name")}},
		{"an empty branch", protocol.UpdateProjectRequest{DefaultBranch: str("")}},
		{"a dev command on two lines", protocol.UpdateProjectRequest{DevCommand: str("a\nb")}},
		{"a dev command that is too long", protocol.UpdateProjectRequest{DevCommand: str(strings.Repeat("x", 501))}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.Update(context.Background(), project.ID, tc.in)
			_ = wantCode(t, err, protocol.ErrorCodeInvalidArgument)
		})
	}
	got, err := e.svc.Get(context.Background(), project.ID)
	if err != nil || got.DefaultBranch != "main" || got.DevCommand != "" {
		t.Errorf("project = %+v, %v; a refused update must change nothing", got, err)
	}
	e.noEvent(t)
}

func TestUpdateThatChangesNothingPublishesNothing(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	e.drainEvents()
	for name, in := range map[string]protocol.UpdateProjectRequest{
		"an empty update": {},
		"the same values": {Name: str(project.Name), DefaultBranch: str("main"), BypassLocked: flag(false)},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := e.svc.Update(context.Background(), project.ID, in)
			if err != nil || got.Name != project.Name {
				t.Errorf("Update = %+v, %v", got, err)
			}
		})
	}
	e.noEvent(t)
}

func TestUpdateOfAnUnknownProject(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Update(context.Background(), "nope", protocol.UpdateProjectRequest{Name: str("x")})
	perr := wantCode(t, err, protocol.ErrorCodeNotFound)
	if perr.Details["id"] != "nope" {
		t.Errorf("details = %v", perr.Details)
	}
	_, err = e.svc.Get(context.Background(), "nope")
	_ = wantCode(t, err, protocol.ErrorCodeNotFound)
}

func TestUpdatingTheBranchNeedsTheRepositoryFolder(t *testing.T) {
	e := newEnv(t)
	project := e.folder(t, "small-repo")
	if err := os.Rename(project.Path, project.Path+"-moved"); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Update(context.Background(), project.ID, protocol.UpdateProjectRequest{DefaultBranch: str("main")})
	_ = wantCode(t, err, protocol.ErrorCodeUnavailable)
	renamed, err := e.svc.Update(context.Background(), project.ID, protocol.UpdateProjectRequest{Name: str("still works")})
	if err != nil || renamed.Name != "still works" {
		t.Errorf("a rename must not need the folder: %+v, %v", renamed, err)
	}
}
