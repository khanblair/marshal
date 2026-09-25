package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/coder/acp-go-sdk"
)

const methodNotFoundCode = -32601

// requestCode returns the JSON-RPC error code of an error, or 0 when it is not a request error.
func requestCode(err error) int {
	var reqErr *acp.RequestError
	if errors.As(err, &reqErr) {
		return reqErr.Code
	}
	return 0
}

// wantShapes fails the test when the updates do not have the given order.
func wantShapes(t *testing.T, h *harness, want ...string) {
	t.Helper()
	if got := h.client.shapes(); !reflect.DeepEqual(got, want) {
		t.Errorf("updates = %v, want %v", got, want)
	}
}

func TestInitialize(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	resp, err := h.conn.Initialize(h.ctx, acp.InitializeRequest{ProtocolVersion: acp.ProtocolVersionNumber})
	if err != nil {
		t.Fatal(err)
	}
	caps := resp.AgentCapabilities
	if resp.ProtocolVersion != acp.ProtocolVersionNumber {
		t.Errorf("protocol version = %d", resp.ProtocolVersion)
	}
	if !caps.LoadSession {
		t.Error("loadSession is not advertised")
	}
	if caps.SessionCapabilities.Resume == nil {
		t.Error("session resume is not advertised")
	}
	if caps.SessionCapabilities.Close == nil {
		t.Error("session close is not advertised")
	}
	if resp.AgentInfo == nil || resp.AgentInfo.Name != agentName {
		t.Errorf("agent info = %+v", resp.AgentInfo)
	}
	if _, err := h.conn.Authenticate(h.ctx, acp.AuthenticateRequest{MethodId: "anything"}); err != nil {
		t.Errorf("authenticate: %v", err)
	}
}

func TestUnsupportedMethodsAreNotFound(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	sid := h.newSession()
	calls := map[string]func() error{
		"logout": func() error {
			_, err := h.conn.Logout(h.ctx, acp.LogoutRequest{})
			return err
		},
		"list sessions": func() error {
			_, err := h.conn.ListSessions(h.ctx, acp.ListSessionsRequest{})
			return err
		},
		"set mode": func() error {
			_, err := h.conn.SetSessionMode(h.ctx, acp.SetSessionModeRequest{SessionId: sid, ModeId: "x"})
			return err
		},
		"set config option": func() error {
			_, err := h.conn.SetSessionConfigOption(h.ctx, acp.SetSessionConfigOptionRequest{
				ValueId: &acp.SetSessionConfigOptionValueId{SessionId: sid, ConfigId: "x", Value: "y"},
			})
			return err
		},
		"unstable fork": func() error {
			_, err := h.conn.UnstableForkSession(h.ctx, acp.UnstableForkSessionRequest{
				SessionId: sid, Cwd: h.cwd,
			})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if code := requestCode(call()); code != methodNotFoundCode {
				t.Errorf("error code = %d, want %d", code, methodNotFoundCode)
			}
		})
	}
}

func TestNewSession(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	sid := h.newSession()
	if !validSessionID(string(sid)) {
		t.Errorf("session id %q is not a valid id", sid)
	}
	st, err := h.agent.store.load(string(sid))
	if err != nil {
		t.Fatalf("session was not saved at once: %v", err)
	}
	if st.Cwd != h.cwd || st.Turn != 0 || len(st.Turns) != 0 {
		t.Errorf("saved state = %+v", st)
	}

	_, err = h.conn.NewSession(h.ctx, acp.NewSessionRequest{Cwd: "relative/dir", McpServers: []acp.McpServer{}})
	if code := requestCode(err); code != -32602 {
		t.Errorf("relative cwd: error = %v (code %d), want invalid params", err, code)
	}
}

func TestPromptNeedsAnOpenSession(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	if _, err := h.prompt("nope", "hello"); err == nil {
		t.Error("prompt on an unknown session succeeded")
	}

	sid := h.newSession()
	if _, err := h.conn.CloseSession(h.ctx, acp.CloseSessionRequest{SessionId: sid}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.prompt(sid, "hello"); err == nil {
		t.Error("prompt on a closed session succeeded")
	}
	// Closing forgets the open session, not the saved file.
	if _, err := h.agent.store.load(string(sid)); err != nil {
		t.Errorf("closed session is gone from disk: %v", err)
	}
}

func TestDefaultScenario(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	sid := h.newSession()
	h.mustPrompt(sid, "please look at the project")

	wantShapes(t, h,
		"agent", // the turn message
		"agent",
		"tool:read:in_progress", "update:completed",
		"agent",
		"tool:edit:in_progress", "update:completed",
		"agent",
	)
	if first := h.client.agentTexts()[0]; !strings.HasPrefix(first, "Turn 1. I remember 0 earlier turns.") {
		t.Errorf("first agent message = %q", first)
	}

	got, err := h.readFile("notes/plan.md")
	if err != nil {
		t.Fatalf("the edit did not write its file: %v", err)
	}
	if !strings.Contains(got, "# Plan") {
		t.Errorf("notes/plan.md = %q", got)
	}
	updates := h.client.toolUpdates()
	if diff := updates[len(updates)-1].Content[1].Diff; diff == nil || diff.Path != filepath.Join(h.cwd, "notes", "plan.md") {
		t.Errorf("the edit did not report a diff of the file: %+v", updates[len(updates)-1].Content)
	}
}

func TestToolCallIDsAreUniqueAcrossTurns(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	sid := h.newSession()
	h.mustPrompt(sid, "one")
	h.mustPrompt(sid, "two")

	seen := map[acp.ToolCallId]bool{}
	for _, call := range h.client.toolStarts() {
		if seen[call.ToolCallId] {
			t.Errorf("tool call id %q was used twice", call.ToolCallId)
		}
		seen[call.ToolCallId] = true
	}
	if len(seen) != 4 {
		t.Errorf("got %d tool calls, want 4", len(seen))
	}
}

func TestApprove(t *testing.T) {
	tests := []struct {
		name       string
		choice     string
		wantShapes []string
		wantFile   bool
		wantText   string
	}{
		{
			name:   "allow continues",
			choice: allowOptionID,
			wantShapes: []string{
				"agent", "agent",
				"tool:execute:pending", "update:completed",
				"agent",
				"tool:edit:in_progress", "update:completed",
				"agent",
			},
			wantFile: true,
			wantText: "The fix is in notes/fix.md",
		},
		{
			name:   "deny apologizes and stops",
			choice: denyOptionID,
			wantShapes: []string{
				"agent", "agent",
				"tool:execute:pending", "update:failed",
				"agent",
			},
			wantFile: false,
			wantText: "Sorry",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, harnessOptions{choice: tt.choice})
			sid := h.newSession()
			h.mustPrompt(sid, "fix the bug @scenario:approve")

			wantShapes(t, h, tt.wantShapes...)
			texts := h.client.agentTexts()
			if last := texts[len(texts)-1]; !strings.Contains(last, tt.wantText) {
				t.Errorf("last message = %q, want it to contain %q", last, tt.wantText)
			}
			_, err := h.readFile("notes/fix.md")
			if wrote := err == nil; wrote != tt.wantFile {
				t.Errorf("notes/fix.md written = %v, want %v", wrote, tt.wantFile)
			}
			requests := h.client.permissionRequests()
			if len(requests) != 1 {
				t.Fatalf("got %d permission requests, want 1", len(requests))
			}
			assertAllowDenyOptions(t, requests[0])
		})
	}
}

// assertAllowDenyOptions checks that a request offers one allow and one deny choice, and that it
// is about a command.
func assertAllowDenyOptions(t *testing.T, req acp.RequestPermissionRequest) {
	t.Helper()
	kinds := map[acp.PermissionOptionId]acp.PermissionOptionKind{}
	for _, o := range req.Options {
		kinds[o.OptionId] = o.Kind
	}
	want := map[acp.PermissionOptionId]acp.PermissionOptionKind{
		allowOptionID: acp.PermissionOptionKindAllowOnce,
		denyOptionID:  acp.PermissionOptionKindRejectOnce,
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("options = %v, want %v", kinds, want)
	}
	if req.ToolCall.Title == nil || *req.ToolCall.Title != "Run rm -rf build" {
		t.Errorf("permission request title = %v", req.ToolCall.Title)
	}
}
