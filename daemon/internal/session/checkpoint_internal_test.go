package session

import "testing"

// A chat is not a card, and the harness has nothing to say about it: no role's ceilings, no restore
// point, no loop to catch. These two calls are the harness's two entry points for a turn, and both
// must return before they reach the manager's store. A manager built with no store at all makes that
// provable: a chat that got through would panic here rather than pass by luck.
func TestAChatNeverReachesTheHarness(t *testing.T) {
	m := &Manager{}
	chat := &liveSession{owner: owner{chatID: "01M3C107JB041061050R3GG28A", projectID: "01M3C107JB041061050R3GG28B"}}

	m.checkpointBeforeTurn(chat)
	if m.checkAfterTurn(chat) {
		t.Error("a chat's turn was stopped by the harness")
	}
}
