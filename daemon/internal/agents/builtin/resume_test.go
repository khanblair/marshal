package builtin

import (
	"context"
	"errors"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/providers"
)

func TestResumeRefusesMissingOrUnreadableHistory(t *testing.T) {
	for _, missing := range []bool{true, false} {
		t.Run(map[bool]string{true: "missing reader", false: "read failure"}[missing], func(t *testing.T) {
			cfg := Config{Resolver: &fakeResolver{client: newScriptedClient()}, Logger: quiet()}
			if !missing {
				cfg.History = func(context.Context, string) ([]providers.Message, error) {
					return nil, errors.New("database unavailable")
				}
			}
			a, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			h, err := a.Resume(context.Background(), "saved-session", agents.StartSpec{Cwd: t.TempDir()})
			if err == nil {
				_ = a.Stop(context.Background(), h)
				t.Fatal("resume silently started without the earlier conversation")
			}
			if !errors.Is(err, agents.ErrCannotResume) {
				t.Fatalf("resume error = %v", err)
			}
		})
	}
}

func TestToolResultsAreKeptForTheNextTurn(t *testing.T) {
	dir, _ := worktree(t)
	client := newScriptedClient(
		[]providers.Event{callTool("read-1", "read_file", `{"path":"hello.txt"}`), done(providers.StopToolUse)},
		[]providers.Event{text("I read the file."), done(providers.StopEndTurn)},
		[]providers.Event{text("I remember."), done(providers.StopEndTurn)},
	)
	a := newTestAgent(t, client)
	h, ch := start(t, a, dir, "full-auto")
	send(t, a, h, "Read hello.txt.")
	waitFor[agents.TurnEnded](t, ch)
	send(t, a, h, "What was in the file?")
	waitFor[agents.TurnEnded](t, ch)
	for _, message := range client.requests()[2].Messages {
		for _, part := range message.Parts {
			if part.Kind == providers.PartToolResult && part.ToolUseID == "read-1" {
				return
			}
		}
	}
	t.Fatal("the next turn lost the earlier tool result")
}
