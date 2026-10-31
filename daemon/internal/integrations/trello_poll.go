package integrations

// Reading the linked Trello board on a timer and when a webhook asks (docs/marshal-product-scope.md
// section 19.2). The timer is the fallback for a board whose webhook is not set up or not reachable;
// the webhook only shortens the wait. Both go through RefreshTrello, so there is one way in.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	// trelloPollEvery is how often the board is read when nothing asks sooner.
	trelloPollEvery = 5 * time.Minute
	// trelloDebounce is how long a webhook waits before the read, so a burst of changes is one read.
	trelloDebounce = 3 * time.Second
	// trelloReadLimit bounds one read of a board.
	trelloReadLimit = 3 * time.Minute
)

// TrelloPoller reads the linked board on a timer and on request. Built once; Start begins, Close
// stops it before the store closes.
type TrelloPoller struct {
	svc      *Service
	kick     chan struct{}
	debounce time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewTrelloPoller builds the poller and registers it with the service, so a webhook can ask for a read.
func NewTrelloPoller(svc *Service) (*TrelloPoller, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("integrations: the Trello poller needs a service with a store")
	}
	p := &TrelloPoller{svc: svc, kick: make(chan struct{}, 1), debounce: trelloDebounce}
	svc.mu.Lock()
	svc.trelloKick = p.Kick
	svc.mu.Unlock()
	return p, nil
}

// Kick asks for a read soon. Many kicks before the read are one read.
func (p *TrelloPoller) Kick() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}

// kickTrello asks the poller for a read, and reports whether one is registered.
func (s *Service) kickTrello() bool {
	s.mu.Lock()
	kick := s.trelloKick
	s.mu.Unlock()
	if kick == nil {
		return false
	}
	kick()
	return true
}

// Start begins reading, with one read right away. A second call is a no-op.
func (p *TrelloPoller) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.done = make(chan struct{})
	go p.run(runCtx)
}

// Close stops reading and waits for a read in progress to end.
func (p *TrelloPoller) Close() error {
	p.mu.Lock()
	cancel, done := p.cancel, p.done
	p.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-done
	return nil
}

func (p *TrelloPoller) run(ctx context.Context) {
	defer close(p.done)
	p.poll(ctx)
	ticker := time.NewTicker(trelloPollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		case <-p.kick:
			select {
			case <-ctx.Done():
				return
			case <-time.After(p.debounce):
			}
			p.poll(ctx)
		}
	}
}

// poll reads the linked board once. A daemon with no board linked, or no Trello token, does nothing.
func (p *TrelloPoller) poll(ctx context.Context) {
	link, ok, err := p.svc.TrelloLink(ctx)
	if err != nil || !ok {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, trelloReadLimit)
	defer cancel()
	counts, err := p.svc.RefreshTrello(readCtx, link.ProjectID, nil)
	if errors.Is(err, ErrNotConnected) || errors.Is(err, ErrTrelloNotLinked) {
		return
	}
	if err != nil {
		p.svc.log.Warn("could not read the Trello board", "error", err)
		return
	}
	if counts.Imported > 0 || counts.Updated > 0 {
		p.svc.log.Info("read the Trello board", "imported", counts.Imported, "updated", counts.Updated)
	}
}
