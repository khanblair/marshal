package main

import (
	"context"

	"github.com/coder/acp-go-sdk"
)

// The methods below are part of the acp.Agent interface, but a scripted agent has nothing to offer
// for them. They answer "method not found", which tells a client to stop asking. The unstable
// methods are left out on purpose: the connection already answers those with the same error when
// an agent does not implement them.

// Logout is not supported, because the stub has no accounts.
func (*stubAgent) Logout(context.Context, acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, acp.NewMethodNotFound(acp.AgentMethodLogout)
}

// ListSessions is not supported. A client keeps the session ids it needs.
func (*stubAgent) ListSessions(
	context.Context, acp.ListSessionsRequest,
) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionList)
}

// SetSessionConfigOption is not supported, because the stub has no options.
func (*stubAgent) SetSessionConfigOption(
	context.Context, acp.SetSessionConfigOptionRequest,
) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{},
		acp.NewMethodNotFound(acp.AgentMethodSessionSetConfigOption)
}

// SetSessionMode is not supported, because the stub has no modes.
func (*stubAgent) SetSessionMode(
	context.Context, acp.SetSessionModeRequest,
) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionSetMode)
}
