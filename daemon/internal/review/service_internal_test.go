package review

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

/*
 * The review service's own work: which card may be reviewed, what is posted on the pull request,
 * where the card goes, and what the worker is told. The Reviewer itself is a fake here, so these
 * tests are about the flow and not about how a verdict is reached.
 */

type fakeCards struct {
	card    protocol.Card
	states  []protocol.CardState
	stateFn func(protocol.CardState) error
}

func (f *fakeCards) Card(context.Context, string) (protocol.Card, error) { return f.card, nil }

func (f *fakeCards) SetState(_ context.Context, _ string, s protocol.CardState) (protocol.Card, error) {
	if f.stateFn != nil {
		if err := f.stateFn(s); err != nil {
			return protocol.Card{}, err
		}
	}
	f.states = append(f.states, s)
	f.card.State = s
	return f.card, nil
}

type fakeProjects struct {
	project protocol.Project
	err     error
}

func (f fakeProjects) Get(context.Context, string) (protocol.Project, error) {
	return f.project, f.err
}

type fakeRoles struct {
	role protocol.Role
	err  error
}

func (f fakeRoles) Role(context.Context, string, string) (protocol.Role, error) {
	return f.role, f.err
}

type fakeGit struct {
	remotes []gitx.Remote
	err     error
}

func (f fakeGit) Inspect(context.Context, string) (gitx.RepoInfo, error) {
	if f.err != nil {
		return gitx.RepoInfo{}, f.err
	}
	return gitx.RepoInfo{DefaultBranch: "main", Remotes: f.remotes}, nil
}

type fakeForge struct {
	posts  []github.NewReviewComment
	err    error
	failOn int
}

func (f *fakeForge) CreateReviewComment(_ context.Context, req github.NewReviewComment) (github.ReviewComment, error) {
	f.posts = append(f.posts, req)
	if f.err != nil && (f.failOn == 0 || f.failOn == len(f.posts)) {
		return github.ReviewComment{}, f.err
	}
	return github.ReviewComment{Body: req.Body, Path: req.Path, Line: req.Line}, nil
}

type fakeWorker struct {
	sent   []string
	err    error
	cardID string
}

func (f *fakeWorker) Send(_ context.Context, cardID, text string) error {
	f.cardID, f.sent = cardID, append(f.sent, text)
	return f.err
}

type fakeReviewer struct {
	verdict Verdict
	err     error
	req     Request
	calls   int
}

func (f *fakeReviewer) Review(_ context.Context, req Request) (Verdict, error) {
	f.calls++
	f.req = req
	return f.verdict, f.err
}

// githubOrigin is the remote every test's project has, unless it is testing the case where there is
// none.
func githubOrigin() []gitx.Remote {
	return []gitx.Remote{{Name: "origin", URL: "git@github.com:khan/marshal.git"}}
}

func reviewCard() protocol.Card {
	return protocol.Card{
		ID: "card-0001", ProjectID: "web", Key: "web#1", Title: "Add retry",
		State: protocol.CardStateReview, Branch: "marshal/card-1",
		PullRequest: &protocol.PullRequest{Number: 287, URL: "https://github.com/khan/marshal/pull/287"},
	}
}

// built is the whole service and every fake behind it, so a test can look at what each one was
// asked.
type built struct {
	svc      *Service
	cards    *fakeCards
	projects fakeProjects
	roles    fakeRoles
	git      fakeGit
	forge    *fakeForge
	worker   *fakeWorker
	reviews  *fakeReviewer
}

func newReviewer(t *testing.T, card protocol.Card, opts ...func(*Deps)) *built {
	t.Helper()
	cards := &fakeCards{card: card}
	forge := &fakeForge{}
	worker := &fakeWorker{}
	reviews := &fakeReviewer{}
	deps := Deps{
		Cards:    cards,
		Projects: fakeProjects{project: protocol.Project{ID: "web", Path: "/code/web", DefaultBranch: "main"}},
		Roles:    fakeRoles{role: protocol.Role{ID: "role-1", Name: ReviewerRoleName, Starter: true}},
		Git:      fakeGit{remotes: githubOrigin()},
		Forge:    forge,
		Reviews:  reviews,
		Worker:   worker,
	}
	for _, opt := range opts {
		opt(&deps)
	}
	svc, err := New(deps)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &built{
		svc: svc, cards: cards, projects: deps.Projects.(fakeProjects), roles: deps.Roles.(fakeRoles),
		git: deps.Git.(fakeGit), forge: forge, worker: worker, reviews: reviews,
	}
}

func TestAnApprovedReviewPostsItsSentenceAndMovesTheCardToReady(t *testing.T) {
	b := newReviewer(t, reviewCard())
	b.reviews.verdict = Verdict{Approved: true, Summary: "Reviewer approved. All 2 checks passed on pull request #287."}

	result, err := b.svc.Review(context.Background(), "card-0001")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if !result.Approved || result.Waiting {
		t.Fatalf("result = %+v, want an approval", result)
	}
	if result.State != protocol.CardStateReady || b.cards.card.State != protocol.CardStateReady {
		t.Fatalf("state = %s, want ready", b.cards.card.State)
	}
	if len(b.forge.posts) != 1 || !strings.Contains(b.forge.posts[0].Body, "approved") {
		t.Fatalf("posts = %+v, want the reviewer's sentence on the pull request", b.forge.posts)
	}
	if b.forge.posts[0].Number != 287 || b.forge.posts[0].Repo.String() != "khan/marshal" {
		t.Fatalf("the comment went to the wrong pull request: %+v", b.forge.posts[0])
	}
	if len(b.worker.sent) != 0 {
		t.Fatalf("an approval is not sent to the worker, which would drag the card back to working: %v", b.worker.sent)
	}
}

func TestAReviewThatAsksForChangesMovesTheCardToWorkingAndTellsTheWorker(t *testing.T) {
	b := newReviewer(t, reviewCard())
	b.reviews.verdict = Verdict{
		Summary: "Reviewer asked for changes: build did not pass on pull request #287.",
		Comments: []Comment{
			{Body: "`build` did not pass on this pull request (failure). http://ci/1"},
			{Body: "The retry loop can spin forever.", Path: "cmd/api/main.go", Line: 41},
		},
	}

	result, err := b.svc.Review(context.Background(), "card-0001")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if result.Approved || result.State != protocol.CardStateWorking {
		t.Fatalf("result = %+v, want changes requested and a working card", result)
	}
	if len(b.forge.posts) != 3 {
		t.Fatalf("posts = %d, want the sentence and both comments", len(b.forge.posts))
	}
	line := b.forge.posts[2]
	if line.Path != "cmd/api/main.go" || line.Line != 41 {
		t.Fatalf("a line comment keeps its file and line: %+v", line)
	}
	if !result.ToldTheWorker || len(b.worker.sent) != 1 {
		t.Fatalf("the review was not sent back to the worker: %+v", result)
	}
	text := b.worker.sent[0]
	if b.worker.cardID != "card-0001" {
		t.Fatalf("the message went to %q", b.worker.cardID)
	}
	for _, want := range []string{"Reviewer read pull request #287", "asked for changes", "spin forever", "cmd/api/main.go:41"} {
		if !strings.Contains(text, want) {
			t.Errorf("the worker's message is missing %q: %s", want, text)
		}
	}
}

func TestAReviewWithNothingToSayYetPostsNothingAndLeavesTheCard(t *testing.T) {
	b := newReviewer(t, reviewCard())
	b.reviews.verdict = Verdict{Waiting: true, Summary: "Reviewer is waiting: the checks are still running."}

	result, err := b.svc.Review(context.Background(), "card-0001")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if !result.Waiting || result.State != protocol.CardStateReview {
		t.Fatalf("result = %+v, want the card left in review", result)
	}
	if len(b.forge.posts) != 0 || len(b.cards.states) != 0 || len(b.worker.sent) != 0 {
		t.Fatalf("waiting wrote something: posts=%v states=%v worker=%v",
			b.forge.posts, b.cards.states, b.worker.sent)
	}
}

func TestTheReviewerIsGivenTheCardItsRoleAndThePullRequest(t *testing.T) {
	b := newReviewer(t, reviewCard())
	b.reviews.verdict = Verdict{Approved: true, Summary: "Approved."}

	if _, err := b.svc.Review(context.Background(), "card-0001"); err != nil {
		t.Fatalf("Review: %v", err)
	}
	got := b.reviews.req
	if got.Card.ID != "card-0001" {
		t.Errorf("card = %q", got.Card.ID)
	}
	if got.Role.Name != ReviewerRoleName {
		t.Errorf("role = %q, want the Reviewer role", got.Role.Name)
	}
	if got.Target.Number != 287 || got.Target.Head != "marshal/card-1" || got.Target.Base != "main" {
		t.Errorf("target = %+v", got.Target)
	}
}

func TestACardWithNoPullRequestIsRefused(t *testing.T) {
	card := reviewCard()
	card.PullRequest = nil
	b := newReviewer(t, card)

	_, err := b.svc.Review(context.Background(), "card-0001")
	assertRefusal(t, err, "review_no_pull_request")
	if b.reviews.calls != 0 {
		t.Fatal("the Reviewer was asked to read a pull request that does not exist")
	}
}

func TestACardThatIsNotInReviewIsRefused(t *testing.T) {
	card := reviewCard()
	card.State = protocol.CardStateWorking
	b := newReviewer(t, card)

	_, err := b.svc.Review(context.Background(), "card-0001")
	assertRefusal(t, err, "review_not_in_review")
	if b.reviews.calls != 0 {
		t.Fatal("a card whose agent is still writing was reviewed anyway")
	}
}

func TestAProjectWithNoReviewerRoleIsRefused(t *testing.T) {
	b := newReviewer(t, reviewCard(), func(d *Deps) {
		d.Roles = fakeRoles{err: errors.New("no such role")}
	})

	_, err := b.svc.Review(context.Background(), "card-0001")
	assertRefusal(t, err, "review_no_role")
	if b.reviews.calls != 0 {
		t.Fatal("the review ran under no role")
	}
}

func TestAProjectWhoseOriginIsNotGitHubIsRefused(t *testing.T) {
	b := newReviewer(t, reviewCard(), func(d *Deps) {
		d.Git = fakeGit{remotes: []gitx.Remote{{Name: "origin", URL: "https://gitlab.com/khan/marshal.git"}}}
	})

	_, err := b.svc.Review(context.Background(), "card-0001")
	assertRefusal(t, err, "review_not_github")
}

func TestAProjectWithNoOriginRemoteIsRefused(t *testing.T) {
	b := newReviewer(t, reviewCard(), func(d *Deps) {
		d.Git = fakeGit{remotes: []gitx.Remote{{Name: "upstream", URL: "git@github.com:other/marshal.git"}}}
	})

	_, err := b.svc.Review(context.Background(), "card-0001")
	assertRefusal(t, err, "review_not_github")
}

func TestABadTokenIsNamedAsASignInProblem(t *testing.T) {
	b := newReviewer(t, reviewCard())
	b.reviews.verdict = Verdict{Approved: true, Summary: "Approved."}
	b.forge.err = &github.APIError{Status: 401, Message: "Bad credentials"}

	_, err := b.svc.Review(context.Background(), "card-0001")
	assertRefusal(t, err, "github_unauthorized")
	if strings.Contains(err.Error(), "Bad credentials") {
		t.Errorf("the sign-in sentence should not repeat the forge's own words: %v", err)
	}
}

func TestACardIsMovedEvenWhenTheWorkerCannotBeReached(t *testing.T) {
	b := newReviewer(t, reviewCard())
	b.reviews.verdict = Verdict{Summary: "Reviewer asked for changes.", Comments: []Comment{{Body: "Fix it."}}}
	b.worker.err = errors.New("no session")

	result, err := b.svc.Review(context.Background(), "card-0001")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if result.ToldTheWorker {
		t.Error("the worker was not reached, so the result must not claim it was")
	}
	if result.State != protocol.CardStateWorking || b.cards.card.State != protocol.CardStateWorking {
		t.Fatalf("state = %s, want working", b.cards.card.State)
	}
	if len(b.forge.posts) != 2 {
		t.Fatalf("the review still reached the pull request: %+v", b.forge.posts)
	}
}

func TestAReviewWithNoWorkerStillMovesTheCard(t *testing.T) {
	b := newReviewer(t, reviewCard(), func(d *Deps) { d.Worker = nil })
	b.reviews.verdict = Verdict{Summary: "Reviewer asked for changes."}

	result, err := b.svc.Review(context.Background(), "card-0001")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if result.State != protocol.CardStateWorking || result.ToldTheWorker {
		t.Fatalf("result = %+v", result)
	}
}

func TestAMoveThatFailsIsReportedAndNothingIsPretended(t *testing.T) {
	b := newReviewer(t, reviewCard())
	b.reviews.verdict = Verdict{Approved: true, Summary: "Approved."}
	b.cards.stateFn = func(protocol.CardState) error { return errors.New("the database is gone") }

	if _, err := b.svc.Review(context.Background(), "card-0001"); err == nil {
		t.Fatal("a card that could not be moved is an error")
	}
}

func TestANewReviewerNeedsEveryPart(t *testing.T) {
	full := func() Deps {
		return Deps{
			Cards:    &fakeCards{card: reviewCard()},
			Projects: fakeProjects{},
			Roles:    fakeRoles{},
			Git:      fakeGit{},
			Forge:    &fakeForge{},
			Reviews:  &fakeReviewer{},
		}
	}
	if _, err := New(full()); err != nil {
		t.Fatalf("a complete set of parts should build: %v", err)
	}
	for name, drop := range map[string]func(*Deps){
		"cards":    func(d *Deps) { d.Cards = nil },
		"projects": func(d *Deps) { d.Projects = nil },
		"roles":    func(d *Deps) { d.Roles = nil },
		"git":      func(d *Deps) { d.Git = nil },
		"forge":    func(d *Deps) { d.Forge = nil },
		"reviews":  func(d *Deps) { d.Reviews = nil },
	} {
		deps := full()
		drop(&deps)
		if _, err := New(deps); err == nil {
			t.Errorf("a review with no %s should not build", name)
		}
	}
}

// assertRefusal checks that an error is a refusal carrying the reason a client branches on.
func assertRefusal(t *testing.T, err error, reason string) {
	t.Helper()
	var refusal *protocol.Error
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if refusal.Code != protocol.ErrorCodeRefused {
		t.Fatalf("code = %s, want refused", refusal.Code)
	}
	if refusal.Details["reason"] != reason {
		t.Fatalf("reason = %q, want %q", refusal.Details["reason"], reason)
	}
}
