// Package pullrequest turns a card's work into a real pull request (docs/architecture.md section 8,
// docs/backend-checklist.md B5.4, build-plan 5.6). When a card's agent has finished work on its
// branch, the branch is opened as a pull request against the project's default branch, the link is
// recorded on the card, and the card moves to In review so a person (and the Reviewer role) can see
// it. The forge's own facts travel back: the number, the address, and, in Phase 6, the checks.
//
// It depends on the github.Client interface and never on a token or a URL, the way every caller of
// gitx never touches exec.Cmd: Phase 6's GitHub App implementation goes behind the same interface,
// so nothing here changes when it arrives.
package pullrequest

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

// Cards is the part of the projects module this service needs. The projects service implements it.
type Cards interface {
	// Card reads one card.
	Card(ctx context.Context, id string) (protocol.Card, error)
	// SetPullRequest records a card's pull request link.
	SetPullRequest(ctx context.Context, id string, pr protocol.PullRequest) (protocol.Card, error)
	// SetState moves a card to a state with no manual-move rules: the daemon's own moves, like the
	// session manager's, go through here.
	SetState(ctx context.Context, id string, state protocol.CardState) (protocol.Card, error)
}

// Projects is the part of the projects module this service needs to read a project.
type Projects interface {
	// Get reads one project.
	Get(ctx context.Context, id string) (protocol.Project, error)
}

// Git is the part of gitx this service needs: reading a repository's remotes to learn which forge
// and repository a project is.
type Git interface {
	// Inspect reads a repository's facts, including its remotes.
	Inspect(ctx context.Context, path string) (gitx.RepoInfo, error)
}

// Deps are the parts the service is built from.
type Deps struct {
	// Client talks to the forge.
	Client github.Client
	// Cards reads and writes the card.
	Cards Cards
	// Projects reads the project.
	Projects Projects
	// Git reads the project's remotes.
	Git Git
	// After is called once a pull request has been opened and its link stored, with the card's id.
	// The daemon wires the Reviewer here, so every pull request is read as soon as it exists
	// (build-plan 5.7). Its error is logged and never fails the open: the pull request is there
	// either way, and a review that could not run is a thing to retry, not a thing that undid it.
	After func(ctx context.Context, cardID string) error
	// Log is where problems are written. A nil logger discards.
	Log *slog.Logger
}

// Service implements the pull-request flow. It is safe for use by many goroutines.
type Service struct {
	client   github.Client
	cards    Cards
	projects Projects
	git      Git
	after    func(ctx context.Context, cardID string) error
	log      *slog.Logger
}

// New builds a Service. The forge client and the cards it writes are required.
func New(deps Deps) (*Service, error) {
	if deps.Client == nil || deps.Cards == nil || deps.Projects == nil || deps.Git == nil {
		return nil, errors.New("the pull-request service needs a forge client, the cards, the projects, and Git")
	}
	log := deps.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{
		client: deps.Client, cards: deps.Cards, projects: deps.Projects, git: deps.Git,
		after: deps.After, log: log,
	}, nil
}

// Open opens a pull request for a card's branch and answers the card as it now stands. It is
// idempotent: a card that already has a pull request is returned unchanged, so a double click
// (or a retry) opens one pull request and not two.
//
// A card with no branch, a project with no GitHub remote, and a forge that refuses the request all
// answer with a plain sentence and leave the card as it was. The card is only moved to In review
// after the pull request exists and its link is stored.
func (s *Service) Open(ctx context.Context, cardID string) (protocol.Card, error) {
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		return protocol.Card{}, err
	}
	if card.PullRequest != nil && card.PullRequest.Number > 0 {
		return card, nil
	}
	if strings.TrimSpace(card.Branch) == "" {
		return protocol.Card{}, protocol.Refused("This card has no branch yet. Start it first.").With("cardId", cardID)
	}
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return protocol.Card{}, err
	}
	repo, base, err := s.resolve(ctx, project)
	if err != nil {
		return protocol.Card{}, err
	}
	pr, err := s.client.CreatePullRequest(ctx, github.NewPullRequest{
		Repo:  repo,
		Title: card.Title,
		Body:  card.Body,
		Head:  card.Branch,
		Base:  base,
	})
	if err != nil {
		return protocol.Card{}, translate(err, cardID)
	}
	updated, err := s.cards.SetPullRequest(ctx, cardID, protocol.PullRequest{Number: pr.Number, URL: pr.URL})
	if err != nil {
		return protocol.Card{}, err
	}
	answer := updated
	if updated.State == protocol.CardStateWorking || updated.State == protocol.CardStateNeeds {
		moved, err := s.cards.SetState(ctx, cardID, protocol.CardStateReview)
		switch {
		case err == nil:
			answer = moved
		case qualityBlocking(err):
			// The card's changes have a blocking code smell, so it stayed in Working and the finding
			// has already gone back to its agent (docs/architecture.md section 17.1). The Reviewer
			// reads a card in review, so there is nothing to read yet: the review runs when the card
			// really gets there, which is the person's own move once the smell is fixed.
			s.log.Info("kept a card in Working until its code smells are fixed",
				"card_id", cardID, "number", pr.Number)
			return answer, nil
		default:
			// The pull request exists and is recorded; the move failing must not undo that.
			s.log.Warn("could not move a card to review after opening its pull request",
				"card_id", cardID, "number", pr.Number, "error", err)
		}
	}
	s.review(ctx, cardID)
	return answer, nil
}

// qualityBlocking reports whether a refused move to In review was the quality module's own refusal
// (docs/architecture.md section 17.1): the card's changes have a blocking code smell, and the
// finding has already gone back to the card's agent by the time this is asked.
func qualityBlocking(err error) bool {
	var answer *protocol.Error
	if !errors.As(err, &answer) {
		return false
	}
	return answer.Code == protocol.ErrorCodeRefused &&
		answer.Details["reason"] == string(protocol.MoveRefusalReasonQualityBlocking)
}

// review runs the Reviewer over the pull request that was just opened. Every pull request is read:
// that is what "the Reviewer role runs on every pull request" means (build-plan 5.7). A review that
// cannot run is logged and never undone the open, because the pull request exists either way.
func (s *Service) review(ctx context.Context, cardID string) {
	if s.after == nil {
		return
	}
	if err := s.after(ctx, cardID); err != nil {
		s.log.Warn("could not review the pull request that was just opened",
			"card_id", cardID, "error", err)
	}
}

// resolve reads the project's GitHub repository and the branch to merge into: the project's own
// default branch when it is set, and otherwise the repository's.
func (s *Service) resolve(ctx context.Context, project protocol.Project) (github.Repository, string, error) {
	info, err := s.git.Inspect(ctx, project.Path)
	if err != nil {
		return github.Repository{}, "", fmt.Errorf("read the remotes of project %s: %w", project.ID, err)
	}
	repo, ok := repositoryFromRemotes(info.Remotes)
	if !ok {
		return github.Repository{}, "", protocol.Refused(
			"This project's origin remote is not a GitHub repository, so Marshal cannot open a pull request.").
			With("projectId", project.ID)
	}
	base := project.DefaultBranch
	if strings.TrimSpace(base) == "" {
		base = info.DefaultBranch
	}
	if strings.TrimSpace(base) == "" {
		return github.Repository{}, "", protocol.Refused("This project has no default branch to merge into.").
			With("projectId", project.ID)
	}
	return repo, base, nil
}

// translate turns a forge failure into a plain sentence for the person who asked. A bad token is
// named as a sign-in problem, a missing repository is named as such, and a refusal carries the
// forge's own message so the reason is not hidden.
func translate(err error, cardID string) error {
	var api *github.APIError
	if errors.As(err, &api) {
		switch {
		case api.Unauthorized():
			return protocol.Refused("GitHub refused the saved token. Sign in again in Settings.").
				With("cardId", cardID).With("reason", "github_unauthorized")
		case api.NotFound():
			return protocol.Refused("GitHub could not find the repository or the branch for this card.").
				With("cardId", cardID).With("reason", "github_not_found")
		default:
			return protocol.Refused("GitHub refused to open the pull request: "+api.Message).
				With("cardId", cardID).With("reason", "github_refused")
		}
	}
	return fmt.Errorf("open a pull request for card %s: %w", cardID, err)
}

// repositoryFromRemotes is the origin remote's owner and name, or false when there is no GitHub
// origin. Only the origin remote is read: a fork or a mirror under another name must not be
// mistaken for the place a card's pull request belongs.
func repositoryFromRemotes(remotes []gitx.Remote) (github.Repository, bool) {
	for _, remote := range remotes {
		if remote.Name != "origin" {
			continue
		}
		if repo, ok := repositoryFromURL(remote.URL); ok {
			return repo, true
		}
	}
	return github.Repository{}, false
}

// repositoryFromURL reads owner and name out of a GitHub address. The parsing is the forge
// package's, so the pull-request service and the review that follows it always agree about which
// repository a card's work belongs to. Any other host is not this service's business.
func repositoryFromURL(raw string) (github.Repository, bool) {
	return github.RepositoryFromURL(raw)
}
