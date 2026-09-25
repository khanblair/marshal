package acp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	sdk "github.com/coder/acp-go-sdk"
)

// fakeEnv selects the fake agent: this test binary, started again as a child, speaking the
// protocol. Its value is a comma separated list of behaviors. The stub agent covers the normal
// paths, and the fake covers what the stub does not do: sign-in, modes and options, the older way
// to resume, protocol errors, and crashes.
const fakeEnv = "MARSHAL_FAKE_AGENT"

const (
	fakeSessionID = "fake-1"
	bigOutput     = 20 << 10
)

// fakeAgent is the agent side of the fake.
type fakeAgent struct {
	flags map[string]bool
	conn  *sdk.AgentSideConnection

	mu       sync.Mutex
	signedIn bool
	turns    int
	history  int
	mode     string
	model    string
	thinking string
}

var _ sdk.Agent = (*fakeAgent)(nil)
var _ sdk.AgentLoader = (*fakeAgent)(nil)

// runFakeAgent serves the protocol on standard input and output until the client goes away.
func runFakeAgent(spec string) {
	f := &fakeAgent{flags: map[string]bool{}, mode: "default", model: "sonnet", thinking: "low"}
	for _, flag := range strings.Split(spec, ",") {
		f.flags[flag] = true
	}
	if f.flags["crash-init"] {
		fmt.Fprintln(os.Stderr, "boom: the agent cannot start")
		os.Exit(3)
	}
	f.conn = sdk.NewAgentSideConnection(f, os.Stdout, os.Stdin)
	<-f.conn.Done()
}

func (f *fakeAgent) Initialize(context.Context, sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	if f.flags["hang-init"] {
		select {}
	}
	version := sdk.ProtocolVersionNumber
	if f.flags["bad-version"] {
		version = sdk.ProtocolVersionNumber + 1
	}
	caps := sdk.AgentCapabilities{LoadSession: f.flags["load"]}
	if f.flags["resume"] {
		caps.SessionCapabilities.Resume = &sdk.SessionResumeCapabilities{}
	}
	if f.flags["close"] {
		caps.SessionCapabilities.Close = &sdk.SessionCloseCapabilities{}
	}
	caps.McpCapabilities.Http = f.flags["mcp"]
	var methods []sdk.AuthMethod
	if f.flags["auth-agent"] {
		methods = append(methods, sdk.AuthMethod{Agent: &sdk.AuthMethodAgent{Id: "login", Name: "Log in"}})
	}
	if f.flags["auth-terminal"] {
		methods = append(methods, sdk.AuthMethod{
			Terminal: &sdk.AuthMethodTerminalInline{Id: "tty", Name: "Terminal login", Type: "terminal"},
		})
	}
	return sdk.InitializeResponse{
		ProtocolVersion: sdk.ProtocolVersion(version), AgentCapabilities: caps, AuthMethods: methods,
	}, nil
}

func (f *fakeAgent) Authenticate(_ context.Context, req sdk.AuthenticateRequest) (sdk.AuthenticateResponse, error) {
	if f.flags["auth-fails"] || req.MethodId != "login" {
		return sdk.AuthenticateResponse{}, sdk.NewInvalidParams(map[string]any{"error": "no such account"})
	}
	f.mu.Lock()
	f.signedIn = true
	f.mu.Unlock()
	return sdk.AuthenticateResponse{}, nil
}

// needsSignIn says whether a call must be refused until the client has signed in.
func (f *fakeAgent) needsSignIn() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return (f.flags["auth-agent"] || f.flags["auth-terminal"]) && !f.signedIn
}

func (f *fakeAgent) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	if f.needsSignIn() {
		return sdk.NewSessionResponse{}, sdk.NewAuthRequired(nil)
	}
	modes, options := f.controls()
	return sdk.NewSessionResponse{SessionId: fakeSessionID, Modes: modes, ConfigOptions: options}, nil
}

func (f *fakeAgent) LoadSession(ctx context.Context, req sdk.LoadSessionRequest) (sdk.LoadSessionResponse, error) {
	if f.flags["unknown-session"] {
		return sdk.LoadSessionResponse{}, sdk.NewInvalidParams(map[string]any{"error": "no such session"})
	}
	f.mu.Lock()
	f.history = 1
	f.mu.Unlock()
	for _, update := range []sdk.SessionUpdate{
		sdk.UpdateUserMessageText("old question"), sdk.UpdateAgentMessageText("old answer"),
	} {
		if err := f.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: req.SessionId, Update: update}); err != nil {
			return sdk.LoadSessionResponse{}, err
		}
	}
	modes, options := f.controls()
	return sdk.LoadSessionResponse{Modes: modes, ConfigOptions: options}, nil
}

func (f *fakeAgent) ResumeSession(context.Context, sdk.ResumeSessionRequest) (sdk.ResumeSessionResponse, error) {
	if f.flags["unknown-session"] {
		return sdk.ResumeSessionResponse{}, sdk.NewInvalidParams(map[string]any{"error": "no such session"})
	}
	f.mu.Lock()
	f.history = 1
	f.mu.Unlock()
	modes, options := f.controls()
	return sdk.ResumeSessionResponse{Modes: modes, ConfigOptions: options}, nil
}

// controls are the modes and options that the "controls" behavior offers.
func (f *fakeAgent) controls() (*sdk.SessionModeState, []sdk.SessionConfigOption) {
	if !f.flags["controls"] {
		return nil, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	modes := &sdk.SessionModeState{
		CurrentModeId: sdk.SessionModeId(f.mode),
		AvailableModes: []sdk.SessionMode{
			{Id: "default", Name: "Default"}, {Id: "acceptEdits", Name: "Accept edits"},
			{Id: "plan", Name: "Plan"}, {Id: "bypassPermissions", Name: "Bypass permissions"},
		},
	}
	model := ungroupedOption(selectSpec{"model", "Model", sdk.SessionConfigOptionCategoryModel, f.model,
		[]string{"sonnet", "opus"}})
	thinking := groupedOption(selectSpec{"thinking", "Thinking", sdk.SessionConfigOptionCategoryThoughtLevel,
		f.thinking, []string{"low", "high"}})
	options := []sdk.SessionConfigOption{model, thinking}
	if f.flags["mode-option"] {
		modes = nil
		options = append(options, ungroupedOption(selectSpec{"mode", "Mode", sdk.SessionConfigOptionCategoryMode,
			f.mode, []string{"default", "plan"}}))
	}
	return modes, options
}

// selectSpec describes a select option that the fake offers.
type selectSpec struct {
	id      string
	title   string
	cat     sdk.SessionConfigOptionCategory
	current string
	values  []string
}

// ungroupedOption builds a select option with a flat list of values.
func ungroupedOption(spec selectSpec) sdk.SessionConfigOption {
	list := make(sdk.SessionConfigSelectOptionsUngrouped, 0, len(spec.values))
	for _, v := range spec.values {
		list = append(list, sdk.SessionConfigSelectOption{
			Name: strings.ToUpper(v[:1]) + v[1:], Value: sdk.SessionConfigValueId(v),
		})
	}
	return sdk.SessionConfigOption{Select: &sdk.SessionConfigOptionSelect{
		Id: sdk.SessionConfigId(spec.id), Name: spec.title, Category: &spec.cat, Type: "select",
		CurrentValue: sdk.SessionConfigValueId(spec.current),
		Options:      sdk.SessionConfigSelectOptions{Ungrouped: &list},
	}}
}

// groupedOption builds a select option whose values are inside one group.
func groupedOption(spec selectSpec) sdk.SessionConfigOption {
	opt := ungroupedOption(spec)
	grouped := sdk.SessionConfigSelectOptionsGrouped{
		{Group: "levels", Name: "Levels", Options: *opt.Select.Options.Ungrouped},
	}
	opt.Select.Options = sdk.SessionConfigSelectOptions{Grouped: &grouped}
	return opt
}

func (f *fakeAgent) SetSessionMode(_ context.Context, req sdk.SetSessionModeRequest) (sdk.SetSessionModeResponse, error) {
	f.mu.Lock()
	f.mode = string(req.ModeId)
	f.mu.Unlock()
	return sdk.SetSessionModeResponse{}, nil
}

func (f *fakeAgent) SetSessionConfigOption(
	_ context.Context, req sdk.SetSessionConfigOptionRequest,
) (sdk.SetSessionConfigOptionResponse, error) {
	if req.ValueId == nil {
		return sdk.SetSessionConfigOptionResponse{}, sdk.NewInvalidParams(nil)
	}
	f.mu.Lock()
	switch req.ValueId.ConfigId {
	case "model":
		f.model = string(req.ValueId.Value)
	case "thinking":
		f.thinking = string(req.ValueId.Value)
	case "mode":
		f.mode = string(req.ValueId.Value)
	}
	f.mu.Unlock()
	return sdk.SetSessionConfigOptionResponse{ConfigOptions: []sdk.SessionConfigOption{}}, nil
}

func (f *fakeAgent) Cancel(context.Context, sdk.CancelNotification) error { return nil }

func (f *fakeAgent) CloseSession(context.Context, sdk.CloseSessionRequest) (sdk.CloseSessionResponse, error) {
	return sdk.CloseSessionResponse{}, nil
}

func (f *fakeAgent) ListSessions(context.Context, sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error) {
	return sdk.ListSessionsResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodSessionList)
}

func (f *fakeAgent) Logout(context.Context, sdk.LogoutRequest) (sdk.LogoutResponse, error) {
	return sdk.LogoutResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodLogout)
}

// Prompt plays one turn. The text of the prompt can pick a behavior: @rich, @stop:<reason>.
func (f *fakeAgent) Prompt(ctx context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	text := fakePromptText(req.Prompt)
	switch {
	case f.flags["prompt-error"]:
		return sdk.PromptResponse{}, sdk.NewInternalError(map[string]any{"error": "the model is overloaded"})
	case f.flags["exit-on-prompt"]:
		fmt.Fprintln(os.Stderr, "fatal: out of memory")
		os.Exit(1)
	case f.flags["hang"]:
		// Never answer, and ignore the cancel request too.
		select {}
	}
	f.mu.Lock()
	f.turns++
	summary := fmt.Sprintf("turn=%d history=%d mode=%s model=%s thinking=%s prompt=%q",
		f.turns, f.history, f.mode, f.model, f.thinking, text)
	f.mu.Unlock()
	if err := f.update(ctx, req.SessionId, sdk.UpdateAgentMessageText(summary)); err != nil {
		return sdk.PromptResponse{}, err
	}
	if strings.Contains(text, "@rich") {
		if err := f.richTurn(ctx, req.SessionId); err != nil {
			return sdk.PromptResponse{}, err
		}
	}
	return sdk.PromptResponse{StopReason: stopReasonIn(text)}, nil
}

// stopReasonIn reads an @stop:<reason> marker, and defaults to a normal end.
func stopReasonIn(text string) sdk.StopReason {
	_, rest, found := strings.Cut(text, "@stop:")
	if !found {
		return sdk.StopReasonEndTurn
	}
	reason, _, _ := strings.Cut(rest, " ")
	return sdk.StopReason(reason)
}

// update sends one session update.
func (f *fakeAgent) update(ctx context.Context, id sdk.SessionId, u sdk.SessionUpdate) error {
	return f.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: id, Update: u})
}

// richTurn sends the kinds of update that the stub does not: thoughts, plans, big tool output,
// diffs, content that is ignored, and a call to a client method that is not offered.
func (f *fakeAgent) richTurn(ctx context.Context, id sdk.SessionId) error {
	big := strings.Repeat("x", bigOutput)
	updates := []sdk.SessionUpdate{
		sdk.UpdateAgentThoughtText("thinking hard"),
		sdk.UpdateUserMessageText("an echo that is ignored"),
		sdk.UpdateAgentMessage(sdk.ImageBlock("aGk=", "image/png")),
		sdk.UpdateAgentMessageText(""),
		sdk.UpdatePlan(
			sdk.PlanEntry{Content: "Read the code", Status: sdk.PlanEntryStatusCompleted, Priority: sdk.PlanEntryPriorityHigh},
			sdk.PlanEntry{Content: "Fix it", Status: sdk.PlanEntryStatusInProgress, Priority: sdk.PlanEntryPriorityMedium},
		),
		sdk.StartToolCall("t1", "Read big.txt",
			sdk.WithStartKind(sdk.ToolKindRead),
			sdk.WithStartRawInput(map[string]any{"file_path": "big.txt", "command": "cat big.txt"}),
			sdk.WithStartContent([]sdk.ToolCallContent{sdk.ToolContent(sdk.TextBlock(big))}),
		),
		sdk.UpdateToolCall("t1",
			sdk.WithUpdateStatus(sdk.ToolCallStatusCompleted),
			sdk.WithUpdateTitle("Read big.txt fully"),
			sdk.WithUpdateContent([]sdk.ToolCallContent{sdk.ToolDiffContent("/work/a.txt", "new text", "old text")}),
		),
	}
	for _, u := range updates {
		if err := f.update(ctx, id, u); err != nil {
			return err
		}
	}
	_, err := f.conn.ReadTextFile(ctx, sdk.ReadTextFileRequest{SessionId: id, Path: "/etc/hosts"})
	verdict := "the client read the file"
	if re, ok := err.(*sdk.RequestError); ok && re.Code == -32601 {
		verdict = "the client says method not found"
	}
	return f.update(ctx, id, sdk.UpdateAgentMessageText(verdict))
}

// fakePromptText joins the text blocks of a prompt with a separator that shows the block borders.
func fakePromptText(blocks []sdk.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Text != nil {
			parts = append(parts, b.Text.Text)
		}
	}
	return strings.Join(parts, "|")
}
