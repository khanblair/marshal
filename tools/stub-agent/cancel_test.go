package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
)

func TestCancelStopsARunningTurn(t *testing.T) {
	tests := []struct {
		name string
		// files are the scenarios, and wait says what to wait for before cancelling.
		files func(t *testing.T) map[string]string
		wait  func(h *harness) <-chan struct{}
		opts  harnessOptions
	}{
		{
			name:  "during a pause",
			files: hangFiles,
			wait:  func(h *harness) <-chan struct{} { return h.client.updated },
			// A speed of 1 keeps the long pause long.
			opts: harnessOptions{speed: 1},
		},
		{
			name: "while waiting for permission",
			files: func(t *testing.T) map[string]string {
				return map[string]string{"default.json": scenarioFile(t, map[string]any{
					"type": "permission", "id": "ask", "title": "Run it", "kind": "execute",
					"command": "make",
				})}
			},
			wait: func(h *harness) <-chan struct{} { return h.client.asked },
			opts: harnessOptions{noChoice: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.scenarios = tt.files(t)
			h := newHarness(t, tt.opts)
			sid := h.newSession()

			type result struct {
				resp acp.PromptResponse
				err  error
			}
			done := make(chan result, 1)
			go func() {
				resp, err := h.prompt(sid, "start")
				done <- result{resp, err}
			}()
			waitFor(t, h.ctx, tt.wait(h))
			if err := h.conn.Cancel(h.ctx, acp.CancelNotification{SessionId: sid}); err != nil {
				t.Fatal(err)
			}

			select {
			case res := <-done:
				if res.err != nil || res.resp.StopReason != acp.StopReasonCancelled {
					t.Fatalf("prompt = %+v, %v; want stop reason cancelled", res.resp, res.err)
				}
			case <-h.ctx.Done():
				t.Fatal("the turn did not stop after the cancel")
			}

			// The cancelled turn is part of the history, and the session can go on.
			st, err := h.agent.store.load(string(sid))
			if err != nil || st.Turn != 1 || len(st.Turns) != 1 {
				t.Fatalf("saved state after cancel = %+v, %v", st, err)
			}
		})
	}
}

// waitFor waits for a token on a channel, or fails when the context ends first.
func waitFor(t *testing.T, ctx context.Context, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal("timed out waiting for the agent")
	}
}

func TestSecondPromptCancelsTheFirst(t *testing.T) {
	h := newHarness(t, harnessOptions{scenarios: hangFiles(t), speed: 1})
	sid := h.newSession()

	firstDone := make(chan acp.StopReason, 1)
	go func() {
		resp, err := h.prompt(sid, "first")
		if err != nil {
			t.Errorf("first prompt: %v", err)
		}
		firstDone <- resp.StopReason
	}()
	waitFor(t, h.ctx, h.client.updated)

	// The connection cancels the running prompt of a session when another one arrives. The
	// second turn then waits for the first to stop, and numbers itself after it.
	secondDone := make(chan acp.StopReason, 1)
	go func() {
		resp, err := h.prompt(sid, "second")
		if err != nil {
			t.Errorf("second prompt: %v", err)
		}
		secondDone <- resp.StopReason
	}()
	select {
	case reason := <-firstDone:
		if reason != acp.StopReasonCancelled {
			t.Errorf("first stop reason = %q", reason)
		}
	case <-h.ctx.Done():
		t.Fatal("the first turn did not stop")
	}
	waitForTurn(t, h, 2)
	if err := h.conn.Cancel(h.ctx, acp.CancelNotification{SessionId: sid}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondDone:
	case <-h.ctx.Done():
		t.Fatal("the second turn did not stop")
	}

	var intros []string
	for _, text := range h.client.agentTexts() {
		if strings.HasPrefix(text, "Turn ") {
			intros = append(intros, strings.TrimSpace(text))
		}
	}
	want := []string{"Turn 1. I remember 0 earlier turns.", "Turn 2. I remember 1 earlier turns."}
	if !reflect.DeepEqual(intros, want) {
		t.Errorf("turn messages = %q, want %q", intros, want)
	}
}

// waitForTurn waits until the agent has sent the message of the given turn.
func waitForTurn(t *testing.T, h *harness, turn int) {
	t.Helper()
	prefix := fmt.Sprintf("Turn %d.", turn)
	deadline := time.After(testTimeout)
	for {
		for _, text := range h.client.agentTexts() {
			if strings.HasPrefix(text, prefix) {
				return
			}
		}
		select {
		case <-h.client.updated:
		case <-deadline:
			t.Fatalf("turn %d never started", turn)
		}
	}
}
