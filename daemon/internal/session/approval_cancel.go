package session

import (
	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// A revoked native request is no longer answerable. Resolve the same durable
// row and history block the person sees, without sending a stale answer back.
func (m *Manager) cancelApproval(ls *liveSession, requestID string) {
	ls.approvalMu.Lock()
	defer ls.approvalMu.Unlock()
	m.mu.Lock()
	var cancelled []*pendingApproval
	for id, pa := range m.approvals {
		if pa.ls == ls && pa.requestID == requestID {
			cancelled = append(cancelled, pa)
			delete(m.approvals, id)
		}
	}
	m.mu.Unlock()
	for _, pa := range cancelled {
		if err := m.decideApproval(m.ctx, pa, protocol.ApprovalDecisionDenied, audit.ActorDaemon); err != nil {
			m.log.Error("could not cancel approval", "error", err)
		}
		m.resolveApprovalHistory(m.ctx, pa, protocol.ChatApprovalStateDenied)
		m.auditDecision(m.ctx, pa, protocol.ApprovalDecisionDenied, audit.ActorDaemon)
		m.bus.Publish(string(ls.topic()), string(protocol.EventTypeApprovalResolved), protocol.ApprovalResolvedEventData{
			CardID: pa.cardID, ChatID: pa.chatID, ApprovalID: pa.approvalID, State: protocol.ChatApprovalStateDenied, DecidedBy: audit.ActorDaemon, At: protocol.NewTimestamp(m.cfg.Now().UTC()),
		}, true)
	}
	if len(cancelled) > 0 && !m.hasPendingApproval(ls) && !ls.wasStopRequested() {
		m.clearApprovalNeeds(m.ctx, ls.cardID)
		if err := m.setSessionState(m.ctx, ls, protocol.SessionStateWorking); err != nil {
			m.log.Error("could not restore session after approval cancellation", "error", err)
		}
		m.publishState(ls, protocol.SessionStateWorking, "")
	}
}

func (m *Manager) hasPendingApproval(ls *liveSession) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, pa := range m.approvals {
		if pa.ls == ls {
			return true
		}
	}
	return false
}
