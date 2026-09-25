package session

import (
	"context"
	"errors"
	"fmt"
)

// Close stops every live session's agent process, so nothing is orphaned when the daemon exits,
// and waits for their pump goroutines to finish. It does NOT write "stopped" to a sessions row
// that was awake or working: their persisted state must stay exactly as it was, so RestoreAll on
// the next start treats them as needing a resume. This is the one place Stop (the method, which
// does write "stopped": the user asked the card to stop) and "the process ends" (Close: the daemon
// itself is exiting) must not be confused with each other, and it is easy to get backwards.
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closed = true
	live := make([]*liveSession, 0, len(m.sessions))
	for _, ls := range m.sessions {
		live = append(live, ls)
	}
	m.mu.Unlock()

	var errs []error
	for _, ls := range live {
		ls.setStopRequested()
		ctx, cancel := context.WithTimeout(context.Background(), closeStopTimeout)
		if err := ls.agent.Stop(ctx, ls.handle); err != nil {
			errs = append(errs, fmt.Errorf("stop the session of card %s: %w", ls.cardID, err))
		}
		cancel()
	}
	// Each session's own pump goroutine drains the rest of its events and, because stopRequested
	// is set, leaves its row untouched (see finishPump). Waiting here means every log file is
	// closed, and every process really gone, before this call returns.
	m.pumpWG.Wait()
	m.cancel()
	return errors.Join(errs...)
}
