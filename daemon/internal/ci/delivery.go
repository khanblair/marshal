package ci

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	gh "github.com/khanblair/marshal/daemon/internal/github"
	githubapp "github.com/khanblair/marshal/daemon/internal/integrations/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// This file turns what arrives into a run. Two things arrive: a verified GitHub delivery, and the
// answer to the polling backup's question. Both become a runRecord, and everything after that is
// the same code, which is the whole reason the backup exists as a backup rather than as a second
// implementation of the loop.

// kindWorkflowRun is the delivery kind that carries a run's own state. It is the one the App is set
// up to receive for CI; a `check_run` delivery is a single job's state, which is not a workflow's,
// so it is not read here.
const kindWorkflowRun = "workflow_run"

// runRecord is one workflow run, however Marshal heard about it: from GitHub's delivery or from its
// own question about a branch.
type runRecord struct {
	// ID is the forge's numeric run id.
	ID int64
	// Branch is the branch the run is on.
	Branch string
	// Workflow is the workflow's name as the forge words it.
	Workflow string
	// Status is where the run is, mapped to the wire's five states.
	Status protocol.CIState
	// URL is the run's address on the web.
	URL string
	// StartedAt is when the run started, zero while it is queued.
	StartedAt time.Time
	// Repo is the repository the run is in.
	Repo gh.Repository
}

// Delivery is the sink the connections service hands verified GitHub deliveries to
// (githubapp.Sink). A delivery of a kind nothing in Marshal handles is accepted and ignored: the
// signature already proved it came from GitHub, and answering an error would make GitHub redeliver
// a delivery there is nothing wrong with.
func (s *Service) Delivery(ctx context.Context, event githubapp.Event) error {
	if event.Kind != kindWorkflowRun {
		return nil
	}
	record, err := parseWorkflowRun(event.Body)
	if err != nil {
		// A delivery Marshal cannot read is logged and accepted: the body came from GitHub, and a
		// redelivery of the same unreadable body would fail the same way.
		s.log.Warn("a CI delivery could not be read", "delivery", event.Delivery, "error", err)
		return nil
	}
	if err := s.apply(ctx, record); err != nil {
		s.log.Warn("a CI run could not be recorded", "delivery", event.Delivery, "error", err)
		return err
	}
	return nil
}

// workflowRunPayload is the part of a `workflow_run` delivery Marshal reads. GitHub sends much
// more; only these fields decide anything.
type workflowRunPayload struct {
	Action string `json:"action"`
	Run    struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		Branch     string `json:"head_branch"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
		URL        string `json:"html_url"`
		StartedAt  string `json:"run_started_at"`
	} `json:"workflow_run"`
	Repository struct {
		FullName string `json:"full_name"`
		Name     string `json:"name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

// parseWorkflowRun reads a delivery's body into a run.
func parseWorkflowRun(body []byte) (runRecord, error) {
	var payload workflowRunPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return runRecord{}, fmt.Errorf("read a workflow_run delivery: %w", err)
	}
	if payload.Run.ID <= 0 {
		return runRecord{}, errors.New("a workflow_run delivery has no run id")
	}
	repo, err := repositoryOf(payload.Repository.Owner.Login, payload.Repository.Name, payload.Repository.FullName)
	if err != nil {
		return runRecord{}, err
	}
	branch := strings.TrimSpace(payload.Run.Branch)
	if branch == "" {
		return runRecord{}, errors.New("a workflow_run delivery names no branch")
	}
	record := runRecord{
		ID:       payload.Run.ID,
		Branch:   branch,
		Workflow: workflowName(payload.Run.Name, branch),
		Status:   runState(payload.Run.Status, payload.Run.Conclusion),
		URL:      payload.Run.URL,
		Repo:     repo,
	}
	if started, ok := parseTime(payload.Run.StartedAt); ok {
		record.StartedAt = started
	}
	return record, nil
}

// recordFromRun turns the forge's own answer to the polling backup's question into a run.
func recordFromRun(repo gh.Repository, run gh.WorkflowRun) (runRecord, error) {
	if run.ID <= 0 {
		return runRecord{}, errors.New("a workflow run has no id")
	}
	branch := strings.TrimSpace(run.Branch)
	if branch == "" {
		return runRecord{}, errors.New("a workflow run names no branch")
	}
	record := runRecord{
		ID:       run.ID,
		Branch:   branch,
		Workflow: workflowName(run.Name, branch),
		Status:   runState(run.Status, run.Conclusion),
		URL:      run.URL,
		Repo:     repo,
	}
	if started, ok := parseTime(run.StartedAt); ok {
		record.StartedAt = started
	}
	return record, nil
}

// repositoryOf builds a repository from a delivery's own fields, falling back to the full name when
// the owner and name are not given separately.
func repositoryOf(owner, name, fullName string) (gh.Repository, error) {
	owner, name = strings.TrimSpace(owner), strings.TrimSpace(name)
	if owner == "" || name == "" {
		parts := strings.SplitN(strings.TrimSpace(fullName), "/", 2)
		if len(parts) != 2 {
			return gh.Repository{}, errors.New("a delivery names no repository")
		}
		owner, name = parts[0], parts[1]
	}
	repo := gh.Repository{Owner: owner, Name: name}
	if !repo.Valid() {
		return gh.Repository{}, errors.New("a delivery names no repository")
	}
	return repo, nil
}

// workflowName names a workflow, keeping the branch out of the name. GitHub sends an empty name for
// a run it cannot name, and a screen needs something to draw, so an unnamed run is named after its
// own run id rather than left blank.
func workflowName(name, branch string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	if branch != "" {
		return "run on " + branch
	}
	return "run"
}

// runState maps the forge's status and conclusion onto the wire's five states.
//
// Two of the forge's conclusions are not a pass and not a failure: `skipped` is a run that did not
// run, and `neutral` is one that decided nothing. They are recorded as `cancelled`, which is the
// wire's word for "did not produce a result", because section 17's rule is that a check which did
// not run must never be read as green.
func runState(status, conclusion string) protocol.CIState {
	switch strings.TrimSpace(status) {
	case "queued", "requested", "waiting", "pending":
		return protocol.CIStateQueued
	case "in_progress":
		return protocol.CIStateRunning
	case "completed":
		switch strings.TrimSpace(conclusion) {
		case "success":
			return protocol.CIStatePassed
		case "failure", "timed_out", "action_required", "startup_failure":
			return protocol.CIStateFailed
		default:
			return protocol.CIStateCancelled
		}
	default:
		return protocol.CIStateQueued
	}
}

// parseTime reads an RFC 3339 time. A time that is absent or unreadable is not an error: a queued
// run has no start time, and a run whose time cannot be read is still a run.
func parseTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

// apply records a run and, when it failed on a card's branch, runs the fix loop. It is the one path
// every source of a run goes through: a verified delivery and the polling backup both call it with
// the forge the daemon is connected to, and a simulated failure calls it with the synthetic forge
// slice 3 builds, so the loop the loop limits bound is the same one in every case.
func (s *Service) apply(ctx context.Context, record runRecord) error {
	return s.applyRun(ctx, record, s.forge)
}

// applyRun records a run against the forge it came from, tells the screens, and runs the fix loop
// when it failed.
func (s *Service) applyRun(ctx context.Context, record runRecord, forge Forge) error {
	project, err := s.projectFor(ctx, record.Repo)
	if err != nil {
		if errors.Is(err, ErrNoProject) {
			s.log.Debug("a CI run is not in any project Marshal manages",
				"repository", record.Repo.String(), "branch", record.Branch)
			return nil
		}
		return err
	}
	card, err := s.cardForBranch(ctx, project.ID, record.Branch)
	if err != nil {
		return err
	}
	cardID := ""
	if card != nil {
		cardID = card.ID
	}
	row, newFailure, err := s.record(ctx, project.ID, cardID, record)
	if err != nil {
		return err
	}
	if card != nil {
		s.setCardCI(ctx, *card, row)
	}
	s.publishProject(ctx, project.ID)
	if newFailure && s.failures != nil {
		failure := Failure{ProjectID: project.ID, Branch: record.Branch, Workflow: record.Workflow}
		if card != nil {
			failure.CardID, failure.CardKey, failure.CardTitle = card.ID, card.Key, card.Title
		}
		s.failures.CIFailed(ctx, failure)
	}
	if row.Status != string(protocol.CIStateFailed) || card == nil {
		return nil
	}
	return s.fix(ctx, project, *card, row, record.Repo, forge)
}

// setCardCI writes the state of the run Marshal just recorded onto the card whose branch it is on.
// The run was recorded this instant, so it is the newest one on that branch and its state is the
// card's; a card with several workflows on one branch therefore shows the newest of them, which is
// what one badge can honestly say. A write that fails is logged and not returned: the run itself is
// recorded, and a badge that could not be written is not worth losing the fix loop over.
func (s *Service) setCardCI(ctx context.Context, card protocol.Card, row db.CiRun) {
	state := protocol.CIState(row.Status)
	if _, err := s.cards.SetCI(ctx, card.ID, &state); err != nil {
		s.log.Warn("could not record a card's CI state", "card_id", card.ID, "error", err)
	}
}

// projectFor resolves the repository a run is in to the project Marshal manages. It answers
// ErrNoProject for a repository no project has, which is the ordinary state of a delivery from
// someone else's repository reaching a public webhook address.
func (s *Service) projectFor(ctx context.Context, repo gh.Repository) (protocol.Project, error) {
	list, err := s.projects.List(ctx)
	if err != nil {
		return protocol.Project{}, err
	}
	for _, project := range list.Projects {
		known, ok := s.repository(ctx, project)
		if ok && known.Owner == repo.Owner && known.Name == repo.Name {
			return project, nil
		}
	}
	return protocol.Project{}, ErrNoProject
}

// cardForBranch answers the card whose branch a run is on, or nil when no card owns it: the default
// branch, or a branch a person pushed for their own reasons. A run with no card is recorded all the
// same, so the CI health page shows it, but nothing is fixed for it.
func (s *Service) cardForBranch(ctx context.Context, projectID, branch string) (*protocol.Card, error) {
	cards, err := s.cards.Cards(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, card := range cards {
		if strings.TrimSpace(card.Branch) != "" && card.Branch == branch {
			found := card
			return &found, nil
		}
	}
	return nil, nil
}

// record writes the run's newest state and answers the row it landed on, and whether the run newly
// failed: it is failed now, and it was not the same failed run when Marshal last looked. A failed run
// delivered again, or seen again by the polling backup, is not a new failure. The upsert keeps
// `rerun_at` for a run that is the same run and starts it over for a new one, so section 9's "rerun
// the failed jobs once" survives a restart and is not repeated for a run that has not changed.
func (s *Service) record(ctx context.Context, projectID, cardID string, record runRecord) (db.CiRun, bool, error) {
	now := s.now().UTC()
	params := db.UpsertCiRunParams{
		ID:        strconv.FormatInt(record.ID, 10),
		ProjectID: projectID,
		CardID:    optionalID(cardID),
		Branch:    record.Branch,
		Workflow:  record.Workflow,
		Status:    string(record.Status),
		Url:       record.URL,
		UpdatedAt: now.UnixMilli(),
	}
	if !record.StartedAt.IsZero() {
		params.StartedAt = record.StartedAt.UnixMilli()
	}
	var row db.CiRun
	newFailure := false
	err := s.store.Write(ctx, func(q *db.Queries) error {
		before, beforeErr := q.GetCiRunFor(ctx, db.GetCiRunForParams{
			ProjectID: projectID, Branch: record.Branch, Workflow: record.Workflow,
		})
		if beforeErr != nil && !errors.Is(beforeErr, sql.ErrNoRows) {
			return beforeErr
		}
		if err := q.UpsertCiRun(ctx, params); err != nil {
			return err
		}
		failed := params.Status == string(protocol.CIStateFailed)
		newFailure = failed && (beforeErr != nil || before.ID != params.ID || before.Status != params.Status)
		got, err := q.GetCiRunFor(ctx, db.GetCiRunForParams{
			ProjectID: projectID, Branch: record.Branch, Workflow: record.Workflow,
		})
		if err != nil {
			return err
		}
		row = got
		return nil
	})
	if err != nil {
		return db.CiRun{}, false, err
	}
	return row, newFailure, nil
}

// optionalID turns an empty id into a null, so a run with no card stores no card rather than an
// empty string that would look like a card named "".
func optionalID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}
