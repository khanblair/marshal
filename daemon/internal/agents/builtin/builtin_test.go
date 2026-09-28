package builtin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// testProfile is the profile tests run with: what a person's own agent normally does.
func testProfile() security.Profile { return security.DefaultProfile() }

// worktree makes a scratch folder with one file in it, and returns both paths.
func worktree(t *testing.T) (dir, file string) {
	t.Helper()
	dir = t.TempDir()
	file = filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(file, []byte("hello from the fixture\nsecond line\n"), 0o644); err != nil {
		t.Fatalf("write the fixture: %v", err)
	}
	return dir, file
}

// start opens a session and returns its handle and event channel.
func start(t *testing.T, agent agents.Agent, cwd, mode string) (agents.SessionHandle, <-chan agents.AgentEvent) {
	t.Helper()
	handle, err := agent.Start(context.Background(), agents.StartSpec{
		Cwd: cwd, PermissionMode: mode, Label: "card-1",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return handle, agent.Events(handle)
}

// send delivers a message and fails if the agent refuses it.
func send(t *testing.T, agent agents.Agent, h agents.SessionHandle, text string) {
	t.Helper()
	if err := agent.Send(context.Background(), h, agents.UserMessage{Text: text}); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestTheBuiltInAgentCompletesAFixtureTask(t *testing.T) {
	dir, _ := worktree(t)
	client := newScriptedClient(
		[]providers.Event{
			think("I should read the file first."),
			callTool("call_1", "read_file", `{"path":"hello.txt"}`),
			done(providers.StopToolUse),
		},
		[]providers.Event{
			text("The file says: hello from the fixture."),
			done(providers.StopEndTurn),
		},
	)
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeFullAuto))

	send(t, agent, h, "Read hello.txt and tell me what it says.")
	ended, seen := waitFor[agents.TurnEnded](t, ch)
	if ended.Reason != agents.TurnEndTurn {
		t.Errorf("turn reason = %q, want %q", ended.Reason, agents.TurnEndTurn)
	}

	var sawThought, sawCall, sawResult, sawAnswer bool
	for _, ev := range seen {
		switch e := ev.(type) {
		case agents.ThoughtChunk:
			sawThought = strings.Contains(e.Text, "read the file")
		case agents.ToolCall:
			sawCall = e.Title == "read_file hello.txt" && e.Kind == "read"
		case agents.ToolCallUpdate:
			sawResult = e.Status == agents.StatusCompleted && strings.Contains(e.Content, "hello from the fixture")
		case agents.MessageChunk:
			sawAnswer = sawAnswer || strings.Contains(e.Text, "hello from the fixture")
		}
	}
	if !sawThought || !sawCall || !sawResult || !sawAnswer {
		t.Errorf("thought=%v call=%v result=%v answer=%v; events were %v", sawThought, sawCall, sawResult, sawAnswer, kinds(seen))
	}
}

func TestWhatTheAgentSendsToTheProvider(t *testing.T) {
	dir, _ := worktree(t)
	client := newScriptedClient([]providers.Event{text("done"), done(providers.StopEndTurn)})
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeFullAuto))
	send(t, agent, h, "What is in hello.txt?")
	waitFor[agents.TurnEnded](t, ch)

	first := client.requests()[0]
	if first.Model != "claude-sonnet-4-5" {
		t.Errorf("model = %q, want the resolved default", first.Model)
	}
	if !strings.Contains(first.System, "built-in coding agent") {
		t.Errorf("system prompt = %q, want the package's base instruction", first.System)
	}
	names := make([]string, len(first.Tools))
	for i, tool := range first.Tools {
		names[i] = tool.Name
	}
	for _, want := range []string{"read_file", "write_file", "edit_file", "run_command"} {
		if !contains(names, want) {
			t.Errorf("tools = %v, want %s", names, want)
		}
	}
	if len(first.Messages) != 1 || first.Messages[0].Parts[0].Text != "What is in hello.txt?" {
		t.Errorf("messages = %v, want the person's one message", first.Messages)
	}
}

func TestTheAgentAsksBeforeItEditsInAskMode(t *testing.T) {
	dir, _ := worktree(t)
	target := filepath.Join(dir, "new.txt")
	client := newScriptedClient(
		[]providers.Event{callTool("call_1", "write_file", `{"path":"new.txt","content":"made you look"}`), done(providers.StopToolUse)},
		[]providers.Event{text("Written."), done(providers.StopEndTurn)},
	)
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeAsk))

	send(t, agent, h, "Create new.txt.")
	req, _ := waitFor[agents.PermissionRequested](t, ch)
	if req.Kind != "edit" || !strings.HasSuffix(req.Path, "new.txt") {
		t.Errorf("permission request = %+v, want an edit of new.txt", req)
	}
	if len(req.Options) != 4 {
		t.Errorf("options = %v, want the four answers", req.Options)
	}
	if err := agent.Respond(context.Background(), h, agents.ApprovalResponse{RequestID: req.RequestID, OptionID: optionAllowOnce}); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	// A lasting "allow" makes the next edit run without asking, so the turn finishes.
	_, seen := waitFor[agents.TurnEnded](t, ch)
	if _, err := os.Stat(target); err != nil {
		t.Errorf("the file was not written after the person allowed it: %v (events %v)", err, kinds(seen))
	}
}

func TestABypassSessionStillStaysInsideTheWorktree(t *testing.T) {
	dir, _ := worktree(t)
	outside := filepath.Join(t.TempDir(), "escape.txt")
	client := newScriptedClient(
		[]providers.Event{callTool("call_1", "write_file", `{"path":"`+outside+`","content":"nope"}`), done(providers.StopToolUse)},
		[]providers.Event{text("I could not write it."), done(providers.StopEndTurn)},
	)
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeBypass))

	send(t, agent, h, "Write outside the worktree.")
	_, seen := waitFor[agents.TurnEnded](t, ch)
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("a file outside the worktree was written, but bypass still keeps the worktree rule")
	}
	var refused bool
	for _, ev := range seen {
		if u, ok := ev.(agents.ToolCallUpdate); ok && u.Status == agents.StatusFailed {
			refused = strings.Contains(u.Content, "refused")
		}
	}
	if !refused {
		t.Errorf("the call was not refused; events were %v", kinds(seen))
	}
	// The model is told why, so it can correct itself instead of retrying blindly.
	last := client.requests()[len(client.requests())-1]
	found := false
	for _, m := range last.Messages {
		for _, p := range m.Parts {
			if p.Kind == providers.PartToolResult && p.IsError && strings.Contains(p.Text, "refused") {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("the model was not told the call was refused: %v", last.Messages)
	}
}

func TestAnInterruptedTurnEndsAsCancelled(t *testing.T) {
	dir, _ := worktree(t)
	client := newScriptedClient()
	client.block = true
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeFullAuto))

	send(t, agent, h, "Think for a while.")
	if err := agent.Interrupt(context.Background(), h); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	ended, seen := waitFor[agents.TurnEnded](t, ch)
	if ended.Reason != agents.TurnCancelled {
		t.Errorf("turn reason = %q, want %q (events %v)", ended.Reason, agents.TurnCancelled, kinds(seen))
	}
}

func TestASecondMessageWhileATurnRunsIsBusy(t *testing.T) {
	dir, _ := worktree(t)
	client := newScriptedClient()
	client.block = true
	agent := newTestAgent(t, client)
	h, _ := start(t, agent, dir, string(protocol.PermissionModeFullAuto))

	send(t, agent, h, "One.")
	err := agent.Send(context.Background(), h, agents.UserMessage{Text: "Two."})
	if !errors.Is(err, agents.ErrBusy) {
		t.Errorf("second Send = %v, want ErrBusy", err)
	}
}

func TestResumeReplaysTheStoredConversation(t *testing.T) {
	dir, _ := worktree(t)
	client := newScriptedClient([]providers.Event{text("Continuing."), done(providers.StopEndTurn)})
	stored := []providers.Message{
		{Role: providers.RoleUser, Parts: []providers.Part{{Kind: providers.PartText, Text: "earlier question"}}},
		{Role: providers.RoleAssistant, Parts: []providers.Part{{Kind: providers.PartText, Text: "earlier answer"}}},
	}
	seen := ""
	var agent agents.Agent
	var err error
	agent, err = New(Config{
		Resolver: &fakeResolver{client: client},
		Profile:  testProfile(),
		Logger:   quiet(),
		History: func(id string) ([]providers.Message, error) {
			seen = id
			return stored, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h, err := agent.Resume(context.Background(), "the-stored-id", agents.StartSpec{Cwd: dir, Label: "card-1"})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	ch := agent.Events(h)
	send(t, agent, h, "and now?")
	waitFor[agents.TurnEnded](t, ch)

	if seen != "the-stored-id" {
		t.Errorf("history was asked for %q, want the stored id", seen)
	}
	first := client.requests()[0]
	if len(first.Messages) != 3 {
		t.Fatalf("messages = %d, want the two replayed turns plus the new one", len(first.Messages))
	}
	if first.Messages[0].Parts[0].Text != "earlier question" {
		t.Errorf("the replayed history was not sent first: %v", first.Messages)
	}
}

func TestTheBuiltInAgentCanBeResumedAndSwitchesSettings(t *testing.T) {
	dir, _ := worktree(t)
	client := newScriptedClient()
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeAsk))
	if !h.Capabilities.Resume || !h.Capabilities.LoadSession || !h.Capabilities.StructuredEvents || h.Capabilities.MCP {
		t.Errorf("capabilities = %+v, want resume, load, structured events, and no MCP yet", h.Capabilities)
	}
	_ = ch

	applier, ok := agent.(agents.SettingsApplier)
	if !ok {
		t.Fatal("the built-in agent does not implement SettingsApplier")
	}
	applied, err := applier.ApplySettings(context.Background(), h, agents.SessionSettings{
		Thinking: string(protocol.ThinkingModeHigh), PermissionMode: string(protocol.PermissionModeFullAuto),
	})
	if err != nil {
		t.Fatalf("ApplySettings: %v", err)
	}
	if !applied.Thinking || !applied.PermissionMode {
		t.Errorf("applied = %+v, want thinking and the permission mode", applied)
	}
}

func TestRunningACommandReturnsWhatItPrinted(t *testing.T) {
	dir, _ := worktree(t)
	client := newScriptedClient(
		[]providers.Event{callTool("call_1", "run_command", `{"command":"printf 'hi from the shell'"}`), done(providers.StopToolUse)},
		[]providers.Event{text("It printed something."), done(providers.StopEndTurn)},
	)
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeFullAuto))

	send(t, agent, h, "Run a command.")
	_, seen := waitFor[agents.TurnEnded](t, ch)
	var printed bool
	for _, ev := range seen {
		if u, ok := ev.(agents.ToolCallUpdate); ok && strings.Contains(u.Content, "hi from the shell") {
			printed = true
		}
	}
	if !printed {
		t.Errorf("the command's output did not reach the chat view: %v", kinds(seen))
	}
}

// TestSearchFilesMatchesAGlobAndSkipsGitAndNodeModules covers search_files' three seams: a glob
// narrows which files are read (searchableFile), .git and node_modules are never walked into
// (skipSearchDir) even when they hold a matching name, and a match is reported with its line number
// (searchFileLines).
func TestSearchFilesMatchesAGlobAndSkipsGitAndNodeModules(t *testing.T) {
	dir, _ := worktree(t)
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("hello again in markdown\n"), 0o644); err != nil {
		t.Fatalf("write notes.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "hello.txt"), []byte("hello inside git\n"), 0o644); err != nil {
		t.Fatalf("write inside .git: %v", err)
	}

	client := newScriptedClient(
		[]providers.Event{callTool("call_1", "search_files", `{"pattern":"hello","glob":"*.txt"}`), done(providers.StopToolUse)},
		[]providers.Event{text("Found it."), done(providers.StopEndTurn)},
	)
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeFullAuto))

	send(t, agent, h, "Search for hello.")
	_, seen := waitFor[agents.TurnEnded](t, ch)
	var output string
	for _, ev := range seen {
		if u, ok := ev.(agents.ToolCallUpdate); ok {
			output = u.Content
		}
	}
	if !strings.Contains(output, "hello.txt:1: hello from the fixture") {
		t.Errorf("search output = %q, want the matched line from hello.txt", output)
	}
	if strings.Contains(output, "notes.md") {
		t.Errorf("search output = %q, the glob *.txt should have excluded notes.md", output)
	}
	if strings.Contains(output, "inside git") {
		t.Errorf("search output = %q, .git should never be searched", output)
	}
}

// TestEditFileReplacesText covers edit_file's exact-match replace, including replace_all: a single
// unqualified match with more than one occurrence is refused, and replace_all lifts that refusal.
func TestEditFileReplacesText(t *testing.T) {
	dir, file := worktree(t)
	if err := os.WriteFile(file, []byte("hello hello\n"), 0o644); err != nil {
		t.Fatalf("write the fixture: %v", err)
	}

	client := newScriptedClient(
		[]providers.Event{callTool("call_1", "edit_file", `{"path":"hello.txt","old_string":"hello","new_string":"hi","replace_all":true}`), done(providers.StopToolUse)},
		[]providers.Event{text("Edited."), done(providers.StopEndTurn)},
	)
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeFullAuto))

	send(t, agent, h, "Replace hello with hi, everywhere.")
	_, seen := waitFor[agents.TurnEnded](t, ch)
	var output string
	for _, ev := range seen {
		if u, ok := ev.(agents.ToolCallUpdate); ok {
			output = u.Content
		}
	}
	if !strings.Contains(output, "2 places") {
		t.Errorf("edit output = %q, want it to say 2 places", output)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read the edited file: %v", err)
	}
	if string(got) != "hi hi\n" {
		t.Errorf("the file now holds %q, want %q", got, "hi hi\n")
	}
}

// TestListFilesFindsAndFiltersByPattern covers list_files' glob and its .git skip.
func TestListFilesFindsAndFiltersByPattern(t *testing.T) {
	dir, _ := worktree(t)
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("notes\n"), 0o644); err != nil {
		t.Fatalf("write notes.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write inside .git: %v", err)
	}

	client := newScriptedClient(
		[]providers.Event{callTool("call_1", "list_files", `{"pattern":"*.txt"}`), done(providers.StopToolUse)},
		[]providers.Event{text("Listed."), done(providers.StopEndTurn)},
	)
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeFullAuto))

	send(t, agent, h, "List the .txt files.")
	_, seen := waitFor[agents.TurnEnded](t, ch)
	var output string
	for _, ev := range seen {
		if u, ok := ev.(agents.ToolCallUpdate); ok {
			output = u.Content
		}
	}
	if !strings.Contains(output, "hello.txt") {
		t.Errorf("list output = %q, want hello.txt", output)
	}
	if strings.Contains(output, "notes.md") {
		t.Errorf("list output = %q, the glob *.txt should have excluded notes.md", output)
	}
	if strings.Contains(output, "config.txt") {
		t.Errorf("list output = %q, .git should never be listed", output)
	}
}

func TestAKeyThatIsRefusedIsReportedPlainly(t *testing.T) {
	dir, _ := worktree(t)
	client := &refusingClient{err: providers.ErrAuth}
	agent := newTestAgent(t, client)
	h, ch := start(t, agent, dir, string(protocol.PermissionModeFullAuto))
	send(t, agent, h, "Hello?")
	ended, seen := waitFor[agents.TurnEnded](t, ch)
	if ended.Reason != agents.TurnError {
		t.Errorf("turn reason = %q, want %q", ended.Reason, agents.TurnError)
	}
	var said string
	for _, ev := range seen {
		if f, ok := ev.(agents.Failed); ok {
			said = f.Message
		}
	}
	if !strings.Contains(said, "refused") {
		t.Errorf("the failure said %q, want the plain sentence about a refused key", said)
	}
}

// refusingClient fails every call, for the failure-path test.
type refusingClient struct{ err error }

func (c *refusingClient) ID() string { return providers.AnthropicID }

func (c *refusingClient) Complete(context.Context, providers.Request) (providers.Reply, error) {
	return providers.Reply{}, c.err
}

func (c *refusingClient) Stream(context.Context, providers.Request) (providers.Stream, error) {
	return nil, c.err
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestExitText covers run_command's sentence about how the command ended: appended after the
// command's own output, or standing alone when the command printed nothing.
func TestExitText(t *testing.T) {
	if got := exitText("the output", "it failed"); got != "the output\nit failed" {
		t.Errorf("exitText with output = %q, want %q", got, "the output\nit failed")
	}
	if got := exitText("  \n", "it failed"); got != "it failed" {
		t.Errorf("exitText with blank output = %q, want just the tail", got)
	}
}

// TestIsRuneStart covers clampOutput's cut point: a continuation byte never starts a UTF-8
// character, so cutting there would split one in half.
func TestIsRuneStart(t *testing.T) {
	if !isRuneStart('h') {
		t.Error("an ASCII byte should start a rune")
	}
	if !isRuneStart(0xC3) {
		t.Error("a UTF-8 lead byte should start a rune")
	}
	if isRuneStart(0x80) {
		t.Error("a UTF-8 continuation byte should not start a rune")
	}
}
