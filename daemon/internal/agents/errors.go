package agents

import (
	"errors"
	"strings"
)

// Errors that callers act on. Adapters wrap them with context, so callers compare with errors.Is.
var (
	// ErrBusy means a turn is already running. The caller queues the message.
	ErrBusy = errors.New("a turn is already running in this session")
	// ErrCannotResume means the agent cannot pick the session up again. It is either not able to
	// resume at all, or it no longer knows the session.
	ErrCannotResume = errors.New("the session cannot be resumed")
	// ErrUnknownSession means the handle names no running session, for example after Stop.
	ErrUnknownSession = errors.New("no such session is running")
	// ErrStopped means the session is stopping or has stopped.
	ErrStopped = errors.New("the session has stopped")
	// ErrUnknownRequest means no permission request with that id is waiting. It was probably
	// answered already.
	ErrUnknownRequest = errors.New("no such permission request is waiting")
	// ErrUnknownOption means the answer names an option that the request did not offer.
	ErrUnknownOption = errors.New("the permission request has no such option")
	// ErrUnknownKind means no agent is registered for the kind.
	ErrUnknownKind = errors.New("no agent of that kind is registered")
	// ErrUnsupportedSetting means the agent offers a control for a setting but not the value that
	// was asked for.
	ErrUnsupportedSetting = errors.New("the agent does not offer that setting")
)

// AuthRequiredError means the agent will not work until the user signs in, and signing in needs
// the user: a browser, a terminal, or a key. The API layer shows Error to the user as it is.
type AuthRequiredError struct {
	// Methods are the names of the ways to sign in that the agent offers.
	Methods []string
	// Cause is why an automatic sign-in failed, when one was tried. It is for logs.
	Cause error
}

// Unwrap returns the cause, if there is one.
func (e *AuthRequiredError) Unwrap() error { return e.Cause }

func (e *AuthRequiredError) Error() string {
	if len(e.Methods) == 0 {
		return "the agent needs you to sign in first"
	}
	return "the agent needs you to sign in first (" + strings.Join(e.Methods, ", ") + ")"
}
