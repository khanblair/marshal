package protocol

// The wire shape of a notice, and the sleep settings the automatic sleep of architecture.md section
// 5.2 is driven by (checklist item B5.6, inventory N5). A notice is one message the shell shows in
// its notices panel: the group of idle cards that will sleep soon, a CI failure, a cost warning, or
// a plan that waits for review. Phase 5 produces the sleep kind; the others are reserved by
// NoticeKind and arrive in later phases.
//
// A notice is read from the daemon and acted on through its own calls (dismiss, keep awake, sleep
// now), never kept on the client alone, because the notice is about what the daemon is doing to the
// cards: a client that decides on its own that a card was kept awake would disagree with the
// daemon the next time it loaded.

// Notice is one notice on the wire. A sleep notice names the cards it is about and the moment they
// sleep; an informational notice carries its own two sentences instead.
type Notice struct {
	// ID names the notice for the calls that act on it. It is stable while the notice lives.
	ID string `json:"id"`
	// Kind is what the notice is about (NoticeKindSleep today).
	Kind NoticeKind `json:"kind"`
	// Cards are the cards a notice is about, by their opaque ids. Empty for an informational kind.
	Cards []string `json:"cards,omitempty"`
	// Deadline is the moment a sleep notice's cards sleep, when it has one. Nil for an
	// informational notice, and for a sleep notice that has no deadline yet.
	Deadline *Timestamp `json:"deadline,omitempty"`
	// Text is a notice's one-line title, for an informational kind.
	Text string `json:"text,omitempty"`
	// Sub is the second line under the title, for an informational kind.
	Sub string `json:"sub,omitempty"`
	// ProjectID is the project a notice belongs to, when it belongs to one. Empty for a notice
	// about the whole install.
	ProjectID string `json:"projectId,omitempty"`
	// CreatedAt is when the notice was made.
	CreatedAt Timestamp `json:"createdAt"`
}

// NoticeList is the answer to GET /v1/notices: every notice that is standing, newest first is not
// guaranteed, so a client that cares sorts them.
type NoticeList struct {
	// Notices is every standing notice. It is never null, so JSON has [] and a client never
	// handles both an empty list and a missing one.
	Notices []Notice `json:"notices"`
}

// NewNoticeList makes a list. A nil list becomes an empty one.
func NewNoticeList(notices []Notice) NoticeList {
	out := make([]Notice, len(notices))
	copy(out, notices)
	return NoticeList{Notices: out}
}

// NoticeListEventData is the payload of notice.created and notice.dismissed (architecture.md
// 11.2): the whole list as it now stands, so a client redraws the notices panel from one event and
// never has to hold the difference between two of them.
type NoticeListEventData struct {
	// Notices is every standing notice.
	Notices []Notice `json:"notices"`
}

// NoticeActionResult is the answer to a notice's own call (keep all awake, sleep all now): how many
// cards it changed, so the toast can say a number.
type NoticeActionResult struct {
	// Cards is how many cards the call acted on.
	Cards int `json:"cards"`
}

// The four calls a person makes on a sleep notice, as the body of POST /v1/notices/{id}/actions
// (inventory N5). Two name one card and two are about the whole notice, which is why they are one
// call with one body rather than four addresses: the app draws two buttons per card row and two
// for the group, and the daemon answers each with a count.
//
// They are declared one by one and not in a block, like SleepRestoreAuto below, because an exported
// untyped constant in a block would become a TypeScript union this package does not mean to declare
// (protocol's own conventions test).
// NoticeActionKeepAwake is "Keep awake": hold one card off the idle timer for the keep-awake
// setting's length and take it off its notice.
const NoticeActionKeepAwake = "keep-awake"

// NoticeActionSleepNow is "Sleep now": put one card to sleep at once, whatever its deadline.
const NoticeActionSleepNow = "sleep-now"

// NoticeActionKeepAll is "Keep all awake": hold every card the notice names off the idle timer.
const NoticeActionKeepAll = "keep-all"

// NoticeActionSleepAll is "Sleep all now": put every card the notice names to sleep.
const NoticeActionSleepAll = "sleep-all"

// NoticeActionRequest is the body of POST /v1/notices/{id}/actions. CardID is required by the two
// calls that name one card and ignored by the two that are about the whole notice.
type NoticeActionRequest struct {
	// Action is one of NoticeActionKeepAwake, SleepNow, KeepAll, or SleepAll.
	Action string `json:"action"`
	// CardID is the card a per-card call is about.
	CardID string `json:"cardId,omitempty"`
}

// SleepRestoreAuto is the answer that resumes every card that was awake, right away
// (docs/architecture.md section 5.3). It is declared on its own, not in a block, because an
// exported untyped constant in a block would become a TypeScript union this package does not mean
// to declare (protocol's own conventions test).
const SleepRestoreAuto = "auto"

// SleepRestoreManual leaves the cards that were awake as they are and shows a resume button on each
// one. The other answer for SleepSettings.Restore.
const SleepRestoreManual = "manual"

// SleepSettings are the numbers and choices behind automatic sleep (B5.6, N5). They are stored with
// the install (the `settings` table) and read by the session manager: the idle timer, the sleep
// warning, and Keep awake all use them.
type SleepSettings struct {
	// IdleMinutes is how long a card's session may be idle before it gets a sleep warning.
	IdleMinutes int `json:"idleMinutes"`
	// WarningMinutes is how long the warning stands before the idle cards sleep.
	WarningMinutes int `json:"warningMinutes"`
	// KeepAwakeMinutes is how long "Keep awake" holds a card off the idle timer.
	KeepAwakeMinutes int `json:"keepAwakeMinutes"`
	// Restore is SleepRestoreAuto or SleepRestoreManual.
	Restore string `json:"restore"`
	// Channel is where sleep warnings are sent: "in-app" today, with the messaging integrations
	// (Phase 9) adding their own. Phase 5 stores it and always shows the warning in the app.
	Channel string `json:"channel"`
}

// DefaultSleepSettings is what a fresh install sleeps with: idle after 15 minutes, a 2-minute
// warning, 15 minutes of Keep awake (decision D4), automatic restore, and warnings in the app.
//
// It is here, beside the type, so the settings service and the session manager cannot answer a
// fresh install with two different sets of numbers: the service stores these on the first write,
// and the manager falls back to them when no settings service is wired at all.
func DefaultSleepSettings() SleepSettings {
	return SleepSettings{
		IdleMinutes: 15, WarningMinutes: 2, KeepAwakeMinutes: 15,
		Restore: SleepRestoreAuto, Channel: SleepChannelInApp,
	}
}

// SleepChannelInApp is where a sleep warning goes when nobody has chosen otherwise: the app's own
// notices panel. The messaging integrations of Phase 9 add their own channel names, so this is a
// value of SleepSettings.Channel and not an enum of its own.
const SleepChannelInApp = "in-app"
