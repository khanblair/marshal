package briefs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Deliverer sends a brief to a chat. The daemon's is built from the chat connections a person saved.
type Deliverer interface {
	// Deliver sends one message to the channel. It answers ErrNotConnected for a channel nobody set
	// up, which a brief skips rather than reports as a failure.
	Deliver(ctx context.Context, channel, title, body string) error
}

// ErrNotConnected is what a Deliverer answers for a channel that is not set up.
var ErrNotConnected = errors.New("the channel is not connected")

// errDelivery marks a run whose brief reached a channel that was set up, but could not be sent to it.
var errDelivery = errors.New("a brief could not be sent to every channel")

// SetDeliverer gives briefs a way to send themselves. Without one a brief is only kept in the run
// history.
func (s *Service) SetDeliverer(d Deliverer) { s.deliver = d }

// budget is how much of a message a chat takes. Telegram and Discord count characters across the title
// and the body, which they show as one message; ntfy counts the bytes of the body. Each is a little
// under what the service allows.
type budget struct {
	characters int
	bytes      int
}

func budgetFor(channel string) budget {
	switch channel {
	case ChannelDiscord:
		return budget{characters: 1900}
	case ChannelNtfy:
		return budget{bytes: 3900}
	}
	return budget{characters: 3900}
}

const trimmedNote = "\n\n... trimmed. The full brief is in Marshal: Settings, Schedules, History."

func labelFor(channel string) string {
	for _, c := range Channels() {
		if c.ID == channel {
			return c.Label
		}
	}
	return channel
}

// forChat is a brief's text as a chat shows it. Telegram, Discord, and ntfy all show plain text, so the
// "## " of a heading would print as it is; the heading stands on its own line instead.
func forChat(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "## ")
	}
	return strings.Join(lines, "\n")
}

// fit cuts text to the budget at the end of a line, and says it was cut. extra is what the chat adds
// around the text, such as a title and the blank line after it.
func fit(text string, b budget, extra int) string {
	if fits(text, b, extra) {
		return text
	}
	cut := []rune(text)
	for len(cut) > 0 && !fits(string(cut)+trimmedNote, b, extra) {
		cut = cut[:len(cut)-1]
	}
	kept := string(cut)
	if i := strings.LastIndex(kept, "\n"); i > 0 {
		kept = kept[:i]
	}
	return strings.TrimRight(kept, "\n ") + trimmedNote
}

func fits(text string, b budget, extra int) bool {
	if b.bytes > 0 && len(text) > b.bytes {
		return false
	}
	return b.characters == 0 || utf8.RuneCountInString(text)+extra <= b.characters
}

// send delivers the brief to each channel the schedule names and says what happened to each. failed
// is true when a channel that is set up could not be sent to.
func (s *Service) send(ctx context.Context, sched protocol.Schedule, brief Brief) (lines []string, failed bool) {
	if len(sched.Deliver) == 0 {
		return nil, false
	}
	if s.deliver == nil {
		return []string{"No chat is set up to send briefs to, so it is kept here only."}, false
	}
	for _, channel := range sched.Deliver {
		err := s.deliver.Deliver(ctx, channel, brief.Title, fit(forChat(brief.Body), budgetFor(channel), titleRoom(brief.Title)))
		switch {
		case err == nil:
			lines = append(lines, fmt.Sprintf("Sent to %s.", labelFor(channel)))
		case errors.Is(err, ErrNotConnected):
			lines = append(lines, fmt.Sprintf("%s is not connected, so it was skipped.", labelFor(channel)))
		default:
			failed = true
			lines = append(lines, fmt.Sprintf("Could not send to %s: %v", labelFor(channel), err))
		}
	}
	return lines, failed
}

// Preview is the message a chat would get for this schedule now: the title, a blank line, and the body as
// a chat shows it (without the cut a chat's length asks for). Nothing is sent. A schedule with no parts
// is a brief made the old way, and previews as its history text.
func (s *Service) Preview(ctx context.Context, sched protocol.Schedule, since time.Time) (string, error) {
	if len(sched.Sections) == 0 {
		return s.handleLegacy(ctx, sched, since)
	}
	brief, err := s.Build(ctx, sched, since)
	if err != nil {
		return "", err
	}
	return brief.Title + "\n\n" + forChat(brief.Body), nil
}

// titleRoom is what a chat adds before the body: the title and the blank line after it.
func titleRoom(title string) int { return utf8.RuneCountInString(title) + 2 }
