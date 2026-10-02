// Package review runs the Reviewer role over a card's pull request (docs/architecture.md section 8,
// docs/backend-checklist.md B5.4, build-plan 5.7). A card that is In review has a pull request the
// Reviewer reads; what it finds is posted on the pull request, sent back to the worker that wrote
// the branch, and acted on: an approval moves the card to Ready to merge, and a request for changes
// moves it back to Working so the agent fixes what was found.
//
// The half that reads a diff and writes prose about it needs a model, so it sits behind the
// Reviewer interface: Phase 5 ships ChecksReviewer, which reads the checks the forge reports (the
// Reviewer role's own instruction is "Approve only when every check passes"), and a later phase
// adds a second implementation that reads the diff through the same interface. Nothing here depends
// on which of them it is given, the way internal/pullrequest never depends on *github.TokenClient.
//
// Nothing here signs a person in and nothing here talks to a real repository: every call goes
// through the forge client's interface, and a test drives the whole flow with fakes.
package review

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// ReviewerRoleName is the starter role that reads pull requests. It is the name Marshal ships the
// role under (internal/roles/seed.go) and the name the app's own role list uses, so a project that
// renamed its copy is the only project whose review runs under another name.
const ReviewerRoleName = "Reviewer"

// Cards is the part of the projects module this service needs.
type Cards interface {
	// Card reads one card.
	Card(ctx context.Context, id string) (protocol.Card, error)
	// SetState moves a card to a state with no manual-move rules: the daemon's own moves.
	SetState(ctx context.Context, id string, state protocol.CardState) (protocol.Card, error)
}

// Projects is the part of the projects module this service needs to read a project.
type Projects interface {
	// Get reads one project.
	Get(ctx context.Context, id string) (protocol.Project, error)
}

// Roles reads the role a review runs as, so the words on the pull request and the sentence sent
// back to the worker name the role the project actually keeps.
type Roles interface {
	// Role reads one role by name, the project's own version of it when it has one.
	Role(ctx context.Context, name, projectID string) (protocol.Role, error)
}

// Git is the part of gitx this service needs: reading a repository's remotes to learn which forge
// and repository a card's work is in.
type Git interface {
	// Inspect reads a repository's facts, including its remotes.
	Inspect(ctx context.Context, path string) (gitx.RepoInfo, error)
}

// Forge is the part of the forge client this service needs: posting one comment on a pull request.
type Forge interface {
	// CreateReviewComment posts a comment on a pull request or on one line of its diff.
	CreateReviewComment(ctx context.Context, req github.NewReviewComment) (github.ReviewComment, error)
}

// Worker is how a review reaches the agent that wrote the branch: a message into the card's own
// session, which wakes it if it is asleep. The session manager implements it.
type Worker interface {
	// Send delivers a message into a card's session.
	Send(ctx context.Context, cardID, text string) error
}

// Target is the pull request a review is about: the forge's own name for the repository, the pull
// request's number, and the branches involved.
type Target struct {
	// Repo is the repository the pull request is in.
	Repo github.Repository
	// Number is the pull request's number, which a person reads.
	Number int
	// Head is the branch the work is on (the card's branch), which the checks are read from.
	Head string
	// Base is the branch it is being merged into.
	Base string
}

// Comment is one thing the Reviewer said. An empty Path is a comment on the pull request itself;
// otherwise it is a comment on that line of the file's diff.
type Comment struct {
	// Body is the comment's text. It cannot be empty.
	Body string
	// Path is the file the comment is on, empty for a comment on the pull request.
	Path string
	// Line is the line in the file's diff. Used only when Path is set.
	Line int
}

// Verdict is what the Reviewer answered.
type Verdict struct {
	// Approved is true when the Reviewer read the pull request and approved it. It is false when
	// the Reviewer asked for changes, and false with Waiting true when it could not decide yet.
	Approved bool
	// Waiting is true when the Reviewer has nothing to say yet, such as checks that are still
	// running. The card is left exactly as it is.
	Waiting bool
	// Summary is the one sentence posted on the pull request and sent to the worker.
	Summary string
	// Comments are what the Reviewer found, one by one. They are posted after the summary.
	Comments []Comment
}

// Request is what a Reviewer is given: the card, the role it runs as, and the pull request.
type Request struct {
	// Card is the card whose branch is under review.
	Card protocol.Card
	// Role is the role the review runs as: its model, its instructions, and its name.
	Role protocol.Role
	// Target is the pull request being read.
	Target Target
}

// Reviewer reads a pull request and answers with a verdict. ChecksReviewer is the Phase 5
// implementation; a model-driven one goes behind the same interface later.
type Reviewer interface {
	Review(ctx context.Context, req Request) (Verdict, error)
}

// Deps are the parts the service is built from.
type Deps struct {
	// Cards reads and moves the card.
	Cards Cards
	// Projects reads the project.
	Projects Projects
	// Roles reads the Reviewer role.
	Roles Roles
	// Git reads the project's remotes.
	Git Git
	// Forge posts the review's comments on the pull request.
	Forge Forge
	// Reviews is the Reviewer itself.
	Reviews Reviewer
	// Worker delivers the review to the agent that wrote the branch. Nil means nothing is sent
	// back, and the review then only reaches the pull request.
	Worker Worker
	// Log is where problems are written. Nil discards.
	Log *slog.Logger
}

// Service runs the Reviewer over a card's pull request. It is safe for use by many goroutines.
type Service struct {
	cards    Cards
	projects Projects
	roles    Roles
	git      Git
	forge    Forge
	reviews  Reviewer
	worker   Worker
	log      *slog.Logger
}

// New builds a Service. Everything but Worker is required.
func New(deps Deps) (*Service, error) {
	if deps.Cards == nil || deps.Projects == nil || deps.Roles == nil || deps.Git == nil ||
		deps.Forge == nil || deps.Reviews == nil {
		return nil, errors.New("the review needs the cards, the projects, the roles, Git, the forge, and a reviewer")
	}
	log := deps.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{
		cards: deps.Cards, projects: deps.Projects, roles: deps.Roles, git: deps.Git,
		forge: deps.Forge, reviews: deps.Reviews, worker: deps.Worker, log: log,
	}, nil
}

// Result says what a review did.
type Result struct {
	// Approved is true when the Reviewer approved the pull request.
	Approved bool
	// Waiting is true when the Reviewer had nothing to say yet and the card was left as it was.
	Waiting bool
	// State is the card's state after the review.
	State protocol.CardState
	// Reason is a plain sentence for a person, empty when the review finished cleanly.
	Reason string
	// Comments is how many comments were posted on the pull request.
	Comments int
	// ToldTheWorker is true when the review was delivered into the card's session, so the agent
	// that wrote the branch has it. It is false when the worker was not reached, which is logged.
	ToldTheWorker bool
}

// Review runs the Reviewer over a card's pull request. The card must be In review and must have a
// pull request; anything else is refused, because the Reviewer reads a pull request and not a
// branch somebody is still writing to.
func (s *Service) Review(ctx context.Context, cardID string) (Result, error) {
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		return Result{}, err
	}
	if card.PullRequest == nil || card.PullRequest.Number <= 0 {
		return Result{}, protocol.Refused("This card has no pull request to review yet.").
			With("cardId", cardID).With("reason", "review_no_pull_request")
	}
	if card.State != protocol.CardStateReview {
		return Result{}, protocol.Refused("Only a card that is in review is reviewed.").
			With("cardId", cardID).With("reason", "review_not_in_review")
	}
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return Result{}, err
	}
	role, err := s.roles.Role(ctx, ReviewerRoleName, project.ID)
	if err != nil {
		return Result{}, protocol.Refused("This project has no Reviewer role, so nothing read the pull request.").
			With("cardId", cardID).With("reason", "review_no_role")
	}
	target, err := s.target(ctx, project, card)
	if err != nil {
		return Result{}, err
	}
	verdict, err := s.reviews.Review(ctx, Request{Card: card, Role: role, Target: target})
	if err != nil {
		return Result{}, translate(err, cardID)
	}
	if verdict.Waiting {
		return Result{Waiting: true, State: card.State, Reason: verdict.Summary}, nil
	}
	comments, err := s.post(ctx, target, verdict)
	if err != nil {
		return Result{}, translate(err, cardID)
	}
	result := Result{Approved: verdict.Approved, State: card.State, Comments: comments}
	if verdict.Approved {
		moved, err := s.cards.SetState(ctx, cardID, protocol.CardStateReady)
		if err != nil {
			return Result{}, err
		}
		result.State = moved.State
		return result, nil
	}
	// A request for changes goes back to the agent that wrote the branch. The message is what
	// makes the card's own session start the turn that reads it, so the card is moved first: a
	// delivery that cannot be made must still leave the card where the Reviewer put it.
	if _, err := s.cards.SetState(ctx, cardID, protocol.CardStateWorking); err != nil {
		return Result{}, err
	}
	result.State = protocol.CardStateWorking
	result.Reason = verdict.Summary
	if s.worker == nil {
		return result, nil
	}
	if err := s.worker.Send(ctx, cardID, workerText(role, target, verdict)); err != nil {
		s.log.Warn("could not send a review back to the worker", "card_id", cardID, "error", err)
		return result, nil
	}
	result.ToldTheWorker = true
	return result, nil
}

// post writes the Reviewer's sentence and every comment it made onto the pull request, and answers
// how many were written.
func (s *Service) post(ctx context.Context, target Target, verdict Verdict) (int, error) {
	written := 0
	if summary := strings.TrimSpace(verdict.Summary); summary != "" {
		if err := s.comment(ctx, target, Comment{Body: summary}); err != nil {
			return written, err
		}
		written++
	}
	for _, one := range verdict.Comments {
		if strings.TrimSpace(one.Body) == "" {
			continue
		}
		if err := s.comment(ctx, target, one); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

// comment posts one comment on the pull request.
func (s *Service) comment(ctx context.Context, target Target, one Comment) error {
	_, err := s.forge.CreateReviewComment(ctx, github.NewReviewComment{
		Repo:   target.Repo,
		Number: target.Number,
		Body:   one.Body,
		Path:   one.Path,
		Line:   one.Line,
	})
	return err
}

// target reads the project's GitHub repository, the branches, and the pull request's number.
func (s *Service) target(ctx context.Context, project protocol.Project, card protocol.Card) (Target, error) {
	info, err := s.git.Inspect(ctx, project.Path)
	if err != nil {
		return Target{}, fmt.Errorf("read the remotes of project %s: %w", project.ID, err)
	}
	repo, ok := originRepository(info.Remotes)
	if !ok {
		return Target{}, protocol.Refused(
			"This project's origin remote is not a GitHub repository, so its pull request cannot be read.").
			With("projectId", project.ID).With("reason", "review_not_github")
	}
	base := project.Target()
	if strings.TrimSpace(base) == "" {
		base = info.DefaultBranch
	}
	number := 0
	if card.PullRequest != nil {
		number = card.PullRequest.Number
	}
	return Target{Repo: repo, Number: number, Head: card.Branch, Base: base}, nil
}

// translate turns a forge failure into a plain sentence for the person who asked, the same way the
// pull-request service does, so a bad token reads as a sign-in problem wherever it is hit.
func translate(err error, cardID string) error {
	switch {
	case errors.Is(err, github.ErrNotConnected):
		return protocol.Refused("GitHub is not connected. Connect it in Settings, under Integrations.").
			With("cardId", cardID).With("reason", "github_not_connected")
	case errors.Is(err, github.ErrReconnect):
		return protocol.Refused("GitHub access has expired. Reconnect GitHub in Settings.").
			With("cardId", cardID).With("reason", "github_reconnect")
	}
	var api *github.APIError
	if errors.As(err, &api) {
		switch {
		case api.Unauthorized():
			return protocol.Refused("GitHub refused the saved token. Sign in again in Settings.").
				With("cardId", cardID).With("reason", "github_unauthorized")
		case api.NotFound():
			return protocol.Refused("GitHub could not find the pull request for this card.").
				With("cardId", cardID).With("reason", "github_not_found")
		default:
			return protocol.Refused("GitHub refused the review: "+api.Message).
				With("cardId", cardID).With("reason", "github_refused")
		}
	}
	return fmt.Errorf("review card %s: %w", cardID, err)
}

// workerText is what the agent that wrote the branch is sent: the Reviewer's sentence, and then
// every comment it made, so the agent has the whole review in its own conversation.
func workerText(role protocol.Role, target Target, verdict Verdict) string {
	name := roleName(role)
	var b strings.Builder
	fmt.Fprintf(&b, "%s read pull request #%d and asked for changes.", name, target.Number)
	if summary := strings.TrimSpace(verdict.Summary); summary != "" {
		b.WriteString("\n\n")
		b.WriteString(summary)
	}
	for _, one := range verdict.Comments {
		body := strings.TrimSpace(one.Body)
		if body == "" {
			continue
		}
		b.WriteString("\n\n- ")
		if one.Path != "" {
			fmt.Fprintf(&b, "%s:%d: ", one.Path, one.Line)
		}
		b.WriteString(body)
	}
	return b.String()
}

// roleName is a role's name, or "The Reviewer" when a role somehow has none: a sentence a person
// reads must not start with a blank.
func roleName(role protocol.Role) string {
	if name := strings.TrimSpace(role.Name); name != "" {
		return name
	}
	return "The Reviewer"
}

// originRepository is the origin remote's owner and name, or false when there is no GitHub origin.
// Only the origin remote is read: a fork or a mirror under another name is not where a card's pull
// request lives.
func originRepository(remotes []gitx.Remote) (github.Repository, bool) {
	for _, remote := range remotes {
		if remote.Name != "origin" {
			continue
		}
		if repo, ok := github.RepositoryFromURL(remote.URL); ok {
			return repo, true
		}
	}
	return github.Repository{}, false
}
