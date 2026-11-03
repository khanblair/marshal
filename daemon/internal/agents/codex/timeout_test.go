package codex

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

func TestUncertainTurnStartStopsBeforeAnotherSend(t *testing.T) {
	a := adapter(t, "hang-turn", time.Second)
	h := start(t, a, spec(t))
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	defer cancel()
	if err := a.Send(ctx, h, agents.UserMessage{Text: "first"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled send = %v", err)
	}
	if err := a.Send(context.Background(), h, agents.UserMessage{Text: "second"}); err == nil {
		t.Fatal("another turn accepted after uncertain start")
	}
}
