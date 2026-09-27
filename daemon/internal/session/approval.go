package session

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// pendingApproval is one permission request an agent is blocked on, kept in memory from the moment
// the agent asks until someone answers it or the session ends. The row in the approvals table is
// the durable copy (it survives a restart, and is what the Approvals view reads); this is the live
// half, holding what answering it needs and which session is waiting.
type pendingApproval struct {
	// ls is the session that asked. Its agent is what Respond answers, and its owner gives the topic
	// the answer is announced on.
	ls *liveSession
	// approvalID is the approval's own opaque id (the approvals row id, and the one in the URL).
	approvalID string
	// requestID is the id the agent gave this request inside its own protocol. Respond needs it.
	requestID string
	// options are the answers the agent offered, kept so a plain approve or deny can pick the right
	// one and so an explicit OptionID can be checked against them.
	options []agents.PermissionOption
	// cardID and chatID are the owner's ids, kept so the resolved event carries them even though ls
	// is still in hand.
	cardID string
	chatID string
	// state is the session state the session was in when the agent asked (working, during a turn).
	// It is restored after an approve, so the session reads as busy again while the turn goes on.
	state protocol.SessionState
}

// onPermissionRequested reacts to an agent asking for permission (agents.PermissionRequested): the
// harness is asked first, and a request it already has an answer for is answered by the daemon; the
// rest is recorded, holds the session at waiting-approval, and is announced, so every view of the
// same row draws it (N7). The agent's turn is blocked until it is answered either way.
//
// It runs on the pump goroutine, which cannot return an error, so a request that cannot be recorded
// is logged and the agent's turn is left blocked: the alternative, answering "cancelled" without a
// person having said so, would take a decision away from them.
func (m *Manager) onPermissionRequested(ls *liveSession, e agents.PermissionRequested) {
	if cfg, decided := m.harnessConfig(ls); decided {
		outcome := cfg.Decide(harness.Request{Kind: e.Kind, Path: e.Path, Command: e.Command})
		// A request the daemon cannot answer after all (the agent offered no option for the
		// decision, or refused it) falls through to the person below, which is always safe.
		if outcome.Decision != harness.DecisionAsk && m.answerWithoutAsking(ls, e, outcome, cfg.Mode) {
			return
		}
	}
	m.holdPermission(ls, e)
}

// holdPermission records a permission request, holds the session at waiting-approval, and announces
// it so every view of the same row draws it (N7). The agent's turn is blocked until Respond answers,
// so nothing else happens for this session until then.
func (m *Manager) holdPermission(ls *liveSession, e agents.PermissionRequested) {
	ctx := m.ctx
	now := m.cfg.Now().UTC()
	id, err := protocol.NewID(now, m.cfg.Entropy)
	if err != nil {
		m.log.Error("could not make an approval id", ls.noun()+"_id", ls.key(), "error", err)
		return
	}
	at := now
	if fromID, ok := protocol.IDTime(id); ok {
		// The id carries the time, so the stored row needs no time of its own and still reports one.
		at = fromID
	}
	approval := protocol.Approval{
		ID: id, CardID: ls.cardID, ChatID: ls.chatID, SessionID: ls.sessionRowID,
		ToolCallID: e.ToolCallID, Title: e.Title, Kind: e.Kind, Path: e.Path, Command: e.Command,
		Options: wireApprovalOptions(e.Options), State: protocol.ChatApprovalStateWaiting,
		At: protocol.NewTimestamp(at),
	}
	requestJSON, err := json.Marshal(approval)
	if err != nil {
		m.log.Error("could not encode a permission request", ls.noun()+"_id", ls.key(), "error", err)
		return
	}
	if err := m.store.Write(ctx, func(q *db.Queries) error {
		return q.CreateApproval(ctx, db.CreateApprovalParams{
			ID: id, SessionID: ls.sessionRowID, RequestJSON: string(requestJSON),
		})
	}); err != nil {
		m.log.Error("could not record a permission request", ls.noun()+"_id", ls.key(), "error", err)
		return
	}
	m.mu.Lock()
	m.approvals[id] = &pendingApproval{
		ls: ls, approvalID: id, requestID: e.RequestID, options: e.Options,
		cardID: ls.cardID, chatID: ls.chatID, state: protocol.SessionStateWorking,
	}
	m.mu.Unlock()
	if err := m.setSessionState(ctx, ls, protocol.SessionStateWaitingApproval); err != nil {
		m.log.Error("could not record that a session is waiting for approval", ls.noun()+"_id", ls.key(), "error", err)
	}
	m.publishState(ls, protocol.SessionStateWaitingApproval, "")
	m.bus.Publish(string(ls.topic()), string(protocol.EventTypeApprovalRequested),
		protocol.ApprovalRequestedEventData{CardID: ls.cardID, ChatID: ls.chatID, Approval: approval}, true)
}

// Respond answers a waiting approval: it delivers the person's decision to the agent, records it,
// audits it, and announces it so every view of the same row updates together. Actor is who answered
// (audit.ActorPerson from a screen, audit.ActorDaemon when the daemon answers on a person's behalf).
//
// It is safe to call twice for the same approval, and only the first answer counts: the second
// finds nothing waiting and is refused, exactly as if it had arrived after the first.
func (m *Manager) Respond(ctx context.Context, approvalID string, decision protocol.ApprovalDecision, optionID, actor string) error {
	if !decision.Valid() {
		return protocol.InvalidArgument("A decision must be approved or denied.").With("decision", string(decision))
	}
	pa, err := m.takePending(ctx, approvalID)
	if err != nil {
		return err
	}
	option, err := chooseOption(pa.options, decision, optionID)
	if err != nil {
		// Put the request back: nothing was answered, so it is still waiting.
		m.returnPending(pa)
		return err
	}
	if err := pa.ls.agent.Respond(ctx, pa.ls.handle, agents.ApprovalResponse{RequestID: pa.requestID, OptionID: option}); err != nil {
		m.returnPending(pa)
		return protocol.Refused("The agent did not take the answer.").With("approvalId", approvalID).WithCause(err)
	}
	now := m.cfg.Now().UTC()
	if err := m.decideApproval(ctx, pa, decision, actor); err != nil {
		// The agent was answered, so the request is done even though the record of it is not; log it
		// and go on rather than undoing an answer the agent already has.
		m.log.Error("could not record a decision on an approval", "approval_id", approvalID, "error", err)
	}
	m.auditDecision(ctx, pa, decision, actor)
	m.bus.Publish(string(pa.ls.topic()), string(protocol.EventTypeApprovalResolved),
		protocol.ApprovalResolvedEventData{
			CardID: pa.cardID, ChatID: pa.chatID, ApprovalID: approvalID,
			State: decision.State(), DecidedBy: actor, At: protocol.NewTimestamp(now),
		}, true)
	if err := m.setSessionState(ctx, pa.ls, pa.state); err != nil {
		m.log.Error("could not restore a session's state after an approval", pa.ls.noun()+"_id", pa.ls.key(), "error", err)
	}
	m.publishState(pa.ls, pa.state, "")
	return nil
}

// takePending removes the waiting approval with this id and returns it, or explains why it cannot
// be answered: it is unknown (no row), it was already answered (a row with a decision), or the
// agent that asked is gone (a row still waiting, but nothing in memory is waiting for it, which
// happens after a restart).
func (m *Manager) takePending(ctx context.Context, approvalID string) (*pendingApproval, error) {
	m.mu.Lock()
	pa, ok := m.approvals[approvalID]
	if ok {
		delete(m.approvals, approvalID)
	}
	m.mu.Unlock()
	if ok {
		return pa, nil
	}
	row, err := m.store.Queries().GetApproval(ctx, approvalID)
	switch {
	case err == nil && row.Decision != "":
		return nil, protocol.Conflict("Somebody already answered that request.").With("approvalId", approvalID)
	case err == nil:
		return nil, protocol.Conflict("The agent that asked for this is no longer running.").With("approvalId", approvalID)
	case store.IsNotFound(err):
		return nil, protocol.NotFound("that approval").With("approvalId", approvalID)
	default:
		return nil, fmt.Errorf("read approval %s: %w", approvalID, err)
	}
}

// returnPending puts back a request that was taken but not answered (a bad option, or an agent that
// refused the answer), so a later call can still answer it.
func (m *Manager) returnPending(pa *pendingApproval) {
	m.mu.Lock()
	m.approvals[pa.approvalID] = pa
	m.mu.Unlock()
}

// decideApproval writes the decision to the approvals row. The guarded update changes nothing and
// reports zero rows when the row was already decided, which is not an error here: the agent has
// been answered either way.
func (m *Manager) decideApproval(ctx context.Context, pa *pendingApproval, decision protocol.ApprovalDecision, actor string) error {
	err := m.store.Write(ctx, func(q *db.Queries) error {
		_, err := q.DecideApproval(ctx, db.DecideApprovalParams{
			Decision: string(decision), DecidedBy: actor, ID: pa.approvalID,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("record the decision on approval %s: %w", pa.approvalID, err)
	}
	return nil
}

// auditDecision writes the audit row for an answered approval.
func (m *Manager) auditDecision(ctx context.Context, pa *pendingApproval, decision protocol.ApprovalDecision, actor string) {
	action := audit.ActionDeny
	if decision == protocol.ApprovalDecisionApproved {
		action = audit.ActionApprove
	}
	m.audit.LogAndForget(ctx, audit.Entry{
		Actor: actor, Action: action, Target: pa.approvalID, SessionID: pa.ls.sessionRowID,
		Detail: map[string]any{"cardId": pa.cardID, "chatId": pa.chatID, "decision": string(decision)},
	})
}

// withdrawApprovals answers, as the daemon, every request still waiting for a session that is going
// away, so no request is left waiting for an agent that no longer exists. It is called once, after
// a session's pump ends.
func (m *Manager) withdrawApprovals(ctx context.Context, ls *liveSession) {
	m.mu.Lock()
	var gone []*pendingApproval
	for id, pa := range m.approvals {
		if pa.ls == ls {
			gone = append(gone, pa)
			delete(m.approvals, id)
		}
	}
	m.mu.Unlock()
	for _, pa := range gone {
		if err := m.decideApproval(ctx, pa, protocol.ApprovalDecisionDenied, audit.ActorDaemon); err != nil {
			m.log.Error("could not withdraw a waiting approval", "approval_id", pa.approvalID, "error", err)
		}
		m.auditDecision(ctx, pa, protocol.ApprovalDecisionDenied, audit.ActorDaemon)
		m.bus.Publish(string(ls.topic()), string(protocol.EventTypeApprovalResolved),
			protocol.ApprovalResolvedEventData{
				CardID: pa.cardID, ChatID: pa.chatID, ApprovalID: pa.approvalID,
				State: protocol.ChatApprovalStateDenied, DecidedBy: audit.ActorDaemon,
				At: protocol.NewTimestamp(m.cfg.Now().UTC()),
			}, true)
	}
}

// chooseOption picks the option to hand the agent for a decision. An explicit optionID must be one
// the agent offered. Without one, an approve takes allow_once and a deny takes reject_once, falling
// back to the "always" form when the agent offered only that.
func chooseOption(options []agents.PermissionOption, decision protocol.ApprovalDecision, optionID string) (string, error) {
	if optionID != "" {
		for _, o := range options {
			if o.ID == optionID {
				return o.ID, nil
			}
		}
		return "", protocol.InvalidArgument("The agent did not offer that option.").With("optionId", optionID)
	}
	want := []string{agents.OptionAllowOnce, agents.OptionAllowAlways}
	if decision == protocol.ApprovalDecisionDenied {
		want = []string{agents.OptionRejectOnce, agents.OptionRejectAlways}
	}
	for _, kind := range want {
		for _, o := range options {
			if o.Kind == kind {
				return o.ID, nil
			}
		}
	}
	return "", protocol.Refused("The agent's request offered no option for that answer.")
}

// wireApprovalOptions copies the agent's options to the wire, never nil so the JSON is a list.
func wireApprovalOptions(options []agents.PermissionOption) []protocol.ApprovalOption {
	out := make([]protocol.ApprovalOption, 0, len(options))
	for _, o := range options {
		out = append(out, protocol.ApprovalOption{ID: o.ID, Name: o.Name, Kind: o.Kind})
	}
	return out
}
