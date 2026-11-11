package session_test

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/providers"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// recordedUsage is a recorder that keeps what it is given.
type recordedUsage struct {
	mu   sync.Mutex
	rows []providers.UsageRecord
}

func (r *recordedUsage) Record(_ context.Context, rec providers.UsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, rec)
	return nil
}

func (r *recordedUsage) all() []providers.UsageRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]providers.UsageRecord(nil), r.rows...)
}

func TestATurnThatReportsUsageIsRecordedAgainstItsCardAndProject(t *testing.T) {
	rec := &recordedUsage{}
	e := newEnv(t, func(c *session.Config) { c.Usage = rec })
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.agent.turnUsage = &agents.Usage{Provider: "anthropic", Model: "claude-sonnet-5-5",
		InputTokens: 1200, OutputTokens: 80, CostMicros: 4500}
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "go"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	e.untilType(t, protocol.EventTypeSessionOutput)
	var rows []providers.UsageRecord
	deadline := time.Now().Add(eventTimeout)
	for len(rows) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		rows = rec.all()
	}
	if len(rows) == 0 {
		t.Fatal("no usage was recorded for the turn")
	}
	got := rows[0]
	if got.CardID != card.ID || got.ProjectID != project.ID || got.Provider != "anthropic" ||
		got.Model != "claude-sonnet-5-5" || got.InputTokens != 1200 || got.OutputTokens != 80 || got.CostMicros != 4500 {
		t.Errorf("usage row = %+v", got)
	}
}

func TestATurnThatReportsNoUsageRecordsNothing(t *testing.T) {
	rec := &recordedUsage{}
	e := newEnv(t, func(c *session.Config) { c.Usage = rec })
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.mgr.Send(context.Background(), card.ID, "go"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	e.untilType(t, protocol.EventTypeSessionOutput)
	time.Sleep(100 * time.Millisecond)
	if rows := rec.all(); len(rows) != 0 {
		t.Errorf("recorded %+v, want nothing", rows)
	}
}

func TestCardAndBoardShowTheRecordedCostAndMeasuredContext(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a health check")
	const cost, contextUsed = 9_540_000, 42
	recorder := providers.NewStoreRecorder(e.store, rand.Reader)
	if err := recorder.Record(ctx, providers.UsageRecord{
		CardID: card.ID, ProjectID: project.ID, Provider: "anthropic",
		Model: "claude-sonnet-5-5", CostMicros: cost, At: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	e.agent.turnContext = contextUsed
	startCard(t, e, card)
	if err := e.mgr.Send(ctx, card.ID, "go"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "recorded cost and context", func() bool {
		got, err := e.proj.Card(ctx, card.ID)
		return err == nil && got.CostMicros == cost && got.ContextUsed == contextUsed
	})
	board, err := e.proj.Board(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Cards) != 1 || board.Cards[0].CostMicros != cost || board.Cards[0].ContextUsed != contextUsed {
		t.Fatalf("board cards = %+v, want one card with the recorded cost and context", board.Cards)
	}
}
