package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/acp-go-sdk"
	"go.uber.org/goleak"
)

const (
	// testSpeed shrinks every scenario pause to a few microseconds.
	testSpeed = 0.0001
	// testTimeout bounds one test, so a hang fails with a message instead of stalling the run.
	testTimeout = 20 * time.Second
)

func TestMain(m *testing.M) {
	// The connection logs every close at info level, which only adds noise to a test run.
	slog.SetDefault(slog.New(slog.DiscardHandler))
	goleak.VerifyTestMain(m)
}

// testClient is the client side of the connection. It records what the agent sends and answers
// permission requests the way a test says.
type testClient struct {
	mu       sync.Mutex
	updates  []acp.SessionUpdate
	requests []acp.RequestPermissionRequest
	// choice is the option id to select. An empty choice never answers, like a person who has
	// walked away, until the request is cancelled.
	choice string
	// updated and asked get a token for each update and each permission request.
	updated chan struct{}
	asked   chan struct{}
}

func newTestClient(choice string) *testClient {
	return &testClient{
		choice:  choice,
		updated: make(chan struct{}, 1024),
		asked:   make(chan struct{}, 16),
	}
}

func (c *testClient) SessionUpdate(_ context.Context, n acp.SessionNotification) error {
	c.mu.Lock()
	c.updates = append(c.updates, n.Update)
	c.mu.Unlock()
	select {
	case c.updated <- struct{}{}:
	default:
	}
	return nil
}

func (c *testClient) RequestPermission(
	ctx context.Context, req acp.RequestPermissionRequest,
) (acp.RequestPermissionResponse, error) {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	choice := c.choice
	c.mu.Unlock()
	select {
	case c.asked <- struct{}{}:
	default:
	}
	if choice == "" {
		<-ctx.Done()
		return acp.RequestPermissionResponse{}, ctx.Err()
	}
	return acp.RequestPermissionResponse{
		Outcome: acp.NewRequestPermissionOutcomeSelected(acp.PermissionOptionId(choice)),
	}, nil
}

func (*testClient) ReadTextFile(
	context.Context, acp.ReadTextFileRequest,
) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, acp.NewMethodNotFound(acp.ClientMethodFsReadTextFile)
}

func (*testClient) WriteTextFile(
	context.Context, acp.WriteTextFileRequest,
) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, acp.NewMethodNotFound(acp.ClientMethodFsWriteTextFile)
}

func (*testClient) CreateTerminal(
	context.Context, acp.CreateTerminalRequest,
) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, acp.NewMethodNotFound(acp.ClientMethodTerminalCreate)
}

func (*testClient) KillTerminal(
	context.Context, acp.KillTerminalRequest,
) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, acp.NewMethodNotFound(acp.ClientMethodTerminalKill)
}

func (*testClient) TerminalOutput(
	context.Context, acp.TerminalOutputRequest,
) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, acp.NewMethodNotFound(acp.ClientMethodTerminalOutput)
}

func (*testClient) ReleaseTerminal(
	context.Context, acp.ReleaseTerminalRequest,
) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, acp.NewMethodNotFound(acp.ClientMethodTerminalRelease)
}

func (*testClient) WaitForTerminalExit(
	context.Context, acp.WaitForTerminalExitRequest,
) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, acp.NewMethodNotFound(acp.ClientMethodTerminalWaitForExit)
}

// recorded returns a copy of the updates so far.
func (c *testClient) recorded() []acp.SessionUpdate {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]acp.SessionUpdate(nil), c.updates...)
}

// permissionRequests returns a copy of the permission requests so far.
func (c *testClient) permissionRequests() []acp.RequestPermissionRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]acp.RequestPermissionRequest(nil), c.requests...)
}

// forget drops the recorded updates, to look at the next phase of a test on its own.
func (c *testClient) forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updates = nil
}

// shapes describes each update in one short word, for checking the order of a turn.
func (c *testClient) shapes() []string {
	var out []string
	for _, u := range c.recorded() {
		out = append(out, shape(u))
	}
	return out
}

func shape(u acp.SessionUpdate) string {
	switch {
	case u.AgentMessageChunk != nil:
		return "agent"
	case u.UserMessageChunk != nil:
		return "user"
	case u.ToolCall != nil:
		return fmt.Sprintf("tool:%s:%s", u.ToolCall.Kind, u.ToolCall.Status)
	case u.ToolCallUpdate != nil && u.ToolCallUpdate.Status != nil:
		return "update:" + string(*u.ToolCallUpdate.Status)
	}
	return "other"
}

// agentTexts returns the text of each agent message chunk, in order.
func (c *testClient) agentTexts() []string {
	var out []string
	for _, u := range c.recorded() {
		if u.AgentMessageChunk != nil {
			out = append(out, u.AgentMessageChunk.Content.Text.Text)
		}
	}
	return out
}

// userTexts returns the text of each user message chunk, in order.
func (c *testClient) userTexts() []string {
	var out []string
	for _, u := range c.recorded() {
		if u.UserMessageChunk != nil {
			out = append(out, u.UserMessageChunk.Content.Text.Text)
		}
	}
	return out
}

// toolStarts returns the tool calls that were announced, in order.
func (c *testClient) toolStarts() []acp.SessionUpdateToolCall {
	var out []acp.SessionUpdateToolCall
	for _, u := range c.recorded() {
		if u.ToolCall != nil {
			out = append(out, *u.ToolCall)
		}
	}
	return out
}

// toolUpdates returns the tool call updates, in order.
func (c *testClient) toolUpdates() []acp.SessionToolCallUpdate {
	var out []acp.SessionToolCallUpdate
	for _, u := range c.recorded() {
		if u.ToolCallUpdate != nil {
			out = append(out, *u.ToolCallUpdate)
		}
	}
	return out
}

// updateText returns the text content of a tool call update.
func updateText(u acp.SessionToolCallUpdate) string {
	for _, c := range u.Content {
		if c.Content != nil && c.Content.Content.Text != nil {
			return c.Content.Content.Text.Text
		}
	}
	return ""
}

// harnessOptions say how to build a harness. The zero value is a default agent with the built-in
// scenarios, tiny pauses, and a client that allows every permission request.
type harnessOptions struct {
	// scenarios are extra scenario files, as file name to JSON. When set, they replace the
	// built-in scenarios.
	scenarios map[string]string
	scenario  string
	stateDir  string
	cwd       string
	speed     float64
	choice    string
	// noChoice makes the client leave permission requests unanswered.
	noChoice bool
}

// harness is one agent and one client connected by pipes, all in this process.
type harness struct {
	t        *testing.T
	ctx      context.Context
	client   *testClient
	conn     *acp.ClientSideConnection
	agent    *stubAgent
	stateDir string
	cwd      string
	stopOnce sync.Once
	stop     func()
}

func newHarness(t *testing.T, opts harnessOptions) *harness {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), testTimeout)
	t.Cleanup(cancel)

	if opts.stateDir == "" {
		opts.stateDir = filepath.Join(t.TempDir(), "state")
	}
	if opts.cwd == "" {
		opts.cwd = filepath.Join(t.TempDir(), "work")
		if err := os.MkdirAll(opts.cwd, dirMode); err != nil {
			t.Fatal(err)
		}
	}
	if opts.speed == 0 {
		opts.speed = testSpeed
	}
	if opts.scenario == "" {
		opts.scenario = defaultScenarioName
	}
	choice := opts.choice
	if choice == "" && !opts.noChoice {
		choice = allowOptionID
	}

	agent, err := newAgent(
		config{scenario: opts.scenario, stateDir: opts.stateDir, speed: opts.speed},
		loadTestScenarios(t, opts.scenarios),
	)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{
		t: t, ctx: ctx, client: newTestClient(choice), agent: agent,
		stateDir: opts.stateDir, cwd: opts.cwd,
	}
	h.connect()
	t.Cleanup(h.close)
	return h
}

// connect wires the agent and the client together with two pipes, one for each direction.
func (h *harness) connect() {
	agentIn, clientOut := io.Pipe()
	clientIn, agentOut := io.Pipe()
	agentConn := serve(h.agent, agentOut, agentIn)
	h.conn = acp.NewClientSideConnection(h.client, clientOut, clientIn)
	h.stop = func() {
		// Closing all four ends lets both connections see the end of their input and finish,
		// whichever side is in the middle of a write.
		for _, c := range []io.Closer{clientOut, agentOut, agentIn, clientIn} {
			_ = c.Close()
		}
		<-agentConn.Done()
		<-h.conn.Done()
	}
}

// close ends the connection. A test calls it to simulate the process ending, and the cleanup
// calls it again, which does nothing.
func (h *harness) close() {
	h.stopOnce.Do(h.stop)
}

// loadTestScenarios reads the given scenario files, or the built-in ones when there are none.
func loadTestScenarios(t *testing.T, files map[string]string) map[string]scenario {
	t.Helper()
	var (
		found map[string]scenario
		err   error
	)
	if files == nil {
		found, err = loadBuiltinScenarios()
	} else {
		mapFS := fstest.MapFS{}
		for name, data := range files {
			mapFS[name] = &fstest.MapFile{Data: []byte(data)}
		}
		found, err = loadScenarios(mapFS)
	}
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// newSession opens a session in the harness's working folder.
func (h *harness) newSession() acp.SessionId {
	h.t.Helper()
	resp, err := h.conn.NewSession(h.ctx, acp.NewSessionRequest{Cwd: h.cwd, McpServers: []acp.McpServer{}})
	if err != nil {
		h.t.Fatalf("new session: %v", err)
	}
	return resp.SessionId
}

// prompt sends one text prompt.
func (h *harness) prompt(sid acp.SessionId, text string) (acp.PromptResponse, error) {
	h.t.Helper()
	return h.conn.Prompt(h.ctx, acp.PromptRequest{
		SessionId: sid,
		Prompt:    []acp.ContentBlock{acp.TextBlock(text)},
	})
}

// mustPrompt sends a prompt that must end normally.
func (h *harness) mustPrompt(sid acp.SessionId, text string) {
	h.t.Helper()
	resp, err := h.prompt(sid, text)
	if err != nil {
		h.t.Fatalf("prompt %q: %v", text, err)
	}
	if resp.StopReason != acp.StopReasonEndTurn {
		h.t.Fatalf("prompt %q stopped with %q", text, resp.StopReason)
	}
}

// readFile returns a file of the harness's working folder.
func (h *harness) readFile(rel string) (string, error) {
	data, err := os.ReadFile(filepath.Join(h.cwd, rel))
	return string(data), err
}

// scenarioFile builds the JSON text of a scenario from its steps.
func scenarioFile(t *testing.T, steps ...map[string]any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"steps": steps})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// hangFiles is a scenario set with one scenario that says something and then waits for a very
// long time, so only a cancel can end it.
func hangFiles(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"default.json": scenarioFile(t,
			map[string]any{"type": "say", "text": "Starting."},
			map[string]any{"type": "pause", "ms": maxPauseMs},
		),
	}
}
