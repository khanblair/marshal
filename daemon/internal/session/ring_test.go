package session_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// TestRingStaysBoundedUnderHeavyOutput feeds several thousand large events straight into a live
// session's pump (bypassing the turn machinery, which is not what is under test here) and checks
// that the in-memory ring stays near its configured bound instead of growing with the output.
func TestRingStaysBoundedUnderHeavyOutput(t *testing.T) {
	const ringBytes = 8 << 10
	e := newEnv(t, func(c *session.Config) { c.RingBytes = ringBytes })
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	row, err := e.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("GetSessionByCard: %v", err)
	}
	fs, err := e.agent.find(row.AgentSessionID)
	if err != nil {
		t.Fatalf("find the fake session: %v", err)
	}

	const total = 4000
	big := strings.Repeat("x", 512)
	for range total {
		fs.sink.Emit(agents.MessageChunk{Text: big})
	}
	fs.sink.Emit(agents.MessageChunk{Text: "marker-done"})

	deadline := time.Now().Add(eventTimeout)
	var entries []session.LogEntry
	for time.Now().Before(deadline) {
		entries = e.mgr.RecentOutput(card.ID)
		if last := lastMessageText(entries); last == "marker-done" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if last := lastMessageText(entries); last != "marker-done" {
		t.Fatalf("the pump never caught up; last ring entry = %q", last)
	}
	if len(entries) == 0 {
		t.Fatal("the ring is empty after heavy output")
	}
	if len(entries) >= total {
		t.Errorf("the ring holds %d entries out of %d published, want it to have dropped most of them", len(entries), total)
	}
	var textBytes int
	for _, entry := range entries {
		if mc, ok := entry.Event.(agents.MessageChunk); ok {
			textBytes += len(mc.Text)
		}
	}
	// The ring bounds by an estimate that includes JSON overhead per entry (see the report), not
	// raw text alone, so the raw text total stays a little under the configured bound rather than
	// exactly at it.
	if textBytes > ringBytes {
		t.Errorf("the ring holds about %d bytes of text, want it bounded near the configured %d", textBytes, ringBytes)
	}

	// The manager's own live-session bookkeeping did not grow either: the flood went to the one
	// card that has a live session, and an unrelated card still has none.
	if got := e.mgr.RecentOutput("no-such-card"); got != nil {
		t.Errorf("RecentOutput of an unrelated card = %v, want nil", got)
	}
}

func lastMessageText(entries []session.LogEntry) string {
	if len(entries) == 0 {
		return ""
	}
	mc, ok := entries[len(entries)-1].Event.(agents.MessageChunk)
	if !ok {
		return ""
	}
	return mc.Text
}
