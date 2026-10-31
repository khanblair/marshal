package integrations

// Writing the changes a person approved to Trello (docs/marshal-product-scope.md section 19.2). It is
// the only place Marshal writes to Trello. Every write is made as the linked person, one at a time
// with a short wait between, after a check that Trello did not change the same thing meanwhile.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/integrations/trello"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	reasonConflict = "Trello changed this since Marshal last read it. Apply it anyway, or skip it."
	reasonGone     = "That card, list, or item is gone from Trello."
	reasonStale    = "That change is no longer pending."
	reasonArchived = "The Trello card is archived."
	reasonToken    = "Trello refused the token. Connect Trello again in Settings."
	reasonSlow     = "Trello asked Marshal to slow down. Try again in a minute."
)

// applyRun is the state of one apply: what Trello holds now for each card it touched, and the board's
// lists, which are read once and only when a change needs them.
type applyRun struct {
	env   trelloEnv
	live  map[string]trello.Card
	lists []trello.List
	read  bool
	snaps map[string]storedSnap
}

// ApplyTrello writes the chosen pending changes to Trello. A change in conflict is skipped unless it
// is also named in OverrideIDs. A change that cannot be written is skipped with the reason and the
// rest go on, except when Trello refuses the token or asks Marshal to slow down: then the rest wait.
func (s *Service) ApplyTrello(ctx context.Context, projectID string, req protocol.ApplyTrelloRequest) (protocol.ApplyTrelloResult, error) {
	s.trelloMu.Lock()
	defer s.trelloMu.Unlock()
	env, err := s.trelloEnvFor(ctx, projectID)
	if err != nil {
		return protocol.ApplyTrelloResult{}, err
	}
	ops, snaps, err := s.trelloOps(ctx, env, projectID)
	if err != nil {
		return protocol.ApplyTrelloResult{}, err
	}
	chosen := toSet(req.ChangeIDs)
	override := toSet(req.OverrideIDs)
	run := &applyRun{env: env, live: map[string]trello.Card{}, snaps: snaps}
	result := protocol.ApplyTrelloResult{Applied: []string{}, Skipped: []protocol.TrelloApplySkip{}}
	seen := map[string]bool{}
	halt := ""
	for _, op := range ops {
		id := op.change.ID
		if !chosen[id] {
			continue
		}
		seen[id] = true
		if halt != "" {
			result.Skipped = append(result.Skipped, protocol.TrelloApplySkip{ChangeID: id, Reason: halt})
			continue
		}
		reason, fatal := s.applyOne(ctx, run, op, override[id])
		if reason == "" {
			result.Applied = append(result.Applied, id)
			continue
		}
		result.Skipped = append(result.Skipped, protocol.TrelloApplySkip{ChangeID: id, Reason: reason})
		if fatal {
			halt = reason
		}
	}
	for _, id := range req.ChangeIDs {
		if !seen[id] {
			result.Skipped = append(result.Skipped, protocol.TrelloApplySkip{ChangeID: id, Reason: reasonStale})
		}
	}
	pending, err := s.PendingTrello(ctx, projectID)
	if err != nil {
		return result, err
	}
	result.Pending = pending
	s.notifyTrello(projectID)
	return result, nil
}

func toSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// applyOne carries out one change. It answers "" when it was written, or the reason it was not, and
// whether the reason should stop the rest of the apply.
func (s *Service) applyOne(ctx context.Context, run *applyRun, op trelloOp, override bool) (string, bool) {
	if op.change.Kind == protocol.TrelloChangeKindCreateCard {
		return s.failureReason(s.applyCreate(ctx, run, op))
	}
	snap, ok := run.snaps[op.card.ID]
	if !ok {
		return reasonStale, false
	}
	live, reason, fatal := s.liveCard(ctx, run, snap.TrelloID)
	if reason != "" {
		return reason, fatal
	}
	if live.Closed {
		return reasonArchived, false
	}
	doneID, err := s.doneListID(ctx, run, op)
	if err != nil {
		return s.failureReason(err)
	}
	if !override && (op.change.Conflict || liveConflict(op, live, snap.Snap, doneID) != "") {
		return reasonConflict, false
	}
	if err := s.pause(ctx); err != nil {
		return s.failureReason(err)
	}
	// writeOp answers the snapshot even when it failed part way, and the next change on this card must
	// build on that, or the record of what reached Trello is lost.
	cur, err := s.writeOp(ctx, run, op, snap)
	run.snaps[op.card.ID] = cur
	if err != nil {
		return s.failureReason(err)
	}
	return "", false
}

// pause waits between two writes, so a long apply stays under Trello's rate limit.
func (s *Service) pause(ctx context.Context) error {
	if s.trelloPace <= 0 {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(s.trelloPace):
		return nil
	}
}

// liveCard reads what Trello holds for a card, once per apply.
func (s *Service) liveCard(ctx context.Context, run *applyRun, trelloID string) (trello.Card, string, bool) {
	if card, ok := run.live[trelloID]; ok {
		return card, "", false
	}
	if err := s.pause(ctx); err != nil {
		reason, fatal := s.failureReason(err)
		return trello.Card{}, reason, fatal
	}
	card, err := run.env.client.GetCard(ctx, trelloID)
	if err != nil {
		reason, fatal := s.failureReason(err)
		return trello.Card{}, reason, fatal
	}
	run.live[trelloID] = card
	return card, "", false
}

// boardLists reads the board's lists once, for the changes that need to name one.
func (run *applyRun) boardLists(ctx context.Context) ([]trello.List, error) {
	if run.read {
		return run.lists, nil
	}
	lists, err := run.env.client.ListLists(ctx, run.env.link.BoardID)
	if err != nil {
		return nil, err
	}
	run.lists, run.read = lists, true
	return lists, nil
}

// failureReason turns an error from a write into the sentence a person reads, and says whether it
// should stop the rest of the apply.
func (s *Service) failureReason(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	var api *trello.APIError
	if errors.As(err, &api) {
		switch api.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return reasonToken, true
		case http.StatusTooManyRequests:
			return reasonSlow, true
		case http.StatusNotFound:
			return reasonGone, false
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "Marshal stopped before this change was written.", true
	}
	s.log.Warn("a Trello change could not be written", "error", err)
	return "Trello could not take this change: " + shorten(err.Error()), false
}

func shorten(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > 160 {
		return text[:160] + "..."
	}
	return text
}

// doneListID is the id of the board's done list when the change is a move to Done, and "" otherwise.
func (s *Service) doneListID(ctx context.Context, run *applyRun, op trelloOp) (string, error) {
	if op.change.Kind != protocol.TrelloChangeKindMoveDone {
		return "", nil
	}
	lists, err := run.boardLists(ctx)
	if err != nil {
		return "", err
	}
	done, _ := doneListOf(lists)
	return done.ID, nil
}

// liveConflict says what Trello holds that differs from the snapshot on the thing a change writes,
// or "" when nothing does. It is what stops an apply from overwriting an edit made in Trello since
// Marshal last read the board.
func liveConflict(op trelloOp, live trello.Card, snap trelloSnap, doneID string) string {
	switch op.change.Kind {
	case protocol.TrelloChangeKindUpdateCard:
		if op.name != nil && live.Name != snap.Name {
			return fmt.Sprintf("Trello renamed this card to %q.", live.Name)
		}
		if op.desc != nil && live.Desc != snap.Desc {
			return "Trello changed this card's description."
		}
	case protocol.TrelloChangeKindMoveDone:
		if live.IDList != snap.ListID && live.IDList != doneID {
			return "Trello moved this card to another list."
		}
	case protocol.TrelloChangeKindSetItem:
		return liveItemConflict(op, live, snap)
	}
	return ""
}

func liveItemConflict(op trelloOp, live trello.Card, snap trelloSnap) string {
	for _, cl := range live.Checklists {
		base := snap.list(cl.ID)
		if base == nil {
			continue
		}
		for _, it := range cl.Items {
			known := base.item(it.ID)
			if known != nil && known.Done != it.Done() && op.conflictKey == "item:"+it.ID {
				return fmt.Sprintf("Trello changed %q.", it.Name)
			}
		}
	}
	return ""
}
