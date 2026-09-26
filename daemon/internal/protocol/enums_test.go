package protocol_test

import (
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// names turns a list of string-typed values into plain strings.
func names[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return out
}

// allEnums lists every fixed list of the protocol by its Go type name. The TypeScript tests
// compare the generated arrays with the golden file made from it.
func allEnums() map[string][]string {
	return map[string][]string{
		"CardState":             names(protocol.CardStateValues()),
		"PermissionMode":        names(protocol.PermissionModeValues()),
		"ThinkingMode":          names(protocol.ThinkingModeValues()),
		"AgentKind":             names(protocol.AgentKindValues()),
		"AgentStatus":           names(protocol.AgentStatusValues()),
		"SessionState":          names(protocol.SessionStateValues()),
		"FeedKind":              names(protocol.FeedKindValues()),
		"NoticeKind":            names(protocol.NoticeKindValues()),
		"ActivityKind":          names(protocol.ActivityKindValues()),
		"ActivityState":         names(protocol.ActivityStateValues()),
		"ChatMessageKind":       names(protocol.ChatMessageKindValues()),
		"ChatPlanState":         names(protocol.ChatPlanStateValues()),
		"ChatApprovalState":     names(protocol.ChatApprovalStateValues()),
		"CIState":               names(protocol.CIStateValues()),
		"CardViewMode":          names(protocol.CardViewModeValues()),
		"ProjectSource":         names(protocol.ProjectSourceValues()),
		"ErrorCode":             names(protocol.ErrorCodeValues()),
		"TopicKind":             names(protocol.TopicKindValues()),
		"EventType":             names(protocol.EventTypeValues()),
		"ResyncReason":          names(protocol.ResyncReasonValues()),
		"FrameType":             names(protocol.FrameTypeValues()),
		"DeviceKind":            names(protocol.DeviceKindValues()),
		"NeedsReasonKind":       names(protocol.NeedsReasonKindValues()),
		"LabelColor":            names(protocol.LabelColorValues()),
		"MoveRefusalReason":     names(protocol.MoveRefusalReasonValues()),
		"ChatTargetKind":        names(protocol.ChatTargetKindValues()),
		"ChatRefusalReason":     names(protocol.ChatRefusalReasonValues()),
		"HoldRefusalReason":     names(protocol.HoldRefusalReasonValues()),
		"ViewRefusalReason":     names(protocol.ViewRefusalReasonValues()),
		"TerminalKey":           names(protocol.TerminalKeyValues()),
		"TerminalRefusalReason": names(protocol.TerminalRefusalReasonValues()),
		"DiffFileStatus":        names(protocol.DiffFileStatusValues()),
		"DiffLineKind":          names(protocol.DiffLineKindValues()),
		"Theme":                 names(protocol.ThemeValues()),
		"ProjectView":           names(protocol.ProjectViewValues()),
		"Swimlane":              names(protocol.SwimlaneValues()),
		"FilterKey":             names(protocol.FilterKeyValues()),
		"SortDirection":         names(protocol.SortDirectionValues()),
		"ProgressStatus":        names(protocol.ProgressStatusValues()),
	}
}

func TestEnumsGolden(t *testing.T) {
	testutil.Golden(t, "enums", allEnums())
}

// The words below are the ones the app's screens use (apps/web/src/mock), as wire values.
func TestEnumsCarryTheWordsTheScreensUse(t *testing.T) {
	want := map[string][]string{
		"CardState":      {"backlog", "planning", "working", "needs", "review", "ready", "merging", "done"},
		"PermissionMode": {"ask", "auto-edits", "plan", "full-auto", "bypass"},
		"ThinkingMode":   {"low", "medium", "high", "extra-high"},
		"AgentKind":      {"claude", "gemini", "codex", "builtin"},
		"AgentStatus":    {"supported", "untested", "missing"},
		"SessionState":   {"starting", "awake", "working", "waiting-approval", "sleep-warning", "asleep", "waking", "stopped"},
		"FeedKind":       {"brief", "merge", "schedule", "approval", "plan", "ci", "tool"},
		"NoticeKind":     {"sleep", "ci-main", "cost", "plan", "ci"},
		"ActivityKind":   {"file", "command", "test", "tool", "approval"},
		"CIState":        {"queued", "running", "passed", "failed", "cancelled"},
		"CardViewMode":   {"chat", "terminal"},
	}
	got := allEnums()
	for name, values := range want {
		if !equalStrings(got[name], values) {
			t.Errorf("%s = %v, want %v", name, got[name], values)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestValid(t *testing.T) {
	if !protocol.CardStateNeeds.Valid() || protocol.CardState("needs-you").Valid() {
		t.Error("CardState.Valid accepts the wrong words")
	}
	if !protocol.PermissionModeBypass.Valid() || protocol.PermissionMode("Bypass permissions").Valid() {
		t.Error("PermissionMode.Valid must accept wire values only, not labels")
	}
	if !protocol.ThinkingModeExtraHigh.Valid() || protocol.ThinkingMode("Extra high").Valid() {
		t.Error("ThinkingMode.Valid must accept wire values only, not labels")
	}
	if !protocol.AgentKindClaude.Valid() || protocol.AgentKind("").Valid() {
		t.Error("AgentKind.Valid accepts the wrong words")
	}
}
