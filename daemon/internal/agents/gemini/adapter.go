package gemini

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/acp"
)

// Adapter is Gemini CLI as an agents.Agent. It is the ACP adapter, with the model and the sign-in
// handled the way Gemini needs. Everything else (sending, interrupting, events, permission
// requests, stopping) is the ACP adapter's own.
type Adapter struct {
	*acp.Adapter
}

var _ agents.Agent = (*Adapter)(nil)

// New returns an adapter for the Gemini CLI at cfg.Path. It starts nothing.
func New(cfg Config) (agents.Agent, error) {
	inner, err := acp.New(acpConfig(cfg))
	if err != nil {
		return nil, fmt.Errorf("gemini adapter: %w", err)
	}
	return &Adapter{Adapter: inner}, nil
}

// Factory returns the factory that the session manager registers for the gemini kind.
func Factory(cfg Config) agents.Factory {
	return func() (agents.Agent, error) { return New(cfg) }
}

// Start starts a new Gemini CLI process and a session in it.
func (a *Adapter) Start(ctx context.Context, spec agents.StartSpec) (agents.SessionHandle, error) {
	spec, model, err := withModel(spec)
	if err != nil {
		return agents.SessionHandle{}, err
	}
	h, err := a.Adapter.Start(ctx, spec)
	if err != nil {
		return agents.SessionHandle{}, signInMessage(err)
	}
	return withModelApplied(h, model), nil
}

// Resume starts a new Gemini CLI process and loads the session with the given id. Gemini has no
// resume request of its own, so the ACP adapter loads the session and drops the replay of its
// history.
func (a *Adapter) Resume(
	ctx context.Context, sessionID string, spec agents.StartSpec,
) (agents.SessionHandle, error) {
	spec, model, err := withModel(spec)
	if err != nil {
		return agents.SessionHandle{}, err
	}
	h, err := a.Adapter.Resume(ctx, sessionID, spec)
	if err != nil {
		return agents.SessionHandle{}, signInMessage(err)
	}
	return withModelApplied(h, model), nil
}

// Capabilities says what Gemini CLI can do as the ACP adapter found it, and that the model can be
// chosen, which the environment makes possible whether or not the protocol does.
func (a *Adapter) Capabilities() agents.Capabilities {
	caps := a.Adapter.Capabilities()
	caps.ModelSwitching = true
	return caps
}

// SignInError is what Start and Resume return when Gemini CLI needs the person to sign in. Its text
// is a plain sentence that the API layer shows as it is, and errors.As still finds the
// *agents.AuthRequiredError inside, with the names of the ways to sign in.
type SignInError struct {
	cause error
}

// Error says what to do.
func (*SignInError) Error() string {
	return "Sign in to Gemini CLI in a terminal first. Run \"gemini\" there once, and choose how to sign in."
}

// Unwrap returns the error from the ACP adapter.
func (e *SignInError) Unwrap() error { return e.cause }

// signInMessage turns the ACP adapter's sign-in error into a SignInError, and leaves every other
// error as it is.
func signInMessage(err error) error {
	var auth *agents.AuthRequiredError
	if errors.As(err, &auth) {
		return &SignInError{cause: err}
	}
	return err
}
