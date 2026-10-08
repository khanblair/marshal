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
// What reaches a phone: an approval request (sent at once, with the agent's own answers as
// actions), a card that finished, a card that needs the person for any reason but a permission, CI
// turning red, and each new standing notice (`notice.created` carries the whole list, so only what
// is new is sent). Every other event is deliberately ignored rather than sent with a made-up title:
// a notice that says "something happened" is worse than no notice.

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
	case protocol.CardMovedEventData:
		return cardMoved(data)
	case protocol.CIEventData:
		return s.branchTurnedRed(data)
	default:
		return Event{}, false
	}
}

// cardMoved turns a card's move into a notice when the move is one a person wants to hear about: a
// card that finished, or one that now waits on the person for a reason an approval does not cover.
// A card that was already in the state it moved to, or that waits on a permission (its own
// actionable notice), says nothing.
func cardMoved(data protocol.CardMovedEventData) (Event, bool) {
	card := data.Card
	switch {
	case card.State == protocol.CardStateDone && data.From != protocol.CardStateDone:
		return Event{Type: EventCardDone, Title: "Finished: " + card.Title, CardID: card.ID}, true
	case card.State == protocol.CardStateNeeds && data.From != protocol.CardStateNeeds && card.NeedsReason != nil:
		return needsYou(card), card.NeedsReason.Kind != protocol.NeedsReasonKindApprovalNeeded
	default:
		return Event{}, false
	}
}

// needsYou reads why a card waits: a red CI is its own kind of notice, and every other reason is the
// agent needing the person, with the reason's own sentence underneath.
func needsYou(card protocol.Card) Event {
	reason := card.NeedsReason
	if reason.Kind == protocol.NeedsReasonKindCIFailed {
		return Event{Type: EventCIFailed, Title: "CI failed: " + card.Title, Body: reason.Text, CardID: card.ID}
	}
	return Event{Type: EventAgentStuck, Title: needsYouTitle(card), Body: reason.Text, CardID: card.ID}
}

// needsYouTitle names the card by its project and number ahead of its title, so a notice in a chat
// says which project it is about. A card with no key reads as its title alone.
func needsYouTitle(card protocol.Card) string {
	if card.Key == "" {
		return card.Title + " needs you"
	}
	return card.Key + " needs you: " + card.Title
}

// branchTurnedRed tells the person when a project's default branch goes red, once: a branch that
// stays red through the next run is not news again, and it is told again only after it recovered.
func (s *Service) branchTurnedRed(data protocol.CIEventData) (Event, bool) {
	project := data.Project
	if project == nil {
		return Event{}, false
	}
	s.mu.Lock()
	before := s.ciStatus[project.ProjectID]
	s.ciStatus[project.ProjectID] = project.Status
	s.mu.Unlock()
	if project.Status != protocol.CIStateFailed || before == protocol.CIStateFailed {
		return Event{}, false
	}
	return Event{Type: EventCIFailed, Title: "CI failed on " + project.ProjectID}, true
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
