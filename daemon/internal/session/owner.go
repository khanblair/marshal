package session

import "github.com/khanblair/marshal/daemon/internal/protocol"

// owner says whose session something is: a card's or a project chat's (docs/architecture.md 16.2).
// A session belongs to exactly one of the two, so exactly one of cardID and chatID is set. Card
// and chat ids are opaque ids that never collide, which lets the manager key its live sessions by
// key() alone.
//
// Nearly everything a session does is the same for both: it starts or resumes an agent, sends a
// message, streams output, records history, sleeps, and stops. What differs is small and is
// decided by asking the owner: which topic the events go to, which id an event carries, what a
// refusal says, and that a chat has no card to move, pause, or fail over to "needs you".
type owner struct {
	cardID string
	chatID string
	// projectID is the project the card or chat belongs to, kept so a project can find its own
	// sessions without asking the database.
	projectID string
}

// cardOwner is the owner of a card's session.
func cardOwner(card protocol.Card) owner {
	return owner{cardID: card.ID, projectID: card.ProjectID}
}

// chatOwner is the owner of a chat's session.
func chatOwner(chat protocol.Chat) owner {
	return owner{chatID: chat.ID, projectID: chat.ProjectID}
}

// isChat reports whether the session is a chat's.
func (o owner) isChat() bool { return o.chatID != "" }

// key is the id the manager files the live session under.
func (o owner) key() string {
	if o.isChat() {
		return o.chatID
	}
	return o.cardID
}

// noun is the word for the owner in a sentence or a log line.
func (o owner) noun() string {
	if o.isChat() {
		return "chat"
	}
	return "card"
}

// topic is where the events of the session are published: card:<id> or chat:<id>.
func (o owner) topic() protocol.Topic {
	if o.isChat() {
		return protocol.ChatTopic(o.chatID)
	}
	return protocol.CardTopic(o.cardID)
}

// about marks an answer with the id of the card or chat it is about, under the name each has always
// had in `details`.
func (o owner) about(err *protocol.Error) *protocol.Error {
	if o.isChat() {
		return err.With("chatId", o.chatID)
	}
	return err.With("cardId", o.cardID)
}
