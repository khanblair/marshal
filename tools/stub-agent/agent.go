package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/coder/acp-go-sdk"
)

const (
	agentName            = "marshal-stub-agent"
	version              = "0.0.0"
	scenarioMarkerPrefix = "@scenario:"
)

// stubAgent is the fake agent. It answers every prompt by playing a scenario, and it keeps each
// session on disk so a new process can pick the session up again.
type stubAgent struct {
	scenarios       map[string]scenario
	defaultScenario string
	speed           float64
	store           store

	// ready closes once attach has set peer. The connection starts reading before the caller has
	// a chance to hand it over, so a request that arrives early waits here instead of racing.
	ready      chan struct{}
	attachOnce sync.Once
	peer       peer

	mu       sync.Mutex
	sessions map[acp.SessionId]*session
}

var (
	_ acp.Agent       = (*stubAgent)(nil)
	_ acp.AgentLoader = (*stubAgent)(nil)
)

// newAgent builds an agent that plays the given scenarios. The default scenario must exist.
func newAgent(cfg config, scenarios map[string]scenario) (*stubAgent, error) {
	if _, ok := scenarios[cfg.scenario]; !ok {
		return nil, fmt.Errorf("unknown scenario %q (known: %s)", cfg.scenario, scenarioNames(scenarios))
	}
	return &stubAgent{
		scenarios:       scenarios,
		defaultScenario: cfg.scenario,
		speed:           cfg.speed,
		store:           store{dir: cfg.stateDir},
		ready:           make(chan struct{}),
		sessions:        make(map[acp.SessionId]*session),
	}, nil
}

// serve starts speaking ACP over the given streams. Standard output is the protocol channel, so
// nothing else may write to it.
func serve(agent *stubAgent, out io.Writer, in io.Reader) *acp.AgentSideConnection {
	conn := acp.NewAgentSideConnection(agent, out, in)
	agent.attachOnce.Do(func() {
		agent.peer = conn
		close(agent.ready)
	})
	return conn
}

// connection returns the client side of the connection, waiting for serve to hand it over.
func (a *stubAgent) connection(ctx context.Context) (peer, error) {
	select {
	case <-a.ready:
		return a.peer, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for the connection: %w", ctx.Err())
	}
}

// Initialize says what this agent can do. Resume and close are advertised through the session
// capabilities, and loadSession is the older flag for replaying history.
func (*stubAgent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentInfo:       &acp.Implementation{Name: agentName, Version: version},
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: true,
			SessionCapabilities: acp.SessionCapabilities{
				Resume: &acp.SessionResumeCapabilities{},
				Close:  &acp.SessionCloseCapabilities{},
			},
		},
	}, nil
}

// Authenticate accepts everything, because the stub has no accounts.
func (*stubAgent) Authenticate(
	context.Context, acp.AuthenticateRequest,
) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

// NewSession starts a session and saves it at once, so a new process can load it before the first
// turn has finished.
func (a *stubAgent) NewSession(
	_ context.Context, req acp.NewSessionRequest,
) (acp.NewSessionResponse, error) {
	cwd, err := workingFolder(req.Cwd)
	if err != nil {
		return acp.NewSessionResponse{}, err
	}
	st := sessionState{ID: newSessionID(), Cwd: cwd}
	if err := a.store.save(st); err != nil {
		return acp.NewSessionResponse{}, fmt.Errorf("create session: %w", err)
	}
	a.adopt(acp.SessionId(st.ID), newSession(st))
	return acp.NewSessionResponse{SessionId: acp.SessionId(st.ID)}, nil
}

// LoadSession restores a saved session and replays its history to the client.
func (a *stubAgent) LoadSession(
	ctx context.Context, req acp.LoadSessionRequest,
) (acp.LoadSessionResponse, error) {
	sess, err := a.restore(req.SessionId, req.Cwd)
	if err != nil {
		return acp.LoadSessionResponse{}, err
	}
	if err := a.replay(ctx, req.SessionId, sess.snapshot().Turns); err != nil {
		return acp.LoadSessionResponse{}, err
	}
	return acp.LoadSessionResponse{}, nil
}

// ResumeSession restores a saved session without replaying anything.
func (a *stubAgent) ResumeSession(
	_ context.Context, req acp.ResumeSessionRequest,
) (acp.ResumeSessionResponse, error) {
	if _, err := a.restore(req.SessionId, req.Cwd); err != nil {
		return acp.ResumeSessionResponse{}, err
	}
	return acp.ResumeSessionResponse{}, nil
}

// Prompt plays one scenario as one turn and saves the turn afterwards, also when it was cancelled.
func (a *stubAgent) Prompt(ctx context.Context, req acp.PromptRequest) (acp.PromptResponse, error) {
	sess, err := a.session(req.SessionId)
	if err != nil {
		return acp.PromptResponse{}, err
	}
	user := promptText(req.Prompt)
	sc, err := a.scenarioFor(user)
	if err != nil {
		return acp.PromptResponse{}, err
	}
	client, err := a.connection(ctx)
	if err != nil {
		return acp.PromptResponse{}, err
	}
	turnCtx, end, err := sess.beginTurn(ctx)
	if err != nil {
		return acp.PromptResponse{}, err
	}
	defer end()

	st := sess.snapshot()
	r := &runner{
		peer: client, sid: req.SessionId, cwd: st.Cwd, speed: a.speed,
		turn: st.Turn + 1, earlier: len(st.Turns),
	}
	stop, err := r.play(turnCtx, sc.Steps)
	if turnCtx.Err() != nil {
		// A cancelled turn is not an error. The client gets the stop reason it asked for.
		stop, err = acp.StopReasonCancelled, nil
	}
	if err != nil {
		return acp.PromptResponse{}, fmt.Errorf("scenario %s: %w", sc.Name, err)
	}
	if err := sess.record(a.store, user, r.transcript()); err != nil {
		return acp.PromptResponse{}, err
	}
	return acp.PromptResponse{StopReason: stop}, nil
}

// Cancel stops the running turn of a session. The connection cancels the prompt's own context as
// well, and this covers a caller that reaches the agent without the connection.
func (a *stubAgent) Cancel(_ context.Context, req acp.CancelNotification) error {
	if sess := a.find(req.SessionId); sess != nil {
		sess.stop()
	}
	return nil
}

// CloseSession stops the running turn and forgets the open session. The saved file stays, so the
// session can be loaded or resumed later.
func (a *stubAgent) CloseSession(
	_ context.Context, req acp.CloseSessionRequest,
) (acp.CloseSessionResponse, error) {
	a.mu.Lock()
	sess := a.sessions[req.SessionId]
	delete(a.sessions, req.SessionId)
	a.mu.Unlock()
	if sess != nil {
		sess.stop()
	}
	return acp.CloseSessionResponse{}, nil
}

// find returns the open session with this id, or nil.
func (a *stubAgent) find(id acp.SessionId) *session {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessions[id]
}

// session returns the open session with this id, or an error the client can read.
func (a *stubAgent) session(id acp.SessionId) (*session, error) {
	if sess := a.find(id); sess != nil {
		return sess, nil
	}
	return nil, unknownSession(id, "load or resume it first")
}

// adopt records a session as open. If another call opened the same id first, that one wins.
func (a *stubAgent) adopt(id acp.SessionId, sess *session) *session {
	a.mu.Lock()
	defer a.mu.Unlock()
	if open, ok := a.sessions[id]; ok {
		return open
	}
	a.sessions[id] = sess
	return sess
}

// restore opens a saved session, unless it is already open, and moves it to the working folder
// the client names, because the client is the one that knows where the work is now.
func (a *stubAgent) restore(id acp.SessionId, cwd string) (*session, error) {
	folder, err := workingFolder(cwd)
	if err != nil {
		return nil, err
	}
	sess := a.find(id)
	if sess == nil {
		st, err := a.store.load(string(id))
		if errors.Is(err, errSessionNotFound) {
			return nil, unknownSession(id, "it was never saved in this state folder")
		}
		if err != nil {
			return nil, err
		}
		sess = a.adopt(id, newSession(st))
	}
	if err := sess.moveTo(a.store, folder); err != nil {
		return nil, err
	}
	return sess, nil
}

// replay sends the saved history to the client as it was said.
func (a *stubAgent) replay(ctx context.Context, id acp.SessionId, turns []turnRecord) error {
	client, err := a.connection(ctx)
	if err != nil {
		return err
	}
	for _, t := range turns {
		if err := sendUpdate(ctx, client, id, acp.UpdateUserMessageText(t.User)); err != nil {
			return err
		}
		if t.Agent == "" {
			continue
		}
		if err := sendUpdate(ctx, client, id, acp.UpdateAgentMessageText(t.Agent)); err != nil {
			return err
		}
	}
	return nil
}

// scenarioFor picks the scenario for a prompt: the one named by an @scenario marker in the text,
// or else the default. An unknown name is an error, so a typo in a test fails loudly.
func (a *stubAgent) scenarioFor(text string) (scenario, error) {
	name := scenarioMarker(text)
	if name == "" {
		name = a.defaultScenario
	}
	sc, ok := a.scenarios[name]
	if !ok {
		return scenario{}, acp.NewInvalidParams(map[string]any{
			"error": fmt.Sprintf("unknown scenario %q (known: %s)", name, scenarioNames(a.scenarios)),
		})
	}
	return sc, nil
}

// scenarioMarker returns the name in the first @scenario:<name> marker of a text, or "".
func scenarioMarker(text string) string {
	_, rest, found := strings.Cut(text, scenarioMarkerPrefix)
	if !found {
		return ""
	}
	end := strings.IndexFunc(rest, func(c rune) bool { return !isNameRune(c) })
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// promptText joins the text blocks of a prompt. Other kinds of block do not change what the
// scripted agent does.
func promptText(blocks []acp.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Text != nil {
			parts = append(parts, b.Text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// sendUpdate sends one session update to the client.
func sendUpdate(ctx context.Context, client peer, id acp.SessionId, u acp.SessionUpdate) error {
	err := client.SessionUpdate(ctx, acp.SessionNotification{SessionId: id, Update: u})
	if err != nil {
		return fmt.Errorf("send session update: %w", err)
	}
	return nil
}

// unknownSession is the error for a session id that is not open, with a hint on what to do.
func unknownSession(id acp.SessionId, hint string) error {
	return acp.NewInvalidParams(map[string]any{
		"error": fmt.Sprintf("session %q not found: %s", id, hint),
	})
}

// workingFolder checks the folder a client names for a session. ACP requires an absolute path.
func workingFolder(cwd string) (string, error) {
	if !filepath.IsAbs(cwd) {
		return "", acp.NewInvalidParams(map[string]any{
			"error": fmt.Sprintf("cwd %q must be an absolute path", cwd),
		})
	}
	return filepath.Clean(cwd), nil
}
