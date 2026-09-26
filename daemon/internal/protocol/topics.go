package protocol

import (
	"fmt"
	"slices"
	"strings"
)

// Topic names something a client can subscribe to on the event stream: "home", "me", or a kind and
// an id joined by a colon, such as "project:web-dashboard", "card:01J8Z3Y7K2M9Q4R6T8V0W1X2Y3", or
// "chat:<id>". Build topics with the constructors below and read them with ParseTopic.
type Topic string

// TopicKind is the part of a topic before the colon.
type TopicKind string

const (
	// TopicKindHome is the Home dashboard. It has no id.
	TopicKindHome TopicKind = "home"
	// TopicKindProject is one project, by its short id.
	TopicKindProject TopicKind = "project"
	// TopicKindCard is one card, by its opaque id.
	TopicKindCard TopicKind = "card"
	// TopicKindChat is one chat, by its opaque id.
	TopicKindChat TopicKind = "chat"
	// TopicKindMe is the person the connection's token belongs to: their profile, preferences, and
	// progress. It has no id. Solo use has one person; Phase 9 scopes it to the caller's own user.
	TopicKindMe TopicKind = "me"
)

// TopicKindValues lists every topic kind.
func TopicKindValues() []TopicKind {
	return []TopicKind{TopicKindHome, TopicKindProject, TopicKindCard, TopicKindChat, TopicKindMe}
}

// Valid reports whether k is a topic kind.
func (k TopicKind) Valid() bool { return slices.Contains(TopicKindValues(), k) }

// HomeTopic is the topic of the Home dashboard.
const HomeTopic Topic = "home"

// MeTopic is the topic of the person using Marshal, so their profile and preferences follow them
// between devices.
const MeTopic Topic = "me"

const topicSeparator = ":"

// ProjectTopic is the topic of one project, by its short id.
func ProjectTopic(projectID string) Topic { return newTopic(TopicKindProject, projectID) }

// CardTopic is the topic of one card, by its opaque id.
func CardTopic(cardID string) Topic { return newTopic(TopicKindCard, cardID) }

// ChatTopic is the topic of one chat, by its opaque id.
func ChatTopic(chatID string) Topic { return newTopic(TopicKindChat, chatID) }

func newTopic(kind TopicKind, id string) Topic {
	return Topic(string(kind) + topicSeparator + id)
}

// ParseTopic splits a topic into its kind and id. The id is empty for the home and me topics. It
// checks the shape of the id (a project id or an opaque id) so a subscription cannot hold junk.
func ParseTopic(topic Topic) (TopicKind, string, error) {
	switch topic {
	case HomeTopic:
		return TopicKindHome, "", nil
	case MeTopic:
		return TopicKindMe, "", nil
	}
	kindText, id, found := strings.Cut(string(topic), topicSeparator)
	kind := TopicKind(kindText)
	if !found || kind == TopicKindHome || kind == TopicKindMe || !kind.Valid() {
		return "", "", fmt.Errorf("parse topic %q: unknown kind", topic)
	}
	if kind == TopicKindProject && !ValidProjectID(id) {
		return "", "", fmt.Errorf("parse topic %q: not a project id", topic)
	}
	if kind != TopicKindProject && !ValidID(id) {
		return "", "", fmt.Errorf("parse topic %q: not an id", topic)
	}
	return kind, id, nil
}
