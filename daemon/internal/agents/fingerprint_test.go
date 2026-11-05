package agents_test

import (
	"encoding/json"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestEditFingerprintComparesCanonicalArguments(t *testing.T) {
	a := agents.ToolInputFingerprint(json.RawMessage(`{"file_path":"a.go","new_string":"first"}`))
	b := agents.ToolInputFingerprint(json.RawMessage(`{"new_string":"first", "file_path":"a.go"}`))
	c := agents.ToolInputFingerprint(json.RawMessage(`{"file_path":"a.go","new_string":"next"}`))
	if len(a) != 64 || a != b || a == c {
		t.Fatal("edit fingerprints lost content changes or depended on object ordering")
	}
	if agents.ToolInputFingerprint(json.RawMessage(`{`)) != "" {
		t.Fatal("malformed edit data was treated as known progress")
	}
}
