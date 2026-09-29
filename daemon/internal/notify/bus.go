package notify

import (
	"context"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// This file is what makes the router part of a running daemon: the event bus on one side, and the
// routing table and grouping on the other. It is deliberately thin, and it is the only place that
// knows an event payload's shape, so a payload that changes is one type switch away rather than
// something every notifier has to care about.
//
// Only two kinds of event reach a phone today:
//
//   - `approval.requested` is actionable, so it is sent at once with the answers the agent offered
//     as its actions. A person who is away should not find out their approval waited on a grouping
//     window (B9.4's five seconds is the ceiling, not the target).
//   - `notice.created` carries the whole standing list, so only the notices that are new since the
//     last one are sent; a person is not re-buzzed about a notice they were already told about, and
//     the ones that arrive together are grouped by the router itself.
//
// Every other event type is deliberately ignored rather than sent with a made-up title: a notice
// that says "something happened" is worse than no notice.

// Follow runs the router against the daemon's bus until ctx ends: it flushes grouped notices on the
// window while it consumes events, so one call is all the daemon has to make.
//
// It runs the flusher in a goroutine of its own because the two jobs are independent - grouping is
// a clock, and consuming is a reader - and the router already keeps both under its mutex.
func (s *Service) Follow(ctx context.Context, bus *events.Bus) {
	if bus == nil {
		return
	}
	go s.Run(ctx)
	sub := bus.Subscribe(events.AllTopics())
	defer sub.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-sub.C():
			if !ok {
				return
			}
			if notice, wanted := s.eventOf(ev); wanted {
				s.Notify(ctx, notice)
			}
		}
	}
}

// eventOf reads one event as an Event the router can route. The second result is false for every
// event no one should be buzzed about.
func (s *Service) eventOf(ev events.Event) (Event, bool) {
	switch data := ev.Data.(type) {
	case protocol.ApprovalRequestedEventData:
		return approvalNotice(data), true
	case protocol.NoticeListEventData:
		return s.newNotice(data)
	default:
		return Event{}, false
	}
}

// approvalNotice is one waiting permission request, offered as the answers the agent gave. Each
// action carries the approval's id and the option's own value, so taking one from a chat is enough
// to answer it without guessing.
func approvalNotice(data protocol.ApprovalRequestedEventData) Event {
	approval := data.Approval
	title := approval.Title
	if title == "" {
		title = "Marshal needs your approval"
	}
	body := approval.Path
	if body == "" && approval.Command != "" {
		body = approval.Command
	}
	actions := make([]chatbot.Action, 0, len(approval.Options))
	for _, option := range approval.Options {
		actions = append(actions, chatbot.Action{
			Label: optionLabel(option),
			Data:  "approval:" + approval.ID + ":" + optionValue(option),
		})
	}
	return Event{
		Type:    EventApproval,
		Title:   title,
		Body:    body,
		Actions: actions,
	}
}

// A notice carries no link to a screen. The daemon does not know its own external address here -
// it may be on a tailnet name only a phone resolves - so a person follows a notice in the app's own
// list rather than through a URL this router would have to guess at.

// newNotice reads one standing notice that this router has not sent yet. The payload is the whole
// list, so a notice is sent the first time it appears and never again; the router remembers what it
// has seen. A notice with no text of its own is not worth a buzz.
func (s *Service) newNotice(data protocol.NoticeListEventData) (Event, bool) {
	var (
		title string
		body  string
		count int
	)
	for _, notice := range data.Notices {
		if notice.ID == "" || s.seenNotice(notice.ID) {
			continue
		}
		count++
		if title == "" {
			title = notice.Text
			body = notice.Sub
		}
	}
	if count == 0 {
		return Event{}, false
	}
	if count > 1 {
		body = ""
		for _, notice := range data.Notices {
			if notice.ID == "" {
				continue
			}
			if body != "" {
				body += "\n"
			}
			body += "- " + notice.Text
		}
		title = "Marshal has " + itoa(count) + " things to say"
	}
	return Event{Type: EventNeedsYou, Title: title, Body: body}, true
}

// seenNotice records an id as sent and reports whether it had already been. It is on the router so
// a restart forgets them: a notice that is standing again is worth a new buzz.
func (s *Service) seenNotice(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sent == nil {
		s.sent = make(map[string]struct{})
	}
	if _, ok := s.sent[id]; ok {
		return true
	}
	s.sent[id] = struct{}{}
	return false
}

// optionLabel is the words a person presses. An option the agent gave no name for falls back to its
// own id, so there is never an empty button.
func optionLabel(option protocol.ApprovalOption) string {
	if option.Name != "" {
		return option.Name
	}
	return optionValue(option)
}

// optionValue is the option id Marshal answers with - the same words the agent is given back, passed
// through unchanged.
func optionValue(option protocol.ApprovalOption) string { return option.ID }

// itoa writes a small count as text, so a sentence reads "3 things" without importing strconv for
// one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
