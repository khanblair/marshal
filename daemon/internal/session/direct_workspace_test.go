package session_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

func directMode(c *session.Config) { c.DirectWorkspace = true }

func workspaceLockDir(dir string) func(*session.Config) {
	return func(c *session.Config) { c.WorkspaceLockDir = dir }
}

func TestDirectCardUsesProjectFolderWithoutStagingOrCommittingUserEdits(t *testing.T) {
	e := newEnv(t, directMode)
	t.Cleanup(func() { _ = e.mgr.Close() })
	p := e.project(t, "small-repo")
	ctx := context.Background()
	path := filepath.Join(p.Path, "README.md")
	if err := os.WriteFile(path, []byte("user staged content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.git.Run(ctx, p.Path, "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user working content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeIndex, _ := e.git.Run(ctx, p.Path, "diff", "--cached")
	beforeHead, _ := e.git.Rev(ctx, p.Path, "HEAD")
	c := e.card(t, p.ID, "Direct edit")
	if _, err := e.mgr.Start(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.agent.startSpecs()[0]; got.Cwd != p.Path || !strings.Contains(got.Instructions, "native subagents") {
		t.Fatalf("start spec = %+v", got)
	}
	if err := e.mgr.Send(ctx, c.ID, "read the files"); err != nil {
		t.Fatal(err)
	}
	waitDirectResult(t, e, p.Path, c.ID)
	if err := e.mgr.Stop(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	index, _ := e.git.Run(ctx, p.Path, "diff", "--cached")
	head, _ := e.git.Rev(ctx, p.Path, "HEAD")
	if index != beforeIndex || head != beforeHead {
		t.Fatal("direct card changed the user's staged files or branch commit")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "user working content\n" {
		t.Fatalf("user file changed: %q, %v", data, err)
	}
	worktrees, _ := e.git.Run(ctx, p.Path, "worktree", "list", "--porcelain")
	if strings.Count(worktrees, "worktree ") != 1 {
		t.Fatalf("created extra worktrees: %s", worktrees)
	}
	if _, err := e.mgr.RestoreCheckpoint(ctx, c.ID, "any", false); err == nil {
		t.Fatal("shared-folder destructive restore was allowed")
	}
}

func TestDirectCardsSerializeMainSessionsAndAllowDifferentProjects(t *testing.T) {
	e := newEnv(t, directMode)
	t.Cleanup(func() { _ = e.mgr.Close() })
	p := e.project(t, "small-repo")
	other := e.project(t, "small-repo")
	a, b := e.card(t, p.ID, "First"), e.card(t, p.ID, "Second")
	c := e.card(t, other.ID, "Different project")
	ctx := context.Background()
	if _, err := e.mgr.Start(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Start(ctx, b.ID); err == nil {
		t.Fatal("two independent main sessions edited the project")
	}
	if _, err := e.mgr.Start(ctx, c.ID); err != nil {
		t.Fatalf("different project was blocked: %v", err)
	}
	if err := e.mgr.Stop(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Start(ctx, b.ID); err != nil {
		t.Fatalf("stopped editor kept the project: %v", err)
	}
}

func TestDirectWorkspaceLeaseIsSharedAcrossManagersAndDataDirs(t *testing.T) {
	lockDir := filepath.Join(t.TempDir(), "shared-workspace-locks")
	first := newEnv(t, directMode, workspaceLockDir(lockDir))
	t.Cleanup(func() { _ = first.mgr.Close() })
	project := first.project(t, "small-repo")
	firstCard := first.card(t, project.ID, "First daemon editor")

	second := newEnv(t, directMode, workspaceLockDir(lockDir))
	t.Cleanup(func() { _ = second.mgr.Close() })
	secondProject, err := second.proj.Create(context.Background(), protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceFolder, Path: project.Path,
	})
	if err != nil {
		t.Fatalf("register the same folder in a second data directory: %v", err)
	}
	secondCard := second.card(t, secondProject.ID, "Second daemon editor")

	if _, err := first.mgr.Start(context.Background(), firstCard.ID); err != nil {
		t.Fatalf("start the first manager's editor: %v", err)
	}
	_, err = second.mgr.Start(context.Background(), secondCard.ID)
	perr := wantCode(t, err, protocol.ErrorCodeRefused)
	if perr.Details["reason"] != "project_editor_busy" || perr.Details["owner"] != "another Marshal instance" {
		t.Fatalf("cross-manager refusal details = %#v", perr.Details)
	}

	if err := first.mgr.Stop(context.Background(), firstCard.ID); err != nil {
		t.Fatalf("stop the first manager's editor: %v", err)
	}
	if _, err := second.mgr.Start(context.Background(), secondCard.ID); err != nil {
		t.Fatalf("second manager could not acquire the released project lease: %v", err)
	}
	if err := second.mgr.Stop(context.Background(), secondCard.ID); err != nil {
		t.Fatalf("stop the second manager's editor: %v", err)
	}
	if _, err := first.mgr.Start(context.Background(), firstCard.ID); err != nil {
		t.Fatalf("restart the first manager's editor: %v", err)
	}
	if err := first.mgr.Close(); err != nil {
		t.Fatalf("close the first manager: %v", err)
	}
	if _, err := second.mgr.Start(context.Background(), secondCard.ID); err != nil {
		t.Fatalf("second manager could not acquire the lease after close: %v", err)
	}
	if err := second.mgr.Stop(context.Background(), secondCard.ID); err != nil {
		t.Fatalf("stop the second manager's editor after close: %v", err)
	}
}

func TestDirectPendingStartupAndFailedStartKeepOwnershipCorrect(t *testing.T) {
	e := newEnv(t, directMode)
	t.Cleanup(func() { _ = e.mgr.Close() })
	p := e.project(t, "small-repo")
	a, b := e.card(t, p.ID, "First"), e.card(t, p.ID, "Second")
	e.agent.startHold = make(chan struct{})
	ctx := context.Background()
	done := make(chan error, 1)
	go func() { _, err := e.mgr.Start(ctx, a.ID); done <- err }()
	deadline := time.Now().Add(eventTimeout)
	for {
		if _, err := e.git.Rev(ctx, p.Path, gitx.DirectBaseRef(a.ID)); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first start did not reserve project")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := e.mgr.Start(ctx, b.ID); err == nil {
		t.Fatal("pending startup did not reserve the original folder")
	}
	close(e.agent.startHold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.Stop(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	e.agent.mu.Lock()
	e.agent.startErr = errors.New("not installed")
	e.agent.mu.Unlock()
	if _, err := e.mgr.Start(ctx, b.ID); err == nil {
		t.Fatal("fake failing start succeeded")
	}
	e.agent.mu.Lock()
	e.agent.startErr = nil
	e.agent.mu.Unlock()
	if _, err := e.mgr.Start(ctx, b.ID); err != nil {
		t.Fatalf("failed startup leaked editor ownership: %v", err)
	}
}

func TestDirectCompletionRetainsOwnershipAfterWorkerStops(t *testing.T) {
	e := newEnv(t, directMode)
	t.Cleanup(func() { _ = e.mgr.Close() })
	p := e.project(t, "small-repo")
	a, b := e.card(t, p.ID, "First"), e.card(t, p.ID, "Second")
	ctx := context.Background()
	if _, err := e.mgr.Start(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	release, err := e.mgr.AcquireCompletion(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := e.mgr.Send(ctx, a.ID, "edit during checks"); err == nil {
		t.Fatal("send raced completion checks")
	}
	if err := e.mgr.StopCardSession(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Start(ctx, b.ID); err == nil {
		t.Fatal("checks lost ownership when worker stopped")
	}
	release()
	if _, err := e.mgr.Start(ctx, b.ID); err != nil {
		t.Fatalf("completed lease not released: %v", err)
	}
}

func TestDirectCardsAndCodingChatsShareOwnership(t *testing.T) {
	e := newChatEnv(t, directMode)
	p := e.project(t, "small-repo")
	c := e.card(t, p.ID, "Coding card")
	chat := e.newChat(t, p.ID, protocol.CreateChatRequest{
		AgentKind: protocol.AgentKindClaude, PermissionMode: protocol.PermissionModeAutoEdits,
		Target: &protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: "Worker"},
	})
	ctx := context.Background()
	if _, err := e.mgr.Start(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	reader := e.newChat(t, p.ID, protocol.CreateChatRequest{AgentKind: protocol.AgentKindClaude})
	if err := e.mgr.SendChat(ctx, reader, "plan the next task"); err != nil {
		t.Fatalf("read-only Orchestrator should coexist with the editor: %v", err)
	}
	if specs := e.agent.startSpecs(); !strings.Contains(strings.Join(specs[len(specs)-1].DisallowedTools, " "), "Write") {
		t.Fatal("coexisting Orchestrator was allowed to edit")
	}
	if err := e.mgr.SendChat(ctx, chat, "edit the project"); err == nil {
		t.Fatal("coding chat bypassed card ownership")
	}
	if err := e.mgr.Stop(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.SendChat(ctx, chat, "edit the project"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Start(ctx, c.ID); err == nil {
		t.Fatal("card bypassed coding chat ownership")
	}
	if err := e.mgr.StopChatSession(ctx, chat.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Start(ctx, c.ID); err != nil {
		t.Fatalf("chat shutdown retained project: %v", err)
	}
}

func TestDirectResumePreservesLegacyWorktreeWithoutStartingAnAgent(t *testing.T) {
	e := newEnv(t)
	p := e.project(t, "small-repo")
	c := e.card(t, p.ID, "Old worktree card")
	ctx := context.Background()
	if _, err := e.mgr.Start(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	path, _, err := e.proj.Worktree(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "unfinished.txt"), []byte("keep this"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatal(err)
	}
	fresh := e.reopen(t, directMode)
	t.Cleanup(func() { _ = fresh.Close() })
	before := len(e.agent.startSpecs())
	if _, err := fresh.Start(ctx, c.ID); err == nil || !strings.Contains(err.Error(), "separate worktree") {
		t.Fatalf("legacy resume = %v", err)
	}
	if len(e.agent.startSpecs()) != before {
		t.Fatal("legacy card started an agent")
	}
	if data, err := os.ReadFile(filepath.Join(path, "unfinished.txt")); err != nil || string(data) != "keep this" {
		t.Fatalf("legacy work lost: %q %v", data, err)
	}
}

func waitDirectResult(t *testing.T, e *env, path, id string) {
	t.Helper()
	deadline := time.Now().Add(eventTimeout)
	for time.Now().Before(deadline) {
		if _, err := e.git.Rev(context.Background(), path, gitx.DirectResultRef(id)); err == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the turn did not save its result snapshot")
}

func TestDirectFreshStartPreservesLegacyMetadataWithoutASessionRow(t *testing.T) {
	e := newEnv(t)
	p := e.project(t, "small-repo")
	c := e.card(t, p.ID, "Legacy metadata")
	ctx := context.Background()
	if _, err := e.mgr.Start(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := e.proj.Card(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	row, err := e.store.Queries().GetSessionByCard(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.Write(ctx, func(q *db.Queries) error { _, err := q.DeleteSession(ctx, row.ID); return err }); err != nil {
		t.Fatal(err)
	}
	fresh := e.reopen(t, directMode)
	t.Cleanup(func() { _ = fresh.Close() })
	if _, err := fresh.Start(ctx, c.ID); err == nil || !strings.Contains(err.Error(), "separate worktree") {
		t.Fatalf("fresh legacy start = %v", err)
	}
	after, err := e.proj.Card(ctx, c.ID)
	if err != nil || after.Worktree != legacy.Worktree || after.Branch != legacy.Branch {
		t.Fatalf("legacy metadata changed: before=%+v after=%+v error=%v", legacy, after, err)
	}
}

func TestDirectResumeRefusesLaterProjectEdits(t *testing.T) {
	e := newEnv(t, directMode)
	t.Cleanup(func() { _ = e.mgr.Close() })
	p := e.project(t, "small-repo")
	c := e.card(t, p.ID, "First card")
	ctx := context.Background()
	if _, err := e.mgr.Start(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.Stop(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Path, "later.txt"), []byte("later changes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Start(ctx, c.ID); err == nil || !strings.Contains(err.Error(), "changed since this card last ran") {
		t.Fatalf("stale card resumed: %v", err)
	}
	other := e.card(t, p.ID, "Track later changes")
	if _, err := e.mgr.Start(ctx, other.ID); err != nil {
		t.Fatalf("failed resume leaked folder ownership: %v", err)
	}
}

func TestDirectHandoffKeepsProjectCoordinationInstructionsAndRules(t *testing.T) {
	e := newEnv(t, directMode)
	t.Cleanup(func() { _ = e.mgr.Close() })
	to := newFakeAgent(agents.Capabilities{Resume: true, StructuredEvents: true})
	if err := e.registry.Register(protocol.AgentKindGemini, func() (agents.Agent, error) { return to, nil }); err != nil {
		t.Fatal(err)
	}
	p := e.project(t, "small-repo")
	c := e.card(t, p.ID, "Hand off direct work")
	ctx := context.Background()
	if _, err := e.mgr.Start(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Handoff(ctx, c.ID, protocol.HandoffRequest{To: protocol.AgentKindGemini, Summary: "Continue the task"}); err != nil {
		t.Fatal(err)
	}
	spec := to.startSpecs()[0]
	if spec.Cwd != p.Path || !strings.Contains(spec.Instructions, "native subagents") || !strings.Contains(strings.Join(spec.DisallowedTools, " "), "worktree") {
		t.Fatalf("handoff lost direct workspace instructions or rules: %+v", spec)
	}
}

type delayedDrainAgent struct {
	agents.Agent
	delayed atomic.Bool
	stopped chan agents.SessionHandle
}

func (a *delayedDrainAgent) Stop(ctx context.Context, handle agents.SessionHandle) error {
	if a.delayed.CompareAndSwap(false, true) {
		a.stopped <- handle
		return nil
	}
	return a.Agent.Stop(ctx, handle)
}

func TestDirectDelayedShutdownReleasesFolderAfterPumpActuallyEnds(t *testing.T) {
	e := newEnv(t, directMode)
	t.Cleanup(func() { _ = e.mgr.Close() })
	a := &delayedDrainAgent{Agent: e.agent, stopped: make(chan agents.SessionHandle, 1)}
	// Use a second registered kind so the existing Claude factory remains unchanged.
	if err := e.registry.Register(protocol.AgentKindGemini, func() (agents.Agent, error) { return a, nil }); err != nil {
		t.Fatal(err)
	}
	p := e.project(t, "small-repo")
	c := e.card(t, p.ID, "Slow shutdown")
	kind := protocol.AgentKindGemini
	ctx := context.Background()
	if _, err := e.proj.UpdateCard(ctx, c.ID, protocol.UpdateCardRequest{Agent: &kind}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Start(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.Stop(ctx, c.ID); err == nil {
		t.Fatal("stop completed before the pump drained")
	}
	other := e.card(t, p.ID, "Next card")
	if _, err := e.mgr.Start(ctx, other.ID); err == nil {
		t.Fatal("another editor started while the old pump was still alive")
	}
	if err := e.agent.Stop(ctx, <-a.stopped); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(eventTimeout)
	for {
		if _, err := e.mgr.Start(ctx, other.ID); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the drained pump left a permanent folder reservation")
		}
		time.Sleep(time.Millisecond)
	}
}
