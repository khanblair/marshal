package trust

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func read(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestClaudeKeepsOtherKeysAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	file := filepath.Join(home, ".claude.json")
	start := `{"theme":"dark","projects":{"/a":{"allowedTools":[],"hasTrustDialogAccepted":true}}}`
	if err := os.WriteFile(file, []byte(start), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := Claude("/wt/one"); err != nil {
			t.Fatal(err)
		}
	}
	root := read(t, file)
	if string(root["theme"]) != `"dark"` {
		t.Fatalf("theme lost: %s", root["theme"])
	}
	var projects map[string]map[string]json.RawMessage
	if err := json.Unmarshal(root["projects"], &projects); err != nil {
		t.Fatal(err)
	}
	if string(projects["/wt/one"]["hasTrustDialogAccepted"]) != "true" || string(projects["/a"]["allowedTools"]) != "[]" {
		t.Fatalf("unexpected projects: %v", projects)
	}
}

func TestClaudeLeavesMissingFileAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if err := Claude("/wt/one"); err == nil {
		t.Fatal("want an error for a missing file")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); err == nil {
		t.Fatal("file must not be created")
	}
}

func TestGeminiCreatesAndAdds(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sub", "trustedFolders.json")
	t.Setenv("GEMINI_CLI_TRUSTED_FOLDERS_PATH", file)
	if err := Gemini("/wt/one"); err != nil {
		t.Fatal(err)
	}
	if got := string(read(t, file)["/wt/one"]); got != `"TRUST_FOLDER"` {
		t.Fatalf("got %s", got)
	}
}
