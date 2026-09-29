package chatbot

import "github.com/khanblair/marshal/daemon/internal/protocol"

// The names of the checks both bots' tests report. They are the row labels a screen shows, so they
// are words rather than ids.
const (
	// CheckSummary is the check whose message is the one sentence a connection row shows. It is the
	// same name every connection test in Marshal uses, so the integrations list reads a chat bot's
	// row back the same way it reads GitHub's (internal/integrations.CheckSummary).
	CheckSummary = "Summary"
	// CheckBot is the check that the service answered who the bot is, which is what proves a token.
	CheckBot = "Bot"
	// CheckChat is the check that a message reaches the chat or channel, which is what proves where
	// notices go.
	CheckChat = "Chat"
)

// summaryCheck is the one sentence a connection row shows, built from the checks themselves: the
// first failure if there is one, and otherwise a short account of what was proven, downgraded to a
// warning when something could not be checked. It mirrors the same helper in
// internal/integrations; it is duplicated rather than shared because a package-private helper
// crossing a package boundary for nine lines would couple the two for no gain.
func summaryCheck(checks []protocol.TestCheck, works, partly string) protocol.TestCheck {
	for _, check := range checks {
		if check.State == protocol.CheckStateFailed {
			return protocol.TestCheck{
				Name:    CheckSummary,
				State:   protocol.CheckStateFailed,
				Message: check.Message,
				Fix:     check.Fix,
			}
		}
	}
	warned := 0
	for _, check := range checks {
		if check.State == protocol.CheckStateWarning {
			warned++
		}
	}
	summary := protocol.TestCheck{Name: CheckSummary, State: protocol.CheckStatePassed}
	switch warned {
	case 0:
		summary.Message = works
	default:
		summary.State = protocol.CheckStateWarning
		summary.Message = partly
	}
	return summary
}
