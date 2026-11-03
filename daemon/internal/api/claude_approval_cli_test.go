package api_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/agents/claude"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// This subprocess speaks the actual CLI wire protocol. The production Claude
// adapter, session manager, approval storage, and HTTP routes are all exercised.
func runApprovalCLI() {
	emit := func(value any) { _ = json.NewEncoder(os.Stdout).Encode(value) }
	id := ""
	for _, arg := range os.Args {
		if value, ok := strings.CutPrefix(arg, "--session-id="); ok {
			id = value
		}
	}
	emit(map[string]any{"type": "system", "subtype": "init", "session_id": id})
	cwd, _ := os.Getwd()
	path := filepath.Join(cwd, "approved.txt")
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		var message struct {
			Type    string `json:"type"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Response struct {
				RequestID string `json:"request_id"`
				Response  struct {
					Behavior string `json:"behavior"`
					Input    struct {
						Path    string `json:"file_path"`
						Content string `json:"content"`
					} `json:"updatedInput"`
				} `json:"response"`
			} `json:"response"`
		}
		if json.Unmarshal(scan.Bytes(), &message) != nil {
			continue
		}
		if message.Type == "user" {
			emit(map[string]any{"type": "control_request", "request_id": "write-request", "request": map[string]any{"subtype": "can_use_tool", "tool_name": "Write", "tool_use_id": "write-call", "input": map[string]any{"file_path": path, "content": "approved content\n"}}})
			if message.Message.Content == "cancel" {
				emit(map[string]any{"type": "control_cancel_request", "request_id": "write-request"})
				emit(map[string]any{"type": "result", "subtype": "success", "is_error": false})
			}
		}
		if message.Type == "control_response" && message.Response.RequestID == "write-request" {
			if message.Response.Response.Behavior == "allow" {
				if err := os.WriteFile(message.Response.Response.Input.Path, []byte(message.Response.Response.Input.Content), 0o600); err != nil {
					fmt.Fprintln(os.Stderr, err)
					return
				}
			}
			emit(map[string]any{"type": "result", "subtype": "success", "result": "answered", "is_error": false})
		}
	}
}

func TestClaudeApprovalThroughHTTPAndReopenedHistory(t *testing.T) {
	for _, decision := range []protocol.ApprovalDecision{protocol.ApprovalDecisionApproved, protocol.ApprovalDecisionDenied} {
		t.Run(string(decision), func(t *testing.T) {
			st := newStack(t, withAgent(func() (agents.Agent, error) {
				return claude.New(claude.Config{Path: os.Args[0], Env: []string{"MARSHAL_APPROVAL_CLI=1"}})
			}))
			project, _ := st.addProject("small-repo")
			card := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards", protocol.CreateCardRequest{Title: "Approval round trip", PermissionMode: protocol.PermissionModeAsk}).want(t, http.StatusCreated))
			base := "/v1/cards/" + card.ID
			st.do(http.MethodPost, base+"/start", nil).want(t, http.StatusOK)
			st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "write file"}).want(t, http.StatusNoContent)
			var approvalID string
			deadline := time.Now().Add(15 * time.Second)
			for time.Now().Before(deadline) {
				current := decode[protocol.Card](t, st.do(http.MethodGet, base, nil).want(t, http.StatusOK))
				if current.NeedsReason != nil && current.NeedsReason.ApprovalID != "" {
					approvalID = current.NeedsReason.ApprovalID
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if approvalID == "" {
				t.Fatalf("approval did not reach the card read API: card=%s history=%s", st.do(http.MethodGet, base, nil).Body, st.do(http.MethodGet, base+"/messages", nil).Body)
			}
			st.do(http.MethodPost, "/v1/approvals/"+approvalID, protocol.DecideApprovalRequest{Decision: decision}).want(t, http.StatusNoContent)
			st.do(http.MethodPost, "/v1/approvals/"+approvalID, protocol.DecideApprovalRequest{Decision: decision}).want(t, http.StatusConflict)
			deadline = time.Now().Add(15 * time.Second)
			for time.Now().Before(deadline) {
				current := decode[protocol.Card](t, st.do(http.MethodGet, base, nil).want(t, http.StatusOK))
				if current.Session != nil && *current.Session == protocol.SessionStateAwake {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			page := decode[protocol.Page[protocol.ChatMessage]](t, st.do(http.MethodGet, base+"/messages", nil).want(t, http.StatusOK))
			current := decode[protocol.Card](t, st.do(http.MethodGet, base, nil).want(t, http.StatusOK))
			if current.Session == nil || *current.Session != protocol.SessionStateAwake {
				t.Fatalf("answered turn did not become idle: %+v", current.Session)
			}
			found := false
			for _, message := range page.Items {
				if message.Approval != nil {
					found = true
					if message.Approval.ID != approvalID || message.Approval.State != decision.State() {
						t.Fatalf("stored approval = %+v", message.Approval)
					}
				}
			}
			if !found {
				t.Fatal("resolved approval missing from reopened history")
			}
			path, _, err := st.proj.Worktree(t.Context(), card.ID)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(path, "approved.txt"))
			if decision == protocol.ApprovalDecisionApproved {
				if err != nil || string(data) != "approved content\n" {
					t.Fatalf("approved write = %q, %v", data, err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatalf("denied tool ran: %q, %v", data, err)
			}
		})
	}
}

func TestClaudeCancelledApprovalCannotLeaveDeadButtons(t *testing.T) {
	st := newStack(t, withAgent(func() (agents.Agent, error) {
		return claude.New(claude.Config{Path: os.Args[0], Env: []string{"MARSHAL_APPROVAL_CLI=1"}})
	}))
	project, _ := st.addProject("small-repo")
	card := decode[protocol.Card](t, st.do(http.MethodPost, "/v1/projects/"+project.ID+"/cards", protocol.CreateCardRequest{Title: "Cancel callback", PermissionMode: protocol.PermissionModeAsk}).want(t, http.StatusCreated))
	base := "/v1/cards/" + card.ID
	st.do(http.MethodPost, base+"/start", nil).want(t, http.StatusOK)
	st.do(http.MethodPost, base+"/messages", protocol.SendMessageRequest{Text: "cancel"}).want(t, http.StatusNoContent)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		page := decode[protocol.Page[protocol.ChatMessage]](t, st.do(http.MethodGet, base+"/messages", nil).want(t, http.StatusOK))
		for _, message := range page.Items {
			if message.Approval == nil || message.Approval.State != protocol.ChatApprovalStateDenied {
				continue
			}
			current := decode[protocol.Card](t, st.do(http.MethodGet, base, nil).want(t, http.StatusOK))
			if current.Session == nil || *current.Session != protocol.SessionStateAwake {
				continue
			}
			if current.NeedsReason != nil && current.NeedsReason.ApprovalID != "" {
				t.Fatal("cancelled approval still attached to card")
			}
			st.do(http.MethodPost, "/v1/approvals/"+message.Approval.ID, protocol.DecideApprovalRequest{Decision: protocol.ApprovalDecisionApproved}).want(t, http.StatusConflict)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("cancelled approval was not resolved in reopened history")
}
