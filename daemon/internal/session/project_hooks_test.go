package session_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

func TestAwakeCardsCountsOnlyTheProjectsOwnLiveSessions(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	first := e.project(t, "small-repo")
	second := e.project(t, "small-repo")
	for _, p := range []protocol.Project{first, first, second} {
		card := e.card(t, p.ID, "A card")
		if _, err := e.mgr.Start(ctx, card.ID); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
	for project, want := range map[string]int{first.ID: 2, second.ID: 1, "nobody": 0} {
		got, err := e.mgr.AwakeCards(ctx, project)
		if err != nil || got != want {
			t.Errorf("AwakeCards(%q) = %d, %v, want %d", project, got, err, want)
		}
	}
}

func TestStopProjectSessionsStopsThatProjectAndNoOther(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	doomed := e.project(t, "small-repo")
	kept := e.project(t, "small-repo")
	doomedCard := e.card(t, doomed.ID, "Doomed")
	keptCard := e.card(t, kept.ID, "Kept")
	for _, id := range []string{doomedCard.ID, keptCard.ID} {
		if _, err := e.mgr.Start(ctx, id); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}

	if err := e.mgr.StopProjectSessions(ctx, doomed.ID); err != nil {
		t.Fatalf("StopProjectSessions: %v", err)
	}

	err := e.mgr.Send(ctx, doomedCard.ID, "anyone there?")
	if !errors.Is(err, session.ErrNoLiveSession) {
		t.Errorf("Send to a stopped project's card = %v, want no live session", err)
	}
	if err := e.mgr.Send(ctx, keptCard.ID, "still there?"); err != nil {
		t.Errorf("Send to the other project's card: %v", err)
	}
	if got, _ := e.mgr.AwakeCards(ctx, doomed.ID); got != 0 {
		t.Errorf("AwakeCards after stopping = %d, want 0", got)
	}
	// Stopping a project with nothing running is not an error.
	if err := e.mgr.StopProjectSessions(ctx, doomed.ID); err != nil {
		t.Errorf("StopProjectSessions with nothing running: %v", err)
	}
}

func TestACardCanBeStartedAgainAfterItsAgentFailedToStart(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	e.agent.startErr = errors.New("the agent program is missing")
	if _, err := e.mgr.Start(ctx, card.ID); err == nil {
		t.Fatal("Start with a failing agent should return an error")
	}
	branches, err := e.git.Branches(ctx, project.Path, "marshal/")
	if err != nil || len(branches) != 0 {
		t.Fatalf("branches after the failed start = %v, %v, want none: the branch is undone with the worktree", branches, err)
	}

	e.agent.startErr = nil
	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("Start again once the agent works: %v", err)
	}
}

// Removing a card's logs deletes the folder its session wrote to, and a card that never had a
// session has nothing to remove.
func TestRemoveCardLogs(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Ship it")

	// A card that was never started has no session row and no folder.
	if err := e.mgr.RemoveCardLogs(ctx, card.ID); err != nil {
		t.Fatalf("RemoveCardLogs for a card with no session: %v", err)
	}

	if _, err := e.mgr.Start(ctx, card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(ctx, card.ID)
	if err != nil {
		t.Fatalf("read the session: %v", err)
	}
	dir := filepath.Join(e.dataDir, "logs", "sessions", row.ID)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the session log folder was not made: %v", err)
	}
	if err := e.mgr.StopCardSession(ctx, card.ID); err != nil {
		t.Fatalf("StopCardSession: %v", err)
	}
	if err := e.mgr.RemoveCardLogs(ctx, card.ID); err != nil {
		t.Fatalf("RemoveCardLogs: %v", err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("the session log folder is still there")
	}
	// Removing it again is fine: the folder is already gone.
	if err := e.mgr.RemoveCardLogs(ctx, card.ID); err != nil {
		t.Errorf("RemoveCardLogs twice: %v", err)
	}
}

// Stopping a card that has no live session is not an error, because a card is often deleted after
// its session already ended.
func TestStopCardSessionWithNothingRunning(t *testing.T) {
	e := newEnv(t)
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Nothing running")
	if err := e.mgr.StopCardSession(context.Background(), card.ID); err != nil {
		t.Errorf("StopCardSession with no live session: %v", err)
	}
}

// A fork's worktree starts from the branch of the card it came from, not from the project's
// default branch. The test advances the default branch after the source card started, so the two
// commits are different and the assertion can tell which base was used.
func TestAForkStartsFromItsSourceCommit(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	ctx := context.Background()
	project := e.project(t, "small-repo")
	source := e.card(t, project.ID, "Source")
	if _, err := e.mgr.Start(ctx, source.ID); err != nil {
		t.Fatalf("start the source: %v", err)
	}
	sourceRow, err := e.store.Queries().GetCard(ctx, source.ID)
	if err != nil {
		t.Fatalf("read the source card: %v", err)
	}
	sourceCommit := headOf(t, e, sourceRow.WorktreePath)

	// Advance the default branch in the repository folder, so a fork that used it would land on a
	// different commit than the one the source card started from.
	writeFile(t, filepath.Join(project.Path, "later.txt"), "later\n")
	for _, args := range [][]string{
		{"add", "-A"},
		{"-c", "user.name=Test", "-c", "user.email=test@example.test", "commit", "-m", "advance the default branch"},
	} {
		if _, err := e.git.Run(ctx, project.Path, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	defaultCommit := headOf(t, e, project.Path)
	if defaultCommit == sourceCommit {
		t.Fatal("the default branch did not move, so the test cannot tell the two bases apart")
	}

	fork, err := e.proj.ForkCard(ctx, source.ID)
	if err != nil {
		t.Fatalf("ForkCard: %v", err)
	}
	if _, err := e.mgr.Start(ctx, fork.ID); err != nil {
		t.Fatalf("start the fork: %v", err)
	}
	forkRow, err := e.store.Queries().GetCard(ctx, fork.ID)
	if err != nil {
		t.Fatalf("read the fork: %v", err)
	}
	if got := headOf(t, e, forkRow.WorktreePath); got != sourceCommit {
		t.Errorf("the fork's worktree is at %s, want the source card's commit %s", got, sourceCommit)
	}
	if got := headOf(t, e, forkRow.WorktreePath); got == defaultCommit {
		t.Errorf("the fork started from the default branch (%s) instead of its source", defaultCommit)
	}
}

// headOf is the commit a worktree or repository folder is on.
func headOf(t *testing.T, e *env, dir string) string {
	t.Helper()
	out, err := e.git.Run(context.Background(), dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD in %s: %v", dir, err)
	}
	return strings.TrimSpace(out)
}

// writeFile writes a small file for a test that needs a commit.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// A seeded session is a row the screens can draw, with no process behind it: the manager does not
// know it as live, so nothing is started and nothing is restored for it.
func TestSeedSessionWritesARowWithoutStartingAnything(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Seeded")

	if err := e.mgr.SeedSession(ctx, card.ID, protocol.SessionStateWorking); err != nil {
		t.Fatalf("SeedSession: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(ctx, card.ID)
	if err != nil {
		t.Fatalf("read the session: %v", err)
	}
	if row.State != string(protocol.SessionStateWorking) || row.AgentSessionID != "" {
		t.Errorf("session = %+v, want working with no agent session id", row)
	}
	// Nothing is live, so the card does not count as awake in this process.
	awake, err := e.mgr.AwakeCards(ctx, project.ID)
	if err != nil || awake != 0 {
		t.Errorf("AwakeCards = %d, %v; want 0: a seeded session has no process", awake, err)
	}
	// A seeded row is not resumed on the next start: it is not one of the states RestoreAll takes.
	rows, err := e.store.Queries().ListResumableSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// (The states RestoreAll resumes are starting, awake, and working. A working row IS resumable,
	// so the daemon must not restore when a fixture is loaded: cmd/marshald skips it. This test
	// records that the row is resumable, which is why that skip exists.)
	if len(rows) != 1 {
		t.Errorf("resumable sessions = %d, want the one seeded row", len(rows))
	}

	// Seeding again leaves the row alone: a card has one session for its whole life.
	before := row.ID
	if err := e.mgr.SeedSession(ctx, card.ID, protocol.SessionStateAwake); err != nil {
		t.Fatalf("SeedSession twice: %v", err)
	}
	after, err := e.store.Queries().GetSessionByCard(ctx, card.ID)
	if err != nil || after.ID != before || after.State != row.State {
		t.Errorf("session after a second seed = %+v, %v; want it unchanged", after, err)
	}
}

// Seeding refuses a state Marshal does not have, and a card that is not there.
func TestSeedSessionRefusals(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Seeded")
	if err := e.mgr.SeedSession(ctx, card.ID, protocol.SessionState("dozing")); err == nil {
		t.Error("SeedSession accepted a state Marshal does not have")
	}
	if err := e.mgr.SeedSession(ctx, "01M3C107JB041061050R3GG28A", protocol.SessionStateAwake); err == nil {
		t.Error("SeedSession accepted a card that is not there")
	}
}
