package gemini

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	sdk "github.com/coder/acp-go-sdk"
)

// fakeEnv switches the fake on. Its value starts with a word of its own, and the rest is a comma
// separated list of behaviors: "signin" (the session request says that a sign-in is needed),
// "noplan" (the plan mode is not offered), and "authcall" (the fake ends the run if the client
// asks it to authenticate, which Marshal must never do).
const fakeEnv = "MARSHAL_FAKE_GEMINI"

// fakeGemini is the agent side of the fake. It offers what Gemini CLI 0.35.1 offers on the
// protocol: the loadSession request and no resume request, approval modes named default, autoEdit,
// yolo, and plan, no model or thinking option, and four ways to sign in.
type fakeGemini struct {
	flags map[string]bool
	conn  *sdk.AgentSideConnection

	mu   sync.Mutex
	mode string
}

var _ sdk.Agent = (*fakeGemini)(nil)
var _ sdk.AgentLoader = (*fakeGemini)(nil)

// runFakeGemini serves the protocol on standard input and output until the client goes away.
func runFakeGemini(spec string) {
	f := &fakeGemini{flags: map[string]bool{}, mode: "default"}
	for _, flag := range strings.Split(spec, ",") {
		f.flags[flag] = true
	}
	f.conn = sdk.NewAgentSideConnection(f, os.Stdout, os.Stdin)
	<-f.conn.Done()
}

func (f *fakeGemini) Initialize(context.Context, sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	caps := sdk.AgentCapabilities{LoadSession: true}
	caps.McpCapabilities.Http = true
	caps.McpCapabilities.Sse = true
	methods := []sdk.AuthMethod{
		{Agent: &sdk.AuthMethodAgent{Id: "oauth-personal", Name: "Log in with Google"}},
		{Agent: &sdk.AuthMethodAgent{Id: "gemini-api-key", Name: "Gemini API key"}},
		{Agent: &sdk.AuthMethodAgent{Id: "vertex-ai", Name: "Vertex AI"}},
		{Agent: &sdk.AuthMethodAgent{Id: "gateway", Name: "AI API Gateway"}},
	}
	return sdk.InitializeResponse{
		ProtocolVersion: sdk.ProtocolVersionNumber, AgentCapabilities: caps, AuthMethods: methods,
	}, nil
}

func (f *fakeGemini) Authenticate(context.Context, sdk.AuthenticateRequest) (sdk.AuthenticateResponse, error) {
	if f.flags["authcall"] {
		fmt.Fprintln(os.Stderr, "the client asked to authenticate")
		os.Exit(9)
	}
	return sdk.AuthenticateResponse{}, nil
}

// modes are the approval modes that the fake offers.
func (f *fakeGemini) modes() *sdk.SessionModeState {
	f.mu.Lock()
	defer f.mu.Unlock()
	available := []sdk.SessionMode{
		{Id: "default", Name: "Default"}, {Id: "autoEdit", Name: "Auto Edit"}, {Id: "yolo", Name: "YOLO"},
	}
	if !f.flags["noplan"] {
		available = append(available, sdk.SessionMode{Id: "plan", Name: "Plan"})
	}
	return &sdk.SessionModeState{CurrentModeId: sdk.SessionModeId(f.mode), AvailableModes: available}
}

func (f *fakeGemini) NewSession(context.Context, sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	if f.flags["signin"] {
		return sdk.NewSessionResponse{}, sdk.NewAuthRequired(nil)
	}
	return sdk.NewSessionResponse{SessionId: "gemini-session-1", Modes: f.modes()}, nil
}

func (f *fakeGemini) LoadSession(context.Context, sdk.LoadSessionRequest) (sdk.LoadSessionResponse, error) {
	if f.flags["signin"] {
		return sdk.LoadSessionResponse{}, sdk.NewAuthRequired(nil)
	}
	return sdk.LoadSessionResponse{Modes: f.modes()}, nil
}

func (f *fakeGemini) ResumeSession(context.Context, sdk.ResumeSessionRequest) (sdk.ResumeSessionResponse, error) {
	return sdk.ResumeSessionResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodSessionResume)
}

func (f *fakeGemini) SetSessionMode(_ context.Context, req sdk.SetSessionModeRequest) (sdk.SetSessionModeResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = string(req.ModeId)
	return sdk.SetSessionModeResponse{}, nil
}

func (f *fakeGemini) SetSessionConfigOption(
	context.Context, sdk.SetSessionConfigOptionRequest,
) (sdk.SetSessionConfigOptionResponse, error) {
	return sdk.SetSessionConfigOptionResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodSessionSetConfigOption)
}

func (f *fakeGemini) CloseSession(context.Context, sdk.CloseSessionRequest) (sdk.CloseSessionResponse, error) {
	return sdk.CloseSessionResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodSessionClose)
}

func (f *fakeGemini) ListSessions(context.Context, sdk.ListSessionsRequest) (sdk.ListSessionsResponse, error) {
	return sdk.ListSessionsResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodSessionList)
}

func (f *fakeGemini) Logout(context.Context, sdk.LogoutRequest) (sdk.LogoutResponse, error) {
	return sdk.LogoutResponse{}, sdk.NewMethodNotFound(sdk.AgentMethodLogout)
}

func (f *fakeGemini) Cancel(context.Context, sdk.CancelNotification) error { return nil }

// Prompt answers with what the process was started with, so a test can see how Gemini CLI would
// have been configured: the mode it is in, the model it reads from its environment, and its
// arguments.
func (f *fakeGemini) Prompt(ctx context.Context, req sdk.PromptRequest) (sdk.PromptResponse, error) {
	f.mu.Lock()
	mode := f.mode
	f.mu.Unlock()
	summary := fmt.Sprintf("mode=%s model=%s no_browser=%s args=%s",
		mode, os.Getenv("GEMINI_MODEL"), os.Getenv("NO_BROWSER"), strings.Join(os.Args[1:], " "))
	err := f.conn.SessionUpdate(ctx, sdk.SessionNotification{
		SessionId: req.SessionId, Update: sdk.UpdateAgentMessageText(summary),
	})
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, err
}
