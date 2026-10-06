package briefs

import (
	"slices"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The parts a brief can be built from, by the ids a schedule stores. They are plain strings, not an
// enum on the wire, so a part can be added without changing the protocol.
const (
	SectionNeedsYou = "needs-you"
	SectionWorking  = "working"
	SectionInReview = "in-review"
	SectionFinished = "finished"
	SectionCardCI   = "card-ci"
	SectionMainCI   = "main-ci"
	SectionCalendar = "calendar"
	SectionStale    = "stale"
)

// The chat channels a brief can be sent to. They are the connections' own ids.
const (
	ChannelTelegram = "telegram"
	ChannelDiscord  = "discord"
	ChannelNtfy     = "ntfy"
)

// The starter templates' keys, and the key of a schedule made from nothing (empty).
const (
	TemplateMorning     = "morning"
	TemplateWindDown    = "wind-down"
	TemplateWeekly      = "weekly"
	TemplateFirstMeet   = "first-meeting"
	TemplateCIWatch     = "ci-pr"
	TemplateNeedsDigest = "needs-you"
	TemplateStale       = "stale"
)

// Sections lists every part, in the order the editor shows them and a brief writes them.
func Sections() []protocol.ScheduleSection {
	return []protocol.ScheduleSection{
		{ID: SectionCalendar, Label: "Calendar", Hint: "Your Google Calendar events: today's, tomorrow's, or the coming week's."},
		{ID: SectionNeedsYou, Label: "Needs you", Hint: "Cards waiting on you, and how long since each last moved."},
		{ID: SectionWorking, Label: "Still working", Hint: "Cards an agent is working on right now."},
		{ID: SectionInReview, Label: "In review", Hint: "Cards in review, with their pull requests."},
		{ID: SectionFinished, Label: "Finished", Hint: "Cards that finished since the last brief."},
		{ID: SectionCardCI, Label: "Card CI failures", Hint: "Cards whose CI failed since the last brief."},
		{ID: SectionMainCI, Label: "Main branch CI", Hint: "Whether each project's main branch is passing, from GitHub."},
		{ID: SectionStale, Label: "Stale cards", Hint: "Cards that have not moved in a week."},
	}
}

// Channels lists the chats a brief can go to.
func Channels() []protocol.ScheduleChannel {
	return []protocol.ScheduleChannel{
		{ID: ChannelTelegram, Label: "Telegram"},
		{ID: ChannelDiscord, Label: "Discord"},
		{ID: ChannelNtfy, Label: "ntfy"},
	}
}

// ValidSection says whether id is a part a brief can have.
func ValidSection(id string) bool {
	return slices.ContainsFunc(Sections(), func(s protocol.ScheduleSection) bool { return s.ID == id })
}

// ValidChannel says whether id is a chat a brief can go to.
func ValidChannel(id string) bool {
	return slices.ContainsFunc(Channels(), func(c protocol.ScheduleChannel) bool { return c.ID == id })
}

// The days are cron's own: Sunday is 0 and Monday is 1.
func weekdays() []int { return []int{1, 2, 3, 4, 5} }
func sunday() []int   { return []int{0} }
func monday() []int   { return []int{1} }

const (
	runOnWake   = "Run once on wake"
	skip        = "Skip"
	triggerCron = "Cron"
)

// Templates lists the starter schedules. The times are suggestions the person changes: a template is
// made switched off, and nothing runs until they turn it on.
func Templates() []protocol.ScheduleTemplate {
	return []protocol.ScheduleTemplate{
		{
			Key: TemplateMorning, Name: "Morning brief", Icon: "sunrise",
			Summary: "Today's calendar, what waits on you, what is in review, and where CI stands.",
			Trigger: triggerCron, When: "Every weekday at 8:00", Time: "08:00", Days: weekdays(), Missed: runOnWake,
			Sections: []string{SectionCalendar, SectionNeedsYou, SectionWorking, SectionInReview, SectionMainCI, SectionFinished},
		},
		{
			Key: TemplateWindDown, Name: "Evening wind-down", Icon: "sunset",
			Summary: "What finished today, what is still running, what waits on you, and tomorrow's calendar.",
			Trigger: triggerCron, When: "Every weekday at 18:00", Time: "18:00", Days: weekdays(), Missed: skip,
			Sections: []string{SectionFinished, SectionWorking, SectionNeedsYou, SectionCardCI, SectionCalendar},
		},
		{
			Key: TemplateWeekly, Name: "Weekly review", Icon: "calendar-check",
			Summary: "The week's finished work, what is still open, stale cards, CI, and the coming week's calendar.",
			Trigger: triggerCron, When: "Every Sunday at 18:00", Time: "18:00", Days: sunday(), Missed: runOnWake,
			Sections: []string{SectionFinished, SectionInReview, SectionNeedsYou, SectionStale, SectionMainCI, SectionCalendar},
		},
		{
			Key: TemplateFirstMeet, Name: "Before my first meeting", Icon: "calendar",
			Summary: "A short brief a little before your first calendar event of the day.",
			Trigger: "Event", When: "When 30 minutes before my first calendar event", Time: "", Days: []int{}, Missed: skip,
			Sections: []string{SectionCalendar, SectionNeedsYou, SectionMainCI},
		},
		{
			Key: TemplateCIWatch, Name: "CI and PR watch", Icon: "git-pull-request",
			Summary: "A midday check of failing CI and pull requests waiting on you. Says nothing when all is well.",
			Trigger: triggerCron, When: "Every weekday at 13:00", Time: "13:00", Days: weekdays(), Missed: skip,
			Sections: []string{SectionMainCI, SectionCardCI, SectionInReview}, QuietWhenEmpty: true,
		},
		{
			Key: TemplateNeedsDigest, Name: "Needs-you digest", Icon: "hand",
			Summary: "Every card waiting on you and how long it has waited. Says nothing when none are.",
			Trigger: triggerCron, When: "Every weekday at 14:00", Time: "14:00", Days: weekdays(), Missed: skip,
			Sections: []string{SectionNeedsYou}, QuietWhenEmpty: true,
		},
		{
			Key: TemplateStale, Name: "Stale cards", Icon: "archive",
			Summary: "Cards that have not moved in a week, once a week. Says nothing when there are none.",
			Trigger: triggerCron, When: "Every Monday at 9:00", Time: "09:00", Days: monday(), Missed: skip,
			Sections: []string{SectionStale}, QuietWhenEmpty: true,
		},
	}
}

// TemplateByKey finds a starter by its key.
func TemplateByKey(key string) (protocol.ScheduleTemplate, bool) {
	for _, t := range Templates() {
		if t.Key == key {
			return t, true
		}
	}
	return protocol.ScheduleTemplate{}, false
}

// Catalog is what GET /v1/schedules/catalog answers: the starters, the parts, and the channels.
func Catalog(now time.Time) protocol.ScheduleCatalog {
	return protocol.ScheduleCatalog{
		Templates: Templates(), Sections: Sections(), Channels: Channels(),
		ServerTime: protocol.NewTimestamp(now),
	}
}

// Lookback is the furthest back a brief of this template reads, however long ago the last one ran, so
// the first brief of a new schedule does not report on weeks of work. Daily briefs reach back far
// enough to cover a weekend; weekly ones, a week and a day.
func Lookback(template string) time.Duration {
	const day = 24 * time.Hour
	switch template {
	case TemplateWeekly, TemplateStale:
		return 8 * day
	default:
		return 4 * day
	}
}
