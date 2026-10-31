package integrations

import "time"

// SetTrelloPaceForTest is a test seam: the daemon has no setting for the wait between two writes to
// Trello, which is Trello's rate limit. Zero writes without waiting.
func (s *Service) SetTrelloPaceForTest(pace time.Duration) { s.trelloPace = pace }
