package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/coder/acp-go-sdk"
)

// restart ends a harness, like a process that exits, and starts a new agent on the same state
// folder and the same working folder.
func restart(t *testing.T, old *harness) *harness {
	t.Helper()
	old.close()
	return newHarness(t, harnessOptions{stateDir: old.stateDir, cwd: old.cwd})
}

// reopen brings a saved session back with LoadSession or ResumeSession.
func (h *harness) reopen(method string, sid acp.SessionId) error {
	h.t.Helper()
	switch method {
	case "load":
		_, err := h.conn.LoadSession(h.ctx, acp.LoadSessionRequest{
			SessionId: sid, Cwd: h.cwd, McpServers: []acp.McpServer{},
		})
		return err
	case "resume":
		_, err := h.conn.ResumeSession(h.ctx, acp.ResumeSessionRequest{SessionId: sid, Cwd: h.cwd})
		return err
	}
	h.t.Fatalf("unknown method %q", method)
	return nil
}

// firstTurnMessage returns the turn message of the newest turn, trimmed.
func firstTurnMessage(t *testing.T, h *harness) string {
	t.Helper()
	for _, text := range h.client.agentTexts() {
		if strings.HasPrefix(text, "Turn ") {
			return strings.TrimSpace(text)
		}
	}
	t.Fatal("no turn message was sent")
	return ""
}

func TestSessionSurvivesARestart(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		wantReplay bool
	}{
		{name: "load replays the history", method: "load", wantReplay: true},
		{name: "resume does not replay", method: "resume", wantReplay: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := newHarness(t, harnessOptions{})
			sid := first.newSession()
			first.mustPrompt(sid, "remember this @scenario:resume")
			if got := firstTurnMessage(t, first); got != "Turn 1. I remember 0 earlier turns." {
				t.Fatalf("first turn message = %q", got)
			}
			saidBefore := first.client.agentTexts()

			second := restart(t, first)
			if err := second.reopen(tt.method, sid); err != nil {
				t.Fatalf("%s: %v", tt.method, err)
			}
			if tt.wantReplay {
				wantUser := []string{"remember this @scenario:resume"}
				if got := second.client.userTexts(); !reflect.DeepEqual(got, wantUser) {
					t.Errorf("replayed user messages = %q, want %q", got, wantUser)
				}
				replayed := strings.Join(second.client.agentTexts(), "")
				for _, said := range saidBefore {
					if !strings.Contains(replayed, strings.TrimSpace(said)) {
						t.Errorf("replay lacks %q, got %q", strings.TrimSpace(said), replayed)
					}
				}
			} else if got := second.client.recorded(); len(got) != 0 {
				t.Errorf("resume replayed %d updates, want none", len(got))
			}

			second.client.forget()
			second.mustPrompt(sid, "the answer is main.go")
			if got := firstTurnMessage(t, second); got != "Turn 2. I remember 1 earlier turns." {
				t.Errorf("turn message after %s = %q", tt.method, got)
			}
		})
	}
}

func TestTurnCounterKeepsCountingAcrossRestarts(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	sid := h.newSession()
	h.mustPrompt(sid, "one")
	h.mustPrompt(sid, "two")

	for i, method := range []string{"load", "resume", "load"} {
		h = restart(t, h)
		if err := h.reopen(method, sid); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		h.client.forget()
		h.mustPrompt(sid, "again")
		wantTurn := i + 3
		want := fmt.Sprintf("Turn %d. I remember %d earlier turns.", wantTurn, wantTurn-1)
		if got := firstTurnMessage(t, h); got != want {
			t.Errorf("after %s: turn message = %q, want %q", method, got, want)
		}
	}
}

func TestReopenFailsForUnknownSessions(t *testing.T) {
	tests := []struct{ name, id string }{
		{name: "never saved", id: "stub-neversaved"},
		{name: "path traversal", id: "../../etc/passwd"},
		{name: "separator", id: "a/b"},
	}
	for _, tt := range tests {
		for _, method := range []string{"load", "resume"} {
			t.Run(tt.name+" "+method, func(t *testing.T) {
				h := newHarness(t, harnessOptions{})
				err := h.reopen(method, acp.SessionId(tt.id))
				if code := requestCode(err); code != -32602 {
					t.Errorf("error = %v (code %d), want invalid params", err, code)
				}
			})
		}
	}
}

func TestReopenMovesTheSessionToTheNewFolder(t *testing.T) {
	first := newHarness(t, harnessOptions{})
	sid := first.newSession()
	first.mustPrompt(sid, "one")

	second := restart(t, first)
	moved := second.cwd + "-moved"
	second.cwd = moved
	if err := second.reopen("resume", sid); err != nil {
		t.Fatal(err)
	}
	second.mustPrompt(sid, "two")
	// The default scenario writes notes/plan.md into the folder the client named last.
	if _, err := second.readFile("notes/plan.md"); err != nil {
		t.Errorf("the turn did not work in the new folder: %v", err)
	}
}
