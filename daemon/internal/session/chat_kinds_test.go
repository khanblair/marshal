package session_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// What a chat's agent is started with, by the kind of chat it is: where it runs, with which tools and
// rules, and with which instructions. The fake agent keeps every StartSpec it is given, so each test
// reads the spec the manager built.

// fakeWorkspace is the ChatWorkspace the manager makes the Integrator's folder with.
type fakeWorkspace struct {
	mu    sync.Mutex
	path  string
	err   error
	asked []string
}

func (w *fakeWorkspace) Ensure(_ context.Context, projectID string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.asked = append(w.asked, projectID)
	return w.path, w.err
}

func (w *fakeWorkspace) askedFor() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.asked...)
}

// fakeChatAttacher is the ChatAttacher: it gives each chat one server named for it, and instructions
// that say which chat they are for, and remembers who was attached and detached.
type fakeChatAttacher struct {
	mu       sync.Mutex
	attached []protocol.Chat
	detached []string
	err      error
}

func (a *fakeChatAttacher) AttachChat(_ context.Context, chat protocol.Chat) (session.Attachment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return session.Attachment{}, a.err
	}
	a.attached = append(a.attached, chat)
	return session.Attachment{
		Servers:      []agents.MCPServer{{Name: "marshal", Command: "/bin/marshald", Args: []string{"mcp", "--card", chat.ID}}},
		Instructions: "instructions for " + chat.ID,
	}, nil
}

func (a *fakeChatAttacher) DetachChat(_ context.Context, chatID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.detached = append(a.detached, chatID)
}

func (a *fakeChatAttacher) detachedIDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.detached...)
}

// kindEnv is a chat env whose manager has a workspace maker and a chat attacher, and a project.
type kindEnv struct {
	*chatEnv
	clock     *testClock
	workspace *fakeWorkspace
	attacher  *fakeChatAttacher
	project   protocol.Project
}

func newKindEnv(t *testing.T) *kindEnv {
	t.Helper()
	clock := newTestClock()
	ce := newChatEnv(t, func(c *session.Config) { c.Now = clock.Now })
	k := &kindEnv{
		chatEnv: ce, clock: clock, attacher: &fakeChatAttacher{},
		workspace: &fakeWorkspace{path: filepath.Join(t.TempDir(), "integrator")},
	}
	k.mgr.SetChatWorkspace(k.workspace)
	k.mgr.SetChatAttacher(k.attacher)
	k.project = k.chatEnv.project(t, "small-repo")
	return k
}

// integrator makes the project's Integrator chat through the chats service.
func (k *kindEnv) integrator(t *testing.T) protocol.Chat {
	t.Helper()
	chat, err := k.chats.EnsureSystemChat(context.Background(), k.project.ID, protocol.ChatSystemIntegrator)
	if err != nil {
		t.Fatalf("EnsureSystemChat: %v", err)
	}
	return chat
}

// lastSpec is the newest spec the fake agent was given.
func (k *kindEnv) lastSpec(t *testing.T) agents.StartSpec {
	t.Helper()
	specs := k.agent.startSpecs()
	if len(specs) == 0 {
		t.Fatal("the agent was never started")
	}
	return specs[len(specs)-1]
}

// settle sends a message and waits until the chat's turn is over and it rests awake.
func (k *kindEnv) settle(t *testing.T, chatID, text string, history int) {
	t.Helper()
	k.send(t, chatID, text)
	k.waitForChatHistory(t, chatID, history)
	waitForRowState(t, k.chatEnv, chatID, protocol.SessionStateAwake)
}

func TestAnOrdinaryChatStartsInTheOwnersFolderWithItsRoleAndTheBoardTool(t *testing.T) {
	k := newKindEnv(t)
	chat := k.newChat(t, k.project.ID, protocol.CreateChatRequest{
		Target: &protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: "Tester"}, PermissionMode: protocol.PermissionModeAutoEdits,
	})
	k.send(t, chat.ID, "Write tests for the export")
	spec := k.lastSpec(t)

	if spec.Cwd != k.project.Path || spec.PermissionMode != string(protocol.PermissionModeAutoEdits) {
		t.Errorf("spec = %+v, want the owner's folder and the mode the chat was made with", spec)
	}
	if spec.Instructions != "instructions for "+chat.ID || len(spec.MCPServers) != 1 || spec.MCPServers[0].Args[2] != chat.ID {
		t.Errorf("instructions %q and servers %+v, want what the attacher gave this chat", spec.Instructions, spec.MCPServers)
	}
	if !slices.Equal(spec.AllowedTools, []string{"mcp__marshal"}) || len(spec.DisallowedTools) != 0 {
		t.Errorf("tool rules = %v / %v, want only Marshal's own tools allowed", spec.AllowedTools, spec.DisallowedTools)
	}
	cfg, ok := k.mgr.HarnessConfigForChat(chat.ID)
	if !ok || cfg.Mode != protocol.PermissionModeAutoEdits || cfg.ProtectRefs || cfg.Containment != nil {
		t.Errorf("harness = %+v, %v, want the stored mode and no extra rule", cfg, ok)
	}
}

func TestAWokenChatKeepsItsServersAndIsNotToldItsInstructionsAgain(t *testing.T) {
	k := newKindEnv(t)
	chat := k.newChat(t, k.project.ID, protocol.CreateChatRequest{})
	k.settle(t, chat.ID, "first", 2)
	if err := k.mgr.StopChatSession(context.Background(), chat.ID); err != nil {
		t.Fatalf("StopChatSession: %v", err)
	}
	k.send(t, chat.ID, "second")
	specs := k.agent.startSpecs()
	if len(specs) != 2 {
		t.Fatalf("the agent was given %d specs, want a start and a resume", len(specs))
	}
	resume := specs[1]
	if resume.Instructions != "" {
		t.Errorf("the resume was given instructions %q, want none: the conversation has them", resume.Instructions)
	}
	if len(resume.MCPServers) != 1 || !slices.Equal(resume.AllowedTools, []string{"mcp__marshal"}) {
		t.Errorf("the resume's tools = %+v / %v, want the server again", resume.MCPServers, resume.AllowedTools)
	}
}

func TestAChatThatCannotBeAttachedStillTalks(t *testing.T) {
	k := newKindEnv(t)
	k.attacher.err = errors.New("the memory cannot be read")
	chat := k.newChat(t, k.project.ID, protocol.CreateChatRequest{
		Target: &protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: "Worker"},
	})
	k.send(t, chat.ID, "hello")
	spec := k.lastSpec(t)
	if len(spec.MCPServers) != 0 || spec.Instructions != "" || len(spec.AllowedTools) != 0 {
		t.Errorf("spec = %+v, want a plain agent when the attachment failed", spec)
	}
}

func TestAChatWithNoAttacherAtAllStartsAsItAlwaysDid(t *testing.T) {
	e := newChatEnv(t)
	project := e.project(t, "small-repo")
	chat := e.newChat(t, project.ID, protocol.CreateChatRequest{
		Target: &protocol.ChatTarget{Kind: protocol.ChatTargetKindRole, ID: "Worker"},
	})
	e.send(t, chat.ID, "hello")
	spec := e.agent.startSpecs()[0]
	if spec.Cwd != project.Path || spec.Instructions != "" || len(spec.MCPServers) != 0 || len(spec.AllowedTools)+len(spec.DisallowedTools) != 0 {
		t.Errorf("spec = %+v, want the owner's folder and nothing else", spec)
	}
}

func TestTheOrchestratorChatCannotChangeTheOwnersFolder(t *testing.T) {
	for _, target := range []protocol.ChatTarget{
		{Kind: protocol.ChatTargetKindOrchestrator},
		{Kind: protocol.ChatTargetKindRole, ID: "Orchestrator"},
	} {
		t.Run(string(target.Kind), func(t *testing.T) {
			k := newKindEnv(t)
			// It was made to edit, which is the default; the Orchestrator is read-only anyway.
			chat := k.newChat(t, k.project.ID, protocol.CreateChatRequest{
				Target: &target, PermissionMode: protocol.PermissionModeFullAuto,
			})
			k.send(t, chat.ID, "Plan the export work")
			spec := k.lastSpec(t)

			if spec.Cwd != k.project.Path {
				t.Errorf("cwd = %q, want the owner's folder", spec.Cwd)
			}
			if spec.PermissionMode != string(protocol.PermissionModeAsk) {
				t.Errorf("permission mode = %q, want one that edits nothing without a person", spec.PermissionMode)
			}
			for _, denied := range []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"} {
				if !slices.Contains(spec.DisallowedTools, denied) {
					t.Errorf("disallowed = %v, want %s among them", spec.DisallowedTools, denied)
				}
			}
			if !slices.Contains(spec.AllowedTools, "mcp__marshal") {
				t.Errorf("allowed = %v, want Marshal's own tools, which is how it creates cards", spec.AllowedTools)
			}
			cfg, ok := k.mgr.HarnessConfigForChat(chat.ID)
			if !ok || cfg.Mode != protocol.PermissionModePlan {
				t.Fatalf("harness = %+v, %v, want plan-only whatever the chat was made with", cfg, ok)
			}
			if got := cfg.Decide(harness.Request{Kind: "edit", Path: k.project.Path + "/src/a.go"}); got.Decision != harness.DecisionDeny {
				t.Errorf("an edit = %+v, want it refused", got)
			}
			if got := cfg.Decide(harness.Request{Kind: "execute", Command: "touch x"}); got.Decision != harness.DecisionDeny {
				t.Errorf("a command = %+v, want it refused", got)
			}
			if got := cfg.Decide(harness.Request{Kind: "read", Path: k.project.Path + "/README.md"}); got.Decision != harness.DecisionAllow {
				t.Errorf("a read = %+v, want it allowed: a planner reads the project", got)
			}
		})
	}
}

func TestTheIntegratorChatRunsInItsOwnWorkspaceAndNeverTheOwnersFolder(t *testing.T) {
	k := newKindEnv(t)
	chat := k.integrator(t)
	k.send(t, chat.ID, "Merge task t-1")
	spec := k.lastSpec(t)

	if spec.Cwd != k.workspace.path || spec.Cwd == k.project.Path {
		t.Errorf("cwd = %q, want the Integrator's workspace %q and not %q", spec.Cwd, k.workspace.path, k.project.Path)
	}
	if got := k.workspace.askedFor(); len(got) != 1 || got[0] != k.project.ID {
		t.Errorf("the workspace was asked for %v, want this project's", got)
	}
	if spec.PermissionMode != string(protocol.PermissionModeAutoEdits) {
		t.Errorf("permission mode = %q, want auto-edits", spec.PermissionMode)
	}
	if spec.Instructions != "instructions for "+chat.ID || len(spec.MCPServers) != 1 {
		t.Errorf("instructions %q and servers %+v, want what the attacher gave the Integrator chat", spec.Instructions, spec.MCPServers)
	}
	for _, want := range []string{"mcp__marshal", "Bash(git add:*)", "Bash(git commit:*)", "Bash(git merge:*)"} {
		if !slices.Contains(spec.AllowedTools, want) {
			t.Errorf("allowed = %v, want %s", spec.AllowedTools, want)
		}
	}
	for _, want := range []string{"Bash(git push:*)", "Bash(git rebase:*)", "Bash(git reset --hard:*)", "Bash(git branch -D:*)"} {
		if !slices.Contains(spec.DisallowedTools, want) {
			t.Errorf("disallowed = %v, want %s", spec.DisallowedTools, want)
		}
	}
	for _, rule := range spec.AllowedTools {
		if strings.HasPrefix(rule, "Bash(git push") || strings.HasPrefix(rule, "Bash(git rebase") {
			t.Errorf("the allowed rule %q names a command the Integrator may not run", rule)
		}
	}
}

// Claude Code matches a rule by how a command starts, so an allow rule that a dangerous flag can
// follow would let it through: "git reset HEAD --hard" starts with "git reset HEAD", and "git checkout
// --quiet main" starts with "git checkout --".
func TestTheIntegratorsAllowRulesLeaveNoRoomForADangerousFlag(t *testing.T) {
	k := newKindEnv(t)
	k.send(t, k.integrator(t).ID, "Merge task t-1")
	allowed := k.lastSpec(t).AllowedTools
	for _, rule := range allowed {
		switch {
		case strings.HasPrefix(rule, "Bash(git reset"), rule == "Bash(git checkout --:*)":
			t.Errorf("the allow rule %q lets a flag that moves the branch follow it", rule)
		case strings.HasPrefix(rule, "Bash(git checkout"):
			if rule != "Bash(git checkout --ours:*)" && rule != "Bash(git checkout --theirs:*)" {
				t.Errorf("the allow rule %q is not one of the two that name only files", rule)
			}
		case strings.HasPrefix(rule, "Bash(git branch"):
			if rule != "Bash(git branch --show-current:*)" {
				t.Errorf("the allow rule %q is more than reading the current branch", rule)
			}
		}
	}
	if !slices.Contains(allowed, "Bash(git restore:*)") {
		t.Error("the Integrator cannot unstage or restore a file: git restore is not allowed")
	}
}

func TestTheIntegratorsPermissionRequestsAreHeldToItsWorkspaceAndTheRefGuard(t *testing.T) {
	k := newKindEnv(t)
	chat := k.integrator(t)
	k.send(t, chat.ID, "Merge task t-1")

	cfg, ok := k.mgr.HarnessConfigForChat(chat.ID)
	if !ok || !cfg.ProtectRefs || cfg.Containment == nil || cfg.Containment.Worktree != k.workspace.path {
		t.Fatalf("harness = %+v, %v, want the ref guard and the workspace as its folder", cfg, ok)
	}
	tests := []struct {
		name string
		req  harness.Request
		want harness.Decision
	}{
		{"an edit in the workspace", harness.Request{Kind: "edit", Path: k.workspace.path + "/src/a.go"}, harness.DecisionAllow},
		{"an edit in the owner's folder", harness.Request{Kind: "edit", Path: k.project.Path + "/src/a.go"}, harness.DecisionDeny},
		{"git add", harness.Request{Kind: "execute", Command: "git add -A"}, harness.DecisionAllow},
		{"git commit", harness.Request{Kind: "execute", Command: "git commit -m resolve"}, harness.DecisionAllow},
		{"git merge --continue", harness.Request{Kind: "execute", Command: "git merge --continue"}, harness.DecisionAllow},
		{"a push", harness.Request{Kind: "execute", Command: "git push origin integrator"}, harness.DecisionDeny},
		{"a rebase", harness.Request{Kind: "execute", Command: "git rebase main"}, harness.DecisionDeny},
		{"a hard reset", harness.Request{Kind: "execute", Command: "git reset --hard HEAD~1"}, harness.DecisionDeny},
		{"a checkout of another branch", harness.Request{Kind: "execute", Command: "git checkout main"}, harness.DecisionDeny},
		{"git in the owner's folder", harness.Request{Kind: "execute", Command: "git -C " + k.project.Path + " status"}, harness.DecisionDeny},
	}
	for _, tc := range tests {
		if got := cfg.Decide(tc.req); got.Decision != tc.want {
			t.Errorf("%s: Decide = %+v, want %s", tc.name, got, tc.want)
		}
	}
}

func TestTheIntegratorChatDoesNotStartWithoutAWorkspaceOrInTheOwnersFolder(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*kindEnv)
	}{
		{"no workspace maker", func(k *kindEnv) { k.mgr.SetChatWorkspace(nil) }},
		{"a workspace that cannot be made", func(k *kindEnv) { k.workspace.err = errors.New("git is missing") }},
		{"an empty workspace folder", func(k *kindEnv) { k.workspace.path = "" }},
		{"the owner's own folder", func(k *kindEnv) { k.workspace.path = k.project.Path }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := newKindEnv(t)
			chat := k.integrator(t)
			tc.setup(k)
			err := k.chats.Send(context.Background(), chat.ID, "Merge task t-1")
			wantCode(t, err, protocol.ErrorCodeUnavailable)
			if got := fakeStarts(k.agent); got != 0 {
				t.Errorf("%d agents were started, want none", got)
			}
			if specs := k.agent.startSpecs(); len(specs) != 0 {
				t.Errorf("the agent was asked to run in %q", specs[0].Cwd)
			}
		})
	}
}

func TestResettingTheIntegratorEndsItsConversationAndKeepsTheChat(t *testing.T) {
	k := newKindEnv(t)
	chat := k.integrator(t)
	k.settle(t, chat.ID, "Merge task t-1", 2)
	before := k.row(t, chat.ID)
	if before.AgentSessionID == "" {
		t.Fatal("the Integrator never started its agent")
	}

	if err := k.mgr.ResetChatSession(context.Background(), chat.ID); err != nil {
		t.Fatalf("ResetChatSession: %v", err)
	}
	after := k.row(t, chat.ID)
	if after.AgentSessionID != "" || after.State != string(protocol.SessionStateStarting) || after.ID != before.ID {
		t.Errorf("the session after a reset = %+v, want the same row, starting, with no conversation", after)
	}
	if k.mgr.RecentOutput(chat.ID) != nil {
		t.Error("the Integrator still has a live session after a reset")
	}
	k.untilChatState(t, chat.ID, protocol.SessionStateStarting)
	// The chat and what was said in it stay, so the owner can still read the earlier merge.
	if got := k.chatHistory(t, chat.ID); len(got) < 2 {
		t.Errorf("the history after a reset has %d events, want it kept", len(got))
	}
	if got, err := k.chats.Chat(context.Background(), chat.ID); err != nil || got.System != protocol.ChatSystemIntegrator {
		t.Errorf("the chat after a reset = %+v, %v, want it kept", got, err)
	}

	// The next task starts a new conversation, with its instructions again.
	k.send(t, chat.ID, "Merge task t-2")
	specs := k.agent.startSpecs()
	if len(specs) != 2 || fakeStarts(k.agent) != 2 {
		t.Fatalf("the agent was given %d specs and started %d times, want a fresh start for the second task", len(specs), fakeStarts(k.agent))
	}
	if specs[1].Instructions != "instructions for "+chat.ID {
		t.Errorf("the second start's instructions = %q, want them sent again to the fresh conversation", specs[1].Instructions)
	}
	if k.row(t, chat.ID).AgentSessionID == before.AgentSessionID {
		t.Error("the second task went to the same agent conversation")
	}
}

func TestResettingAChatThatNeverTalkedOrIsAsleepIsHarmless(t *testing.T) {
	k := newKindEnv(t)
	chat := k.integrator(t)
	if err := k.mgr.ResetChatSession(context.Background(), chat.ID); err != nil {
		t.Fatalf("a reset before the first message: %v", err)
	}
	k.settle(t, chat.ID, "task", 2)
	if err := k.mgr.StopChatSession(context.Background(), chat.ID); err != nil {
		t.Fatalf("StopChatSession: %v", err)
	}
	if err := k.mgr.ResetChatSession(context.Background(), chat.ID); err != nil {
		t.Fatalf("a reset of a sleeping chat: %v", err)
	}
	if row := k.row(t, chat.ID); row.AgentSessionID != "" || row.State != string(protocol.SessionStateStarting) {
		t.Errorf("row = %+v, want a conversation that is forgotten", row)
	}
	if err := k.mgr.ResetChatSession(context.Background(), "01M3C107JB041061050R3GG28Z"); err != nil {
		t.Errorf("a reset of a chat that has no session: %v", err)
	}
}

func TestAnIdleSystemChatSleepsAndTheNextMessageWakesIt(t *testing.T) {
	k := newKindEnv(t)
	chat := k.integrator(t)
	k.settle(t, chat.ID, "Merge task t-1", 2)
	before := k.row(t, chat.ID)

	k.clock.advance(14 * time.Minute)
	k.mgr.CheckIdle(context.Background())
	if k.mgr.RecentOutput(chat.ID) == nil {
		t.Fatal("the Integrator slept before it had been idle for the idle time")
	}

	k.clock.advance(2 * time.Minute)
	k.mgr.CheckIdle(context.Background())
	k.untilChatState(t, chat.ID, protocol.SessionStateAsleep)
	slept := k.row(t, chat.ID)
	if slept.State != string(protocol.SessionStateAsleep) || slept.AgentSessionID != before.AgentSessionID {
		t.Fatalf("the idle Integrator's session = %+v, want asleep on the same conversation", slept)
	}
	if k.mgr.RecentOutput(chat.ID) != nil {
		t.Error("the idle Integrator still has a live process")
	}

	// A message wakes it through a resume of the same conversation, in its own workspace.
	k.send(t, chat.ID, "Merge task t-2")
	k.untilChatState(t, chat.ID, protocol.SessionStateWaking)
	k.untilChatState(t, chat.ID, protocol.SessionStateAwake)
	specs := k.agent.startSpecs()
	if fakeStarts(k.agent) != 1 || len(specs) != 2 {
		t.Fatalf("%d starts and %d specs, want one start and one resume", fakeStarts(k.agent), len(specs))
	}
	if specs[1].Cwd != k.workspace.path || specs[1].Instructions != "" {
		t.Errorf("the resume = %+v, want the workspace and no instructions", specs[1])
	}
}

func TestAChatAPersonMadeIsNeverSleptOnTheTimer(t *testing.T) {
	k := newKindEnv(t)
	chat := k.newChat(t, k.project.ID, protocol.CreateChatRequest{})
	k.settle(t, chat.ID, "hello", 2)
	k.clock.advance(5 * time.Hour)
	k.mgr.CheckIdle(context.Background())
	if k.mgr.RecentOutput(chat.ID) == nil || k.row(t, chat.ID).State != string(protocol.SessionStateAwake) {
		t.Errorf("an ordinary chat was slept after five idle hours: %+v", k.row(t, chat.ID))
	}
}

func TestABusySystemChatIsNotSlept(t *testing.T) {
	k := newKindEnv(t)
	chat := k.integrator(t)
	hold := make(chan struct{})
	k.agent.hold = hold
	k.send(t, chat.ID, "Merge task t-1")
	waitForRowState(t, k.chatEnv, chat.ID, protocol.SessionStateWorking)

	k.clock.advance(time.Hour)
	k.mgr.CheckIdle(context.Background())
	if k.mgr.RecentOutput(chat.ID) == nil || k.row(t, chat.ID).State != string(protocol.SessionStateWorking) {
		t.Errorf("a working Integrator was slept: %+v", k.row(t, chat.ID))
	}
	close(hold)
	waitForRowState(t, k.chatEnv, chat.ID, protocol.SessionStateAwake)
}

func TestDeletingAChatGivesUpWhatItWasAttached(t *testing.T) {
	k := newKindEnv(t)
	chat := k.newChat(t, k.project.ID, protocol.CreateChatRequest{})
	k.settle(t, chat.ID, "hello", 2)
	if err := k.chats.Remove(context.Background(), chat.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := k.attacher.detachedIDs(); !slices.Equal(got, []string{chat.ID}) {
		t.Errorf("detached = %v, want the deleted chat", got)
	}
}
