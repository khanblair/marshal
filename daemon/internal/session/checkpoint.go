package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// A card's restore points (docs/architecture.md section 10, docs/backend-checklist.md B5.3,
// build-plan 5.21). Marshal makes one before every agent turn, so a card that goes wrong can be put
// back to where it was before the turn. There is no prototype for this: the mock has no checkpoint
// list and no restore action anywhere, so the semantics below are Marshal's own, designed from the
// architecture doc's data model, and are recorded as rulings.
//
//   - A checkpoint is a real commit on the card's own branch plus a row naming it. Marshal commits
//     whatever the worktree holds, so the checkpoint is a complete state and restoring it is an
//     ordinary reset, not a copy of files. The commit is kept on a hidden ref named by the
//     checkpoint's own id, so it survives the branch moving on (internal/gitx/checkpoint.go).
//   - A checkpoint is made before a turn, in the card's own worktree, and never for a chat (a chat
//     has no worktree) or for a card that never started (it has none either).
//   - Restoring puts the worktree and the branch back to the commit. The conversation is asked for
//     separately; see RestoreCheckpoint.
//   - A card keeps its newest maxCheckpointsPerCard restore points. The oldest rows and their hidden
//     refs go; the commits themselves are left for Git's own garbage collection.

// maxCheckpointsPerCard is how many restore points one card keeps. One is made before every turn, so
// a card stopped and resumed many times would otherwise grow a list with no end.
const maxCheckpointsPerCard = 20

// checkpointBeforeTurn makes a card's restore point, just before an agent turn. It never returns an
// error: a checkpoint that cannot be made must not stop the turn the person asked for, so the reason
// is logged and the turn goes on.
func (m *Manager) checkpointBeforeTurn(ls *liveSession) {
	if ls.isChat() {
		return
	}
	row, err := m.store.Queries().GetCard(m.ctx, ls.cardID)
	if err != nil {
		m.log.Warn("could not read a card to make its checkpoint", "card_id", ls.cardID, "error", err)
		return
	}
	if row.WorktreePath == "" {
		// A card that has not started has no worktree and nothing to restore.
		return
	}
	turns, err := m.store.Queries().CountSessionEventsOfKind(m.ctx, db.CountSessionEventsOfKindParams{
		CardID: ls.cardID, Kind: string(history.KindUser),
	})
	if err != nil {
		m.log.Warn("could not count a card's turns to label its checkpoint", "card_id", ls.cardID, "error", err)
	}
	m.makeCheckpoint(ls.cardID, row.WorktreePath, fmt.Sprintf("before turn %d", turns+1))
}

// makeCheckpoint commits a card's worktree, records the row, and trims the card's oldest restore
// points. It returns the checkpoint it made, or nil when one could not be made.
func (m *Manager) makeCheckpoint(cardID, worktree, label string) *protocol.Checkpoint {
	now := m.cfg.Now()
	id, err := protocol.NewID(now, m.cfg.Entropy)
	if err != nil {
		m.log.Error("could not make a checkpoint id", "card_id", cardID, "error", err)
		return nil
	}
	ref := gitx.CheckpointRef(id)
	state, err := m.git.Checkpoint(m.ctx, worktree, ref, checkpointMessage(label))
	if err != nil {
		m.log.Error("could not make a checkpoint", "card_id", cardID, "error", err)
		return nil
	}
	params := db.InsertCheckpointParams{
		ID: id, CardID: cardID, GitRef: state.SHA, Label: label, CreatedAt: now.UnixMilli(),
	}
	if err := m.store.Write(m.ctx, func(q *db.Queries) error { return q.InsertCheckpoint(m.ctx, params) }); err != nil {
		m.log.Error("could not store a checkpoint", "card_id", cardID, "error", err)
		return nil
	}
	m.trimCheckpoints(cardID)
	return &protocol.Checkpoint{
		ID: id, CardID: cardID, SHA: state.SHA, Label: label, CreatedAt: protocol.NewTimestamp(now),
	}
}

// checkpointMessage is the commit message a checkpoint is made with, so a person who reads the
// branch's own log sees why Marshal made it.
func checkpointMessage(label string) string {
	if strings.TrimSpace(label) == "" {
		return "Marshal checkpoint"
	}
	return "Marshal checkpoint: " + label
}

// trimCheckpoints removes a card's oldest restore points beyond maxCheckpointsPerCard, and deletes
// their hidden refs. It never fails a caller: a card that keeps one restore point too many is a
// smaller problem than a turn that did not start.
func (m *Manager) trimCheckpoints(cardID string) {
	rows, err := m.store.Queries().ListCheckpointsForCard(m.ctx, db.ListCheckpointsForCardParams{
		CardID: cardID, Limit: -1,
	})
	if err != nil || len(rows) <= maxCheckpointsPerCard {
		return
	}
	worktree := m.worktreeOf(cardID)
	for _, row := range rows[maxCheckpointsPerCard:] {
		if err := m.store.Write(m.ctx, func(q *db.Queries) error {
			return q.DeleteCheckpoint(m.ctx, db.DeleteCheckpointParams{CardID: cardID, ID: row.ID})
		}); err != nil {
			m.log.Warn("could not remove an old checkpoint", "card_id", cardID, "error", err)
			continue
		}
		if worktree != "" {
			if err := m.git.DeleteCheckpointRef(m.ctx, worktree, gitx.CheckpointRef(row.ID)); err != nil {
				m.log.Warn("could not remove an old checkpoint's ref", "card_id", cardID, "error", err)
			}
		}
	}
}

// worktreeOf reads a card's worktree path, or empty when it has none or cannot be read.
func (m *Manager) worktreeOf(cardID string) string {
	row, err := m.store.Queries().GetCard(m.ctx, cardID)
	if err != nil {
		return ""
	}
	return row.WorktreePath
}

// ListCheckpoints returns a card's restore points, newest first, for the card panel's checkpoint
// list. A card that has never started has none, which is an empty list and not an error.
func (m *Manager) ListCheckpoints(ctx context.Context, cardID string) (protocol.CheckpointList, error) {
	if _, err := m.projects.Card(ctx, cardID); err != nil {
		return protocol.CheckpointList{}, err
	}
	rows, err := m.store.Queries().ListCheckpointsForCard(ctx, db.ListCheckpointsForCardParams{
		CardID: cardID, Limit: -1,
	})
	if err != nil {
		return protocol.CheckpointList{}, fmt.Errorf("read the checkpoints of card %s: %w", cardID, err)
	}
	checkpoints := make([]protocol.Checkpoint, 0, len(rows))
	for _, row := range rows {
		checkpoints = append(checkpoints, protocol.Checkpoint{
			ID: row.ID, CardID: row.CardID, SHA: row.GitRef, Label: row.Label,
			CreatedAt: protocol.NewTimestamp(time.UnixMilli(row.CreatedAt).UTC()),
		})
	}
	return protocol.NewCheckpointList(cardID, checkpoints, m.cfg.Now()), nil
}

// RestoreCheckpoint puts a card's worktree and branch back to one of its restore points, and answers
// the card as it now is. `conversation` asks for the card's own conversation to be restored as well.
//
// The worktree restore is the whole of it: the branch and the files are reset to the checkpoint's
// commit, and files the worktree holds that the commit does not are removed. Restoring the
// conversation is narrower than it sounds, because an agent's own context cannot be rewound through
// any adapter Marshal has (docs/architecture.md 4.1 has no rewind call): what Marshal does is record
// the restore point in the card's history as the boundary the conversation is restored from, so the
// card and every client that read it agree where the work stands. That is a ruling, and the report
// says so plainly rather than claiming a rewind that does not happen.
//
// A card being merged, and a card whose agent is running a turn, are refused: a restore in the
// middle of a turn would race the agent's own writes to the worktree.
func (m *Manager) RestoreCheckpoint(ctx context.Context, cardID, checkpointID string, conversation bool) (protocol.Card, error) {
	row, err := m.store.Queries().GetCheckpoint(ctx, db.GetCheckpointParams{CardID: cardID, ID: checkpointID})
	if err != nil {
		if store.IsNotFound(err) {
			return protocol.Card{}, protocol.NotFound("That checkpoint is not on this card.").
				With("cardId", cardID).With("checkpointId", checkpointID)
		}
		return protocol.Card{}, fmt.Errorf("read checkpoint %s of card %s: %w", checkpointID, cardID, err)
	}
	card, err := m.store.Queries().GetCard(ctx, cardID)
	if err != nil {
		return protocol.Card{}, fmt.Errorf("read card %s to restore a checkpoint: %w", cardID, err)
	}
	if card.WorktreePath == "" {
		return protocol.Card{}, protocol.Refused("This card has no worktree to restore.").With("cardId", cardID)
	}
	if ls := m.liveOf(cardID); ls != nil && ls.isBusy() {
		return protocol.Card{}, protocol.Refused("This card's agent is working. Wait for the turn to end, then restore.").
			With("cardId", cardID).With("reason", "restore_turn_running")
	}
	if err := m.git.RestoreCheckpoint(ctx, card.WorktreePath, row.GitRef); err != nil {
		return protocol.Card{}, fmt.Errorf("restore card %s to checkpoint %s: %w", cardID, checkpointID, err)
	}
	m.noteRestore(cardID, row, conversation)
	return m.projects.Card(ctx, cardID)
}

// noteRestore records a restore in the audit log and in the card's own history, so the moment the
// worktree went back is readable in the card's chat and accountable in the audit log.
func (m *Manager) noteRestore(cardID string, row db.Checkpoint, conversation bool) {
	short := row.GitRef
	if len(short) > 8 {
		short = short[:8]
	}
	m.audit.LogAndForget(m.ctx, audit.Entry{
		Actor: audit.ActorPerson, Action: audit.ActionCheckpointRestored, Target: row.ID,
		SessionID: m.sessionRowIDFor(cardID),
		Detail: map[string]any{
			cardIDDetailKey: cardID, "checkpointId": row.ID, "sha": row.GitRef,
			"label": row.Label, "conversation": conversation,
		},
	})
	summary := fmt.Sprintf("Restored this card to checkpoint %s (%s). The worktree and the branch are back at that commit.",
		short, checkpointName(row.Label))
	if conversation {
		summary += " The conversation is restored from here; the agent's own context is not rewound, so it picks up from the restored worktree."
	}
	m.appendCardRecord(cardID, summary)
}

// checkpointName is a checkpoint's label, or a plain word for one that has none.
func checkpointName(label string) string {
	if strings.TrimSpace(label) == "" {
		return "no label"
	}
	return label
}

// sessionRowIDFor reads the id of a card's session row, or empty when the card has none. It is what
// a history record made outside a live session is filed under.
func (m *Manager) sessionRowIDFor(cardID string) string {
	row, err := m.store.Queries().GetSessionByCard(m.ctx, cardID)
	if err != nil {
		return ""
	}
	return row.ID
}

// appendCardRecord writes one system record into a card's history, for a moment that happened
// outside a live session and so has no session of its own to record through.
func (m *Manager) appendCardRecord(cardID, summary string) {
	if m.cfg.History == nil {
		return
	}
	if err := m.cfg.History.Append(m.ctx, cardID, m.sessionRowIDFor(cardID),
		[]history.Record{{Kind: history.KindSystem, Summary: summary}}); err != nil {
		m.log.Error("could not store a card's history record", "card_id", cardID, "error", err)
	}
}
