package pullrequest

import (
	"context"
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestOpenTargetsTheIntegrationBranch(t *testing.T) {
	client := &fakeClient{next: github.PullRequest{Number: 3, URL: "https://github.com/acme/web/pull/3", State: "open"}}
	svc, err := New(Deps{
		Client: client,
		Cards:  &fakeCards{card: workingCard()},
		Projects: fakeProjects{project: protocol.Project{
			ID: "web", Path: "/code/web", DefaultBranch: "main", IntegrationBranch: "development",
		}},
		Git: fakeGit{info: gitx.RepoInfo{
			DefaultBranch: "main",
			Remotes:       []gitx.Remote{{Name: "origin", URL: "https://github.com/acme/web.git"}},
		}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := svc.Open(context.Background(), "card-1"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(client.created) != 1 || client.created[0].Base != "development" {
		t.Errorf("created = %+v, want one pull request into development", client.created)
	}
}

// denyPush is a policy that allows no project to publish.
type denyPush struct{}

func (denyPush) AllowPush(context.Context, string) bool { return false }

// A project that does not allow publishing has no pull request opened and nothing sent to GitHub.
func TestOpenIsRefusedForAProjectThatDoesNotAllowPublishing(t *testing.T) {
	client := &fakeClient{next: github.PullRequest{Number: 3, URL: "https://github.com/acme/web/pull/3", State: "open"}}
	svc, err := New(Deps{
		Client:   client,
		Cards:    &fakeCards{card: workingCard()},
		Projects: fakeProjects{project: protocol.Project{ID: "web", Path: "/code/web", DefaultBranch: "main"}},
		Git: fakeGit{info: gitx.RepoInfo{
			DefaultBranch: "main",
			Remotes:       []gitx.Remote{{Name: "origin", URL: "https://github.com/acme/web.git"}},
		}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc.SetPolicy(denyPush{})
	_, err = svc.Open(context.Background(), "card-1")
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Details["reason"] != "push_not_allowed" {
		t.Fatalf("Open = %v, want a refusal with reason push_not_allowed", err)
	}
	if len(client.created) != 0 {
		t.Errorf("a pull request was created for a project that does not allow publishing: %+v", client.created)
	}
}
