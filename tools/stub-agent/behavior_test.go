package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/coder/acp-go-sdk"
)

func TestStuckLoopRepeatsSixTimes(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	sid := h.newSession()
	h.mustPrompt(sid, "build it @scenario:stuck-loop")

	starts := h.client.toolStarts()
	if len(starts) != 6 {
		t.Fatalf("got %d tool calls, want 6", len(starts))
	}
	ids := map[acp.ToolCallId]bool{}
	for _, call := range starts {
		ids[call.ToolCallId] = true
		if call.Title != starts[0].Title {
			t.Errorf("title %q differs from %q", call.Title, starts[0].Title)
		}
	}
	if len(ids) != 6 {
		t.Errorf("got %d distinct tool call ids, want 6", len(ids))
	}
	updates := h.client.toolUpdates()
	if len(updates) != 6 {
		t.Fatalf("got %d tool call updates, want 6", len(updates))
	}
	for _, u := range updates {
		if *u.Status != acp.ToolCallStatusFailed || updateText(u) != updateText(updates[0]) {
			t.Errorf("update = %s %q, want the same failure every time", *u.Status, updateText(u))
		}
	}
}

func TestFailTestScenarioFailsThenPasses(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	sid := h.newSession()
	h.mustPrompt(sid, "fix the test @scenario:fail-test")

	var statuses []acp.ToolCallStatus
	for _, u := range h.client.toolUpdates() {
		statuses = append(statuses, *u.Status)
	}
	want := []acp.ToolCallStatus{
		acp.ToolCallStatusFailed, acp.ToolCallStatusCompleted, acp.ToolCallStatusCompleted,
	}
	if !reflect.DeepEqual(statuses, want) {
		t.Errorf("tool statuses = %v, want %v", statuses, want)
	}
	if _, err := h.readFile("internal/auth/refresh.go"); err != nil {
		t.Errorf("the fix did not write its file: %v", err)
	}
}

func TestPathEscapeIsRefused(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "abs.txt")
	tests := []struct {
		name string
		kind string
		path string
	}{
		{name: "parent folder", kind: "edit", path: "../escape.txt"},
		{name: "parent folder after a subfolder", kind: "edit", path: "sub/../../escape.txt"},
		{name: "absolute path", kind: "edit", path: outside},
		{name: "read of a parent path", kind: "read", path: "../escape.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := map[string]any{"type": "tool", "id": "w", "title": "Touch a file", "kind": tt.kind}
			if tt.kind == "edit" {
				step["write"] = map[string]any{"path": tt.path, "content": "x"}
			} else {
				step["path"] = tt.path
			}
			h := newHarness(t, harnessOptions{scenarios: map[string]string{
				"default.json": scenarioFile(t, step),
			}})
			sid := h.newSession()
			h.mustPrompt(sid, "go")

			updates := h.client.toolUpdates()
			if len(updates) != 1 || *updates[0].Status != acp.ToolCallStatusFailed {
				t.Fatalf("tool updates = %+v, want one failed update", updates)
			}
			if text := updateText(updates[0]); !strings.Contains(text, "outside the working folder") {
				t.Errorf("failure text = %q", text)
			}
			if starts := h.client.toolStarts(); len(starts[0].Locations) != 0 {
				t.Errorf("the escaping path was announced as a location: %+v", starts[0].Locations)
			}
			for _, escaped := range []string{
				filepath.Join(filepath.Dir(h.cwd), "escape.txt"), outside,
			} {
				if _, err := os.Stat(escaped); err == nil {
					t.Errorf("%s was written", escaped)
				}
			}
		})
	}
}

func TestPathInsideTheFolderIsWritten(t *testing.T) {
	h := newHarness(t, harnessOptions{scenarios: map[string]string{
		"default.json": scenarioFile(t, map[string]any{
			"type": "tool", "id": "w", "title": "Write", "kind": "edit",
			"write": map[string]any{"path": "sub/../deep/ok.txt", "content": "fine"},
		}),
	}})
	sid := h.newSession()
	h.mustPrompt(sid, "go")
	if got, err := h.readFile("deep/ok.txt"); err != nil || got != "fine" {
		t.Errorf("deep/ok.txt = %q, %v", got, err)
	}
}

func TestFailedToolWritesNothing(t *testing.T) {
	h := newHarness(t, harnessOptions{scenarios: map[string]string{
		"default.json": scenarioFile(t, map[string]any{
			"type": "tool", "id": "w", "title": "Write", "kind": "edit", "status": "failed",
			"write": map[string]any{"path": "never.txt", "content": "x"},
		}),
	}})
	sid := h.newSession()
	h.mustPrompt(sid, "go")
	if _, err := h.readFile("never.txt"); err == nil {
		t.Error("a failed tool call wrote its file")
	}
}

func TestScenarioMarkerOverridesTheDefault(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantTools int
		wantError string
	}{
		{name: "no marker uses the default", text: "hello", wantTools: 0},
		{name: "marker picks a scenario", text: "build @scenario:stuck-loop", wantTools: 6},
		{name: "marker in the middle", text: "a @scenario:stuck-loop, then more", wantTools: 6},
		{name: "unknown marker fails loudly", text: "@scenario:nope", wantError: "unknown scenario"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, harnessOptions{scenario: "resume"})
			sid := h.newSession()
			resp, err := h.prompt(sid, tt.text)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want it to contain %q", err, tt.wantError)
				}
				return
			}
			if err != nil || resp.StopReason != acp.StopReasonEndTurn {
				t.Fatalf("prompt = %+v, %v", resp, err)
			}
			if got := len(h.client.toolStarts()); got != tt.wantTools {
				t.Errorf("got %d tool calls, want %d", got, tt.wantTools)
			}
		})
	}
}

func TestUnknownScenarioDoesNotCountAsATurn(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	sid := h.newSession()
	if _, err := h.prompt(sid, "@scenario:nope"); err == nil {
		t.Fatal("expected an error")
	}
	h.mustPrompt(sid, "hello")
	if first := h.client.agentTexts()[0]; !strings.HasPrefix(first, "Turn 1. I remember 0 earlier turns.") {
		t.Errorf("first message after the failed prompt = %q", first)
	}
}

func TestResumeScenarioAsksAQuestion(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	sid := h.newSession()
	h.mustPrompt(sid, "start @scenario:resume")
	texts := h.client.agentTexts()
	if len(texts) != 2 || !strings.HasSuffix(texts[1], "?") {
		t.Errorf("agent messages = %q, want the turn message and a question", texts)
	}
}

func TestSmellsScenarioWritesASmellyFile(t *testing.T) {
	h := newHarness(t, harnessOptions{})
	sid := h.newSession()
	h.mustPrompt(sid, "write the report @scenario:smells")

	got, err := h.readFile("src/report.ts")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(got, "\n")
	start, end := -1, -1
	for i, line := range lines {
		if strings.HasPrefix(line, "export function buildReport") {
			start = i
		}
		if start >= 0 && line == "}" {
			end = i
			break
		}
	}
	if start < 0 || end < 0 || end-start+1 < 120 {
		t.Errorf("the function has %d lines, want at least 120", end-start+1)
	}
	if !strings.Contains(got, "86400") || !strings.Contains(got, "1.0725") {
		t.Error("the file has no magic numbers")
	}
}
