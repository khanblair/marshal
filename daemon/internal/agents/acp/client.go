package acp

import (
	"context"

	sdk "github.com/coder/acp-go-sdk"
)

// client is what the agent talks to. It turns the agent's updates into events and its permission
// requests into questions for the caller. The adapter told the agent that it offers no file system
// and no terminal, so those requests are answered with "method not found".
type client struct {
	s *session
}

var _ sdk.Client = (*client)(nil)

// SessionUpdate turns one update from the agent into an event. The protocol library calls it one
// update at a time, in the order the agent sent them.
func (c *client) SessionUpdate(ctx context.Context, n sdk.SessionNotification) error {
	c.s.noteInbound(ctx)
	if !c.s.acceptsUpdatesFor(n.SessionId) {
		return nil
	}
	if ev, ok := convertUpdate(n.Update); ok {
		c.s.emit(ev)
	}
	return nil
}

// RequestPermission asks the caller and waits for the answer.
func (c *client) RequestPermission(
	ctx context.Context, req sdk.RequestPermissionRequest,
) (sdk.RequestPermissionResponse, error) {
	return c.s.askPermission(ctx, req), nil
}

// ReadTextFile is not offered.
func (*client) ReadTextFile(context.Context, sdk.ReadTextFileRequest) (sdk.ReadTextFileResponse, error) {
	return sdk.ReadTextFileResponse{}, sdk.NewMethodNotFound(sdk.ClientMethodFsReadTextFile)
}

// WriteTextFile is not offered.
func (*client) WriteTextFile(context.Context, sdk.WriteTextFileRequest) (sdk.WriteTextFileResponse, error) {
	return sdk.WriteTextFileResponse{}, sdk.NewMethodNotFound(sdk.ClientMethodFsWriteTextFile)
}

// CreateTerminal is not offered.
func (*client) CreateTerminal(context.Context, sdk.CreateTerminalRequest) (sdk.CreateTerminalResponse, error) {
	return sdk.CreateTerminalResponse{}, sdk.NewMethodNotFound(sdk.ClientMethodTerminalCreate)
}

// KillTerminal is not offered.
func (*client) KillTerminal(context.Context, sdk.KillTerminalRequest) (sdk.KillTerminalResponse, error) {
	return sdk.KillTerminalResponse{}, sdk.NewMethodNotFound(sdk.ClientMethodTerminalKill)
}

// TerminalOutput is not offered.
func (*client) TerminalOutput(context.Context, sdk.TerminalOutputRequest) (sdk.TerminalOutputResponse, error) {
	return sdk.TerminalOutputResponse{}, sdk.NewMethodNotFound(sdk.ClientMethodTerminalOutput)
}

// ReleaseTerminal is not offered.
func (*client) ReleaseTerminal(
	context.Context, sdk.ReleaseTerminalRequest,
) (sdk.ReleaseTerminalResponse, error) {
	return sdk.ReleaseTerminalResponse{}, sdk.NewMethodNotFound(sdk.ClientMethodTerminalRelease)
}

// WaitForTerminalExit is not offered.
func (*client) WaitForTerminalExit(
	context.Context, sdk.WaitForTerminalExitRequest,
) (sdk.WaitForTerminalExitResponse, error) {
	return sdk.WaitForTerminalExitResponse{}, sdk.NewMethodNotFound(sdk.ClientMethodTerminalWaitForExit)
}
