package api

import (
	"context"
	"errors"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// The sentences for the refusals that the services report as plain Go errors, and that a person
// can really meet. They say what happened and what to do next, in the words of docs/ui-rules.md.
const (
	messageAgentSignIn = "The agent needs you to sign in before it can start. " +
		"Sign in with the agent's own command in a terminal, then try again."
	messageBranchLeftOver = "This card has a branch left over from an earlier start that did not finish. " +
		"Delete that branch in your repository, then start the card again."
	messageCannotResume = "Marshal could not pick this session back up. The card now needs you."
	messageNoAgent      = "This card has no agent running."
	messageNeverStarted = "This card has not been started, so there is nothing to resume. Start it first."
	messageNotARepo     = "Marshal cannot use the repository folder of this project. " +
		"Check that the folder is still there and is a Git repository."
	messageTookTooLong = "Marshal took too long to finish that. Try again."
	messageAgentSlow   = "The agent took too long to start. Check that it runs in a terminal."
)

// translate turns the errors that the services leave as plain Go errors, but that a person can
// meet, into answers in plain sentences. An error that already is an answer passes through as it
// is, and anything else is left for writeError to report as the daemon's own problem, with its
// real cause in the log. The services are finished and other tasks use them, so the translation
// is made here, on each service's own sentinels.
func translate(err error) error {
	var answer *protocol.Error
	var signIn *agents.AuthRequiredError
	switch {
	case errors.As(err, &answer):
		return err
	case errors.As(err, &signIn):
		refusal := protocol.Refused(messageAgentSignIn).WithCause(err)
		if len(signIn.Methods) > 0 {
			refusal = refusal.With("methods", strings.Join(signIn.Methods, ", "))
		}
		return refusal
	case errors.Is(err, agents.ErrCannotResume):
		return protocol.Refused(messageCannotResume).WithCause(err)
	case errors.Is(err, agents.ErrStopped), errors.Is(err, agents.ErrUnknownSession):
		return protocol.Refused(messageNoAgent).WithCause(err)
	case errors.Is(err, gitx.ErrBranchExists):
		return protocol.Conflict(messageBranchLeftOver).WithCause(err)
	case errors.Is(err, gitx.ErrNotARepo):
		return protocol.Unavailable(messageNotARepo).WithCause(err)
	case errors.Is(err, context.DeadlineExceeded):
		return protocol.Unavailable(messageTookTooLong).WithCause(err)
	}
	return err
}

// sessionError translates what the session manager returned for a card that has no running agent.
// The manager knows the card only through its session, so an unknown card and a card with nothing
// running both come back as "no agent running". The first must read as not found, exactly as an
// id of the wrong shape does, so the card is looked up when the projects service is there.
func (s *Server) sessionError(ctx context.Context, cardID string, err error) error {
	if errors.Is(err, session.ErrNoLiveSession) && s.projects != nil {
		if _, lookupErr := s.projects.Card(ctx, cardID); lookupErr != nil {
			return translate(lookupErr)
		}
	}
	return translate(err)
}

// resumeError translates what Resume returned. The manager answers "not found" for a card that has
// no session to resume, whether the card is unknown or was never started. The first stays not
// found, and the second becomes a refusal that says what to do. A resume that ran past its time
// limit is a resume that failed: the manager has already stopped the session and moved the card to
// needing the person, so "try again" would only meet "the session has stopped".
func (s *Server) resumeError(ctx context.Context, cardID string, err error) error {
	var answer *protocol.Error
	if errors.As(err, &answer) {
		if answer.Code == protocol.ErrorCodeNotFound && s.projects != nil {
			if _, lookupErr := s.projects.Card(ctx, cardID); lookupErr != nil {
				return translate(lookupErr)
			}
			return protocol.Refused(messageNeverStarted).With("cardId", cardID).WithCause(err)
		}
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return protocol.Refused(messageCannotResume).WithCause(err)
	}
	return translate(err)
}

// startError translates what Start returned. A start that ran past its time limit is not one to
// try again at once: the worktree is undone but the card's branch is kept, so the next start
// meets the branch, and the sentence does not say to try again.
func startError(err error) error {
	var answer *protocol.Error
	if !errors.As(err, &answer) && errors.Is(err, context.DeadlineExceeded) {
		return protocol.Unavailable(messageAgentSlow).WithCause(err)
	}
	return translate(err)
}
