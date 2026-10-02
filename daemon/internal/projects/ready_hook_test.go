package projects_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The ready callback runs once for a card that enters Ready to merge, whether the daemon moved it or
// a person did, and for no other move.
func TestTheReadyCallbackRunsWhenACardEntersReady(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	project := e.folder(t, "small-repo")
	var mu sync.Mutex
	var called []string
	e.svc.SetOnReadyToMerge(func(_ context.Context, cardID string) {
		mu.Lock()
		defer mu.Unlock()
		called = append(called, cardID)
	})
	got := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(called)
	}

	byDaemon := e.card(t, project.ID, "Moved by the daemon")
	if _, err := e.svc.SetState(ctx, byDaemon.ID, protocol.CardStateWorking); err != nil {
		t.Fatal(err)
	}
	if len(got()) != 0 {
		t.Fatalf("the callback ran for a move that is not to ready: %v", got())
	}
	if _, err := e.svc.SetState(ctx, byDaemon.ID, protocol.CardStateReady); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetState(ctx, byDaemon.ID, protocol.CardStateReady); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got(), []string{byDaemon.ID}) {
		t.Fatalf("callbacks = %v, want one for the daemon's move, and none for a move to the state it is in", got())
	}

	byHand := e.card(t, project.ID, "Moved by a person")
	if _, err := e.svc.SetState(ctx, byHand.ID, protocol.CardStateReview); err != nil {
		t.Fatal(err)
	}
	passed := protocol.CIStatePassed
	if _, err := e.svc.SetCI(ctx, byHand.ID, &passed); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.MoveCard(ctx, byHand.ID, protocol.MoveCardRequest{State: protocol.CardStateReady}); err != nil {
		t.Fatalf("MoveCard: %v", err)
	}
	if !slices.Equal(got(), []string{byDaemon.ID, byHand.ID}) {
		t.Errorf("callbacks = %v, want the person's move announced too", got())
	}

	e.svc.SetOnReadyToMerge(nil)
	later := e.card(t, project.ID, "After the callback was removed")
	if _, err := e.svc.SetState(ctx, later.ID, protocol.CardStateReady); err != nil {
		t.Fatal(err)
	}
	if len(got()) != 2 {
		t.Errorf("callbacks = %v after the callback was removed, want no more", got())
	}
}
