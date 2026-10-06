package schedules

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// defaultHour is the hour a schedule runs at when neither its `when` text nor its `time` names one,
// so a schedule made without a time still has a sensible trigger rather than failing.
const defaultHour = 9

// ParseCron turns a person's own words into a 5-field standard cron spec: the translation layer the
// Schedules screen needs, because its `when` is a sentence a person typed ("Every weekday at 09:00")
// and its `time`/`days` are the parts the edit form never lets them change after creation.
//
// The shapes it understands are deliberately few, and anything it does not recognise still answers a
// spec rather than an error: the `when` text is a person's, and losing it because Marshal could not
// place it would be worse than running slightly wrong. Recognised:
//
//   - an interval: "Every 15 minutes" -> "*/15 * * * *", "Every 2 hours" -> "0 */2 * * *"
//   - a clock time, from `timeStr` or the HH:MM inside `whenStr`: "09:00" -> "0 9 * * *"
//   - days, from `days` (1 is Monday), or from the words "weekday" and "weekend" in `whenStr`;
//     a full seven-day list is "*" rather than naming every day
func ParseCron(whenStr, timeStr string, days []int) (string, error) {
	when := strings.ToLower(strings.TrimSpace(whenStr))
	if spec, ok := intervalCron(when); ok {
		return spec, nil
	}
	hour, minute := clockOf(when, timeStr)
	return fmt.Sprintf("%d %d * * %s", minute, hour, daysOfWeek(when, days)), nil
}

// intervalCron reads a "every N minutes" or "every N hours" phrase. It reports false for anything
// else, including a sentence that merely starts with "every" and names no interval.
func intervalCron(when string) (string, bool) {
	if !strings.HasPrefix(when, "every ") {
		return "", false
	}
	fields := strings.Fields(when)
	if len(fields) < 3 {
		return "", false
	}
	num, err := strconv.Atoi(fields[1])
	if err != nil || num <= 0 {
		return "", false
	}
	switch {
	case strings.Contains(fields[2], "minute"):
		return fmt.Sprintf("*/%d * * * *", num), true
	case strings.Contains(fields[2], "hour"):
		return fmt.Sprintf("0 */%d * * *", num), true
	default:
		return "", false
	}
}

// clockOf reads the hour and minute a schedule runs at. The `time` field wins when it has one; when
// it is empty the clock is looked for inside `when` ("Every weekday at 09:00"). A part that cannot
// be read leaves its default in place rather than failing the whole spec, so "25:00" keeps the
// default hour and a valid minute.
func clockOf(when, timeStr string) (hour, minute int) {
	hour, minute = defaultHour, 0
	target := strings.TrimSpace(timeStr)
	if target == "" {
		target = clockField(when)
	}
	parts := strings.Split(target, ":")
	if len(parts) != 2 {
		return hour, minute
	}
	if h, err := strconv.Atoi(parts[0]); err == nil && h >= 0 && h < 24 {
		hour = h
	}
	if m, err := strconv.Atoi(parts[1]); err == nil && m >= 0 && m < 60 {
		minute = m
	}
	return hour, minute
}

// clockField finds the HH:MM-looking word inside a sentence, or "" when there is none.
func clockField(when string) string {
	for _, field := range strings.Fields(when) {
		if strings.Contains(field, ":") {
			return field
		}
	}
	return ""
}

// monthNames maps an English month's name (lowercase, both forms) to its cron number.
func monthNames() map[string]int {
	return map[string]int{
		"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3,
		"apr": 4, "april": 4, "may": 5, "jun": 6, "june": 6, "jul": 7, "july": 7,
		"aug": 8, "august": 8, "sep": 9, "sept": 9, "september": 9, "oct": 10, "october": 10,
		"nov": 11, "november": 11, "dec": 12, "december": 12,
	}
}

// ParseOneTime turns a one-time schedule's own words into a 5-field cron spec that fires on one
// exact date: "On 1 October at 14:00" or "On October 1 at 14:00" -> "0 14 1 10 *". now is when the
// schedule was saved, and a date this cannot place, or one that has already passed, becomes
// tomorrow at the clock time it did find - a fresh date rather than one that would never fire or
// would fire the moment it is disabled after its first run.
func ParseOneTime(whenStr, timeStr string, now time.Time) string {
	when := strings.ToLower(strings.TrimSpace(whenStr))
	hour, minute := clockOf(when, timeStr)
	month, day, ok := monthAndDay(when)
	if ok {
		// The spec carries no year, so cron.Next(now) finds the soonest date, whether that is
		// still this year or, when it already passed, the same date next year.
		return fmt.Sprintf("%d %d %d %d *", minute, hour, day, month)
	}
	tomorrow := now.AddDate(0, 0, 1)
	return fmt.Sprintf("%d %d %d %d *", minute, hour, tomorrow.Day(), int(tomorrow.Month()))
}

// monthAndDay finds a month name and a day-of-month number in a one-time schedule's own words,
// in either order ("1 October" or "October 1"). It reports false when it can find no month name.
func monthAndDay(when string) (month, day int, ok bool) {
	fields := strings.Fields(when)
	for i, field := range fields {
		clean := strings.Trim(field, ".,")
		m, known := monthNames()[clean]
		if !known {
			continue
		}
		if n, dayOK := dayNear(fields, i); dayOK {
			return m, n, true
		}
		return m, 1, true
	}
	return 0, 0, false
}

// dayNear reads a day-of-month number from the words next to a month name at index i - "1" in
// "1 October" (the word before) or "October 1" (the word after).
func dayNear(fields []string, i int) (int, bool) {
	for _, j := range []int{i - 1, i + 1} {
		if j < 0 || j >= len(fields) {
			continue
		}
		digits := strings.TrimFunc(fields[j], func(r rune) bool { return r < '0' || r > '9' })
		if digits == "" {
			continue
		}
		if n, err := strconv.Atoi(digits); err == nil && n >= 1 && n <= 31 {
			return n, true
		}
	}
	return 0, false
}

// daysOfWeek writes the cron day-of-week field. A list of one to six days names exactly those days;
// a full seven is every day and is written "*", because naming all seven means the same thing and
// reads worse. With no list, the words in `when` decide: "weekday" is Monday to Friday and "weekend"
// is Saturday and Sunday. Anything else runs every day.
func daysOfWeek(when string, days []int) string {
	switch {
	case len(days) > 0 && len(days) < 7:
		names := make([]string, len(days))
		for i, day := range days {
			names[i] = strconv.Itoa(day)
		}
		return strings.Join(names, ",")
	case strings.Contains(when, "weekday"):
		return "1-5"
	case strings.Contains(when, "weekend"):
		return "0,6"
	default:
		return "*"
	}
}

// Reconcile makes the time and days a schedule stores agree with the words it was typed as. ParseCron
// lets `time` and `days` win over the sentence, so a schedule edited from "Every weekday at 9:00" to
// "Every Monday at 7:30" kept running on weekdays at 9:00 while it read as Monday at 7:30. The
// person's words win here: a clock time in them becomes the time, and the days they name (weekday,
// weekend, every day, or day names) become the days. Words that name neither leave what was sent.
func Reconcile(req protocol.SaveScheduleRequest) (string, []int) {
	when := strings.ToLower(strings.TrimSpace(req.When))
	timeStr, days := req.Time, req.Days
	if req.Trigger == triggerEvent {
		return timeStr, days
	}
	if _, isInterval := intervalCron(when); isInterval {
		return timeStr, days
	}
	if h, m, ok := clockIn(when); ok {
		timeStr = fmt.Sprintf("%02d:%02d", h, m)
	}
	if req.Trigger == "Cron" {
		if named, ok := daysIn(when); ok {
			days = named
		}
	}
	return timeStr, days
}

// clockIn reads the first H:MM or HH:MM in a sentence.
func clockIn(when string) (hour, minute int, ok bool) {
	for _, field := range strings.Fields(when) {
		field = strings.Trim(field, ".,;")
		parts := strings.Split(field, ":")
		if len(parts) != 2 {
			continue
		}
		h, errH := strconv.Atoi(parts[0])
		m, errM := strconv.Atoi(parts[1])
		if errH == nil && errM == nil && h >= 0 && h < 24 && m >= 0 && m < 60 {
			return h, m, true
		}
	}
	return 0, 0, false
}

// dayNames maps a day's name or abbreviation (lowercase) to its cron number.
func dayNames() map[string]int {
	return map[string]int{
		"sunday": 0, "sun": 0, "monday": 1, "mon": 1, "tuesday": 2, "tue": 2, "tues": 2,
		"wednesday": 3, "wed": 3, "thursday": 4, "thu": 4, "thur": 4, "thurs": 4,
		"friday": 5, "fri": 5, "saturday": 6, "sat": 6,
	}
}

// daysIn reads which days a sentence names. An empty list means every day, which is what "every day"
// and "daily" say. It reports false when the sentence names no days at all.
func daysIn(when string) ([]int, bool) {
	switch {
	case strings.Contains(when, "weekday"):
		return []int{1, 2, 3, 4, 5}, true
	case strings.Contains(when, "weekend"):
		return []int{0, 6}, true
	case strings.Contains(when, "every day"), strings.Contains(when, "everyday"), strings.Contains(when, "daily"):
		return []int{}, true
	}
	seen := map[int]bool{}
	names := dayNames()
	for _, field := range strings.FieldsFunc(when, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if day, ok := names[field]; ok {
			seen[day] = true
		}
	}
	if len(seen) == 0 {
		return nil, false
	}
	out := make([]int, 0, len(seen))
	for day := range seen {
		out = append(out, day)
	}
	sort.Ints(out)
	return out, true
}
