package main

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/acp-go-sdk"
)

func TestBuiltinScenariosLoad(t *testing.T) {
	found, err := loadBuiltinScenarios()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"approve", "default", "fail-test", "resume", "smells", "stuck-loop"}
	if got := strings.Split(scenarioNames(found), ", "); !slices.Equal(got, want) {
		t.Errorf("built-in scenarios = %v, want %v", got, want)
	}
	for name, sc := range found {
		if sc.Name != name || sc.Description == "" || len(sc.Steps) == 0 {
			t.Errorf("scenario %s is incomplete: %+v", name, sc)
		}
	}
}

func TestMalformedScenariosGiveAPlainError(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "not json", json: `{`, want: "decode"},
		{name: "text after the scenario", json: `{"steps":[{"type":"say","text":"x"}]} {}`, want: "extra data"},
		{name: "no steps", json: `{"steps":[]}`, want: "no steps"},
		{name: "unknown field", json: `{"steps":[{"type":"say","text":"x","typo":1}]}`, want: "unknown field"},
		{name: "unknown step type", json: `{"steps":[{"type":"shout"}]}`, want: `step 1: unknown step type "shout"`},
		{name: "say without text", json: `{"steps":[{"type":"say"}]}`, want: "say needs text"},
		{name: "negative pause", json: `{"steps":[{"type":"pause","ms":-1}]}`, want: "out of range"},
		{name: "tool without id", json: `{"steps":[{"type":"tool","title":"t","kind":"read"}]}`, want: "needs an id"},
		{name: "unknown kind", json: `{"steps":[{"type":"tool","id":"a","title":"t","kind":"fly"}]}`, want: `unknown tool kind "fly"`},
		{
			name: "unknown status",
			json: `{"steps":[{"type":"tool","id":"a","title":"t","kind":"read","status":"maybe"}]}`,
			want: `unknown tool status "maybe"`,
		},
		{
			name: "write on a read tool",
			json: `{"steps":[{"type":"tool","id":"a","title":"t","kind":"read","write":{"path":"p"}}]}`,
			want: "only an edit tool call",
		},
		{
			name: "write without a path",
			json: `{"steps":[{"type":"tool","id":"a","title":"t","kind":"edit","write":{"content":"c"}}]}`,
			want: "write needs a path",
		},
		{
			name: "write with content and lines",
			json: `{"steps":[{"type":"tool","id":"a","title":"t","kind":"edit","write":{"path":"p","content":"c","lines":["l"]}}]}`,
			want: "content or lines",
		},
		{name: "repeat of zero", json: `{"steps":[{"type":"repeat","count":0,"steps":[{"type":"say","text":"x"}]}]}`, want: "repeat count 0"},
		{name: "repeat without steps", json: `{"steps":[{"type":"repeat","count":2}]}`, want: "repeat needs steps"},
		{name: "unknown stop reason", json: `{"steps":[{"type":"end","reason":"boredom"}]}`, want: `unknown stop reason "boredom"`},
		{
			name: "bad step inside onDeny",
			json: `{"steps":[{"type":"permission","id":"a","title":"t","kind":"execute","onDeny":[{"type":"say"}]}]}`,
			want: "step 1: onDeny: step 1: say needs text",
		},
		{
			name: "bad step inside a repeat",
			json: `{"steps":[{"type":"repeat","count":2,"steps":[{"type":"pause","ms":-5}]}]}`,
			want: "repeat: step 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadScenarios(fstest.MapFS{"bad.json": {Data: []byte(tt.json)}})
			if err == nil {
				t.Fatal("expected an error")
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, "scenario bad.json: ") || !strings.Contains(msg, tt.want) {
				t.Errorf("error = %q, want the file name and %q", msg, tt.want)
			}
			if msg != strings.ToLower(msg[:1])+msg[1:] {
				t.Errorf("error %q does not start in lower case", msg)
			}
		})
	}
}

func TestLoadScenariosNamesThemByFile(t *testing.T) {
	found, err := loadScenarios(fstest.MapFS{
		"one.json":  {Data: []byte(`{"steps":[{"type":"say","text":"1"}]}`)},
		"two.json":  {Data: []byte(`{"steps":[{"type":"say","text":"2"}]}`)},
		"notes.txt": {Data: []byte(`not a scenario`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := scenarioNames(found); got != "one, two" {
		t.Errorf("names = %q", got)
	}
}

func TestNewAgentNeedsTheDefaultScenario(t *testing.T) {
	_, err := newAgent(config{scenario: "missing"}, map[string]scenario{"other": {}})
	if err == nil || !strings.Contains(err.Error(), `unknown scenario "missing" (known: other)`) {
		t.Errorf("error = %v", err)
	}
}

func TestScenarioMarker(t *testing.T) {
	tests := []struct{ text, want string }{
		{"", ""},
		{"no marker", ""},
		{"@scenario:approve", "approve"},
		{"fix the bug @scenario:approve", "approve"},
		{"fix @scenario:approve now", "approve"},
		{"sentence end @scenario:stuck-loop.", "stuck-loop"},
		{"@scenario:", ""},
		{"@scenario:a_b-1 @scenario:other", "a_b-1"},
		{"@scenarios:approve", ""},
	}
	for _, tt := range tests {
		if got := scenarioMarker(tt.text); got != tt.want {
			t.Errorf("scenarioMarker(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestPromptTextJoinsTextBlocksOnly(t *testing.T) {
	blocks := []acp.ContentBlock{
		acp.TextBlock("first"),
		acp.ResourceLinkBlock("file", "file:///x"),
		acp.TextBlock("second"),
	}
	if got := promptText(blocks); got != "first\nsecond" {
		t.Errorf("promptText = %q", got)
	}
}

func TestFileWriteText(t *testing.T) {
	tests := []struct {
		name string
		w    fileWrite
		want string
	}{
		{name: "content", w: fileWrite{Content: "a\nb"}, want: "a\nb"},
		{name: "lines", w: fileWrite{Lines: []string{"a", "b"}}, want: "a\nb\n"},
	}
	for _, tt := range tests {
		if got := tt.w.text(); got != tt.want {
			t.Errorf("%s: text = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestPauseScalesWithSpeed(t *testing.T) {
	r := &runner{speed: 0}
	start := time.Now()
	if err := r.pause(context.Background(), maxPauseMs); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Error("a pause at speed 0 waited")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.speed = 1
	if err := r.pause(ctx, maxPauseMs); err == nil {
		t.Error("a cancelled pause did not stop")
	}
}

func TestEndStepStopsNestedSteps(t *testing.T) {
	h := newHarness(t, harnessOptions{scenarios: map[string]string{
		"default.json": scenarioFile(t,
			map[string]any{
				"type": "repeat", "count": 3,
				"steps": []map[string]any{
					{"type": "say", "text": "once"},
					{"type": "end", "reason": "refusal"},
				},
			},
			map[string]any{"type": "say", "text": "never"},
		),
	}})
	sid := h.newSession()
	resp, err := h.prompt(sid, "go")
	if err != nil || resp.StopReason != acp.StopReasonRefusal {
		t.Fatalf("prompt = %+v, %v; want stop reason refusal", resp, err)
	}
	wantShapes(t, h, "agent", "agent")
}
