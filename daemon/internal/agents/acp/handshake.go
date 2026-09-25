package acp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/buildinfo"
	"github.com/khanblair/marshal/daemon/internal/proc"
)

const (
	clientName = "marshal"
	// authRequiredCode is the protocol's error code for "sign in first".
	authRequiredCode = -32000
	// abandonGrace is how long a process that failed to start gets to exit before it is killed.
	abandonGrace = 500 * time.Millisecond
	// errorTailBytes is how much of the agent's last output goes into a start error.
	errorTailBytes = 1024
)

// open starts a process and a session in it: a new session, or the one named by resumeID.
func (a *Adapter) open(ctx context.Context, spec agents.StartSpec, resumeID string) (agents.SessionHandle, error) {
	if !filepath.IsAbs(spec.Cwd) {
		return agents.SessionHandle{}, fmt.Errorf("the working folder %q must be an absolute path", spec.Cwd)
	}
	// The session outlives this call, so its lifetime must not end with the caller's context.
	life, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := newSession(a, spec, life, cancel)
	if err := s.launch(spec); err != nil {
		cancel()
		return agents.SessionHandle{}, err
	}
	startCtx, cancelStart := context.WithTimeout(ctx, a.cfg.StartTimeout)
	defer cancelStart()
	handle, caps, err := s.handshake(startCtx, spec, resumeID)
	if err == nil {
		err = a.register(handle.ID, s, caps)
	}
	if err != nil {
		return agents.SessionHandle{}, s.abandon(ctx, err)
	}
	s.log.Info("agent session started", "session_id", handle.ID, "resumed", resumeID != "")
	go s.supervise()
	return handle, nil
}

// launch starts the process and the protocol connection to it.
func (s *session) launch(spec agents.StartSpec) error {
	cfg := s.adapter.cfg
	p, err := proc.Start(s.life, proc.Spec{
		Path:  cfg.Path,
		Args:  cfg.Args,
		Dir:   spec.Cwd,
		Env:   slices.Concat(cfg.Env, spec.Env),
		Stdin: true,
	})
	if err != nil {
		return fmt.Errorf("start the agent program: %w", err)
	}
	s.proc = p
	s.conn = sdk.NewClientSideConnection(&client{s: s}, p.Stdin, p.Stdout)
	s.conn.SetLogger(connectionLogger(s.log))
	return nil
}

// abandon cleans up after a start that failed, and returns the error to report. The process is
// stopped, the connection is let go, and what the agent printed last is added to the error.
func (s *session) abandon(ctx context.Context, cause error) error {
	if err := s.proc.Stop(context.WithoutCancel(ctx), abandonGrace); err != nil {
		s.log.Warn("could not stop an agent that failed to start", "err", err)
	}
	s.proc.Wait()
	s.markEnded()
	s.waitConnection()
	s.lifeCancel()
	if tail := strings.TrimSpace(endOf(s.proc.StderrTail(), errorTailBytes)); tail != "" {
		return fmt.Errorf("%w (the agent printed: %s)", cause, tail)
	}
	return cause
}

// endOf returns the last n bytes of text, without a broken character at the start.
func endOf(text string, n int) string {
	if len(text) <= n {
		return text
	}
	cut := len(text) - n
	for cut < len(text) && !utf8.RuneStart(text[cut]) {
		cut++
	}
	return text[cut:]
}

// handshake brings the session up: hello, sign-in if needed, the session itself, and its
// settings. Updates that arrive meanwhile are dropped, and they only count once it is over.
func (s *session) handshake(
	ctx context.Context, spec agents.StartSpec, resumeID string,
) (agents.SessionHandle, agents.Capabilities, error) {
	hello, err := s.initialize(ctx)
	if err != nil {
		return agents.SessionHandle{}, agents.Capabilities{}, err
	}
	s.closeSupported = hello.AgentCapabilities.SessionCapabilities.Close != nil
	var ctl controls
	if resumeID == "" {
		ctl, err = s.newSession(ctx, spec.Cwd, hello.AuthMethods)
	} else {
		ctl, err = s.resumeSession(ctx, resumeID, spec.Cwd, hello)
	}
	if err != nil {
		return agents.SessionHandle{}, agents.Capabilities{}, err
	}
	applied, err := s.applySettings(ctx, spec, ctl)
	if err != nil {
		return agents.SessionHandle{}, agents.Capabilities{}, err
	}
	caps := capabilitiesOf(hello.AgentCapabilities, ctl)
	s.setAccepting(true)
	handle := agents.SessionHandle{
		ID: s.sessionID(), Label: spec.Label,
		Model: spec.Model, Thinking: spec.Thinking, PermissionMode: spec.PermissionMode,
		Applied: applied, Capabilities: caps,
	}
	return handle, caps, nil
}

// initialize says hello and checks that both sides speak the same version. The adapter offers no
// file system and no terminal, so the agent has to use its own tools.
func (s *session) initialize(ctx context.Context) (sdk.InitializeResponse, error) {
	resp, err := s.conn.Initialize(ctx, sdk.InitializeRequest{
		ProtocolVersion:    sdk.ProtocolVersionNumber,
		ClientCapabilities: sdk.ClientCapabilities{},
		ClientInfo:         &sdk.Implementation{Name: clientName, Version: buildinfo.Version},
	})
	if err != nil {
		return resp, fmt.Errorf("say hello to the agent: %w", err)
	}
	if resp.ProtocolVersion != sdk.ProtocolVersionNumber {
		return resp, fmt.Errorf("the agent speaks protocol version %d, and Marshal speaks version %d",
			resp.ProtocolVersion, sdk.ProtocolVersionNumber)
	}
	return resp, nil
}

// controls are the settings that a session offers: modes, and options such as the model.
type controls struct {
	modes   *sdk.SessionModeState
	options []sdk.SessionConfigOption
}

// newSession asks the agent for a new session.
func (s *session) newSession(ctx context.Context, cwd string, methods []sdk.AuthMethod) (controls, error) {
	var resp sdk.NewSessionResponse
	err := s.authRetry(ctx, methods, func(ctx context.Context) error {
		var err error
		resp, err = s.conn.NewSession(ctx, sdk.NewSessionRequest{Cwd: cwd, McpServers: noServers()})
		return err
	})
	if err != nil {
		return controls{}, fmt.Errorf("start a session: %w", err)
	}
	s.setID(resp.SessionId)
	return controls{modes: resp.Modes, options: resp.ConfigOptions}, nil
}

// resumeSession picks up an earlier session: with the resume request if the agent has it, and
// else by loading the session, which replays its history. The replay is dropped, because the
// session logs already hold it.
func (s *session) resumeSession(
	ctx context.Context, id, cwd string, hello sdk.InitializeResponse,
) (controls, error) {
	s.setID(sdk.SessionId(id))
	caps := hello.AgentCapabilities
	if caps.SessionCapabilities.Resume == nil && !caps.LoadSession {
		return controls{}, fmt.Errorf("%w: the agent can neither resume nor load a session",
			agents.ErrCannotResume)
	}
	var ctl controls
	call := func(ctx context.Context) error {
		var err error
		ctl, err = s.reopen(ctx, id, cwd, caps.SessionCapabilities.Resume != nil)
		return err
	}
	err := s.authRetry(ctx, hello.AuthMethods, call)
	var reqErr *sdk.RequestError
	if errors.As(err, &reqErr) {
		// The agent answered, and said no: it does not know the session any more.
		return controls{}, fmt.Errorf("%w: %v", agents.ErrCannotResume, reqErr.Message)
	}
	if err != nil {
		return controls{}, fmt.Errorf("resume the session: %w", err)
	}
	return ctl, nil
}

// reopen sends the request that brings an earlier session back: resume, or else load.
func (s *session) reopen(ctx context.Context, id, cwd string, resume bool) (controls, error) {
	sid := sdk.SessionId(id)
	if resume {
		resp, err := s.conn.ResumeSession(ctx, sdk.ResumeSessionRequest{SessionId: sid, Cwd: cwd})
		return controls{resp.Modes, resp.ConfigOptions}, err
	}
	resp, err := s.conn.LoadSession(ctx, sdk.LoadSessionRequest{SessionId: sid, Cwd: cwd, McpServers: noServers()})
	return controls{resp.Modes, resp.ConfigOptions}, err
}

// noServers is the list of MCP servers that a session is given: none yet. The protocol wants the
// list to exist.
func noServers() []sdk.McpServer { return []sdk.McpServer{} }

// setID records the agent's session id.
func (s *session) setID(id sdk.SessionId) {
	s.mu.Lock()
	s.id = id
	s.mu.Unlock()
}

// authRetry runs a call, and if the agent says that it needs a sign-in, signs in and runs the
// call once more. Signing in only happens on demand, because an agent that is already signed in
// does not need it, and only with a method that the config names, because some agents open a
// browser when asked.
func (s *session) authRetry(
	ctx context.Context, methods []sdk.AuthMethod, call func(context.Context) error,
) error {
	err := call(ctx)
	var reqErr *sdk.RequestError
	if !errors.As(err, &reqErr) || reqErr.Code != authRequiredCode {
		return err
	}
	if err := s.authenticate(ctx, methods); err != nil {
		return err
	}
	return call(ctx)
}

// authenticate signs in with the first method that the config allows to be used on its own. When
// there is none, or it fails, it returns an error that says which ways the user has.
func (s *session) authenticate(ctx context.Context, methods []sdk.AuthMethod) error {
	for _, m := range methods {
		if m.Agent == nil || !slices.Contains(s.adapter.cfg.AutoAuthMethods, m.Agent.Id) {
			continue
		}
		_, err := s.conn.Authenticate(ctx, sdk.AuthenticateRequest{MethodId: m.Agent.Id})
		if err == nil {
			return nil
		}
		s.log.Debug("automatic sign-in failed", "method", m.Agent.Id, "err", err)
		return &agents.AuthRequiredError{Methods: methodNames(methods), Cause: err}
	}
	return &agents.AuthRequiredError{Methods: methodNames(methods)}
}

// methodNames lists the human names of the sign-in methods.
func methodNames(methods []sdk.AuthMethod) []string {
	names := make([]string, 0, len(methods))
	for _, m := range methods {
		switch {
		case m.Agent != nil:
			names = append(names, m.Agent.Name)
		case m.EnvVar != nil:
			names = append(names, m.EnvVar.Name)
		case m.Terminal != nil:
			names = append(names, m.Terminal.Name)
		}
	}
	return names
}

// capabilitiesOf describes what a session's agent can do.
func capabilitiesOf(c sdk.AgentCapabilities, ctl controls) agents.Capabilities {
	return agents.Capabilities{
		Resume:           c.SessionCapabilities.Resume != nil || c.LoadSession,
		LoadSession:      c.LoadSession,
		StructuredEvents: true,
		ModelSwitching:   ctl.selectOption(sdk.SessionConfigOptionCategoryModel) != nil,
		Thinking:         ctl.selectOption(sdk.SessionConfigOptionCategoryThoughtLevel) != nil,
		MCP:              c.McpCapabilities.Http || c.McpCapabilities.Sse,
	}
}
