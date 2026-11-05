package claude

import (
	"encoding/json"
	"testing"
)

func TestClaudeEditEventsIdentifyTheActualInput(t *testing.T) {
	first := toolCallEvent(contentBlock{Name: "Edit", Input: json.RawMessage(`{"file_path":"main.go","old_string":"one","new_string":"two"}`)})
	next := toolCallEvent(contentBlock{Name: "Edit", Input: json.RawMessage(`{"file_path":"main.go","old_string":"two","new_string":"three"}`)})
	repeated := toolCallEvent(contentBlock{Name: "Edit", Input: json.RawMessage(`{"new_string":"two","old_string":"one","file_path":"main.go"}`)})
	if first.EditFingerprint == "" || first.EditFingerprint == next.EditFingerprint || first.EditFingerprint != repeated.EditFingerprint {
		t.Fatal("the stuck detector cannot distinguish Claude edit inputs")
	}
	read := toolCallEvent(contentBlock{Name: "Read", Input: json.RawMessage(`{"file_path":"main.go"}`)})
	if read.EditFingerprint != "" {
		t.Fatal("a read was marked as an edit")
	}
}
