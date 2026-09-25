package events

// Filter says which topics a subscriber follows: an exact set of topics, or every topic. The
// zero value follows nothing.
type Filter struct {
	all    bool
	topics map[string]struct{}
}

// AllTopics follows every topic.
func AllTopics() Filter { return Filter{all: true} }

// Topics follows exactly the topics given.
func Topics(topics ...string) Filter {
	f := Filter{topics: make(map[string]struct{}, len(topics))}
	f.add(topics...)
	return f
}

// Matches reports whether an event on topic passes the filter.
func (f Filter) Matches(topic string) bool {
	if f.all {
		return true
	}
	_, ok := f.topics[topic]
	return ok
}

// Apply returns the events that pass the filter, in their order. The API uses it on the result
// of Since, so a client is replayed only the topics it follows.
func (f Filter) Apply(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, ev := range events {
		if f.Matches(ev.Topic) {
			out = append(out, ev)
		}
	}
	return out
}

func (f *Filter) add(topics ...string) {
	if f.topics == nil {
		f.topics = make(map[string]struct{}, len(topics))
	}
	for _, topic := range topics {
		f.topics[topic] = struct{}{}
	}
}

func (f *Filter) remove(topics ...string) {
	for _, topic := range topics {
		delete(f.topics, topic)
	}
}

// clone copies the filter, so a subscription can change its own set without touching the caller's.
func (f Filter) clone() Filter {
	out := Filter{all: f.all, topics: make(map[string]struct{}, len(f.topics))}
	for topic := range f.topics {
		out.topics[topic] = struct{}{}
	}
	return out
}
