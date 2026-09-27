package session

import (
	"context"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The automatic half of session hold: the idle timer that warns a person before it stops a card's
// agent, the sleep warning it produces, and the awake limit that decides which card is warned when
// a project is at its ceiling (docs/architecture.md sections 5.1 and 5.2, checklist item B5.6,
// inventory N5). The half a person presses - sleep, wake, pause, pin - is hold.go.
//
// The state diagram of section 5.1 is the whole of the rule set, and every branch of it that this
// file implements is here:
//
//   - Awake -> SleepWarning: idle time reached. Idleness is read from the session row's own
//     last-active time, which the manager already moves on every state change, so a person's
//     message, the end of a turn, and a wake all reset it. A card is idle when its row says Awake
//     and nothing has touched it for the idle setting's length.
//   - SleepWarning -> Awake: new activity. A card whose row has left the warning state - because a
//     message started a turn, because the person kept it awake, or because it stopped - leaves its
//     notice on the next pass, which is the same rule read from the other side.
//   - SleepWarning -> Asleep: warning time passed. The group's deadline is when its cards sleep.
//   - A session in Working or WaitingApproval never moves to SleepWarning: only a row that says
//     Awake is ever warned.
//   - A pinned card never sleeps, and the sleep of a card that stopped or started working while
//     its warning stood is refused by Sleep itself, the way a person's own press would be.
//   - The awake limit is checked per project: when a card wakes and the project is over its
//     ceiling, the oldest idle awake card gets a sleep warning.
//
// The timer is a sweep and not a per-card timer: one pass reads every live card's row, decides
// what the row means right now, and then puts the groups whose deadline has passed to sleep. One
// pass is easier to reason about than N timers being cancelled and re-armed on every message, and
// it means a card that was touched while the daemon was busy is seen in the same pass that would
// have slept it. The cost is that a warning can be up to one interval late, so the interval is
// short (sleepWatchInterval).

// SleepSettingsReader reads the settings the automatic sleep is driven by (internal/settings, the
// `settings` table under the key "sleep", B5.6). The manager names only what it needs, the way it
// names HistoryRecorder and RoleLimitsReader.
type SleepSettingsReader interface {
	// Sleep returns the sleep settings as they are stored, with the shipped defaults filled in for
	// an install that has never saved them.
	Sleep(ctx context.Context) (protocol.SleepSettings, error)
}

// AwakeLimitReader answers how many cards a project may keep awake at once (inventory B4.5,
// protocol.LimitKindAwake). internal/providers implements it; the second answer is false when no
// ceiling is set at all, which is the shipped state.
type AwakeLimitReader interface {
	AwakeLimit(ctx context.Context, projectID string) (int, bool, error)
}

// sleepWatchInterval is how often the manager looks for cards that have gone idle. It is short
// against the shortest idle time a person can choose (5 minutes, internal/settings), so a warning
// is at most a few seconds late, and long against the work one pass does (a card row and a session
// row per live card), so an install with many awake cards is not looking at the database
// constantly.
const sleepWatchInterval = 15 * time.Second

// SetSleepSettings gives the manager the reader of the sleep settings. It is set after the manager
// is built, because the settings service is built after it, and it is safe to call while sessions
// are running.
func (m *Manager) SetSleepSettings(r SleepSettingsReader) {
	m.sleepMu.Lock()
	m.sleepSet = r
	m.sleepMu.Unlock()
}

// SetAwakeLimits gives the manager the reader of the awake ceilings, for the same reason and in the
// same way as SetSleepSettings.
func (m *Manager) SetAwakeLimits(r AwakeLimitReader) {
	m.sleepMu.Lock()
	m.awakeSet = r
	m.sleepMu.Unlock()
}

// StartSleepWatch starts the goroutine that warns idle cards and puts them to sleep. cmd/marshald
// calls it once, after the settings and limit readers are wired; it stops when the manager is
// closed. A manager that never starts it - which is every test that drives the sweep itself -
// never sleeps or warns anything on its own, so a test is never racing a timer.
func (m *Manager) StartSleepWatch() {
	go func() {
		ticker := time.NewTicker(sleepWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				m.CheckIdle(m.ctx)
			}
		}
	}()
}

// CheckIdle is one pass of the idle timer: it warns the cards that have gone idle, drops the cards
// that were touched since their warning, and puts the groups whose warning has run out to sleep.
// StartSleepWatch calls it on a ticker; it is exported so a test can drive the timer without
// waiting out a wall-clock interval, and calling it by hand at any time is safe, because it only
// reads the rows, the clock, and the settings.
//
// It is one function and not three because the three things it does are one reading of the same
// rows: which cards have gone idle, which have been touched since, and which groups have run out of
// time.
func (m *Manager) CheckIdle(ctx context.Context) {
	cfg := m.sleepSettings(ctx)
	now := m.cfg.Now()
	for _, ls := range m.liveCardSessions() {
		m.sweepCard(ctx, ls, cfg, now)
	}
	m.sleepExpiredGroups(ctx, now)
}

// sweepCard decides what one idle time and one card mean together, and is the state diagram of
// section 5.1 for a single card. A card whose row cannot be read is left exactly as it is: a
// database that cannot answer is not a reason to warn a person about a card, and the next pass
// asks again.
func (m *Manager) sweepCard(ctx context.Context, ls *liveSession, cfg protocol.SleepSettings, now time.Time) {
	row, err := m.store.Queries().GetSessionByCard(ctx, ls.cardID)
	if err != nil {
		m.log.Warn("could not read a session to see whether it is idle", "card_id", ls.cardID, "error", err)
		return
	}
	state := protocol.SessionState(row.State)
	if inGroup, _ := m.sleepGroupOf(ls.cardID); inGroup {
		if state != protocol.SessionStateSleepWarning {
			// The card left the warning on its own: a message started a turn, or the session
			// stopped. Either way it is not going to sleep on this notice, so it leaves it.
			m.leaveSleepGroup(ls.cardID, protocol.EventTypeNoticeDismissed)
		}
		return
	}
	if state != protocol.SessionStateAwake {
		return
	}
	card, err := m.projects.Card(ctx, ls.cardID)
	if err != nil {
		m.log.Warn("could not read a card to see whether it may sleep", "card_id", ls.cardID, "error", err)
		return
	}
	if card.Pinned {
		return
	}
	if !m.maySleepNow(ls, card) {
		return
	}
	if m.keptOffIdle(ls.cardID, now) {
		return
	}
	if now.Sub(time.UnixMilli(row.LastActiveAt)) < minutes(cfg.IdleMinutes) {
		return
	}
	m.warnCard(ctx, ls, row, now, minutes(cfg.WarningMinutes))
}

// maySleepNow reports whether a card's state allows it to be put to sleep this moment: the rules
// Sleep itself enforces (hold.go), read before a warning is made. A card that could not be slept -
// one whose agent is working and whose pause does not hold it, one waiting on the person, or one
// whose pause is holding a message - is left out of the idle timer entirely, so a notice never
// promises a sleep that would then be refused. What is not read here is the session row: the caller
// has already decided the row says Awake.
func (m *Manager) maySleepNow(ls *liveSession, card protocol.Card) bool {
	if checkSleep(card) != nil {
		return false
	}
	return m.heldMessages(ls.cardID) == 0
}

// warnCard tells a person that an idle card will sleep soon: the session moves to the sleep
// warning state, which the card's own panel draws as "will sleep soon", and the card joins its
// project's notice. The deadline is the group's, so one project's idle cards are warned and sleep
// together (section 5.2's grouped reminders) and a card that joins a standing notice shares what
// is left of its warning.
func (m *Manager) warnCard(ctx context.Context, ls *liveSession, row db.Session, now time.Time, warn time.Duration) {
	if err := m.setRowState(ctx, row, protocol.SessionStateSleepWarning); err != nil {
		m.log.Warn("could not record that a card will sleep soon", "card_id", ls.cardID, "error", err)
		return
	}
	m.joinSleepGroup(ls.projectID, ls.cardID, now, now.Add(warn))
	m.publishSessionState(ls.cardID, row.ID, protocol.SessionStateSleepWarning, "")
	m.log.Info("warned that a card will sleep", "card_id", ls.cardID, "sleep_in", warn)
}

// sleepExpiredGroups puts every group whose deadline has passed to sleep. A card that was unpaused,
// started working, or was moved to wait on the person while the warning stood is left awake: Sleep
// refuses it with the sentence of section 5.1, and the refusal is logged rather than returned,
// because the other cards of the group must still sleep. The refused card is also moved out of the
// sleep-warning state, so it does not go on reading as about to sleep when its notice is gone.
func (m *Manager) sleepExpiredGroups(ctx context.Context, now time.Time) {
	for _, group := range m.dueSleepGroups(now) {
		slept := 0
		for _, cardID := range group.cards {
			if err := m.Sleep(ctx, cardID); err != nil {
				m.log.Info("left a card awake when its sleep notice ran out", "card_id", cardID, "error", err)
				m.clearSleepWarning(ctx, cardID)
				continue
			}
			slept++
		}
		m.log.Info("idle cards went to sleep", "project_id", group.projectID, "cards", slept)
	}
}

// enforceAwakeLimit warns a project's oldest idle awake cards when waking one has taken the project
// past its awake ceiling (section 5.1: "When a new card must wake and the limit is full, the oldest
// idle awake card gets a sleep warning"). The card that just woke is never the one warned: the
// person has just asked for it, and warning it would cancel their own press in the same breath.
//
// Nothing happens when no ceiling is set, which is the shipped state: a limit is only enforced when
// somebody set one.
func (m *Manager) enforceAwakeLimit(ctx context.Context, projectID, exceptCardID string) {
	limit, set := m.awakeLimit(ctx, projectID)
	if !set {
		return
	}
	now := m.cfg.Now()
	cfg := m.sleepSettings(ctx)
	for excess := len(m.liveCardsOf(projectID)) - limit; excess > 0; excess-- {
		victim, row, ok := m.oldestIdleAwake(ctx, projectID, exceptCardID, now)
		if !ok {
			return
		}
		m.log.Info("a project is over its awake limit", "project_id", projectID,
			"limit", limit, "card_id", victim.cardID)
		m.warnCard(ctx, victim, row, now, minutes(cfg.WarningMinutes))
	}
}

// oldestIdleAwake finds the card that has been awake and idle the longest in a project, which is
// the one the awake limit warns first. A card already on a sleep notice, one a person has kept
// awake, one that is pinned, and the card the caller just woke are all passed over; so is a card
// whose session is not Awake, because a working card is not idle and a card waiting on a person
// stays awake.
func (m *Manager) oldestIdleAwake(ctx context.Context, projectID, exceptCardID string, now time.Time) (*liveSession, db.Session, bool) {
	var oldest *liveSession
	var oldestRow db.Session
	var oldestAt time.Time
	for _, ls := range m.liveCardSessions() {
		if ls.projectID != projectID || ls.cardID == exceptCardID {
			continue
		}
		if inGroup, _ := m.sleepGroupOf(ls.cardID); inGroup {
			continue
		}
		if m.keptOffIdle(ls.cardID, now) {
			continue
		}
		card, err := m.projects.Card(ctx, ls.cardID)
		if err != nil || card.Pinned || !m.maySleepNow(ls, card) {
			continue
		}
		row, err := m.store.Queries().GetSessionByCard(ctx, ls.cardID)
		if err != nil || protocol.SessionState(row.State) != protocol.SessionStateAwake {
			continue
		}
		at := time.UnixMilli(row.LastActiveAt)
		if oldest == nil || at.Before(oldestAt) {
			oldest, oldestRow, oldestAt = ls, row, at
		}
	}
	return oldest, oldestRow, oldest != nil
}

// liveCardSessions is a snapshot of the live sessions that belong to a card. A chat's session is
// left out: a chat is never warned and never slept on a timer (section 16.2).
func (m *Manager) liveCardSessions() []*liveSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*liveSession, 0, len(m.sessions))
	for _, ls := range m.sessions {
		if !ls.isChat() {
			out = append(out, ls)
		}
	}
	return out
}

// sleepSettings reads the sleep settings, or the shipped defaults when no settings service is
// wired or the read fails. A daemon that cannot read its own settings sleeps cards on the shipped
// times rather than never sleeping at all, and says so in the log.
func (m *Manager) sleepSettings(ctx context.Context) protocol.SleepSettings {
	m.sleepMu.RLock()
	reader := m.sleepSet
	m.sleepMu.RUnlock()
	if reader == nil {
		return protocol.DefaultSleepSettings()
	}
	cfg, err := reader.Sleep(ctx)
	if err != nil {
		m.log.Warn("could not read the sleep settings", "error", err)
		return protocol.DefaultSleepSettings()
	}
	return filledSleepSettings(cfg)
}

// filledSleepSettings fills in a setting a stored record left at zero - one written by an older
// build, or saved before a field existed - with the shipped default, so a missing idle time never
// means "warn every card at once".
func filledSleepSettings(cfg protocol.SleepSettings) protocol.SleepSettings {
	d := protocol.DefaultSleepSettings()
	if cfg.IdleMinutes <= 0 {
		cfg.IdleMinutes = d.IdleMinutes
	}
	if cfg.WarningMinutes <= 0 {
		cfg.WarningMinutes = d.WarningMinutes
	}
	if cfg.KeepAwakeMinutes <= 0 {
		cfg.KeepAwakeMinutes = d.KeepAwakeMinutes
	}
	if cfg.Restore == "" {
		cfg.Restore = d.Restore
	}
	if cfg.Channel == "" {
		cfg.Channel = d.Channel
	}
	return cfg
}

// keepAwakeLength is how long a person's "keep awake" holds a card off the idle timer, from the
// keep-awake setting (the 15 minutes of decision D4 by default).
func (m *Manager) keepAwakeLength(ctx context.Context) time.Duration {
	return minutes(m.sleepSettings(ctx).KeepAwakeMinutes)
}

// awakeLimit reads a project's awake ceiling, and reports false when no ceiling is set or the
// ceiling cannot be read. A daemon that cannot answer is one that does not evict, rather than one
// that sleeps a card for a guess.
func (m *Manager) awakeLimit(ctx context.Context, projectID string) (int, bool) {
	m.sleepMu.RLock()
	reader := m.awakeSet
	m.sleepMu.RUnlock()
	if reader == nil {
		return 0, false
	}
	limit, set, err := reader.AwakeLimit(ctx, projectID)
	if err != nil {
		m.log.Warn("could not read the awake limit", "project_id", projectID, "error", err)
		return 0, false
	}
	return limit, set
}

// minutes is a count of minutes as a duration.
func minutes(n int) time.Duration { return time.Duration(n) * time.Minute }
