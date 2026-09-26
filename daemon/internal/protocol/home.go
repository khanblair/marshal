package protocol

// The Home dashboard. Slice A of Phase 2 builds the first version: the cards that need you, the
// awake cards, and the tile counts. Slice D adds the charts and the stored daily numbers, so the
// fields a later slice owns are added then, not now.

// HomeSnapshot is the answer to GET /v1/home/dashboard. It is built from stored state and never
// scans every card (B2.3). The `range` query parameter of the route decides how many days the
// charts cover; the lists and the tiles are the same whatever it says.
type HomeSnapshot struct {
	// Needs is the cards that wait on a person, most recently changed first.
	Needs []NeedsCard `json:"needs"`
	// Awake is the cards with a session that is awake or working, oldest first.
	Awake []AwakeCard `json:"awake"`
	// Tiles are the counts the Home tiles show.
	Tiles HomeTiles `json:"tiles"`
	// Stats are the stored numbers the Home charts draw, over the range the request asked for. It
	// is null only for an answer built without the range (a hand-built one in a test); the route
	// always fills it.
	Stats *HomeStats `json:"stats" tstype:"HomeStats | null"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// HomeStats is the chart data of the Home answer (docs/backend-checklist.md B2.3, section S19a):
// the pre-computed per-day numbers over a range of days, read from `daily_stats` in one indexed
// scan. It is never built by counting cards.
type HomeStats struct {
	// Range is how many days the answer covers: 7, 30, or 90.
	Range int `json:"range"`
	// From is the first day the answer covers, midnight in the daemon's own clock.
	From Timestamp `json:"from"`
	// To is the last day the answer covers, which is today.
	To Timestamp `json:"to"`
	// Days is one row per day in the range, oldest first, with every project added together. Never
	// null, so a range with nothing stored is a list of zeroed days rather than nothing to draw.
	Days []HomeStatDay `json:"days"`
	// Projects is the same days, one series per project, for the cost chart that draws a line per
	// project (S19b). Never null.
	Projects []HomeProjectStats `json:"projects"`
}

// HomeStatDay is one day's stored numbers. Every field is what happened that day, not a running
// total.
type HomeStatDay struct {
	// Day is midnight at the start of the day, in the daemon's own clock.
	Day Timestamp `json:"day"`
	// CardsFinished is how many cards reached done that day.
	CardsFinished int `json:"cardsFinished"`
	// Merges is how many cards were merged that day.
	Merges int `json:"merges"`
	// CIFailures is how many CI runs failed that day.
	CIFailures int `json:"ciFailures"`
	// CostMicros is what the day spent, in micro-dollars (Phase 4 fills it).
	CostMicros int64 `json:"costMicros"`
}

// HomeProjectStats is one project's own days, so a chart can draw a line per project.
type HomeProjectStats struct {
	// ProjectID is the project's short id.
	ProjectID string `json:"projectId"`
	// Days is that project's days, oldest first, the same length and days as the answer's own
	// list, with a zeroed day where the project stored nothing.
	Days []HomeStatDay `json:"days"`
}

// The Home activity stream (docs/backend-checklist.md B2.3, section S20, inventory N17). It is the
// stored `activity` table, paged newest first and filterable by kind and project. It is a different
// list from a card's activity (ActivityItem, history.go): the feed's kinds are FeedKind, and a row
// of the stream is about the whole instance rather than one card.

// FeedEntry is one entry of the Home activity stream, newest first.
type FeedEntry struct {
	// ID is the stored row's opaque id.
	ID string `json:"id"`
	// Kind is what happened.
	Kind FeedKind `json:"kind"`
	// Text is the one line the row shows.
	Text string `json:"text"`
	// ProjectID is the project the entry is about, or null when it is about no project in
	// particular (a brief, or a project that was removed).
	ProjectID *string `json:"projectId" tstype:"string | null"`
	// At is when it happened, in UTC.
	At Timestamp `json:"at"`
	// CardID is the opaque id of the card the entry is about, or empty. It is what a client would
	// use on a card route.
	CardID string `json:"cardId"`
	// CardKey is the card's key, `<projectId>#<number>`, which is how the screens name it. Empty
	// when the entry is not about a card.
	CardKey string `json:"cardKey"`
	// JobID is the id of the scheduled job the entry is about, or empty.
	JobID string `json:"jobId"`
}

// ActivityCreatedEventData is the payload of activity.created, published on the home topic after a
// row is written to the activity stream. It carries the row as it is now, not a difference, so a
// client that applies one twice, or after a replay, ends in the same place.
type ActivityCreatedEventData struct {
	// Entry is the row that was appended.
	Entry FeedEntry `json:"entry"`
	// Day is the entry's own project's numbers for that day after this row, when the row also
	// changed them, and null when it left the numbers alone (a brief, or a tool event). It is the
	// project's day rather than the whole instance's, so a client that keeps one series per project
	// updates exactly one of them; the totals are the projects added together, which is what the
	// stored range is built from too.
	Day *HomeStatDay `json:"day" tstype:"HomeStatDay | null"`
}

// NeedsCard is one card in Home's "needs you" list: enough to draw the row and open the card.
type NeedsCard struct {
	// CardID is the card's opaque id.
	CardID string `json:"cardId"`
	// Key is the card's key, such as "api#41".
	Key string `json:"key"`
	// Number is the card's number, the "41" in "#41".
	Number int `json:"number"`
	// ProjectID is the project the card belongs to.
	ProjectID string `json:"projectId"`
	// ProjectName is the project's name, so a list that mixes projects can show it.
	ProjectName string `json:"projectName"`
	// Title is the card's title.
	Title string `json:"title"`
	// Reason is why the card waits on a person.
	Reason NeedsReason `json:"reason"`
	// WaitingSince is when the card entered the needs state, so the app can show how long it has
	// waited. Null when the daemon does not know.
	WaitingSince *Timestamp `json:"waitingSince" tstype:"Timestamp | null"`
	// Role is the role's name. Empty when the card has none.
	Role string `json:"role"`
}

// AwakeCard is one card in Home's "agents awake" list.
type AwakeCard struct {
	// CardID is the card's opaque id.
	CardID string `json:"cardId"`
	// Key is the card's key, such as "api#41".
	Key string `json:"key"`
	// Number is the card's number.
	Number int `json:"number"`
	// ProjectID is the project the card belongs to.
	ProjectID string `json:"projectId"`
	// ProjectName is the project's name.
	ProjectName string `json:"projectName"`
	// Title is the card's title.
	Title string `json:"title"`
	// State is the card's state.
	State CardState `json:"state"`
	// Session is the session's state: awake, working, waking, or asleep.
	Session SessionState `json:"session"`
	// DoingNow is the agent's one-line "doing now". Empty when it is idle.
	DoingNow string `json:"doingNow"`
	// Pinned is true while the card is kept from sleeping on its own.
	Pinned bool `json:"pinned"`
	// Paused is true while a pause holds the card between turns.
	Paused bool `json:"paused"`
	// ContextUsed is how full the agent's context window is, as a percentage.
	ContextUsed int `json:"contextUsed"`
	// AwakeSince is when the session became awake or working, for how long it has run.
	AwakeSince *Timestamp `json:"awakeSince" tstype:"Timestamp | null"`
}

// HomeTiles are the counts Home shows at the top.
type HomeTiles struct {
	// Needs counts the cards that wait on a person.
	Needs int `json:"needs"`
	// Working counts the cards with a session that is working or awake.
	Working int `json:"working"`
	// MergedToday counts the cards that reached done since midnight, in the daemon's time zone.
	MergedToday int `json:"mergedToday"`
}
