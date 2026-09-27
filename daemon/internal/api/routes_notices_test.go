package api_test

// The notice routes and the sleep-settings routes over HTTP (docs/architecture.md sections 5.1, 5.2,
// and 10.1; checklist item B5.6, inventory N5). What the idle timer decides is internal/session's own
// business and is tested there against a fake clock and a fake settings reader; this file tests the
// wire: the shape of the list, the four actions and their answers, and the numbers the Settings
// screen reads and writes.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// sleepNoticeID is the id of a project's sleep notice, as the manager builds it.
func sleepNoticeID(projectID string) string {
	return string(protocol.NoticeKindSleep) + ":" + projectID
}

// notices reads the whole notice list.
func (st *stack) notices() protocol.NoticeList {
	st.t.Helper()
	return decode[protocol.NoticeList](st.t, st.do(http.MethodGet, "/v1/notices", nil).want(st.t, http.StatusOK))
}

// onlyNotice answers the one standing notice, and fails when there is not exactly one.
func onlyNotice(t *testing.T, st *stack) protocol.Notice {
	t.Helper()
	list := st.notices()
	if len(list.Notices) != 1 {
		t.Fatalf("the notices = %+v, want the one sleep notice", list.Notices)
	}
	return list.Notices[0]
}

// pausedAwakeCard starts a card and pauses it, so its session is awake and its card state is
// working-with-paused: the shape the idle timer may warn (architecture.md 5.1, "Working cards don't
// sleep. Pause the card first.").
func (st *stack) pausedAwakeCard(projectID, title string) protocol.Card {
	st.t.Helper()
	card := st.addCard(projectID, title)
	stream := st.dial(protocol.CardTopic(card.ID))
	st.do(http.MethodPost, "/v1/cards/"+card.ID+"/start", nil).want(st.t, http.StatusOK)
	stream.until(stateOf(card.ID, protocol.SessionStateAwake))
	paused := decode[protocol.Card](st.t, st.do(http.MethodPost, "/v1/cards/"+card.ID+"/pause", nil).want(st.t, http.StatusOK))
	if !paused.Paused || paused.State != protocol.CardStateWorking {
		st.t.Fatalf("the card = %+v, want a working card with paused set", paused)
	}
	return paused
}

// idle runs one pass of the idle timer, as StartSleepWatch does on its ticker, at the stack's clock
// moved forward by d. Moving the clock is how a test makes a card idle without waiting out the idle
// setting, and one pass by hand is one tick of the timer.
func (st *stack) idle(d time.Duration) {
	st.t.Helper()
	st.advance(d)
	st.mgr.CheckIdle(context.Background())
}

// A fresh daemon has no notices, and the list is an empty array rather than null, so no client has to
// handle both an empty list and a missing one.
func TestAFreshDaemonHasNoNotices(t *testing.T) {
	st := newStack(t)
	list := st.notices()
	if list.Notices == nil || len(list.Notices) != 0 {
		t.Errorf("the notice list on a fresh daemon = %+v, want an empty array", list)
	}
}

// One whole sleep notice over HTTP: a card goes idle, the daemon warns it, the person keeps it awake,
// and the notice is dismissed. The list and the card both go back to how they were.
func TestAnIdleCardIsWarnedAndKeptAwakeThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	home := st.dial(protocol.HomeTopic)
	card := st.pausedAwakeCard(project.ID, "Idle and waiting")

	st.idle(16 * time.Minute)
	notice := onlyNotice(t, st)
	if notice.Kind != protocol.NoticeKindSleep || notice.ProjectID != project.ID {
		t.Errorf("the notice = %+v, want a sleep notice for project %s", notice, project.ID)
	}
	if notice.ID != sleepNoticeID(project.ID) {
		t.Errorf("the notice id = %q, want %q", notice.ID, sleepNoticeID(project.ID))
	}
	if len(notice.Cards) != 1 || notice.Cards[0] != card.ID {
		t.Errorf("the notice names %v, want just %s", notice.Cards, card.ID)
	}
	if notice.Deadline == nil {
		t.Error("a sleep notice names the moment its cards sleep")
	}
	// The card reads as about to sleep, which is what the card's own panel draws.
	if warned := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK)); warned.Session == nil || *warned.Session != protocol.SessionStateSleepWarning {
		t.Errorf("the warned card's session = %v, want %q", warned.Session, protocol.SessionStateSleepWarning)
	}
	// The whole list went out on the home topic, so a watching shell redraws from one event.
	seen := home.until(ofType(protocol.EventTypeNoticeCreated))
	if data := dataOf[protocol.NoticeListEventData](t, seen[len(seen)-1]); len(data.Notices) != 1 || data.Notices[0].ID != notice.ID {
		t.Errorf("notice.created carried %+v, want the standing notice", data.Notices)
	}

	kept := decode[protocol.NoticeActionResult](t, st.do(http.MethodPost, "/v1/notices/"+notice.ID+"/actions",
		protocol.NoticeActionRequest{Action: protocol.NoticeActionKeepAwake, CardID: card.ID}).want(t, http.StatusOK))
	if kept.Cards != 1 {
		t.Errorf("keep awake answered %d cards, want 1", kept.Cards)
	}
	if list := st.notices(); len(list.Notices) != 0 {
		t.Errorf("the notices after keeping the card awake = %+v, want none", list.Notices)
	}
	after := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK))
	if after.Session == nil || *after.Session != protocol.SessionStateAwake {
		t.Errorf("the card kept awake reads %v, want %q", after.Session, protocol.SessionStateAwake)
	}
	// Keeping it awake holds it off the idle timer, so the notice the person just cleared does not
	// come straight back on the next pass.
	st.mgr.CheckIdle(context.Background())
	if list := st.notices(); len(list.Notices) != 0 {
		t.Errorf("a card that was kept awake was warned again at once: %+v", list.Notices)
	}
	// Dismissing a notice that is already gone is not an error, so a client may call it twice.
	st.do(http.MethodDelete, "/v1/notices/"+notice.ID, nil).want(t, http.StatusNoContent)
}

// Sleeping a whole notice puts its cards to sleep and answers how many went, which is the number the
// toast says.
func TestSleepingAWholeNoticeThroughHTTP(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	home := st.dial(protocol.HomeTopic)
	card := st.pausedAwakeCard(project.ID, "Sleep me")
	st.idle(16 * time.Minute)
	notice := onlyNotice(t, st)

	result := decode[protocol.NoticeActionResult](t, st.do(http.MethodPost, "/v1/notices/"+notice.ID+"/actions",
		protocol.NoticeActionRequest{Action: protocol.NoticeActionSleepAll}).want(t, http.StatusOK))
	if result.Cards != 1 {
		t.Errorf("sleep all answered %d cards, want 1", result.Cards)
	}
	row, err := st.store.Queries().GetSessionByCard(context.Background(), card.ID)
	if err != nil || row.State != string(protocol.SessionStateAsleep) {
		t.Errorf("the slept card's session row = %+v (error %v), want %q", row, err, protocol.SessionStateAsleep)
	}
	if slept := decode[protocol.Card](t, st.do(http.MethodGet, "/v1/cards/"+card.ID, nil).want(t, http.StatusOK)); slept.Session == nil || *slept.Session != protocol.SessionStateAsleep {
		t.Errorf("the card = %v, want session %q", slept.Session, protocol.SessionStateAsleep)
	}
	// A notice that has slept its cards is gone, and the shell is told so.
	if list := st.notices(); len(list.Notices) != 0 {
		t.Errorf("the notices after sleeping them all = %+v, want none", list.Notices)
	}
	dismissed := home.until(ofType(protocol.EventTypeNoticeDismissed))
	if data := dataOf[protocol.NoticeListEventData](t, dismissed[len(dismissed)-1]); len(data.Notices) != 0 {
		t.Errorf("notice.dismissed carried %+v, want an empty list", data.Notices)
	}
}

// The calls that name something Marshal has no meaning for, and the ones whose card may not sleep,
// answer the errors of their own kind: an invalid argument for an action that does not exist, the
// sentence of section 5.1 for a card that stays awake, and no error at all for a notice that is
// already gone.
func TestTheNoticeCallsRefuseWhatMarshalDoesNotDo(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.pausedAwakeCard(project.ID, "Idle and waiting")
	st.idle(16 * time.Minute)
	notice := onlyNotice(t, st)
	if len(notice.Cards) != 1 || notice.Cards[0] != card.ID {
		t.Fatalf("the notice names %v, want just %s", notice.Cards, card.ID)
	}
	base := "/v1/notices/" + notice.ID + "/actions"

	got := st.do(http.MethodPost, base, protocol.NoticeActionRequest{Action: "sing"}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	want := `There is no "sing" action on a notice, so nothing was changed. A notice can be kept awake, slept now, kept all awake, or slept all now.`
	if got.Message != want {
		t.Errorf("message = %q\nwant      %q", got.Message, want)
	}

	// A card that is working and nobody paused stays awake, whoever asks it to sleep, and the
	// refusal is the sentence of architecture.md 5.1.
	busy := st.addCard(project.ID, "Busy")
	stream := st.dial(protocol.CardTopic(busy.ID))
	st.do(http.MethodPost, "/v1/cards/"+busy.ID+"/start", nil).want(t, http.StatusOK)
	stream.until(stateOf(busy.ID, protocol.SessionStateAwake))
	got = st.do(http.MethodPost, base,
		protocol.NoticeActionRequest{Action: protocol.NoticeActionSleepNow, CardID: busy.ID}).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	wantHoldRefusal(t, got, "Working cards don't sleep. Pause the card first.",
		protocol.HoldRefusalReasonSleepWorking)

	// Keeping the whole notice awake answers how many cards it held, and a second call on the notice
	// that has just gone answers zero rather than not found: the notice is derived state, and one
	// that went away between the draw and the press is not a mistake.
	kept := decode[protocol.NoticeActionResult](t, st.do(http.MethodPost, base,
		protocol.NoticeActionRequest{Action: protocol.NoticeActionKeepAll}).want(t, http.StatusOK))
	if kept.Cards != 1 {
		t.Errorf("keep all awake answered %d cards, want 1", kept.Cards)
	}
	again := decode[protocol.NoticeActionResult](t, st.do(http.MethodPost, base,
		protocol.NoticeActionRequest{Action: protocol.NoticeActionKeepAll}).want(t, http.StatusOK))
	if again.Cards != 0 {
		t.Errorf("a second keep all awake answered %d cards, want 0", again.Cards)
	}
	if list := st.notices(); len(list.Notices) != 0 {
		t.Errorf("the notices = %+v, want none", list.Notices)
	}

	// A notice id that names nothing is an empty group, not a malformed address.
	st.do(http.MethodDelete, "/v1/notices/"+sleepNoticeID("no-such-project"), nil).want(t, http.StatusNoContent)
}

// The sleep settings are read and written through the API, because the Settings screen (S26a) is one
// form and one write: a fresh install answers with the shipped defaults, a saved record is read back,
// and a value the screen would not offer is refused with the sentence the form shows rather than
// stored.
func TestTheSleepSettingsAreReadAndWrittenThroughHTTP(t *testing.T) {
	st := newStack(t)
	fresh := decode[protocol.SleepSettings](t, st.do(http.MethodGet, "/v1/settings/sleep", nil).want(t, http.StatusOK))
	if fresh != protocol.DefaultSleepSettings() {
		t.Errorf("a fresh install's sleep settings = %+v, want the shipped defaults %+v",
			fresh, protocol.DefaultSleepSettings())
	}

	saved := protocol.SleepSettings{
		IdleMinutes: 30, WarningMinutes: 5, KeepAwakeMinutes: 45,
		Restore: protocol.SleepRestoreManual, Channel: protocol.SleepChannelInApp,
	}
	if got := decode[protocol.SleepSettings](t, st.do(http.MethodPut, "/v1/settings/sleep", saved).want(t, http.StatusOK)); got != saved {
		t.Errorf("the saved sleep settings = %+v, want %+v", got, saved)
	}
	if got := decode[protocol.SleepSettings](t, st.do(http.MethodGet, "/v1/settings/sleep", nil).want(t, http.StatusOK)); got != saved {
		t.Errorf("the sleep settings read back = %+v, want what was saved %+v", got, saved)
	}

	for _, tc := range []struct {
		name string
		in   protocol.SleepSettings
		want string
	}{
		{
			"an idle time that is not offered",
			protocol.SleepSettings{IdleMinutes: 20, WarningMinutes: 2, KeepAwakeMinutes: 15, Restore: protocol.SleepRestoreAuto, Channel: protocol.SleepChannelInApp},
			"Choose an idle time of 5, 15, 30, or 60 minutes.",
		},
		{
			"a warning that is shorter than nothing",
			protocol.SleepSettings{IdleMinutes: 15, WarningMinutes: 0, KeepAwakeMinutes: 15, Restore: protocol.SleepRestoreAuto, Channel: protocol.SleepChannelInApp},
			"The sleep warning must be at least a minute.",
		},
		{
			"a warning as long as the idle time",
			protocol.SleepSettings{IdleMinutes: 15, WarningMinutes: 15, KeepAwakeMinutes: 15, Restore: protocol.SleepRestoreAuto, Channel: protocol.SleepChannelInApp},
			"The sleep warning must be shorter than the idle time.",
		},
		{
			"a keep awake of nothing",
			protocol.SleepSettings{IdleMinutes: 15, WarningMinutes: 2, KeepAwakeMinutes: 0, Restore: protocol.SleepRestoreAuto, Channel: protocol.SleepChannelInApp},
			"Keep awake must be at least a minute.",
		},
		{
			"a restore answer that is not one of the two",
			protocol.SleepSettings{IdleMinutes: 15, WarningMinutes: 2, KeepAwakeMinutes: 15, Restore: "later", Channel: protocol.SleepChannelInApp},
			"Choose whether cards are restored on startup or resumed by hand.",
		},
		{
			"a channel that is not one of them",
			protocol.SleepSettings{IdleMinutes: 15, WarningMinutes: 2, KeepAwakeMinutes: 15, Restore: protocol.SleepRestoreAuto, Channel: "carrier-pigeon"},
			"Choose where sleep warnings go.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := st.do(http.MethodPut, "/v1/settings/sleep", tc.in).
				apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
			if got.Message != tc.want {
				t.Errorf("message = %q\nwant      %q", got.Message, tc.want)
			}
		})
	}
	// Nothing a refusal was about reached the store: the screen reads back what it last saved.
	if got := decode[protocol.SleepSettings](t, st.do(http.MethodGet, "/v1/settings/sleep", nil).want(t, http.StatusOK)); got != saved {
		t.Errorf("a refused write stored something: the settings read back = %+v, want %+v", got, saved)
	}
}

// The numbers the screen saves are the numbers the idle timer runs on: with a 30-minute idle time a
// card that has been idle for 16 minutes is not warned, and one idle for 31 is. This is the join
// between the settings the screen writes and the timer that warns, so it is checked once over HTTP.
func TestTheSavedIdleTimeIsWhatTheIdleTimerUses(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	saved := protocol.SleepSettings{
		IdleMinutes: 30, WarningMinutes: 5, KeepAwakeMinutes: 15,
		Restore: protocol.SleepRestoreAuto, Channel: protocol.SleepChannelInApp,
	}
	st.do(http.MethodPut, "/v1/settings/sleep", saved).want(t, http.StatusOK)

	card := st.pausedAwakeCard(project.ID, "Idle for a while")
	st.idle(16 * time.Minute)
	if list := st.notices(); len(list.Notices) != 0 {
		t.Fatalf("16 minutes into a 30-minute idle time the notices = %+v, want none", list.Notices)
	}
	st.idle(15 * time.Minute)
	notice := onlyNotice(t, st)
	if len(notice.Cards) != 1 || notice.Cards[0] != card.ID {
		t.Errorf("the notice names %v, want just %s", notice.Cards, card.ID)
	}
	if notice.Deadline == nil {
		t.Error("a sleep notice names the moment its cards sleep")
	}
}
