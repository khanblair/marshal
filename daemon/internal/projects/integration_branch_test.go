package projects_test

import (
	"context"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// folderOn makes a project from a fresh copy of a fixture after git has run the given commands in it.
func (e *env) folderOn(t *testing.T, gitArgs ...[]string) protocol.Project {
	t.Helper()
	path := testutil.Fixture(t, "small-repo")
	for _, args := range gitArgs {
		if _, err := e.git.Run(context.Background(), path, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	project, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceFolder, Path: path,
	})
	if err != nil {
		t.Fatalf("create a project: %v", err)
	}
	return project
}

// storedIntegrationBranch is the choice the database holds, before the default fills an empty one.
func (e *env) storedIntegrationBranch(t *testing.T, id string) string {
	t.Helper()
	row, err := e.store.Queries().GetProject(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return row.IntegrationBranch
}

func TestCreatePrefillsTheIntegrationBranchWithTheCurrentBranch(t *testing.T) {
	e := newEnv(t)
	project := e.folderOn(t, []string{"checkout", "-b", "development"})
	if project.DefaultBranch != "main" || project.IntegrationBranch != "development" || project.Target() != "development" {
		t.Errorf("project = %+v; want default main, integration and target development", project)
	}
	if got := e.storedIntegrationBranch(t, project.ID); got != "development" {
		t.Errorf("stored = %q, want development", got)
	}
}

func TestCreateLeavesTheIntegrationBranchEmptyOnADetachedHead(t *testing.T) {
	e := newEnv(t)
	project := e.folderOn(t, []string{"checkout", "--detach"})
	if got := e.storedIntegrationBranch(t, project.ID); got != "" {
		t.Errorf("stored = %q, want nothing chosen on a detached head", got)
	}
	if project.IntegrationBranch != "main" || project.Target() != "main" {
		t.Errorf("project = %+v; want the default branch to stand in", project)
	}
}

func TestUpdateSetsAndClearsTheIntegrationBranch(t *testing.T) {
	e := newEnv(t)
	project := e.folderOn(t, []string{"branch", "development"})
	e.drainEvents()
	set, err := e.svc.Update(context.Background(), project.ID,
		protocol.UpdateProjectRequest{IntegrationBranch: str("  development ")})
	if err != nil || set.IntegrationBranch != "development" || set.DefaultBranch != "main" {
		t.Fatalf("set = %+v, %v; want the integration branch development and the default kept", set, err)
	}
	if got := e.storedIntegrationBranch(t, project.ID); got != "development" {
		t.Errorf("stored = %q, want development", got)
	}
	ev := e.nextType(t, protocol.EventTypeProjectUpdated, protocol.HomeTopic)
	if data, ok := ev.Data.(protocol.ProjectEventData); !ok || data.Project.IntegrationBranch != "development" {
		t.Errorf("event data = %+v", ev.Data)
	}
	cleared, err := e.svc.Update(context.Background(), project.ID, protocol.UpdateProjectRequest{IntegrationBranch: str("")})
	if err != nil || cleared.IntegrationBranch != "main" || cleared.Target() != "main" {
		t.Fatalf("cleared = %+v, %v; want the default branch back", cleared, err)
	}
	if got := e.storedIntegrationBranch(t, project.ID); got != "" {
		t.Errorf("stored = %q after a clear, want nothing", got)
	}
	e.nextType(t, protocol.EventTypeProjectUpdated, protocol.HomeTopic)
}

func TestUpdateRefusesAnIntegrationBranchThatIsNotInTheRepository(t *testing.T) {
	e := newEnv(t)
	project := e.folderOn(t, []string{"branch", "development"})
	if _, err := e.svc.Update(context.Background(), project.ID,
		protocol.UpdateProjectRequest{IntegrationBranch: str("development")}); err != nil {
		t.Fatal(err)
	}
	e.drainEvents()
	for name, value := range map[string]string{"missing": "nope", "not a branch name": "bad name"} {
		t.Run(name, func(t *testing.T) {
			_, err := e.svc.Update(context.Background(), project.ID,
				protocol.UpdateProjectRequest{IntegrationBranch: str(value)})
			_ = wantCode(t, err, protocol.ErrorCodeInvalidArgument)
		})
	}
	if got := e.storedIntegrationBranch(t, project.ID); got != "development" {
		t.Errorf("stored = %q; a refused update must keep the old choice", got)
	}
	e.noEvent(t)
}

func TestTheIntegrationBranchFollowsTheDefaultUntilOneIsChosen(t *testing.T) {
	e := newEnv(t)
	project := e.folderOn(t, []string{"checkout", "--detach"}, []string{"branch", "trunk"})
	got, err := e.svc.Update(context.Background(), project.ID, protocol.UpdateProjectRequest{DefaultBranch: str("trunk")})
	if err != nil || got.IntegrationBranch != "trunk" || got.Target() != "trunk" {
		t.Fatalf("got = %+v, %v; want the new default to be the integration branch while none is chosen", got, err)
	}
	list, err := e.svc.List(context.Background())
	if err != nil || len(list.Projects) != 1 || list.Projects[0].IntegrationBranch != "trunk" {
		t.Errorf("List = %+v, %v; want the effective branch in the list too", list, err)
	}
}
