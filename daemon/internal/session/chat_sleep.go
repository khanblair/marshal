package session

import (
	"context"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The idle sleep of a system chat. The idle timer (sleep.go) has only ever swept cards: a project
// chat is never warned and never slept on a timer, so its agent runs until the daemon stops or the
// chat is archived. A system chat is different. The Integrator chat's agent is long-lived and
// usually idle, so it should not hold a process for hours between merges.
//
// A system chat's agent is put to sleep once it has been idle for the idle time the sleep settings
// give (Settings > Sleep), with no warning: a chat has no notice to show, and nothing is lost,
// because its conversation is kept and its next message wakes it through the same resume a card
// uses (resumeChat). A chat is idle when its row says awake - never working, never waiting for an
// approval - and nothing has touched it since. A chat a person made is never slept this way.

// sleepIdleChats is the chat half of one pass of the idle timer. It is called by CheckIdle, which is
// what StartSleepWatch runs on its ticker.
func (m *Manager) sleepIdleChats(ctx context.Context, cfg protocol.SleepSettings, now time.Time) {
	idle := minutes(cfg.IdleMinutes)
	for _, ls := range m.liveChatSessions() {
		if !m.isSystemChat(ctx, ls.chatID) {
			continue
		}
		m.sleepChatIfIdle(ctx, ls.chatID, now, idle)
	}
}

// sleepChatIfIdle puts one chat to sleep when it is still idle. It reads the chat again under the
// chat's lock, so a message that arrived since the sweep chose it is not lost to a sleep that was
// decided without it: the message either took the lock first, and the chat is busy, or waits for it.
func (m *Manager) sleepChatIfIdle(ctx context.Context, chatID string, now time.Time, idle time.Duration) {
	unlock := m.chatLocks.Lock(chatID)
	defer unlock()
	ls := m.liveOf(chatID)
	if ls == nil || ls.isBusy() {
		return
	}
	row, err := m.chatSession(ctx, chatID)
	if err != nil {
		m.log.Warn("could not read a chat's session to see whether it is idle", "chat_id", chatID, "error", err)
		return
	}
	if protocol.SessionState(row.State) != protocol.SessionStateAwake {
		return
	}
	if now.Sub(time.UnixMilli(row.LastActiveAt)) < idle {
		return
	}
	if err := m.sleepChatLocked(ctx, chatID); err != nil {
		m.log.Warn("could not put an idle system chat to sleep", "chat_id", chatID, "error", err)
		return
	}
	m.log.Info("a system chat went to sleep when it had been idle", "chat_id", chatID, "idle_for", idle)
}

// isSystemChat reports whether a chat is one Marshal keeps for the project itself. A chat that cannot
// be read is not one, so the sweep leaves it alone.
func (m *Manager) isSystemChat(ctx context.Context, chatID string) bool {
	row, err := m.store.Queries().GetChat(ctx, chatID)
	return err == nil && row.System != ""
}

// liveChatSessions is a snapshot of the live sessions that belong to a chat.
func (m *Manager) liveChatSessions() []*liveSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*liveSession, 0, len(m.sessions))
	for _, ls := range m.sessions {
		if ls.isChat() {
			out = append(out, ls)
		}
	}
	return out
}
