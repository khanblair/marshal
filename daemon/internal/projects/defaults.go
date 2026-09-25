package projects

import "context"

// The defaults for the parts that come from modules that are built later. Each does nothing, so
// the service works on its own, and each is replaced with an option when the real module exists.

// noSessions stops nothing, because no session exists yet.
type noSessions struct{}

func (noSessions) StopProjectSessions(context.Context, string) error { return nil }

// noMemory deletes nothing, because no memory folder exists yet.
type noMemory struct{}

func (noMemory) RemoveProjectMemory(context.Context, string) error { return nil }

// noAwake reports no awake cards, because no agent runs yet.
type noAwake struct{}

func (noAwake) AwakeCards(context.Context, string) (int, error) { return 0, nil }
