package claude

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestNewSessionIDMakesValidUUIDs(t *testing.T) {
	seen := map[string]bool{}
	for range 10 {
		id, err := newSessionID()
		if err != nil {
			t.Fatalf("newSessionID: %v", err)
		}
		if !looksLikeUUID(id) {
			t.Errorf("id = %q, does not look like a UUID", id)
		}
		if seen[id] {
			t.Fatalf("id %q was generated twice", id)
		}
		seen[id] = true
	}
}

func TestLooksLikeUUID(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"5b1b1b1a-2b2b-4b2b-8b2b-2b2b2b2b2b2b", true},
		{"5B1B1B1A-2B2B-4B2B-8B2B-2B2B2B2B2B2B", true},
		{"not-a-uuid", false},
		{"", false},
		{"5b1b1b1a-2b2b-4b2b-8b2b-2b2b2b2b2b2", false},   // one digit short
		{"5b1b1b1a-2b2b-4b2b-8b2b-2b2b2b2b2b2bx", false}, // one char too long
		{"5b1b1b1a02b2b-4b2b-8b2b-2b2b2b2b2b2b", false},  // dash in the wrong place
	}
	for _, tt := range tests {
		if got := looksLikeUUID(tt.text); got != tt.want {
			t.Errorf("looksLikeUUID(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestEffortFor(t *testing.T) {
	tests := []struct {
		mode   string
		want   string
		wantOK bool
	}{
		{"low", "low", true},
		{"medium", "medium", true},
		{"high", "high", true},
		{"extra-high", "xhigh", true},
		{"max", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := effortFor(tt.mode)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("effortFor(%q) = (%q, %v), want (%q, %v)", tt.mode, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestKindFor(t *testing.T) {
	tests := []struct{ tool, want string }{
		{"Read", "read"}, {"NotebookRead", "read"},
		{"Edit", "edit"}, {"MultiEdit", "edit"}, {"Write", "edit"}, {"NotebookEdit", "edit"},
		{"Bash", "execute"}, {"BashOutput", "execute"}, {"KillShell", "execute"},
		{"Grep", "search"}, {"Glob", "search"}, {"WebSearch", "search"},
		{"WebFetch", "fetch"},
		{"mcp__example__tool", "other"}, {"AskUserQuestion", "other"},
	}
	for _, tt := range tests {
		if got := kindFor(tt.tool); got != tt.want {
			t.Errorf("kindFor(%q) = %q, want %q", tt.tool, got, tt.want)
		}
	}
}

func TestPathAndCommandFromInput(t *testing.T) {
	edit := json.RawMessage(`{"file_path":"a.go","old_string":"x","new_string":"y"}`)
	if got := pathFromInput(edit); got != "a.go" {
		t.Errorf("pathFromInput(edit) = %q, want a.go", got)
	}
	bash := json.RawMessage(`{"command":"echo hi"}`)
	if got := commandFromInput(bash); got != "echo hi" {
		t.Errorf("commandFromInput(bash) = %q, want %q", got, "echo hi")
	}
	if got := pathFromInput(nil); got != "" {
		t.Errorf("pathFromInput(nil) = %q, want empty", got)
	}
	if got := commandFromInput(json.RawMessage(`not json`)); got != "" {
		t.Errorf("commandFromInput(bad json) = %q, want empty", got)
	}
}

func TestToolResultText(t *testing.T) {
	if got := toolResultText(json.RawMessage(`"plain text"`)); got != "plain text" {
		t.Errorf("string content = %q", got)
	}
	blocks := json.RawMessage(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`)
	if got := toolResultText(blocks); got != "a\nb" {
		t.Errorf("block content = %q, want %q", got, "a\nb")
	}
	if got := toolResultText(nil); got != "" {
		t.Errorf("nil content = %q, want empty", got)
	}
}

func TestTurnReason(t *testing.T) {
	tests := []struct {
		name string
		m    resultLine
		want string
	}{
		{"success", resultLine{Subtype: "success", StopReason: "end_turn"}, agents.TurnEndTurn},
		{"empty subtype treated as success", resultLine{StopReason: "end_turn"}, agents.TurnEndTurn},
		{"max tokens", resultLine{Subtype: "success", StopReason: "max_tokens"}, agents.TurnMaxTokens},
		{"refusal", resultLine{Subtype: "success", StopReason: "refusal"}, agents.TurnRefusal},
		{"max turns", resultLine{Subtype: "error_max_turns", IsError: true}, agents.TurnMaxRequests},
		{"other error", resultLine{Subtype: "error_during_execution", IsError: true}, agents.TurnError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := turnReason(tt.m); got != tt.want {
				t.Errorf("turnReason(%+v) = %q, want %q", tt.m, got, tt.want)
			}
		})
	}
}

func TestCommonArgsRejectsAnUnsupportedThinkingMode(t *testing.T) {
	var c Config
	_, err := c.commonArgs(agents.StartSpec{Thinking: "nonsense"})
	if err == nil {
		t.Fatal("commonArgs succeeded, want an error")
	}
}

func TestStartAndResumeArgsDiffer(t *testing.T) {
	common := []string{"--print"}
	start := startArgs(common, "the-id", "be nice")
	if start[0] != "--session-id=the-id" {
		t.Errorf("startArgs[0] = %q, want --session-id=the-id", start[0])
	}
	if start[len(start)-1] != "--append-system-prompt=be nice" {
		t.Errorf("startArgs did not end with the instructions: %v", start)
	}

	resume := resumeArgs(common, "the-id")
	if resume[0] != "--resume=the-id" {
		t.Errorf("resumeArgs[0] = %q, want --resume=the-id", resume[0])
	}
	for _, a := range resume {
		if a == "--append-system-prompt=be nice" {
			t.Errorf("resumeArgs must not carry instructions: %v", resume)
		}
	}

	noInstructions := startArgs(common, "the-id", "")
	if noInstructions[len(noInstructions)-1] == "--append-system-prompt=" {
		t.Errorf("startArgs with no instructions must not add the flag: %v", noInstructions)
	}
}

func TestToolTitle(t *testing.T) {
	longCommand := strings.Repeat("x", 200)
	wantLong, _ := agents.Truncate(longCommand, maxTitleCommandBytes)
	tests := []struct{ name, path, command, want string }{
		{"Read", "a.go", "", "Read a.go"},
		{"Bash", "", "echo hi", "Bash: echo hi"},
		{"Bash", "", longCommand, "Bash: " + wantLong},
		{"Agent", "", "", "Agent"},
	}
	for _, tt := range tests {
		if got := toolTitle(tt.name, tt.path, tt.command); got != tt.want {
			t.Errorf("toolTitle(%q, %q, %q) = %q, want %q", tt.name, tt.path, tt.command, got, tt.want)
		}
	}
}
