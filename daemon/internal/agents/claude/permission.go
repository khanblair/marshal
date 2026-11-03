package claude

import (
	"encoding/json"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

const permissionAllow = "allow"

type permissionRequest struct {
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype   string          `json:"subtype"`
		ToolName  string          `json:"tool_name"`
		ToolUseID string          `json:"tool_use_id"`
		Input     json.RawMessage `json:"input"`
	} `json:"request"`
}

// The original input is kept byte for byte. Approval authorizes that exact call,
// without applying tool-suggested permission changes or rewriting its arguments.
func (s *session) handlePermissionRequest(line []byte) {
	var request permissionRequest
	if err := json.Unmarshal(line, &request); err != nil || request.RequestID == "" {
		return
	}
	if request.Request.Subtype != "can_use_tool" {
		_ = s.writePermissionError(request.RequestID, "Marshal does not support this control request.")
		return
	}
	s.mu.Lock()
	if s.stopping || s.ended || s.interrupted || !s.turnActive {
		s.mu.Unlock()
		_ = s.writePermissionError(request.RequestID, "The turn is no longer accepting permission requests.")
		return
	}
	if _, exists := s.permissions[request.RequestID]; exists {
		s.mu.Unlock()
		return
	}
	if s.permissions == nil {
		s.permissions = make(map[string]json.RawMessage)
	}
	s.permissions[request.RequestID] = request.Request.Input
	s.mu.Unlock()
	call := toolCallEvent(contentBlock{ID: request.Request.ToolUseID, Name: request.Request.ToolName, Input: request.Request.Input})
	s.emit(agents.PermissionRequested{
		RequestID: request.RequestID, ToolCallID: call.ID, Title: call.Title,
		Kind: call.Kind, Path: call.Path, Command: call.Command,
		Options: []agents.PermissionOption{
			{ID: permissionAllow, Name: "Allow once", Kind: agents.OptionAllowOnce},
			{ID: "deny", Name: "Deny", Kind: agents.OptionRejectOnce},
		},
	})
}

func (s *session) respondPermission(response agents.ApprovalResponse) error {
	s.mu.Lock()
	input, exists := s.permissions[response.RequestID]
	if !exists || s.stopping || s.ended || s.interrupted || !s.turnActive {
		s.mu.Unlock()
		return agents.ErrUnknownRequest
	}
	if !response.Cancelled && response.OptionID != permissionAllow && response.OptionID != "deny" {
		s.mu.Unlock()
		return agents.ErrUnknownRequest
	}
	delete(s.permissions, response.RequestID)
	s.mu.Unlock()
	answer := map[string]any{"behavior": "deny", "message": "The user denied this tool call."}
	if !response.Cancelled && response.OptionID == permissionAllow {
		answer = map[string]any{"behavior": permissionAllow, "updatedInput": input}
	}
	line, err := encodeLine(map[string]any{
		"type": "control_response", "response": map[string]any{
			"subtype": "success", "request_id": response.RequestID, "response": answer,
		},
	})
	if err != nil {
		return err
	}
	if err := s.writeStdin(line); err != nil {
		return fmt.Errorf("answer claude permission: %w", err)
	}
	return nil
}

func (s *session) writePermissionError(id, message string) error {
	line, err := encodeLine(map[string]any{"type": "control_response", "response": map[string]any{
		"subtype": "error", "request_id": id, "error": message,
	}})
	if err != nil {
		return err
	}
	return s.writeStdin(line)
}

func (s *session) cancelPermissionRequest(line []byte) {
	var request struct {
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal(line, &request) != nil {
		return
	}
	s.mu.Lock()
	_, exists := s.permissions[request.RequestID]
	delete(s.permissions, request.RequestID)
	s.mu.Unlock()
	if exists {
		s.emit(agents.PermissionCancelled{RequestID: request.RequestID})
	}
}
