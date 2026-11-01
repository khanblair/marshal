package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/khanblair/marshal/daemon/internal/ci"
	"github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/pullrequest"
	"github.com/khanblair/marshal/daemon/internal/review"
	"github.com/khanblair/marshal/daemon/internal/roles"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// appForge is the CI monitor's forge, answered from the GitHub App saved in Settings (B6.2,
// architecture.md section 9). It resolves the App on every call rather than holding a client, so
// connecting, replacing, or removing GitHub in Settings takes effect without a restart, and a daemon
// nobody has connected answers ci.ErrNoForge - which the monitor treats as "there is nothing to ask"
// rather than as a failure.
type appForge struct {
	svc *integrations.Service
}

// client answers the connected GitHub client, or ci.ErrNoForge when none is usable.
func (f appForge) client(ctx context.Context) (integrations.GitHubClient, error) {
	client, err := f.svc.GitHubClient(ctx)
	if errors.Is(err, integrations.ErrNotConnected) || errors.Is(err, integrations.ErrGitHubReconnect) {
		return nil, ci.ErrNoForge
	}
	return client, err
}

// ListWorkflowRuns lists one branch's workflow runs, newest first.
func (f appForge) ListWorkflowRuns(ctx context.Context, repo github.Repository, branch string) ([]github.WorkflowRun, error) {
	client, err := f.client(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListWorkflowRuns(ctx, repo, branch)
}

// RerunFailedJobs asks GitHub to run one run's failed jobs again.
func (f appForge) RerunFailedJobs(ctx context.Context, repo github.Repository, runID int64) error {
	client, err := f.client(ctx)
	if err != nil {
		return err
	}
	return client.RerunFailedJobs(ctx, repo, runID)
}

// FailedLog reads the log of one run's first failed job, cut to maxBytes.
func (f appForge) FailedLog(ctx context.Context, repo github.Repository, runID int64, maxBytes int) (string, error) {
	client, err := f.client(ctx)
	if err != nil {
		return "", err
	}
	return client.FailedLog(ctx, repo, runID, maxBytes)
}

// lateGitHub is the pull-request and review services' GitHub client. It asks the connections service
// for the connected client on every call, so connecting GitHub in Settings works without a restart.
// It answers github.ErrNotConnected until the service is attached and while nothing is connected.
type lateGitHub struct {
	svc atomic.Pointer[integrations.Service]
}

func (l *lateGitHub) client(ctx context.Context) (github.Client, error) {
	svc := l.svc.Load()
	if svc == nil {
		return nil, github.ErrNotConnected
	}
	client, err := svc.GitHubClient(ctx)
	switch {
	case errors.Is(err, integrations.ErrNotConnected):
		return nil, github.ErrNotConnected
	case errors.Is(err, integrations.ErrGitHubReconnect):
		return nil, github.ErrReconnect
	case err != nil:
		return nil, err
	}
	return client, nil
}

func (l *lateGitHub) CreatePullRequest(ctx context.Context, req github.NewPullRequest) (github.PullRequest, error) {
	client, err := l.client(ctx)
	if err != nil {
		return github.PullRequest{}, err
	}
	return client.CreatePullRequest(ctx, req)
}

func (l *lateGitHub) GetPullRequest(ctx context.Context, repo github.Repository, number int) (github.PullRequest, error) {
	client, err := l.client(ctx)
	if err != nil {
		return github.PullRequest{}, err
	}
	return client.GetPullRequest(ctx, repo, number)
}

func (l *lateGitHub) CreateReviewComment(ctx context.Context, req github.NewReviewComment) (github.ReviewComment, error) {
	client, err := l.client(ctx)
	if err != nil {
		return github.ReviewComment{}, err
	}
	return client.CreateReviewComment(ctx, req)
}

func (l *lateGitHub) ListChecks(ctx context.Context, repo github.Repository, ref string) ([]github.Check, error) {
	client, err := l.client(ctx)
	if err != nil {
		return nil, err
	}
	return client.ListChecks(ctx, repo, ref)
}

var _ github.Client = (*lateGitHub)(nil)

// buildForge builds the two services that talk to GitHub: the pull-request service, which opens a
// card's branch as a pull request, and the review service, which runs the Reviewer role over every
// pull request. Both are always built and reach GitHub through client, so with nothing connected a
// request is refused with a sentence that says to connect GitHub.
//
// The review is wired into the pull-request service's After hook, so a pull request is read as soon
// as it exists (build-plan 5.7).
func buildForge(client github.Client, proj *projects.Service, git *gitx.Git, roleSvc *roles.Service, sessions *session.Manager, log *slog.Logger) (*pullrequest.Service, *review.Service, error) {
	checker, err := review.NewChecksReviewer(client)
	if err != nil {
		return nil, nil, fmt.Errorf("start the review module: %w", err)
	}
	reviewSvc, err := review.New(review.Deps{
		Cards: proj, Projects: proj, Roles: roleSvc, Git: git, Forge: client,
		Reviews: checker, Worker: sessions, Log: log,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("start the review service: %w", err)
	}
	svc, err := pullrequest.New(pullrequest.Deps{
		Client: client, Cards: proj, Projects: proj, Git: git, Log: log,
		After: func(ctx context.Context, cardID string) error {
			_, err := reviewSvc.Review(ctx, cardID)
			return err
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("start the pull-request service: %w", err)
	}
	log.Info("the pull-request and review services are ready")
	return svc, reviewSvc, nil
}

// lateSessions lets the projects service reach the session manager, which is built after it
// because the manager needs the projects service. It does nothing until the manager is set.
type lateSessions struct {
	manager atomic.Pointer[session.Manager]
}

func (l *lateSessions) StopCardSession(ctx context.Context, cardID string) error {
	if m := l.manager.Load(); m != nil {
		return m.StopCardSession(ctx, cardID)
	}
	return nil
}

func (l *lateSessions) RemoveCardLogs(ctx context.Context, cardID string) error {
	if m := l.manager.Load(); m != nil {
		return m.RemoveCardLogs(ctx, cardID)
	}
	return nil
}

func (l *lateSessions) StopProjectSessions(ctx context.Context, projectID string) error {
	if m := l.manager.Load(); m != nil {
		return m.StopProjectSessions(ctx, projectID)
	}
	return nil
}

func (l *lateSessions) AwakeCards(ctx context.Context, projectID string) (int, error) {
	if m := l.manager.Load(); m != nil {
		return m.AwakeCards(ctx, projectID)
	}
	return 0, nil
}

// lateMemory lets the projects service reach the memory module, which is built after it because the
// module reads a project's cards through the projects service. It does nothing until the module is
// set, which means a project removed before then leaves its memory folder alone - a folder the
// person can delete themselves, where deleting it on a guess is not something to do.
type lateMemory struct {
	svc atomic.Pointer[memory.Service]
}

func (l *lateMemory) RemoveProjectMemory(ctx context.Context, projectID string) error {
	if s := l.svc.Load(); s != nil {
		return s.RemoveProjectMemory(ctx, projectID)
	}
	return nil
}
