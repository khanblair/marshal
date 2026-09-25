package acp

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	sdk "github.com/coder/acp-go-sdk"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/proc"
)

func TestConvertUpdateIgnoresWhatTheDaemonDoesNotUse(t *testing.T) {
	ignored := map[string]sdk.SessionUpdate{
		"user echo":        sdk.UpdateUserMessageText("hi"),
		"image":            sdk.UpdateAgentMessage(sdk.ImageBlock("aGk=", "image/png")),
		"empty text":       sdk.UpdateAgentMessageText(""),
		"usage":            {UsageUpdate: &sdk.SessionUsageUpdate{Used: 1, Size: 2}},
		"available cmds":   {AvailableCommandsUpdate: &sdk.SessionAvailableCommandsUpdate{}},
		"mode change":      {CurrentModeUpdate: &sdk.SessionCurrentModeUpdate{CurrentModeId: "plan"}},
		"plan as markdown": {PlanUpdate: &sdk.SessionPlanUpdate{}},
		"nothing at all":   {},
	}
	for name, update := range ignored {
		if ev, ok := convertUpdate(update); ok {
			t.Errorf("%s became %#v, want it ignored", name, ev)
		}
	}
}

func TestConvertUpdateKeepsMessagesAndPlans(t *testing.T) {
	items := sdk.NewPlanUpdateContentItems("p1", []sdk.PlanEntry{{Content: "step", Status: sdk.PlanEntryStatusPending}})
	tests := []struct {
		name   string
		update sdk.SessionUpdate
		want   agents.AgentEvent
	}{
		{"message", sdk.UpdateAgentMessageText("hello"), agents.MessageChunk{Text: "hello"}},
		{"thought", sdk.UpdateAgentThoughtText("hmm"), agents.ThoughtChunk{Text: "hmm"}},
		{"plan update with items", sdk.SessionUpdate{PlanUpdate: &sdk.SessionPlanUpdate{Plan: items}},
			agents.PlanUpdate{Steps: []agents.PlanStep{{Text: "step", Status: agents.PlanPending}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := convertUpdate(tt.update)
			if !ok || describe([]agents.AgentEvent{got}) != describe([]agents.AgentEvent{tt.want}) {
				t.Fatalf("convertUpdate = %#v, %v, want %#v", got, ok, tt.want)
			}
			if plan, isPlan := got.(agents.PlanUpdate); isPlan && plan.Steps[0] != tt.want.(agents.PlanUpdate).Steps[0] {
				t.Errorf("plan = %+v", plan)
			}
		})
	}
}

func TestToolCallDefaults(t *testing.T) {
	got := toolCallEvent(&sdk.SessionUpdateToolCall{ToolCallId: "c1", Title: "Do it"})
	if got.Status != agents.StatusPending || got.ID != "c1" || got.Path != "" || got.Command != "" {
		t.Errorf("tool call = %+v, want a pending call with no path or command", got)
	}
	title, status := "New title", sdk.ToolCallStatusFailed
	update := toolCallUpdateEvent(&sdk.SessionToolCallUpdate{ToolCallId: "c1", Title: &title, Status: &status})
	if update.Title != title || update.Status != agents.StatusFailed {
		t.Errorf("update = %+v", update)
	}
	bare := toolCallUpdateEvent(&sdk.SessionToolCallUpdate{ToolCallId: "c1"})
	if bare.Title != "" || bare.Status != "" {
		t.Errorf("an update with no fields = %+v, want empty fields", bare)
	}
}

func TestFlattenContentSharesOneBudget(t *testing.T) {
	text := func(s string) sdk.ToolCallContent { return sdk.ToolContent(sdk.TextBlock(s)) }
	content := []sdk.ToolCallContent{
		text("aaaa"), text("bbbb"), sdk.ToolDiffContent("/f", "cccc", "dddd"),
		sdk.ToolTerminalRef("term-1"),
	}
	tests := []struct {
		name      string
		limit     int
		wantText  string
		wantDiffs []agents.FileDiff
		wantCut   bool
	}{
		{"everything fits", 100, "aaaa\nbbbb", []agents.FileDiff{{Path: "/f", NewText: "cccc", OldText: "dddd"}}, false},
		{"cut inside the second text", 6, "aaaa\nbb", []agents.FileDiff{{Path: "/f"}}, true},
		{"cut inside the diff", 10, "aaaa\nbbbb", []agents.FileDiff{{Path: "/f", NewText: "cc"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := budget{left: tt.limit}
			gotText, gotDiffs := flattenContent(content, &b)
			if gotText != tt.wantText || len(gotDiffs) != 1 || gotDiffs[0] != tt.wantDiffs[0] || b.cut != tt.wantCut {
				t.Errorf("flattenContent = %q, %+v, cut %v\nwant %q, %+v, cut %v",
					gotText, gotDiffs, b.cut, tt.wantText, tt.wantDiffs, tt.wantCut)
			}
		})
	}
}

func TestToolPathAndRawField(t *testing.T) {
	tests := []struct {
		name  string
		locs  []sdk.ToolCallLocation
		input any
		want  string
	}{
		{"a location wins", []sdk.ToolCallLocation{{Path: "/a"}}, map[string]any{"path": "/b"}, "/a"},
		{"path in the input", nil, map[string]any{"path": "/b"}, "/b"},
		{"file_path in the input", nil, map[string]any{"file_path": "/c"}, "/c"},
		{"a value that is not text", nil, map[string]any{"path": 42}, ""},
		{"an input that is not an object", nil, "just text", ""},
		{"no input", nil, nil, ""},
		{"an empty location falls through", []sdk.ToolCallLocation{{Path: ""}}, map[string]any{"filePath": "/d"}, "/d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toolPath(tt.locs, tt.input); got != tt.want {
				t.Errorf("toolPath = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPermissionEventWithoutOptionalFields(t *testing.T) {
	ev := permissionEvent("perm-1", sdk.RequestPermissionRequest{
		ToolCall: sdk.ToolCallUpdate{ToolCallId: "c9"},
		Options:  []sdk.PermissionOption{{OptionId: "ok", Name: "OK", Kind: sdk.PermissionOptionKindAllowAlways}},
	})
	if ev.RequestID != "perm-1" || ev.ToolCallID != "c9" || ev.Title != "" || ev.Kind != "" ||
		len(ev.Options) != 1 || ev.Options[0] != (agents.PermissionOption{ID: "ok", Name: "OK", Kind: agents.OptionAllowAlways}) {
		t.Errorf("event = %+v", ev)
	}
}

func TestTurnReasonCoversEveryStopReason(t *testing.T) {
	tests := map[sdk.StopReason]string{
		sdk.StopReasonEndTurn:         agents.TurnEndTurn,
		sdk.StopReasonCancelled:       agents.TurnCancelled,
		sdk.StopReasonMaxTokens:       agents.TurnMaxTokens,
		sdk.StopReasonMaxTurnRequests: agents.TurnMaxRequests,
		sdk.StopReasonRefusal:         agents.TurnRefusal,
		"a-reason-from-the-future":    agents.TurnEndTurn,
	}
	for reason, want := range tests {
		if got := turnReason(reason); got != want {
			t.Errorf("turnReason(%q) = %q, want %q", reason, got, want)
		}
	}
}

func TestIsCancellation(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{context.Canceled, true},
		{sdk.NewRequestCancelled(nil), true},
		{errors.Join(errors.New("wrapped"), context.Canceled), true},
		{sdk.NewInternalError(nil), false},
		{errors.New("other"), false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := isCancellation(tt.err); got != tt.want {
			t.Errorf("isCancellation(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestSettingHelpers(t *testing.T) {
	for _, pair := range [][2]string{{"auto-edits", "autoEdits"}, {"Auto edits", "auto_edits"}, {"PLAN", "plan"}} {
		if !sameWord(pair[0], pair[1]) {
			t.Errorf("%q and %q should be the same word", pair[0], pair[1])
		}
	}
	if sameWord("plan", "default") {
		t.Error("plan and default are different words")
	}
	if got := mapped(map[string]string{"ask": "default"}, "ask"); got != "default" {
		t.Errorf("mapped = %q", got)
	}
	if got := mapped(nil, "ask"); got != "ask" {
		t.Errorf("mapped without a map = %q", got)
	}
	all := []sdk.SessionConfigSelectOption{{Name: "Opus 4", Value: "opus"}, {Name: "Sonnet", Value: "sonnet"}}
	if c, ok := matchChoice(all, "opus-4"); !ok || c.Value != "opus" {
		t.Errorf("matchChoice by name = %+v, %v", c, ok)
	}
	if _, ok := matchChoice(all, "haiku"); ok {
		t.Error("matchChoice found a value that is not offered")
	}
	if got := strings.Join(choiceNames(all), ","); got != "opus,sonnet" {
		t.Errorf("choiceNames = %q", got)
	}
	if err := unsupported("model", "x", []string{"a", "b"}); !errors.Is(err, agents.ErrUnsupportedSetting) ||
		!strings.Contains(err.Error(), "a, b") {
		t.Errorf("unsupported = %v", err)
	}
}

func TestChoicesFlattensGroups(t *testing.T) {
	opt := groupedOption(selectSpec{"x", "X", sdk.SessionConfigOptionCategoryModel, "a", []string{"a", "b"}})
	if got := strings.Join(choiceNames(choices(opt.Select)), ","); got != "a,b" {
		t.Errorf("grouped choices = %q, want a,b", got)
	}
	flat := ungroupedOption(selectSpec{"x", "X", sdk.SessionConfigOptionCategoryModel, "a", []string{"a", "b", "c"}})
	if got := len(choices(flat.Select)); got != 3 {
		t.Errorf("flat choices = %d, want 3", got)
	}
	if got := len(choices(&sdk.SessionConfigOptionSelect{})); got != 0 {
		t.Errorf("choices of an empty option = %d", got)
	}
}

func TestMethodNames(t *testing.T) {
	methods := []sdk.AuthMethod{
		{Agent: &sdk.AuthMethodAgent{Id: "a", Name: "Agent login"}},
		{EnvVar: &sdk.AuthMethodEnvVarInline{Id: "e", Name: "API key"}},
		{Terminal: &sdk.AuthMethodTerminalInline{Id: "t", Name: "Terminal"}},
		{},
	}
	if got := strings.Join(methodNames(methods), ","); got != "Agent login,API key,Terminal" {
		t.Errorf("methodNames = %q", got)
	}
	if got := (&agents.AuthRequiredError{}).Error(); got != "the agent needs you to sign in first" {
		t.Errorf("AuthRequiredError with no methods = %q", got)
	}
}

func TestCrashTextAndTail(t *testing.T) {
	if !strings.Contains(crashMessage(true), "middle") || strings.Contains(crashMessage(false), "middle") {
		t.Error("crashMessage does not tell a turn in progress from an idle agent")
	}
	if got := crashDetail(proc.Exit{Code: 2}, ""); got != "exit code 2" {
		t.Errorf("crashDetail = %q", got)
	}
	if got := crashDetail(proc.Exit{Code: -1, Err: errors.New("signal: killed")}, "last words"); got != "signal: killed\nlast words" {
		t.Errorf("crashDetail with a tail = %q", got)
	}
	if got := endOf("héllo", 3); got != "llo" {
		t.Errorf("endOf cut inside a character = %q, want llo", got)
	}
	if got := endOf("short", 100); got != "short" {
		t.Errorf("endOf = %q", got)
	}
}

func TestConnectionLoggerDropsRawTextAndChatter(t *testing.T) {
	var out bytes.Buffer
	log := connectionLogger(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	log.Info("connection closed", "cause", "eof")
	log.Debug("dropping a cancel request")
	log.With("label", "card-7").WithGroup("g").Error("failed to parse incoming message", "raw", "SECRET PROMPT", "err", "bad json")
	text := out.String()
	if strings.Contains(text, "SECRET PROMPT") || strings.Contains(text, "connection closed") || strings.Contains(text, "dropping") {
		t.Errorf("log output should hold neither the raw text nor the chatter:\n%s", text)
	}
	if !strings.Contains(text, "failed to parse incoming message") || !strings.Contains(text, "card-7") || !strings.Contains(text, "bad json") {
		t.Errorf("log output lost the warning or its other attributes:\n%s", text)
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg, err := Config{Path: "agent"}.withDefaults()
	if err != nil {
		t.Fatalf("withDefaults: %v", err)
	}
	if cfg.StopGrace != defaultStopGrace || cfg.StartTimeout != defaultStartTimeout ||
		cfg.InterruptGrace != defaultInterruptGrace || cfg.Logger == nil {
		t.Errorf("defaults = %+v", cfg)
	}
	if _, err := (Config{}).withDefaults(); err == nil {
		t.Error("a config without a path was accepted")
	}
}
