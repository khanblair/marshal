package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidSessionID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"stub-abc123", true},
		{"A_b-9", true},
		{"", false},
		{"..", false},
		{"../x", false},
		{"a/b", false},
		{`a\b`, false},
		{"a b", false},
		{"a.json", false},
		{strings.Repeat("a", maxSessionIDLen), true},
		{strings.Repeat("a", maxSessionIDLen+1), false},
	}
	for _, tt := range tests {
		if got := validSessionID(tt.id); got != tt.want {
			t.Errorf("validSessionID(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestNewSessionIDsAreValidAndDifferent(t *testing.T) {
	a, b := newSessionID(), newSessionID()
	if a == b || !validSessionID(a) || !validSessionID(b) {
		t.Errorf("ids %q and %q", a, b)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	st := store{dir: filepath.Join(t.TempDir(), "nested", "state")}
	want := sessionState{
		ID: "stub-one", Cwd: "/work", Turn: 2,
		Turns: []turnRecord{{User: "a", Agent: "b"}, {User: "c", Agent: "d"}},
	}
	if err := st.save(want); err != nil {
		t.Fatal(err)
	}
	got, err := st.load("stub-one")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loaded %+v, want %+v", got, want)
	}

	// Saving again replaces the file and leaves no temporary files behind.
	want.Turn = 3
	if err := st.save(want); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(st.dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "stub-one.json" {
		t.Errorf("state folder = %v, %v; want only stub-one.json", entries, err)
	}
}

func TestStoreRefusesIDsThatLeaveTheFolder(t *testing.T) {
	root := t.TempDir()
	st := store{dir: filepath.Join(root, "state")}
	if err := os.WriteFile(filepath.Join(root, "secret.json"), []byte(`{"sessionId":"../secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../secret", "..", "a/b", ""} {
		if _, err := st.load(id); !errors.Is(err, errSessionNotFound) {
			t.Errorf("load(%q) error = %v, want session not found", id, err)
		}
		if err := st.save(sessionState{ID: id}); err == nil {
			t.Errorf("save(%q) succeeded", id)
		}
	}
	if _, err := os.Stat(st.dir); err == nil {
		t.Error("a refused id created the state folder")
	}
}

func TestStoreLoadErrors(t *testing.T) {
	st := store{dir: t.TempDir()}
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "corrupt", content: `{`, want: "decode session stub-bad"},
		{name: "wrong id inside", content: `{"sessionId":"stub-other"}`, want: `holds session "stub-other"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(st.dir, "stub-bad.json"), []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := st.load("stub-bad")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
	if _, err := st.load("stub-missing"); !errors.Is(err, errSessionNotFound) {
		t.Errorf("missing file error = %v", err)
	}
}

func TestFailedSaveKeepsTheCounter(t *testing.T) {
	// A file where the state folder should be makes every save fail.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	st := store{dir: filepath.Join(blocker, "state")}
	sess := newSession(sessionState{ID: "stub-one", Cwd: "/work"})

	if err := sess.record(st, "hello", "hi"); err == nil {
		t.Fatal("record succeeded without a place to save")
	}
	if got := sess.snapshot(); got.Turn != 0 || len(got.Turns) != 0 {
		t.Errorf("state after a failed save = %+v, want it unchanged", got)
	}
	if err := sess.moveTo(st, "/elsewhere"); err == nil {
		t.Error("moveTo succeeded without a place to save")
	}
	if got := sess.snapshot(); got.Cwd != "/work" {
		t.Errorf("cwd after a failed move = %q", got.Cwd)
	}
}

func TestSnapshotIsACopy(t *testing.T) {
	st := store{dir: t.TempDir()}
	sess := newSession(sessionState{ID: "stub-one", Cwd: "/work"})
	if err := sess.record(st, "a", "b"); err != nil {
		t.Fatal(err)
	}
	snap := sess.snapshot()
	snap.Turns[0].User = "changed"
	if got := sess.snapshot().Turns[0].User; got != "a" {
		t.Errorf("changing a snapshot changed the session: %q", got)
	}
}
