// Package audit writes the audit log: one append-only row per action a person may need to account
// for (docs/architecture.md section 10, docs/backend-checklist.md B3.5). Approving or denying a
// request, turning bypass on or off, and a commit the secret scanner blocked all write here.
//
// A row is written once and never edited or deleted, so the trail stays true. Reading it is the
// API's job (the routes are read only); nothing here pages or searches, it only records.
package audit

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Actors: who did the thing.
const (
	// ActorPerson is a person, from a screen.
	ActorPerson = "person"
	// ActorAgent is an agent.
	ActorAgent = "agent"
	// ActorDaemon is the daemon acting on its own or on a person's behalf.
	ActorDaemon = "daemon"
)

// Actions: the short verb of a row. They are a fixed, small list; a new one is added here with a
// constant, so the same action is spelled the same way everywhere it is written and read.
const (
	// ActionApprove is an approval a person allowed.
	ActionApprove = "approve"
	// ActionDeny is an approval a person refused.
	ActionDeny = "deny"
	// ActionBypassOn is bypass permissions turned on for a card.
	ActionBypassOn = "bypass.on"
	// ActionBypassOff is bypass permissions turned off for a card.
	ActionBypassOff = "bypass.off"
	// ActionCommitBlocked is a commit the secret scanner stopped: it held something that looks like
	// a credential, so the card was moved to Needs you rather than left to carry on (B3.5).
	ActionCommitBlocked = "commit.blocked"
	// ActionPlanEdited is the steps of a plan a person changed before approving it (B5.2). The plan
	// is still waiting for an answer, so this is a change to what the agent proposed rather than a
	// decision on it, which is why it is its own action and not approve or deny.
	ActionPlanEdited = "plan.edited"
	// ActionLimitReached is a card stopped by one of its role's own ceilings - time, cost, or
	// rounds (B5.3). The detail names which ceiling and the numbers behind it, so a person can see
	// why the card stopped rather than only that it did.
	ActionLimitReached = "limit.reached"
	// ActionStuckPaused is a card the stuck detector paused because it kept repeating itself
	// (B5.3). The detail names the signal that repeated and how many times.
	ActionStuckPaused = "stuck.paused"
	// ActionCheckpointRestored is a card's worktree put back to one of its restore points (B5.3).
	// The detail names the checkpoint and whether the conversation was restored with it.
	ActionCheckpointRestored = "checkpoint.restored"
	// ActionCISimulated is a simulated CI failure a person asked for (B6.4). The detail names the
	// mode that ran and, for the real mode, the marked commit that was pushed, so the log says
	// which of the two a card's branch was changed by.
	ActionCISimulated = "ci.simulated"
)

// Entry is one thing to record. Detail is any value that marshals to JSON (a struct, a map), or nil
// for an action that needs no more than its verb and target.
type Entry struct {
	// Actor is who did it: ActorPerson, ActorAgent, or ActorDaemon.
	Actor string
	// Action is the short verb, one of the Action constants.
	Action string
	// Target is what it was done to: a card id, a tool call id, a path. Empty when there is none.
	Target string
	// SessionID is the session the action belongs to. Empty for an action that belongs to none
	// (turning bypass on, for example).
	SessionID string
	// Detail is the structured payload, or nil.
	Detail any
}

// Recorder writes audit rows. It is safe for use by many goroutines.
type Recorder struct {
	store   *store.Store
	now     func() time.Time
	entropy io.Reader
	log     *slog.Logger
}

// New builds a Recorder. A nil clock means time.Now, a nil entropy reader means crypto/rand.Reader,
// and a nil logger means nothing is logged.
func New(st *store.Store, now func() time.Time, entropy io.Reader, log *slog.Logger) (*Recorder, error) {
	if st == nil {
		return nil, fmt.Errorf("the audit recorder needs the store")
	}
	if now == nil {
		now = time.Now
	}
	if entropy == nil {
		entropy = rand.Reader
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Recorder{store: st, now: now, entropy: entropy, log: log}, nil
}

// Record writes one row. It is a write of its own, so a caller on a request path returns its error
// and a caller that cannot (the pump goroutine) logs it and goes on: an audit row that cannot be
// written must never stop the work it describes.
func (r *Recorder) Record(ctx context.Context, e Entry) error {
	id, err := protocol.NewID(r.now(), r.entropy)
	if err != nil {
		return fmt.Errorf("make an audit row id: %w", err)
	}
	detail := ""
	if e.Detail != nil {
		encoded, err := json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("encode the detail of audit action %s: %w", e.Action, err)
		}
		detail = string(encoded)
	}
	err = r.store.Write(ctx, func(q *db.Queries) error {
		return q.InsertAuditLog(ctx, db.InsertAuditLogParams{
			ID: id, SessionID: e.SessionID, Actor: e.Actor, Action: e.Action,
			Target: e.Target, DetailJSON: detail, CreatedAt: r.now().UnixMilli(),
		})
	})
	if err != nil {
		return fmt.Errorf("record the audit action %s: %w", e.Action, err)
	}
	return nil
}

// LogAndForget records a row and only logs a failure. It is for the paths that cannot return an
// error: the pump reacting to an agent event, and any other goroutine whose job must go on even
// when the audit write does not.
func (r *Recorder) LogAndForget(ctx context.Context, e Entry) {
	if err := r.Record(ctx, e); err != nil {
		r.log.Error("could not write an audit row", "action", e.Action, "target", e.Target, "error", err)
	}
}
