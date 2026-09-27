package review_test

import (
	"context"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/review"
)

/*
 * The ChecksReviewer is the half of the Reviewer role the daemon can run without a model: the role's
 * own instruction is "Approve only when every check passes", and this reads the checks the forge
 * reports. It is driven here against a fake forge client, so no network and no sign-in is involved.
 */

// fakeChecks is the forge client the tests drive: it answers ListChecks and records what it was
// asked, so the ref the checks were read from is part of the test and not an assumption.
type fakeChecks struct {
	checks []github.Check
	err    error
	repo   github.Repository
	ref    string
}

func (f *fakeChecks) ListChecks(_ context.Context, repo github.Repository, ref string) ([]github.Check, error) {
	f.repo, f.ref = repo, ref
	if f.err != nil {
		return nil, f.err
	}
	return f.checks, nil
}

func (f *fakeChecks) CreatePullRequest(context.Context, github.NewPullRequest) (github.PullRequest, error) {
	panic("the checks reviewer never opens a pull request")
}

func (f *fakeChecks) GetPullRequest(context.Context, github.Repository, int) (github.PullRequest, error) {
	panic("the checks reviewer never reads a pull request")
}

func (f *fakeChecks) CreateReviewComment(context.Context, github.NewReviewComment) (github.ReviewComment, error) {
	panic("the checks reviewer never posts a comment itself")
}

// request is a review of pull request 287 on the card's own branch.
func request() review.Request {
	return review.Request{
		Card: protocol.Card{ID: "card-1", Branch: "marshal/card-1"},
		Role: protocol.Role{Name: "Reviewer"},
		Target: review.Target{
			Repo:   github.Repository{Owner: "khan", Name: "marshal"},
			Number: 287,
			Head:   "marshal/card-1",
			Base:   "main",
		},
	}
}

func reviewer(t *testing.T, fake *fakeChecks) *review.ChecksReviewer {
	t.Helper()
	built, err := review.NewChecksReviewer(fake)
	if err != nil {
		t.Fatalf("build a checks reviewer: %v", err)
	}
	return built
}

func TestTheChecksReviewerApprovesWhenEveryCheckPassed(t *testing.T) {
	fake := &fakeChecks{checks: []github.Check{
		{Name: "build", Status: "completed", Conclusion: "success"},
		{Name: "lint", Status: "completed", Conclusion: "success"},
	}}
	verdict, err := reviewer(t, fake).Review(context.Background(), request())
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !verdict.Approved || verdict.Waiting {
		t.Fatalf("every check passed, so the verdict should approve: %+v", verdict)
	}
	if len(verdict.Comments) != 0 {
		t.Fatalf("an approval has nothing to fix: %+v", verdict.Comments)
	}
	if !strings.Contains(verdict.Summary, "2 checks") || !strings.Contains(verdict.Summary, "#287") {
		t.Fatalf("the sentence should name the count and the pull request: %q", verdict.Summary)
	}
}

func TestTheChecksReviewerReadsTheChecksOfTheCardsOwnBranch(t *testing.T) {
	fake := &fakeChecks{checks: []github.Check{{Name: "build", Status: "completed", Conclusion: "success"}}}
	if _, err := reviewer(t, fake).Review(context.Background(), request()); err != nil {
		t.Fatalf("review: %v", err)
	}
	if fake.ref != "marshal/card-1" {
		t.Fatalf("the checks of the card's branch are the ones read, got %q", fake.ref)
	}
	if fake.repo.String() != "khan/marshal" {
		t.Fatalf("the repository is the pull request's own, got %q", fake.repo.String())
	}
}

func TestTheChecksReviewerAsksForChangesAndNamesEachFailingCheck(t *testing.T) {
	fake := &fakeChecks{checks: []github.Check{
		{Name: "build", Status: "completed", Conclusion: "failure"},
		{Name: "lint", Status: "completed", Conclusion: "skipped"},
		{Name: "unit", Status: "completed", Conclusion: "success"},
	}}
	verdict, err := reviewer(t, fake).Review(context.Background(), request())
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if verdict.Approved || verdict.Waiting {
		t.Fatalf("a check did not pass, so the verdict asks for changes: %+v", verdict)
	}
	if len(verdict.Comments) != 2 {
		t.Fatalf("one comment per check that did not pass: %+v", verdict.Comments)
	}
	if !strings.Contains(verdict.Summary, "build") || !strings.Contains(verdict.Summary, "lint") {
		t.Fatalf("the sentence names what did not pass: %q", verdict.Summary)
	}
	if strings.Contains(verdict.Summary, "unit") {
		t.Fatalf("a check that passed is not named: %q", verdict.Summary)
	}
	if !strings.Contains(verdict.Comments[1].Body, "skipped") {
		t.Fatalf("a check that did not run is named as such: %q", verdict.Comments[1].Body)
	}
}

func TestTheChecksReviewerWaitsWhenACheckIsStillRunning(t *testing.T) {
	fake := &fakeChecks{checks: []github.Check{
		{Name: "build", Status: "completed", Conclusion: "success"},
		{Name: "e2e", Status: "in_progress", Conclusion: ""},
	}}
	verdict, err := reviewer(t, fake).Review(context.Background(), request())
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !verdict.Waiting || verdict.Approved {
		t.Fatalf("a running check is nothing to approve or refuse yet: %+v", verdict)
	}
	if len(verdict.Comments) != 0 {
		t.Fatalf("nothing is posted while waiting: %+v", verdict.Comments)
	}
}

func TestTheChecksReviewerWaitsWhenAPullRequestHasNoChecks(t *testing.T) {
	fake := &fakeChecks{}
	verdict, err := reviewer(t, fake).Review(context.Background(), request())
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !verdict.Waiting || verdict.Approved {
		t.Fatalf("nothing ran, so nothing is approved: %+v", verdict)
	}
}

func TestTheChecksReviewerReportsAForgeFailure(t *testing.T) {
	fake := &fakeChecks{err: &github.APIError{Status: 401, Message: "Bad credentials"}}
	if _, err := reviewer(t, fake).Review(context.Background(), request()); err == nil {
		t.Fatal("a refused call is an error, not a verdict")
	}
}

func TestANewChecksReviewerNeedsAForgeClient(t *testing.T) {
	if _, err := review.NewChecksReviewer(nil); err == nil {
		t.Fatal("a checks reviewer with no forge client cannot read anything")
	}
}

func TestTheChecksReviewerNamesTheRoleItRunsAs(t *testing.T) {
	fake := &fakeChecks{checks: []github.Check{{Name: "build", Status: "completed", Conclusion: "success"}}}
	req := request()
	req.Role.Name = "Gatekeeper"
	verdict, err := reviewer(t, fake).Review(context.Background(), req)
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !strings.HasPrefix(verdict.Summary, "Gatekeeper approved") {
		t.Fatalf("a renamed role is the name a person reads: %q", verdict.Summary)
	}
}

var _ review.Reviewer = (*review.ChecksReviewer)(nil)
