package pullrequest

import (
	"context"
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// fakeCards is the projects module's card half, recorded in memory.
type fakeCards struct {
	card     protocol.Card
	setPR    *protocol.PullRequest
	movedTo  *protocol.CardState
	setErr   error
	moveErr  error
	setCalls int
}

func (f *fakeCards) Card(context.Context, string) (protocol.Card, error) { return f.card, nil }
func (f *fakeCards) SetPullRequest(_ context.Context, _ string, pr protocol.PullRequest) (protocol.Card, error) {
	f.setCalls++
	f.setPR = &pr
	if f.setErr != nil {
		return protocol.Card{}, f.setErr
	}
	card := f.card
	card.PullRequest = &pr
	f.card = card
	return card, nil
}
func (f *fakeCards) SetState(_ context.Context, _ string, state protocol.CardState) (protocol.Card, error) {
	if f.moveErr != nil {
		return protocol.Card{}, f.moveErr
	}
	f.movedTo = &state
	card := f.card
	card.State = state
	f.card = card
	return card, nil
}

type fakeProjects struct{ project protocol.Project }

func (f fakeProjects) Get(context.Context, string) (protocol.Project, error) { return f.project, nil }

type fakeGit struct{ info gitx.RepoInfo }

func (f fakeGit) Inspect(context.Context, string) (gitx.RepoInfo, error) { return f.info, nil }

// fakeClient records what was asked of the forge.
type fakeClient struct {
	created []github.NewPullRequest
	next    github.PullRequest
	err     error
}

func (f *fakeClient) CreatePullRequest(_ context.Context, req github.NewPullRequest) (github.PullRequest, error) {
	f.created = append(f.created, req)
	if f.err != nil {
		return github.PullRequest{}, f.err
	}
	return f.next, nil
}
func (f *fakeClient) GetPullRequest(context.Context, github.Repository, int) (github.PullRequest, error) {
	return github.PullRequest{}, nil
}
func (f *fakeClient) CreateReviewComment(context.Context, github.NewReviewComment) (github.ReviewComment, error) {
	return github.ReviewComment{}, nil
}
func (f *fakeClient) ListChecks(context.Context, github.Repository, string) ([]github.Check, error) {
	return nil, nil
}

// newService builds a service around a card on a working branch of a GitHub project.
func newService(t *testing.T, card protocol.Card, client github.Client) (*Service, *fakeCards) {
	t.Helper()
	cards := &fakeCards{card: card}
	svc, err := New(Deps{
		Client:   client,
		Cards:    cards,
		Projects: fakeProjects{project: protocol.Project{ID: "web", Path: "/code/web", DefaultBranch: "main"}},
		Git: fakeGit{info: gitx.RepoInfo{
			DefaultBranch: "main",
			Remotes:       []gitx.Remote{{Name: "origin", URL: "https://github.com/acme/web.git"}},
		}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc, cards
}

func workingCard() protocol.Card {
	return protocol.Card{ID: "card-1", ProjectID: "web", Title: "Add retry", Body: "Closes #1",
		State: protocol.CardStateWorking, Branch: "marshal/card-1"}
}

func TestOpenCreatesThePullRequestAndMovesTheCardToReview(t *testing.T) {
	client := &fakeClient{next: github.PullRequest{Number: 287, URL: "https://github.com/acme/web/pull/287", State: "open"}}
	svc, cards := newService(t, workingCard(), client)

	got, err := svc.Open(context.Background(), "card-1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(client.created) != 1 {
		t.Fatalf("created %d pull requests, want 1", len(client.created))
	}
	req := client.created[0]
	if req.Repo.Owner != "acme" || req.Repo.Name != "web" {
		t.Errorf("repo = %s, want acme/web", req.Repo)
	}
	if req.Head != "marshal/card-1" || req.Base != "main" {
		t.Errorf("head/base = %q/%q, want the card's branch into main", req.Head, req.Base)
	}
	if req.Title != "Add retry" {
		t.Errorf("title = %q, want the card's title", req.Title)
	}
	if cards.setPR == nil || cards.setPR.Number != 287 {
		t.Errorf("stored pull request = %+v, want #287", cards.setPR)
	}
	if cards.movedTo == nil || *cards.movedTo != protocol.CardStateReview {
		t.Errorf("moved to %v, want review", cards.movedTo)
	}
	if got.State != protocol.CardStateReview || got.PullRequest == nil || got.PullRequest.Number != 287 {
		t.Errorf("card = %+v, want review with the pull request", got)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	card := workingCard()
	card.PullRequest = &protocol.PullRequest{Number: 1, URL: "https://github.com/acme/web/pull/1"}
	client := &fakeClient{next: github.PullRequest{Number: 2}}
	svc, cards := newService(t, card, client)

	got, err := svc.Open(context.Background(), "card-1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(client.created) != 0 {
		t.Errorf("opened %d pull requests for a card that already had one", len(client.created))
	}
	if cards.setCalls != 0 || cards.movedTo != nil {
		t.Errorf("changed a card that already had a pull request")
	}
	if got.PullRequest.Number != 1 {
		t.Errorf("pull request = %+v, want the existing one", got.PullRequest)
	}
}

func TestOpenRefusesACardWithNoBranch(t *testing.T) {
	card := workingCard()
	card.Branch = ""
	svc, _ := newService(t, card, &fakeClient{})
	_, err := svc.Open(context.Background(), "card-1")
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeRefused {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

func TestOpenRefusesAProjectWithNoGitHubRemote(t *testing.T) {
	cards := &fakeCards{card: workingCard()}
	svc, err := New(Deps{
		Client:   &fakeClient{},
		Cards:    cards,
		Projects: fakeProjects{project: protocol.Project{ID: "web", Path: "/code/web", DefaultBranch: "main"}},
		Git: fakeGit{info: gitx.RepoInfo{
			Remotes: []gitx.Remote{{Name: "origin", URL: "https://gitlab.example.com/acme/web.git"}},
		}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = svc.Open(context.Background(), "card-1")
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeRefused {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

func TestOpenTranslatesAnUnauthorizedForgeError(t *testing.T) {
	client := &fakeClient{err: &github.APIError{Status: 401, Message: "Bad credentials"}}
	svc, cards := newService(t, workingCard(), client)
	_, err := svc.Open(context.Background(), "card-1")
	var refusal *protocol.Error
	if !errors.As(err, &refusal) || refusal.Code != protocol.ErrorCodeRefused {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if refusal.Details["reason"] != "github_unauthorized" {
		t.Errorf("reason = %v, want github_unauthorized", refusal.Details["reason"])
	}
	if cards.setPR != nil || cards.movedTo != nil {
		t.Errorf("stored or moved a card after a failed call")
	}
}

// refusedMove builds the refusal the projects module answers for a card whose changes have a
// blocking code smell (docs/architecture.md section 17.1).
func refusedMove() *protocol.Error {
	return protocol.Refused("This card's changes have code smells to fix first.").
		With("reason", string(protocol.MoveRefusalReasonQualityBlocking))
}

// newServiceWithReview builds a service around a card and records whether the Reviewer ran.
func newServiceWithReview(t *testing.T, cards *fakeCards, client github.Client) (*Service, *int) {
	t.Helper()
	reviewed := 0
	svc, err := New(Deps{
		Client:   client,
		Cards:    cards,
		Projects: fakeProjects{project: protocol.Project{ID: "web", Path: "/code/web", DefaultBranch: "main"}},
		Git: fakeGit{info: gitx.RepoInfo{
			DefaultBranch: "main",
			Remotes:       []gitx.Remote{{Name: "origin", URL: "https://github.com/acme/web.git"}},
		}},
		After: func(context.Context, string) error { reviewed++; return nil },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc, &reviewed
}

// A card whose changes have a blocking code smell stays in Working after its pull request is
// opened: the pull request is real and is recorded, but the Reviewer reads a card in review, so it
// is not run until the card really gets there.
func TestOpenKeepsACardInWorkingWhenItsChangesBlock(t *testing.T) {
	client := &fakeClient{next: github.PullRequest{Number: 287, URL: "https://github.com/acme/web/pull/287"}}
	cards := &fakeCards{card: workingCard(), moveErr: refusedMove()}
	svc, reviewed := newServiceWithReview(t, cards, client)

	got, err := svc.Open(context.Background(), "card-1")
	if err != nil {
		t.Fatalf("opening a pull request must succeed even when the card cannot move: %v", err)
	}
	if cards.setPR == nil || cards.setPR.Number != 287 {
		t.Errorf("the pull request is recorded either way, got %+v", cards.setPR)
	}
	if got.State != protocol.CardStateWorking {
		t.Errorf("a card with a blocking smell stays in Working, got %s", got.State)
	}
	if *reviewed != 0 {
		t.Errorf("the Reviewer ran %d times for a card that is not in review", *reviewed)
	}
}

// A move to review that failed for any other reason is logged and the open still succeeds, and the
// Reviewer still runs: only the quality module's own refusal means "there is nothing to read yet".
func TestOpenStillReviewsWhenTheMoveFailedForAnotherReason(t *testing.T) {
	client := &fakeClient{next: github.PullRequest{Number: 287, URL: "https://github.com/acme/web/pull/287"}}
	cards := &fakeCards{card: workingCard(), moveErr: errors.New("the store is unhappy")}
	svc, reviewed := newServiceWithReview(t, cards, client)

	if _, err := svc.Open(context.Background(), "card-1"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if *reviewed != 1 {
		t.Errorf("the Reviewer ran %d times, want once", *reviewed)
	}
}

// qualityBlocking is true only for the quality module's own refusal: another refusal of the same
// move, or a plain error, is not the quality gate's.
func TestQualityBlockingIsOnlyTheQualityRefusal(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"the quality refusal", refusedMove(), true},
		{"another refusal of the same move",
			protocol.Refused("In review needs an open pull request.").
				With("reason", string(protocol.MoveRefusalReasonNeedsPullRequest)), false},
		{"a plain error", errors.New("boom"), false},
		{"no error", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := qualityBlocking(tc.err); got != tc.want {
				t.Errorf("qualityBlocking(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestRepositoryFromURLReadsEveryCloneForm(t *testing.T) {
	cases := []struct {
		raw  string
		want github.Repository
		ok   bool
	}{
		{"https://github.com/acme/web.git", github.Repository{Owner: "acme", Name: "web"}, true},
		{"https://github.com/acme/web", github.Repository{Owner: "acme", Name: "web"}, true},
		{"https://github.com/acme/web/", github.Repository{Owner: "acme", Name: "web"}, true},
		{"ssh://git@github.com/acme/web.git", github.Repository{Owner: "acme", Name: "web"}, true},
		{"git@github.com:acme/web.git", github.Repository{Owner: "acme", Name: "web"}, true},
		{"https://github.example.com/acme/web.git", github.Repository{Owner: "acme", Name: "web"}, true},
		{"https://gitlab.com/acme/web.git", github.Repository{}, false},
		{"https://github.com/acme", github.Repository{}, false},
		{"", github.Repository{}, false},
	}
	for _, tc := range cases {
		got, ok := repositoryFromURL(tc.raw)
		if ok != tc.ok || got != tc.want {
			t.Errorf("repositoryFromURL(%q) = %+v, %v; want %+v, %v", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}
