package session_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// Automatic sleep (B5.6, build-plan 5.17, inventory N5, docs/architecture.md 5.1 and 5.2): the idle
// timer that warns a person before it stops a card's agent, the grouped reminder one project's idle
// cards share, the calls a person makes on it, and the awake limit that warns the oldest idle awake
// card when a project is at its ceiling. The half a person presses by hand - sleep, wake, pause,
// pin - is tested in hold_test.go; this is the half the daemon does on its own.
//
// Every test here drives the sweep by hand (Manager.CheckIdle) on a clock it moves itself, so
// nothing waits out a wall-clock interval and no test races the timer StartSleepWatch would start.

// testClock is a clock a test moves by hand, so the idle timer's minutes pass in microseconds.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// advance moves the clock forward.
func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// fakeSleepSettings is the manager's SleepSettingsReader, holding whatever a test saved.
type fakeSleepSettings struct {
	cfg protocol.SleepSettings
	err error
}

func (f *fakeSleepSettings) Sleep(context.Context) (protocol.SleepSettings, error) {
	return f.cfg, f.err
}

// fakeAwakeLimits is the manager's AwakeLimitReader, set with one project's ceiling. It answers the
// same ceiling for every project, which is enough for a test that uses one.
type fakeAwakeLimits struct {
	limit int
	set   bool
	err   error
}

func (f *fakeAwakeLimits) AwakeLimit(context.Context, string) (int, bool, error) {
	return f.limit, f.set, f.err
}

// newSleepEnv builds an env on a clock a test drives, with the shipped sleep settings wired in. A
// test that wants other settings calls setSleep on what comes back.
func newSleepEnv(t *testing.T) (*env, *testClock) {
	t.Helper()
	clock := newTestClock()
	e := newEnv(t, func(c *session.Config) { c.Now = clock.Now })
	t.Cleanup(func() { _ = e.mgr.Close() })
	e.mgr.SetSleepSettings(&fakeSleepSettings{cfg: protocol.DefaultSleepSettings()})
	return e, clock
}

// setSleep swaps in other sleep settings, the way saving the screen would. The manager reads the
// times as they are stored (only a zero is filled from the defaults), so a test may choose any
// positive idle, warning, and keep-awake time.
func setSleep(e *env, idle, warn, keepAwake int) {
	e.mgr.SetSleepSettings(&fakeSleepSettings{cfg: protocol.SleepSettings{
		IdleMinutes: idle, WarningMinutes: warn, KeepAwakeMinutes: keepAwake,
		Restore: protocol.SleepRestoreAuto, Channel: protocol.SleepChannelInApp,
	}})
}

// noticesNow is the standing notice list, read the way GET /v1/notices serves it.
func noticesNow(t *testing.T, e *env) []protocol.Notice {
	t.Helper()
	return e.mgr.Notices()
}

// onlyNotice fails the test unless exactly one notice stands, and returns it.
func onlyNotice(t *testing.T, e *env) protocol.Notice {
	t.Helper()
	notices := noticesNow(t, e)
	if len(notices) != 1 {
		t.Fatalf("notices = %+v, want exactly one", notices)
	}
	return notices[0]
}

// untilNoticeEvent reads events until a notice event of this type arrives on the home topic, and
// returns the list it carried.
func untilNoticeEvent(t *testing.T, e *env, typ protocol.EventType) protocol.NoticeListEventData {
	t.Helper()
	timeout := time.After(eventTimeout)
	for {
		select {
		case ev, ok := <-e.sub.C():
			if !ok {
				t.Fatalf("the subscription closed while waiting for %s", typ)
			}
			if ev.Type != string(typ) {
				continue
			}
			if ev.Topic != string(protocol.HomeTopic) {
				t.Fatalf("a %s event arrived on topic %q, want the home topic", typ, ev.Topic)
			}
			data, ok := ev.Data.(protocol.NoticeListEventData)
			if !ok {
				t.Fatalf("a %s event carried %T, want a notice list", typ, ev.Data)
			}
			return data
		case <-timeout:
			t.Fatalf("no %s event arrived", typ)
			return protocol.NoticeListEventData{}
		}
	}
}

// sleepWarningRow fails the test unless a card's session row reads sleep-warning.
func sleepWarningRow(t *testing.T, e *env, cardID string) {
	t.Helper()
	if got := protocol.SessionState(e.sessionRow(t, cardID).State); got != protocol.SessionStateSleepWarning {
		t.Fatalf("session state of %s = %s, want %s", cardID, got, protocol.SessionStateSleepWarning)
	}
}

// pausedCard makes a card whose agent is running and whose pause holds it: the shape an idle card
// that may sleep has (a Working card sleeps only after it is paused, B5.6).
func pausedCard(t *testing.T, e *env, projectID, title string) protocol.Card {
	t.Helper()
	card := startCard(t, e, e.card(t, projectID, title))
	if _, err := e.mgr.Pause(context.Background(), card.ID); err != nil {
		t.Fatalf("Pause the card %q: %v", title, err)
	}
	return card
}

// A card whose agent is working and whose pause does not hold it is never warned and never slept:
// "working cards never sleep" (B5.6). Its session row stays awake and no notice is made, however
// long it sits there.
func TestAWorkingCardIsNeverWarned(t *testing.T) {
	e, clock := newSleepEnv(t)
	project := e.project(t, "small-repo")
	card := startCard(t, e, e.card(t, project.ID, "Busy for hours"))

	clock.advance(time.Hour)
	e.mgr.CheckIdle(context.Background())

	if notices := noticesNow(t, e); len(notices) != 0 {
		t.Fatalf("a working card got a notice: %+v", notices)
	}
	if got := protocol.SessionState(e.sessionRow(t, card.ID).State); got != protocol.SessionStateAwake {
		t.Fatalf("session state = %s, want it left awake", got)
	}
}

// A paused card that goes idle is warned, its project's notice names it with a deadline the warning
// time ahead, and the list change is announced on the home topic with the whole list.
func TestAnIdlePausedCardIsWarnedAndAnnounced(t *testing.T) {
	e, clock := newSleepEnv(t)
	project := e.project(t, "small-repo")
	card := pausedCard(t, e, project.ID, "Idle soon")

	clock.advance(16 * time.Minute)
	e.mgr.CheckIdle(context.Background())

	sleepWarningRow(t, e, card.ID)
	notice := onlyNotice(t, e)
	if notice.ID != "sleep:"+project.ID {
		t.Errorf("notice id = %q, want sleep:%s", notice.ID, project.ID)
	}
	if notice.Kind != protocol.NoticeKindSleep {
		t.Errorf("notice kind = %q", notice.Kind)
	}
	if len(notice.Cards) != 1 || notice.Cards[0] != card.ID {
		t.Errorf("notice cards = %v, want [%s]", notice.Cards, card.ID)
	}
	if notice.ProjectID != project.ID {
		t.Errorf("notice project = %q, want %q", notice.ProjectID, project.ID)
	}
	if notice.Deadline == nil {
		t.Fatal("a sleep notice has no deadline")
	}
	want := clock.Now().Add(2 * time.Minute)
	if got := notice.Deadline.Time(); !got.Equal(want) {
		t.Errorf("deadline = %s, want %s", got, want)
	}

	data := untilNoticeEvent(t, e, protocol.EventTypeNoticeCreated)
	if len(data.Notices) != 1 || data.Notices[0].ID != notice.ID {
		t.Errorf("notice.created carried %+v", data.Notices)
	}
}

// An idle card that is not in the Working column - here one whose work is In review - sleeps when
// its warning runs out, and the list change is announced; the session keeps its agent session id,
// so a later message wakes the same conversation (docs/architecture.md 5.2).
func TestAnIdleCardSleepsWhenItsWarningRunsOut(t *testing.T) {
	e, clock := newSleepEnv(t)
	project := e.project(t, "small-repo")
	card := startCard(t, e, e.card(t, project.ID, "Waiting for review"))
	if _, err := e.proj.SetState(context.Background(), card.ID, protocol.CardStateReview); err != nil {
		t.Fatalf("move the card to review: %v", err)
	}
	before := e.sessionRow(t, card.ID).AgentSessionID
	if before == "" {
		t.Fatal("the started session has no agent session id")
	}

	clock.advance(16 * time.Minute)
	e.mgr.CheckIdle(context.Background())
	sleepWarningRow(t, e, card.ID)

	clock.advance(2 * time.Minute)
	e.mgr.CheckIdle(context.Background())

	row := e.sessionRow(t, card.ID)
	if got := protocol.SessionState(row.State); got != protocol.SessionStateAsleep {
		t.Fatalf("session state = %s, want %s", got, protocol.SessionStateAsleep)
	}
	if row.AgentSessionID != before {
		t.Fatalf("agent session id = %q, want the id it had, %q", row.AgentSessionID, before)
	}
	if notices := noticesNow(t, e); len(notices) != 0 {
		t.Fatalf("the notice stood after its cards slept: %+v", notices)
	}
	if data := untilNoticeEvent(t, e, protocol.EventTypeNoticeDismissed); len(data.Notices) != 0 {
		t.Errorf("notice.dismissed carried %+v, want none standing", data.Notices)
	}

	// A message wakes the same conversation: no new process is started, and the agent is asked for
	// another turn on the session it already had.
	starts := fakeStarts(e.agent)
	if err := e.mgr.Send(context.Background(), card.ID, "are you still there?"); err != nil {
		t.Fatalf("Send to the sleeping card: %v", err)
	}
	if got := fakeStarts(e.agent); got != starts {
		t.Fatalf("a message started %d new sessions, want the sleeping one resumed", got-starts)
	}
	_, sess := e.liveFakeSession(t, card.ID)
	waitFor(t, "the woken card to answer a turn", func() bool { return fakeTurns(sess) >= 1 })
}

// A pinned card is never warned and never slept, even when it is idle and would otherwise sleep.
func TestAPinnedCardIsNeverWarned(t *testing.T) {
	e, clock := newSleepEnv(t)
	project := e.project(t, "small-repo")
	card := startCard(t, e, e.card(t, project.ID, "Pinned"))
	if _, err := e.proj.SetState(context.Background(), card.ID, protocol.CardStateReview); err != nil {
		t.Fatalf("move the card to review: %v", err)
	}
	if _, err := e.mgr.Pin(context.Background(), card.ID); err != nil {
		t.Fatalf("Pin: %v", err)
	}

	clock.advance(time.Hour)
	e.mgr.CheckIdle(context.Background())

	if notices := noticesNow(t, e); len(notices) != 0 {
		t.Fatalf("a pinned card got a notice: %+v", notices)
	}
	if got := protocol.SessionState(e.sessionRow(t, card.ID).State); got != protocol.SessionStateAwake {
		t.Fatalf("session state = %s, want it left awake", got)
	}
}

// One project gets one reminder ("grouped reminders", 5.2): a card that goes idle while its project
// already has a notice joins it and keeps its deadline rather than pushing it back.
func TestOneProjectGetsOneGroupedReminder(t *testing.T) {
	e, clock := newSleepEnv(t)
	setSleep(e, 15, 30, 15)
	project := e.project(t, "small-repo")
	first := pausedCard(t, e, project.ID, "First idle")

	clock.advance(16 * time.Minute)
	e.mgr.CheckIdle(context.Background())
	deadline := onlyNotice(t, e).Deadline.Time()

	// The second card is started and paused, then left idle long enough to be warned itself, all
	// while the first card's 30-minute warning is still standing.
	second := pausedCard(t, e, project.ID, "Second idle")
	clock.advance(16 * time.Minute)
	e.mgr.CheckIdle(context.Background())

	notice := onlyNotice(t, e)
	if len(notice.Cards) != 2 || !hasID(notice.Cards, first.ID) || !hasID(notice.Cards, second.ID) {
		t.Fatalf("notice cards = %v, want both %s and %s", notice.Cards, first.ID, second.ID)
	}
	if got := notice.Deadline.Time(); !got.Equal(deadline) {
		t.Errorf("the second card moved the deadline to %s, want %s", got, deadline)
	}
}

// hasID reports whether a list of ids holds one.
func hasID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// "Keep awake" takes one card off the notice, moves it out of the sleep warning, and holds it off
// the idle timer for the keep-awake setting's length - so the notice the person just cleared does
// not come straight back, and does come back once the hold has run out.
func TestKeepAwakeHoldsACardOffTheIdleTimer(t *testing.T) {
	// Keep awake (15) is longer than the idle time (5), which is what makes the hold visible: the
	// card is idle again long before its hold runs out.
	e, clock := newSleepEnv(t)
	setSleep(e, 5, 2, 15)
	project := e.project(t, "small-repo")
	card := pausedCard(t, e, project.ID, "Stay with me")

	clock.advance(6 * time.Minute)
	e.mgr.CheckIdle(context.Background())
	sleepWarningRow(t, e, card.ID)

	kept, err := e.mgr.KeepAwake(context.Background(), card.ID)
	if err != nil {
		t.Fatalf("KeepAwake: %v", err)
	}
	if kept.ID != card.ID {
		t.Errorf("KeepAwake answered %q, want the card it kept", kept.ID)
	}
	if notices := noticesNow(t, e); len(notices) != 0 {
		t.Fatalf("the card stayed on a notice: %+v", notices)
	}
	if got := protocol.SessionState(e.sessionRow(t, card.ID).State); got != protocol.SessionStateAwake {
		t.Fatalf("session state = %s, want it moved out of the sleep warning", got)
	}

	// Idle again (5 minutes since the card's last activity) but still held: no new notice.
	clock.advance(5 * time.Minute)
	e.mgr.CheckIdle(context.Background())
	if notices := noticesNow(t, e); len(notices) != 0 {
		t.Fatalf("the card was warned while the keep-awake hold stood: %+v", notices)
	}

	// The hold has run out (15 minutes), and the card is idle, so it is warned again.
	clock.advance(11 * time.Minute)
	e.mgr.CheckIdle(context.Background())
	sleepWarningRow(t, e, card.ID)
}

// Declining a notice holds every card it named and answers how many, so the "Kept N cards awake"
// toast can say a number; keeping one that is already gone is not an error.
func TestKeepAllAwakeClearsTheNoticeAndCounts(t *testing.T) {
	e, clock := newSleepEnv(t)
	project := e.project(t, "small-repo")
	ids := []string{
		pausedCard(t, e, project.ID, "One").ID,
		pausedCard(t, e, project.ID, "Two").ID,
	}
	clock.advance(16 * time.Minute)
	e.mgr.CheckIdle(context.Background())
	notice := onlyNotice(t, e)

	kept, err := e.mgr.KeepAllAwake(context.Background(), notice.ID)
	if err != nil {
		t.Fatalf("KeepAllAwake: %v", err)
	}
	if kept != 2 {
		t.Errorf("KeepAllAwake answered %d, want 2", kept)
	}
	for _, id := range ids {
		if got := protocol.SessionState(e.sessionRow(t, id).State); got != protocol.SessionStateAwake {
			t.Errorf("session state of %s = %s, want awake", id, got)
		}
	}
	if notices := noticesNow(t, e); len(notices) != 0 {
		t.Fatalf("the notice stood after keep all: %+v", notices)
	}
	if again, err := e.mgr.KeepAllAwake(context.Background(), notice.ID); err != nil || again != 0 {
		t.Fatalf("keeping an already-cleared notice answered (%d, %v), want (0, nil)", again, err)
	}
}

// A plain dismiss changes no card but holds its cards off the idle timer, so the notice does not
// come straight back, and dismissing the same notice twice is not an error.
func TestDismissNoticeHoldsTheCardsAndIsIdempotent(t *testing.T) {
	e, clock := newSleepEnv(t)
	setSleep(e, 5, 2, 15)
	project := e.project(t, "small-repo")
	card := pausedCard(t, e, project.ID, "Leave it")

	clock.advance(6 * time.Minute)
	e.mgr.CheckIdle(context.Background())
	notice := onlyNotice(t, e)

	if err := e.mgr.DismissNotice(context.Background(), notice.ID); err != nil {
		t.Fatalf("DismissNotice: %v", err)
	}
	if notices := noticesNow(t, e); len(notices) != 0 {
		t.Fatalf("the notice stood after a dismiss: %+v", notices)
	}
	if got := protocol.SessionState(e.sessionRow(t, card.ID).State); got != protocol.SessionStateAwake {
		t.Errorf("session state = %s, want awake after a dismiss", got)
	}

	// The card is idle again, but the dismiss holds it, so no notice comes back.
	clock.advance(5 * time.Minute)
	e.mgr.CheckIdle(context.Background())
	if notices := noticesNow(t, e); len(notices) != 0 {
		t.Fatalf("the notice came straight back: %+v", notices)
	}
	if err := e.mgr.DismissNotice(context.Background(), notice.ID); err != nil {
		t.Fatalf("a second dismiss failed: %v", err)
	}
}

// "Sleep all now" sleeps the cards a notice still allows to sleep and answers how many went, leaving
// a card that became busy while the notice stood awake and no longer reading as about to sleep.
func TestSleepAllSleepsWhatItCan(t *testing.T) {
	e, clock := newSleepEnv(t)
	project := e.project(t, "small-repo")
	first := pausedCard(t, e, project.ID, "Sleep me")
	second := pausedCard(t, e, project.ID, "Not me")
	clock.advance(16 * time.Minute)
	e.mgr.CheckIdle(context.Background())
	notice := onlyNotice(t, e)

	// The second card is released from its pause, so it is working again and may not sleep.
	if _, err := e.mgr.Unpause(context.Background(), second.ID); err != nil {
		t.Fatalf("Unpause: %v", err)
	}

	slept, err := e.mgr.SleepAll(context.Background(), notice.ID)
	if err != nil {
		t.Fatalf("SleepAll: %v", err)
	}
	if slept != 1 {
		t.Errorf("SleepAll answered %d, want 1", slept)
	}
	if got := protocol.SessionState(e.sessionRow(t, first.ID).State); got != protocol.SessionStateAsleep {
		t.Errorf("session state of the first card = %s, want asleep", got)
	}
	if got := protocol.SessionState(e.sessionRow(t, second.ID).State); got != protocol.SessionStateAwake {
		t.Errorf("session state of the second card = %s, want awake and out of the warning", got)
	}
}

// A card that cannot be slept when its deadline passes is left awake and moved out of the sleep
// warning, so it does not go on reading as about to sleep; the rest of its group still sleeps.
func TestACardThatCannotSleepIsLeftAwakeAndCleared(t *testing.T) {
	e, clock := newSleepEnv(t)
	project := e.project(t, "small-repo")
	first := pausedCard(t, e, project.ID, "Sleeps")
	second := pausedCard(t, e, project.ID, "Released")
	clock.advance(16 * time.Minute)
	e.mgr.CheckIdle(context.Background())
	sleepWarningRow(t, e, second.ID)

	if _, err := e.mgr.Unpause(context.Background(), second.ID); err != nil {
		t.Fatalf("Unpause: %v", err)
	}
	clock.advance(2 * time.Minute)
	e.mgr.CheckIdle(context.Background())

	if got := protocol.SessionState(e.sessionRow(t, first.ID).State); got != protocol.SessionStateAsleep {
		t.Errorf("session state of the sleeping card = %s, want asleep", got)
	}
	if got := protocol.SessionState(e.sessionRow(t, second.ID).State); got != protocol.SessionStateAwake {
		t.Errorf("session state of the released card = %s, want awake and out of the warning", got)
	}
}

// The awake limit warns the oldest idle awake card when a new card takes a project past its ceiling,
// and never the card that just woke (docs/architecture.md 5.1).
func TestTheAwakeLimitWarnsTheOldestIdleCard(t *testing.T) {
	e, clock := newSleepEnv(t)
	e.mgr.SetAwakeLimits(&fakeAwakeLimits{limit: 1, set: true})
	project := e.project(t, "small-repo")

	first := pausedCard(t, e, project.ID, "Awake first")
	if notices := noticesNow(t, e); len(notices) != 0 {
		t.Fatalf("one awake card is over no ceiling, but a notice was made: %+v", notices)
	}

	clock.advance(time.Minute)
	second := startCard(t, e, e.card(t, project.ID, "Awake second"))

	notice := onlyNotice(t, e)
	if len(notice.Cards) != 1 || notice.Cards[0] != first.ID {
		t.Fatalf("the awake limit warned %v, want the oldest idle card %s", notice.Cards, first.ID)
	}
	if second.ID == first.ID {
		t.Fatal("the two cards are the same card")
	}
}

// With no ceiling set - the shipped state - waking as many cards as a person likes warns nobody.
func TestTheAwakeLimitIsNotEnforcedWhenNoneIsSet(t *testing.T) {
	e, _ := newSleepEnv(t)
	e.mgr.SetAwakeLimits(&fakeAwakeLimits{set: false})
	project := e.project(t, "small-repo")
	startCard(t, e, e.card(t, project.ID, "One"))
	startCard(t, e, e.card(t, project.ID, "Two"))

	if notices := noticesNow(t, e); len(notices) != 0 {
		t.Fatalf("a notice was made with no ceiling set: %+v", notices)
	}
}
