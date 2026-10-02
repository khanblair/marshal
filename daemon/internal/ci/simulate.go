package ci

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/khanblair/marshal/daemon/internal/audit"
	gh "github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Simulate CI failure (docs/backend-checklist.md B6.4, build-plan 6.10, N28, decision D5). It has two
// modes, and both end in the same place: a failed run on a card's branch that the fix loop of
// section 9 answers exactly as it answers one GitHub reported.
//
//   - The synthetic mode makes up a run, injects it through `applyRun` with a synthetic forge, and
//     never touches a forge. It is the mode every test uses, and the mode a developer on a machine
//     with no GitHub App connected can use.
//   - The real mode pushes one marked commit to the card's own branch, so the forge's Actions really
//     run and the failure comes back as an ordinary delivery. It needs the App connected, it uses
//     Actions minutes, and Marshal never runs it without a person asking for it.
//
// Both write one audit row and both are registered only on a dev daemon (the route table's
// needsDevMode), so a normal install has no address for either.

// Synthetic mode's own names. The workflow is Marshal's, not a project's: a run the daemon made up
// did not come from any workflow a person wrote, and saying it came from "ci" would put a run in a
// project's own workflow row that the project never ran.
const (
	syntheticWorkflow = "simulated-ci"
	// syntheticRunBase is where Marshal's made-up run ids start, counting up from there. A forge's
	// own run ids are a counter that is nowhere near nine quintillion, so a run id from here cannot
	// collide with one a forge reports and a row Marshal made up stays recognizable as its own.
	syntheticRunBase = int64(9_000_000_000_000_000_000)
)

// The real mode's mark: one new workflow file whose only job is to fail, added to the card's branch
// and committed. It is additive on purpose - the project's own workflows, including its deploy
// workflows (which section 9 never reruns), are not touched - and it is one commit, so the whole
// change is visible in `git log` and is undone by reverting it.
const (
	simulatedWorkflowPath = ".github/workflows/marshal-ci-simulate.yml"
	simulatedWorkflowYAML = `# Written by Marshal's "Simulate CI failure" (dev mode). Delete this file to undo it.
name: marshal-ci-simulate
on: push
jobs:
  simulate:
    runs-on: ubuntu-latest
    steps:
      - name: Fail on purpose
        run: |
          echo "Marshal simulated this failure on purpose."
          exit 1
`
	// simulatedCommitMessage is the commit's own marker, written so the commit is recognizable in a
	// log long after the run that followed it is gone.
	simulatedCommitMessage = "Marshal: simulate a CI failure for %s"
)

// The sentences for what a person can meet when they ask for a simulated failure.
const (
	messageBadSimulateMode = "That is not a mode a simulated CI failure knows. " +
		"Ask for the synthetic mode or the real one."
	messageNoBranchForFailure = "This card has not been started, so it has no branch for a run to fail on. " +
		"Start the card first."
	messageNoSimulateRepository = "Marshal cannot tell which repository this card's project is, " +
		"so it cannot make a run fail for it. Check that the project still points at its folder."
	messageSimulateNoForge = "The real mode needs the GitHub App connected. " +
		"Connect GitHub in Settings, then try again."
	messageSimulateNoWorktree = "Marshal cannot find this card's worktree, " +
		"so it has nowhere to make the marked commit. Start the card first."
	messageSimulateNoRemote = "This project's worktree has no remote to push to, " +
		"so Marshal cannot make a run fail on the forge. Check the project's repository."
	messageSimulateNoGit = "This daemon was started without Git, so it cannot push a marked commit. " +
		"Start Marshal the way you normally do and try again."
	messageSimulatePushFailed = "Marshal made the marked commit but could not push it to this card's branch. " +
		"Check that the project's remote can be reached, then try again."
)

// Simulate runs one mode of a simulated CI failure for one card and answers what happened.
//
// A card that has no branch is refused before either mode runs: a run on no branch is not a failure
// anyone could look at, and the real mode would have nothing to commit to. Everything else that goes
// wrong is one mode's own business and is answered by that mode.
func (s *Service) Simulate(ctx context.Context, cardID string, mode protocol.SimulateMode) (protocol.SimulateCIFailureResult, error) {
	if !mode.Valid() {
		return protocol.SimulateCIFailureResult{}, protocol.InvalidArgument(messageBadSimulateMode)
	}
	card, err := s.cards.Card(ctx, cardID)
	if err != nil {
		return protocol.SimulateCIFailureResult{}, err
	}
	if strings.TrimSpace(card.Branch) == "" {
		return protocol.SimulateCIFailureResult{}, protocol.Refused(messageNoBranchForFailure).With("cardId", card.ID)
	}
	if mode == protocol.SimulateModeReal {
		return s.simulateReal(ctx, card)
	}
	return s.simulateSynthetic(ctx, card)
}

// simulateSynthetic makes up a failed run for a card's branch and injects it through the monitor's
// own entry point, twice: once as the first failure, which asks the forge to rerun the failed jobs,
// and once as the second failure of the same run, which fetches the failed step's log, trims it, and
// sends it to the card's session, or stops the card at its loop limit. That is the whole of section
// 9, and the two injections are how a real failure arrives.
func (s *Service) simulateSynthetic(ctx context.Context, card protocol.Card) (protocol.SimulateCIFailureResult, error) {
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return protocol.SimulateCIFailureResult{}, err
	}
	repo, ok := s.repository(ctx, project)
	if !ok {
		return protocol.SimulateCIFailureResult{}, protocol.Refused(messageNoSimulateRepository).With("cardId", card.ID)
	}
	forge := &syntheticForge{runID: s.nextSyntheticRunID(), log: syntheticLog(card)}
	record := runRecord{
		ID:       forge.runID,
		Branch:   card.Branch,
		Workflow: syntheticWorkflow,
		Status:   protocol.CIStateFailed,
		URL:      "",
		// The run is finished the moment it is made, so it has started: a queued run would leave
		// the card's badge saying nothing happened.
		StartedAt: s.now().UTC(),
		Repo:      repo,
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := s.applyRun(ctx, record, forge); err != nil {
			return protocol.SimulateCIFailureResult{}, err
		}
	}
	row, err := s.runFor(ctx, project.ID, card.Branch, syntheticWorkflow)
	if err != nil {
		return protocol.SimulateCIFailureResult{}, err
	}
	s.auditSimulation(ctx, card, protocol.SimulateModeSynthetic, "")
	return protocol.SimulateCIFailureResult{
		CardID:     card.ID,
		Mode:       protocol.SimulateModeSynthetic,
		Run:        runToWire(row),
		FixStarted: forge.handedOver(s.worker != nil),
	}, nil
}

// simulateReal marks the card's branch and pushes it, so the forge's Actions really run.
//
// It answers with the run Marshal is waiting for rather than a run it holds: the marked commit is
// on its way, the forge has not made a run for it yet, and the failure arrives later as an ordinary
// delivery (the same delivery path a person's own failing push takes). The run in the answer is
// therefore queued, with no id and no address, which is exactly what Marshal knows at this moment.
func (s *Service) simulateReal(ctx context.Context, card protocol.Card) (protocol.SimulateCIFailureResult, error) {
	if s.forge == nil {
		return protocol.SimulateCIFailureResult{}, protocol.Refused(messageSimulateNoForge).With("cardId", card.ID)
	}
	if s.git == nil {
		return protocol.SimulateCIFailureResult{}, protocol.Unsupported(messageSimulateNoGit)
	}
	project, err := s.projects.Get(ctx, card.ProjectID)
	if err != nil {
		return protocol.SimulateCIFailureResult{}, err
	}
	if _, ok := s.repository(ctx, project); !ok {
		return protocol.SimulateCIFailureResult{}, protocol.Refused(messageNoSimulateRepository).With("cardId", card.ID)
	}
	worktree, _, err := s.projects.Worktree(ctx, card.ID)
	if err != nil {
		return protocol.SimulateCIFailureResult{}, err
	}
	if strings.TrimSpace(worktree) == "" {
		return protocol.SimulateCIFailureResult{}, protocol.Refused(messageSimulateNoWorktree).With("cardId", card.ID)
	}
	info, err := s.git.Inspect(ctx, worktree)
	if err != nil {
		return protocol.SimulateCIFailureResult{}, protocol.Unavailable(messageSimulateNoWorktree).WithCause(err)
	}
	commit, err := s.git.CommitFile(ctx, worktree, simulatedWorkflowPath, simulatedWorkflowYAML,
		fmt.Sprintf(simulatedCommitMessage, card.ID))
	if err != nil {
		return protocol.SimulateCIFailureResult{}, protocol.Unavailable(messageSimulateNoWorktree).WithCause(err)
	}
	// The audit row is written before the push: the marked commit is already in the card's working
	// folder, and a push that could not go through does not undo it.
	s.auditSimulation(ctx, card, protocol.SimulateModeReal, commit)
	remote := pushRemote(info.Remotes)
	if remote == "" {
		return protocol.SimulateCIFailureResult{}, protocol.Refused(messageSimulateNoRemote).With("cardId", card.ID)
	}
	if err := s.git.Push(ctx, worktree, remote, card.Branch, project.Target()); err != nil {
		s.log.Warn("could not push a simulated failure's marked commit",
			"card_id", card.ID, "branch", card.Branch, "commit", commit, "error", err)
		return protocol.SimulateCIFailureResult{}, protocol.Unavailable(messageSimulatePushFailed).WithCause(err)
	}
	s.log.Info("pushed a simulated CI failure", "card_id", card.ID, "branch", card.Branch,
		"commit", commit, "remote", remote)
	cardID := card.ID
	return protocol.SimulateCIFailureResult{
		CardID: card.ID,
		Mode:   protocol.SimulateModeReal,
		Run: protocol.CiRun{
			ProjectID: project.ID,
			CardID:    cardID,
			Branch:    card.Branch,
			Status:    protocol.CIStateQueued,
			UpdatedAt: protocol.NewTimestamp(s.now().UTC()),
		},
		Commit: commit,
	}, nil
}

// auditSimulation records the one row both modes write. A row that cannot be written is logged and
// not returned: the failure has already been injected, and losing the record of it must not undo
// the injection. Both modes write the same action and differ only in the detail, so an audit read
// answers "who simulated a CI failure on which card, and how" for either.
func (s *Service) auditSimulation(ctx context.Context, card protocol.Card, mode protocol.SimulateMode, commit string) {
	if s.audit == nil {
		return
	}
	detail := map[string]string{"mode": string(mode), "branch": card.Branch}
	if commit != "" {
		detail["commit"] = commit
	}
	s.audit.LogAndForget(ctx, audit.Entry{
		Actor: audit.ActorPerson, Action: audit.ActionCISimulated,
		Target: card.ID, Detail: detail,
	})
}

// runFor reads back the row a run landed on.
func (s *Service) runFor(ctx context.Context, projectID, branch, workflow string) (db.CiRun, error) {
	var row db.CiRun
	err := s.store.Read(ctx, func(q *db.Queries) error {
		got, err := q.GetCiRunFor(ctx, db.GetCiRunForParams{
			ProjectID: projectID, Branch: branch, Workflow: workflow,
		})
		if err != nil {
			return err
		}
		row = got
		return nil
	})
	if err != nil {
		return db.CiRun{}, fmt.Errorf("read back a simulated run: %w", err)
	}
	return row, nil
}

// nextSyntheticRunID hands out the next made-up run id.
func (s *Service) nextSyntheticRunID() int64 {
	return syntheticRunBase + s.syntheticSeq.Add(1)
}

// pushRemote picks the remote the real mode's marked commit is pushed to: origin, which is the
// remote a project's repository is read from in the first place, else the first remote there is.
func pushRemote(remotes []gitx.Remote) string {
	for _, remote := range remotes {
		if remote.Name == "origin" {
			return remote.Name
		}
	}
	for _, remote := range remotes {
		if strings.TrimSpace(remote.Name) != "" {
			return remote.Name
		}
	}
	return ""
}

// syntheticLog is the failed step's log the synthetic mode's run returns. It reads like the end of a
// real step's log, names no test a project really has, and says in its own words that it is made up,
// so a person reading the card's chat is never left thinking their own suite failed.
func syntheticLog(card protocol.Card) string {
	return strings.Join([]string{
		"Run go test ./...",
		"--- FAIL: TestSimulatedFailure (0.03s)",
		fmt.Sprintf("    simulated_test.go:41: this failure was made up by Marshal for card %s", card.ID),
		"FAIL",
		"FAIL\t./...\t0.412s",
		"Process completed with exit code 1.",
	}, "\n")
}

// syntheticForge stands in for the forge when a failure is simulated. It never opens a socket and
// never reads a key: it answers the rerun with success, answers the failed step's log with the log
// Marshal wrote, and remembers what it was asked, which is how the answer says whether the failure
// reached the loop.
type syntheticForge struct {
	runID int64
	log   string

	mu      sync.Mutex
	reran   bool
	readLog bool
}

// ListWorkflowRuns answers nothing. The polling backup is what reads a branch, and the synthetic
// run is already finished: a forge that answered runs here would be inventing state the backup
// would then record as if GitHub had sent it.
func (f *syntheticForge) ListWorkflowRuns(context.Context, gh.Repository, string) ([]gh.WorkflowRun, error) {
	return nil, nil
}

// RerunFailedJobs records that the loop asked for the rerun it is supposed to ask for once.
func (f *syntheticForge) RerunFailedJobs(_ context.Context, _ gh.Repository, runID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reran = f.reran || runID == f.runID
	return nil
}

// FailedLog records that the loop asked for the failed step's log and answers the made-up one.
func (f *syntheticForge) FailedLog(_ context.Context, _ gh.Repository, _ int64, _ int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readLog = true
	return f.log, nil
}

// handedOver says whether the failure reached the fix loop and was handed on: the rerun was asked
// for, the log was fetched, and there was a session to send it to. This is `fixStarted` on the wire.
func (f *syntheticForge) handedOver(hasWorker bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reran && f.readLog && hasWorker
}
