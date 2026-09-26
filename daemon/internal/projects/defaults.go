package projects

import "context"

// The defaults for the parts that come from modules that are built later. Each does nothing, so
// the service works on its own, and each is replaced with an option when the real module exists.

// noSessions stops nothing, because no session exists yet.
type noSessions struct{}

func (noSessions) StopProjectSessions(context.Context, string) error { return nil }
func (noSessions) StopCardSession(context.Context, string) error     { return nil }
func (noSessions) RemoveCardLogs(context.Context, string) error      { return nil }

// noMemory deletes nothing, because no memory folder exists yet.
type noMemory struct{}

func (noMemory) RemoveProjectMemory(context.Context, string) error { return nil }

// noAwake reports no awake cards, because no agent runs yet.
type noAwake struct{}

func (noAwake) AwakeCards(context.Context, string) (int, error) { return 0, nil }

// noSessionStates reports that no card has a session, so every card sends a null session in the chat
// view.
type noSessionStates struct{}

func (noSessionStates) CardSession(context.Context, string) (*SessionInfo, error) {
	return nil, nil
}

func (noSessionStates) ProjectSessions(context.Context, string) (map[string]SessionInfo, error) {
	return nil, nil
}
