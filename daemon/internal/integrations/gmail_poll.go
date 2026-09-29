package integrations

// Gmail's inbound half (B8.3, docs/marshal-product-scope.md 19.1's "cheap polling as a backup" -
// Gmail has no webhook Marshal can receive without Pub/Sub and a public address, so a ticker is
// the whole mechanism, not a fallback for one). A labeled message becomes a card once, the same
// dedupe external_links gives the Trello sync.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/gmailread"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// pollInterval is how often Gmail is asked for its labeled messages. Docs 18.5's "only fetch at
// brief time" is for briefs; this is the inbound path, and five minutes is the same order of
// magnitude as the fastest brief cadence without asking Google for anything close to real time.
const pollInterval = 5 * time.Minute

// gmailKind is the external_links kind a Gmail message is linked under, mirroring trello.ID.
const gmailKind = "gmail"

// GmailPoller reads a watched label on a timer and turns a message Marshal has not seen into a
// card, once. Built once; Start begins polling, Close stops it before the store closes.
type GmailPoller struct {
	svc *Service

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewGmailPoller builds the poller. svc is required.
func NewGmailPoller(svc *Service) (*GmailPoller, error) {
	if svc == nil || svc.store == nil {
		return nil, fmt.Errorf("integrations: the Gmail poller needs a service with a store")
	}
	return &GmailPoller{svc: svc}, nil
}

// Start begins polling on pollInterval, plus one poll right away so a label already watched at
// startup is not left until the first tick. A second call is a no-op.
func (p *GmailPoller) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		return
	}
	pollCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.done = make(chan struct{})
	go p.run(pollCtx)
}

// Close stops polling and waits for a poll in progress to finish.
func (p *GmailPoller) Close() error {
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

func (p *GmailPoller) run(ctx context.Context) {
	defer close(p.done)
	p.poll(ctx)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

// poll reads the watched label and imports every message Marshal has not already linked. One
// message that fails to import is logged and skipped; it does not stop the rest of the batch.
func (p *GmailPoller) poll(ctx context.Context) {
	config, err := p.svc.readGmail(ctx)
	if err != nil || config.Label == "" || config.ProjectID == "" {
		return
	}
	client, err := p.svc.GmailClient(ctx)
	if errors.Is(err, ErrNotConnected) || errors.Is(err, ErrNoGoogleClient) {
		return
	}
	if err != nil {
		p.svc.log.Error("could not build the Gmail client for the poll", "error", err)
		return
	}
	const maxPerPoll = 20
	messages, err := client.Labeled(ctx, config.Label, maxPerPoll)
	if err != nil {
		p.svc.log.Error("could not read Gmail's labeled messages", "label", config.Label, "error", err)
		return
	}
	cards := p.svc.attachedTrelloCards()
	if cards == nil {
		return
	}
	for _, message := range messages {
		if err := p.importOne(ctx, cards, config.ProjectID, message); err != nil {
			p.svc.log.Error("could not import a labeled email", "message", message.ID, "error", err)
		}
	}
}

// importOne makes a card from one message, unless it is already linked.
func (p *GmailPoller) importOne(ctx context.Context, cards TrelloCards, projectID string, message gmailread.Message) error {
	if _, known, err := p.svc.ExternalCard(ctx, gmailKind, message.ID); err != nil {
		return err
	} else if known {
		return nil
	}
	title := message.Subject
	if title == "" {
		title = "Email from " + message.From
	}
	created, err := cards.CreateCard(ctx, projectID, protocol.CreateCardRequest{
		Title: title, Body: fmt.Sprintf("From: %s\n\n%s", message.From, message.Snippet),
	})
	if err != nil {
		return fmt.Errorf("create a card from the email %s: %w", message.ID, err)
	}
	if err := p.svc.LinkExternal(ctx, created.ID, gmailKind, message.ID); err != nil {
		return err
	}
	p.svc.log.Info("imported a labeled email", "message", message.ID, "card", created.ID)
	return nil
}
