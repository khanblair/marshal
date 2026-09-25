package api_test

import (
	"context"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// blockForever makes a fake agent's Start or Resume wait until its context ends.
const blockForever = -1

// fakeAgent is an agent that does what a test says and nothing else. Every method that the test
// did not think of is the nil embedded interface, so a call that was not expected fails loudly.
// It is for the cases the real stub agent cannot make: a start that is slow, one that is stopped
// half way, and an agent that refuses in a particular way.
type fakeAgent struct {
	agents.Agent
	startDelay  time.Duration // blockForever waits for the context to end
	resumeDelay time.Duration
	startErr    error
	resumeErr   error
	sendErr     error
	stopErr     error
	entered     chan struct{} // receives once when a Start or a Resume begins, if it is not nil

	events   chan agents.AgentEvent
	stopOnce *sync.Once
}

// fakeFactory makes a factory of fake agents that all follow the same script. Each agent gets its
// own event channel, as each real one would.
func fakeFactory(script fakeAgent) agents.Factory {
	return func() (agents.Agent, error) {
		agent := script
		agent.events = make(chan agents.AgentEvent, 16)
		agent.stopOnce = &sync.Once{}
		return &agent, nil
	}
}

func (a *fakeAgent) enter() {
	if a.entered == nil {
		return
	}
	select {
	case a.entered <- struct{}{}:
	default:
	}
}

// wait pauses for the delay, or until the context ends, and reports which of them ended it.
func wait(ctx context.Context, delay time.Duration) error {
	if delay == 0 {
		return nil
	}
	if delay == blockForever {
		<-ctx.Done()
		return ctx.Err()
	}
	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *fakeAgent) handle(spec agents.StartSpec, id string) agents.SessionHandle {
	return agents.SessionHandle{
		ID: id, Label: spec.Label, Capabilities: agents.Capabilities{Resume: true, StructuredEvents: true},
	}
}

// Start waits, then fails or starts a session.
func (a *fakeAgent) Start(ctx context.Context, spec agents.StartSpec) (agents.SessionHandle, error) {
	a.enter()
	if err := wait(ctx, a.startDelay); err != nil {
		return agents.SessionHandle{}, err
	}
	if a.startErr != nil {
		return agents.SessionHandle{}, a.startErr
	}
	return a.handle(spec, "fake-session-"+spec.Label), nil
}

// Resume waits, then fails or picks the session up.
func (a *fakeAgent) Resume(ctx context.Context, sessionID string, spec agents.StartSpec) (agents.SessionHandle, error) {
	a.enter()
	if err := wait(ctx, a.resumeDelay); err != nil {
		return agents.SessionHandle{}, err
	}
	if a.resumeErr != nil {
		return agents.SessionHandle{}, a.resumeErr
	}
	return a.handle(spec, sessionID), nil
}

// Send fails with the error the script names, or takes the message and never answers.
func (a *fakeAgent) Send(context.Context, agents.SessionHandle, agents.UserMessage) error {
	return a.sendErr
}

// Events returns the session's channel, which closes when the session is stopped.
func (a *fakeAgent) Events(agents.SessionHandle) <-chan agents.AgentEvent { return a.events }

// Stop ends the session: it closes the event channel, and then fails with the error the script
// names, if there is one.
func (a *fakeAgent) Stop(context.Context, agents.SessionHandle) error {
	a.stopOnce.Do(func() { close(a.events) })
	return a.stopErr
}

// Capabilities says what every fake agent can do.
func (a *fakeAgent) Capabilities() agents.Capabilities {
	return agents.Capabilities{Resume: true, StructuredEvents: true}
}
