package projects_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/sample"
)

// sampleRequest asks for a project from the sample repository.
func sampleRequest() protocol.CreateProjectRequest {
	return protocol.CreateProjectRequest{Source: protocol.ProjectSourceSample}
}

// The sample becomes an ordinary project: a real repository under the data folder, read like any
// folder, announced like any project, and with the sample's history in it.
func TestCreateFromTheSample(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project, err := e.svc.Create(ctx, sampleRequest())
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.Project{
		ID: "marshal-sample", Name: "marshal-sample", Language: projects.LanguageTypeScript,
		DefaultBranch: "main", DevCommand: "npm run dev", Packages: []string{}, CreatedAt: project.CreatedAt,
	}
	if !sameFolder(t, project.Path, sample.Folder(e.dataDir)) {
		t.Errorf("path = %q, want the sample folder under the data folder", project.Path)
	}
	project.Path = ""
	if !equalProjects(project, want) {
		t.Errorf("project = %+v, want %+v", project, want)
	}
	e.nextType(t, protocol.EventTypeProjectCreated, protocol.HomeTopic)
	stored, err := e.svc.Get(ctx, "marshal-sample")
	if err != nil {
		t.Fatal(err)
	}
	log, err := e.git.Run(ctx, stored.Path, "log", "--format=%s", sample.Branch)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(log, "\n"); len(got) != 2 || got[1] != "Start the sample todo service" {
		t.Errorf("the sample project's history is %q", got)
	}
}

// A name in the request is used instead of the folder's.
func TestTheSampleCanBeNamed(t *testing.T) {
	e := newEnv(t)
	project, err := e.svc.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceSample, Name: "Try Marshal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "Try Marshal" || project.ID != "try-marshal" {
		t.Errorf("project = %s (%s), want Try Marshal (try-marshal)", project.Name, project.ID)
	}
}

// While the sample is a project, a second one is refused with a sentence and the project's id, so
// the app can open the one there is, and nothing else changes.
func TestASecondSampleIsRefused(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, err := e.svc.Create(ctx, sampleRequest())
	if err != nil {
		t.Fatal(err)
	}
	e.drainEvents()
	_, err = e.svc.Create(ctx, sampleRequest())
	refusal := wantCode(t, err, protocol.ErrorCodeConflict)
	if refusal.Message != "The sample project is already in Marshal." || refusal.Details["projectId"] != first.ID {
		t.Errorf("refusal = %q %v, want the sample's sentence and projectId %s", refusal.Message, refusal.Details, first.ID)
	}
	e.noEvent(t)
	list, err := e.svc.List(ctx)
	if err != nil || len(list.Projects) != 1 {
		t.Errorf("projects = %+v, %v; want the one sample", list.Projects, err)
	}
}

// Removing the sample project leaves its folder, and asking again adds that folder back as it is,
// with what the person did in it.
func TestTheSampleComesBackAfterItIsRemoved(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, err := e.svc.Create(ctx, sampleRequest())
	if err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(first.Path, "notes.md")
	if err := os.WriteFile(notes, []byte("tried a thing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Remove(ctx, first.ID, projects.RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	again, err := e.svc.Create(ctx, sampleRequest())
	if err != nil {
		t.Fatalf("add the sample again: %v", err)
	}
	if !sameFolder(t, again.Path, first.Path) {
		t.Errorf("the sample came back at %s, want %s", again.Path, first.Path)
	}
	if _, err := os.Stat(notes); err != nil {
		t.Errorf("the person's file in the sample is gone: %v", err)
	}
}

// A folder in the sample's place that is not a repository is refused with a sentence, left as it
// is, and no project is added.
func TestASampleFolderThatIsNotARepositoryIsRefused(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	folder := sample.Folder(e.dataDir)
	if err := os.MkdirAll(folder, 0o750); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(folder, "keep.txt")
	if err := os.WriteFile(kept, []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.Create(ctx, sampleRequest())
	refusal := wantCode(t, err, protocol.ErrorCodeConflict)
	if !strings.Contains(refusal.Message, "sample folder") || strings.Contains(refusal.Message, e.dataDir) {
		t.Errorf("message = %q, want a sentence about the sample folder without its path", refusal.Message)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the file in the folder is gone: %v", err)
	}
	if list, err := e.svc.List(ctx); err != nil || len(list.Projects) != 0 {
		t.Errorf("projects = %+v, %v; want none", list.Projects, err)
	}
}
