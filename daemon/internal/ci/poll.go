package ci

import (
	"context"
	"fmt"
	"strings"
	"time"

	gh "github.com/khanblair/marshal/daemon/internal/github"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The polling backup (docs/architecture.md section 9, step 1: "a webhook or poll"). The webhook is
// the fast path, and the backup exists for the two ways it can fail to tell Marshal anything: a
// delivery that was sent and lost, and a delivery GitHub never sent at all - a run finished by a
// person in the browser whose event Marshal is not configured for. Both look the same from here,
// which is why the backup asks about the runs Marshal is *waiting* on rather than about runs in
// general.
//
// It is deliberately narrow. It asks only about runs whose state is queued or running and which have
// not changed for a while: a small set at any moment, which is what keeps a five-minute tick from
// being expensive on a repository that runs CI all day. It then writes what it learns through
// `applyRun`, the same path a delivery takes, so the fix loop, the card's badge, and the two
// published events are the same whichever way the state arrived.
//
// A branch Marshal asks about and gets nothing for is remembered for a while, so a project whose
// branch has no CI at all is not asked about on every tick.

const (
	// staleAfter is how long a run Marshal is waiting on may go without a word before the backup
	// asks about it. It is longer than a normal run takes to start, so the backup does not
	// duplicate a delivery that is merely a little slow.
	staleAfter = 10 * time.Minute
	// branchMissTTL is how long "this branch has no runs" is remembered, so a branch with no CI is
	// asked about rarely rather than every tick.
	branchMissTTL = 10 * time.Minute
	// branchMissLimit bounds how many branches are remembered, so a project with very many card
	// branches cannot grow the map without end. It is far more than a project has cards moving at
	// once.
	branchMissLimit = 512
)

// Start begins the backup's ticker, using ctx for the sweeps it runs. It is safe to call once, and
// a service whose interval is zero or which has no forge starts nothing, because there is nothing to
// ask.
func (s *Service) Start(ctx context.Context) {
	s.tickerMu.Lock()
	defer s.tickerMu.Unlock()
	if s.ticker != nil || s.pollEvery <= 0 || s.forge == nil {
		return
	}
	ticker := time.NewTicker(s.pollEvery)
	done := make(chan struct{})
	s.ticker, s.tickerDone = ticker, done
	go s.pollLoop(ctx, ticker, done)
	s.log.Info("the CI polling backup is running", "every", s.pollEvery)
}

// Stop ends the backup's ticker. It is safe to call more than once, and on a service that never
// started.
func (s *Service) Stop() {
	s.tickerMu.Lock()
	ticker, done := s.ticker, s.tickerDone
	s.ticker, s.tickerDone = nil, nil
	s.tickerMu.Unlock()
	if ticker == nil {
		return
	}
	ticker.Stop()
	close(done)
}

// pollLoop runs one sweep per tick until the ticker is stopped.
func (s *Service) pollLoop(ctx context.Context, ticker *time.Ticker, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := s.Poll(ctx); err != nil {
				s.log.Warn("the CI polling backup could not run", "error", err)
			}
			if err := s.PollDefaultBranches(ctx); err != nil {
				s.log.Warn("the CI check of the default branches could not run", "error", err)
			}
		}
	}
}

// Poll runs one sweep: it asks the forge about every run Marshal is still waiting on and records
// what it learns. It is exported so a test drives one sweep without a ticker, and so a future route
// can ask for one now.
//
// The answer is only ever the sweep's own failure to read the store. A branch the forge could not be
// asked about is logged and skipped: one unreachable branch must not stop the backup from catching
// up on the others.
func (s *Service) Poll(ctx context.Context) error {
	if s.forge == nil {
		return ErrNoForge
	}
	cutoff := s.now().Add(-staleAfter).UnixMilli()
	var waiting []db.CiRun
	if err := s.store.Read(ctx, func(q *db.Queries) error {
		rows, err := q.ListCiRunsWaiting(ctx, cutoff)
		if err != nil {
			return err
		}
		waiting = rows
		return nil
	}); err != nil {
		return fmt.Errorf("read the runs Marshal is waiting on: %w", err)
	}
	if len(waiting) == 0 {
		return nil
	}
	for _, group := range groupForPoll(waiting) {
		s.pollBranch(ctx, group)
	}
	return nil
}

// pollGroup is the runs of one branch of one project: what one question to the forge answers.
type pollGroup struct {
	projectID string
	branch    string
	rows      []db.CiRun
}

// groupForPoll gathers the waiting runs by project and branch, so one forge call serves every
// workflow waiting on the same branch. The order is the query's own, so a sweep is deterministic.
func groupForPoll(rows []db.CiRun) []pollGroup {
	var out []pollGroup
	index := map[string]int{}
	for _, row := range rows {
		key := row.ProjectID + "\x00" + row.Branch
		i, ok := index[key]
		if !ok {
			out = append(out, pollGroup{projectID: row.ProjectID, branch: row.Branch})
			i = len(out) - 1
			index[key] = i
		}
		out[i].rows = append(out[i].rows, row)
	}
	return out
}

// pollBranch asks the forge for one branch's newest runs and applies the state of every run Marshal
// is waiting on. A run the forge no longer lists is left alone: the forge's run list is bounded and
// paged, and a run that fell off the end is not evidence that it stopped.
func (s *Service) pollBranch(ctx context.Context, group pollGroup) {
	project, err := s.projects.Get(ctx, group.projectID)
	if err != nil {
		s.log.Warn("the CI polling backup could not read a project",
			"project_id", group.projectID, "error", err)
		return
	}
	repo, ok := s.repository(ctx, project)
	if !ok {
		return
	}
	if s.branchMissed(repo, group.branch) {
		return
	}
	runs, err := s.forge.ListWorkflowRuns(ctx, repo, group.branch)
	if err != nil {
		s.log.Warn("the CI polling backup could not ask the forge about a branch",
			"repository", repo.String(), "branch", group.branch, "error", err)
		return
	}
	found := false
	for _, waiting := range group.rows {
		for _, run := range runs {
			if fmt.Sprint(run.ID) != waiting.ID {
				continue
			}
			found = true
			record, err := recordFromRun(repo, run)
			if err != nil {
				s.log.Warn("a polled run could not be read", "repository", repo.String(), "error", err)
				continue
			}
			if err := s.applyRun(ctx, record, s.forge); err != nil {
				s.log.Warn("a polled run could not be recorded", "run", run.ID, "error", err)
			}
		}
	}
	s.noteBranch(repo, group.branch, found)
}

// branchMissed reports whether this branch is remembered as having no runs, so a sweep skips the
// forge call. The answer is about the moment it was learned, not for good.
func (s *Service) branchMissed(repo gh.Repository, branch string) bool {
	s.missMu.Lock()
	defer s.missMu.Unlock()
	key := branchKey(repo, branch)
	miss, ok := s.branchMisses[key]
	if !ok {
		return false
	}
	if s.now().Sub(miss) >= branchMissTTL {
		delete(s.branchMisses, key)
		return false
	}
	return true
}

// noteBranch remembers a branch the forge listed nothing for, and forgets one it did.
func (s *Service) noteBranch(repo gh.Repository, branch string, found bool) {
	s.missMu.Lock()
	defer s.missMu.Unlock()
	key := branchKey(repo, branch)
	if found {
		delete(s.branchMisses, key)
		return
	}
	if len(s.branchMisses) >= branchMissLimit {
		// The map is full of stale answers; forget them all rather than let it grow. A branch is
		// asked about again, which is the safe direction to fail in.
		s.branchMisses = map[string]time.Time{}
	}
	s.branchMisses[key] = s.now()
}

// branchKey names one branch of one repository for the miss cache.
func branchKey(repo gh.Repository, branch string) string {
	return repo.Owner + "/" + repo.Name + "#" + branch
}

// defaultBranchEvery is how often one project's default branch is asked about. The waiting-run
// sweep above only follows runs a webhook or a card's branch already told Marshal about; this is what
// finds the runs on main nobody announced, so CI health has them without a public webhook address.
const defaultBranchEvery = 5 * time.Minute

// PollDefaultBranches asks the forge for each project's default branch and records its newest run of
// every workflow, through the same path a delivery takes. A project that is not a GitHub repository,
// or was asked about lately, is skipped. A branch the forge could not be asked about is logged and
// left: one project's trouble does not stop the others.
func (s *Service) PollDefaultBranches(ctx context.Context) error {
	if s.forge == nil {
		return ErrNoForge
	}
	list, err := s.projects.List(ctx)
	if err != nil {
		return fmt.Errorf("list the projects to ask about their default branches: %w", err)
	}
	for _, project := range list.Projects {
		s.pollDefaultBranch(ctx, project)
	}
	return nil
}

func (s *Service) pollDefaultBranch(ctx context.Context, project protocol.Project) {
	branch := strings.TrimSpace(project.DefaultBranch)
	if branch == "" || !s.defaultDue(project.ID) {
		return
	}
	repo, ok := s.repository(ctx, project)
	if !ok {
		return
	}
	runs, err := s.forge.ListWorkflowRuns(ctx, repo, branch)
	if err != nil {
		s.log.Warn("the CI check could not ask the forge about a default branch",
			"repository", repo.String(), "branch", branch, "error", err)
		return
	}
	seen := map[string]bool{}
	for _, run := range runs {
		// The list is newest first, so the first run of a workflow is the one that counts.
		if seen[run.Name] {
			continue
		}
		seen[run.Name] = true
		record, err := recordFromRun(repo, run)
		if err != nil {
			s.log.Warn("a default-branch run could not be read", "repository", repo.String(), "error", err)
			continue
		}
		if err := s.applyRun(ctx, record, s.forge); err != nil {
			s.log.Warn("a default-branch run could not be recorded", "run", run.ID, "error", err)
		}
	}
}

// defaultDue says whether a project's default branch is due to be asked about, and notes that it was.
func (s *Service) defaultDue(projectID string) bool {
	s.missMu.Lock()
	defer s.missMu.Unlock()
	if last, ok := s.defaultAsked[projectID]; ok && s.now().Sub(last) < defaultBranchEvery {
		return false
	}
	s.defaultAsked[projectID] = s.now()
	return true
}
