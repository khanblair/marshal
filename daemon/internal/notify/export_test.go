package notify

import "github.com/khanblair/marshal/daemon/internal/events"

// EventOf reaches the private event mapper from this package's own tests, the way other packages in
// this repo expose one symbol rather than widening the production API to match a test. It is the
// mapping that decides what a phone buzzes about, so it is worth testing exactly rather than
// inferring it through a live bus.
func (s *Service) EventOf(ev events.Event) (Event, bool) { return s.eventOf(ev) }
