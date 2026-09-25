package codex

import (
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

func TestNewSaysThatCodexCannotBeStarted(t *testing.T) {
	agent, err := New(Config{Path: "/usr/local/bin/codex"})
	if agent != nil {
		t.Errorf("agent = %v, want none", agent)
	}
	var notStartable NotStartableError
	if !errors.As(err, &notStartable) {
		t.Fatalf("err = %T %v, want a NotStartableError", err, err)
	}
	want := "Codex cannot be started from Marshal yet. Use Claude Code or Gemini CLI for now."
	if err.Error() != want {
		t.Errorf("message = %q, want %q", err.Error(), want)
	}
}

func TestTheRegistryPassesTheSentenceOn(t *testing.T) {
	reg := agents.NewRegistry()
	if err := reg.Register(protocol.AgentKindCodex, Factory(Config{})); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err := reg.New(protocol.AgentKindCodex)
	var notStartable NotStartableError
	if !errors.As(err, &notStartable) {
		t.Errorf("err = %v, want it to hold a NotStartableError, so the API layer can say what happened", err)
	}
}
