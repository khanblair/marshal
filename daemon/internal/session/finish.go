package session

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// FinishHandler is what happens when a card's agent says its work is finished and its turn then ends:
// the merge flow takes the card. The merge queue is one.
type FinishHandler interface {
	CardFinished(ctx context.Context, cardID, summary string)
}

// finishHandler is the manager's handler and the lock that guards it.
type finishHandler struct {
	mu      sync.RWMutex
	handler FinishHandler
}

// SetFinishHandler gives the manager the module that takes a card its agent has finished. It is set
// after the manager is built, like SetAttacher, and a turn that ends later reads it.
func (m *Manager) SetFinishHandler(h FinishHandler) {
	m.finish.mu.Lock()
	m.finish.handler = h
	m.finish.mu.Unlock()
}

// Finish records that a card's agent says it is finished. It is called by the agent's own finish_card
// tool in the middle of its turn, so nothing happens yet: the work is committed and handed on when
// the turn ends (onTurnEnded), which is why a commit made after the call still counts.
func (m *Manager) Finish(_ context.Context, cardID, summary string) error {
	ls, err := m.live(cardID)
	if err != nil {
		return errors.New("this card has no running session to finish")
	}
	ls.finishMu.Lock()
	ls.finishSummary, ls.finished = strings.TrimSpace(summary), true
	ls.finishMu.Unlock()
	return nil
}

// takeFinish says whether the card's agent said it is finished, and clears it.
func (ls *liveSession) takeFinish() (string, bool) {
	ls.finishMu.Lock()
	defer ls.finishMu.Unlock()
	summary, ok := ls.finishSummary, ls.finished
	ls.finishSummary, ls.finished = "", false
	return summary, ok
}

// cardFinished hands a card its agent has finished to the handler, on its own goroutine: the handler
// moves the card and may start a review, and the pump must go on reading the session's events. It
// answers whether the agent had said it was finished.
func (m *Manager) cardFinished(ls *liveSession) bool {
	summary, ok := ls.takeFinish()
	if !ok || ls.isChat() {
		return false
	}
	m.finish.mu.RLock()
	handler := m.finish.handler
	m.finish.mu.RUnlock()
	if handler == nil {
		return true
	}
	go handler.CardFinished(m.ctx, ls.cardID, summary)
	return true
}
