package session

import (
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/netwatch"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// What a card says while its agent waits to ask its model again, and what happens when the network
// ends a turn (docs/architecture.md section 19.5).
//
// A wait is shown on the card's "doing now" line and nowhere else. It is not history and it is not a
// state: the card stays in its column, and the line goes away at the next thing the agent does. A
// turn the network killed is different: the agent has nothing to work with, so its process is ended
// and the card moves to Needs you with a Resume agent button, the same as an agent that stopped by
// itself. Nothing restarts it for the person, because a restart costs tokens.

// networkStoppedNote is what the card's chat says when the network ended a turn and the agent stopped.
const networkStoppedNote = "The network dropped, so the agent stopped. Press Resume agent when you are back online."

// retryText is the "doing now" line for a wait. A network wait starts with the prefix a screen shows as
// a badge. Any other wait is the model being busy.
func retryText(e agents.Retrying) string {
	of := ""
	if e.Max > 0 {
		of = fmt.Sprintf(" of %d", e.Max)
	}
	if e.Network {
		return fmt.Sprintf("%s, retry %d%s", protocol.RetryDoingNowPrefix, e.Attempt, of)
	}
	return fmt.Sprintf("The model is busy, retry %d%s", e.Attempt, of)
}

// busyPrefix starts the "doing now" line of a wait that is not the network.
const busyPrefix = "The model is busy"

// isRetryText says a "doing now" line is one this file wrote, so it is never mistaken for the agent's own.
func isRetryText(line string) bool {
	return strings.HasPrefix(line, protocol.RetryDoingNowPrefix) || strings.HasPrefix(line, busyPrefix)
}

// onRetrying puts a wait on the card. A chat has no card to show it on. The wait is also remembered, so
// a turn that then fails can be told apart as the network's doing. The first wait keeps the agent's own
// line, which the agent sets through its board tool, so it can be put back when the wait is over.
func (m *Manager) onRetrying(ls *liveSession, e agents.Retrying) {
	ls.sawNetworkRetry = e.Network
	if ls.isChat() {
		return
	}
	if !ls.retryShown {
		ls.priorDoing = ""
		if card, err := m.projects.Card(m.ctx, ls.cardID); err == nil && !isRetryText(card.DoingNow) {
			ls.priorDoing = card.DoingNow
		}
	}
	m.setDoingNow(ls, retryText(e))
	ls.retryShown = true
}

// clearRetry takes a wait off the card, when one is shown, and puts back the line the agent had. If the
// agent has set a new line of its own since, that line is left alone.
func (m *Manager) clearRetry(ls *liveSession) {
	if !ls.retryShown {
		return
	}
	ls.retryShown = false
	prior := ls.priorDoing
	ls.priorDoing = ""
	if card, err := m.projects.Card(m.ctx, ls.cardID); err == nil && !isRetryText(card.DoingNow) {
		return
	}
	m.setDoingNow(ls, prior)
}

// clearStaleRetry takes off a wait line left by a session that ended without saying the wait was over,
// such as a daemon that stopped mid-wait. It runs when a session starts, before anything new is shown.
func (m *Manager) clearStaleRetry(ls *liveSession) {
	if ls.isChat() || m.projects == nil {
		return
	}
	if card, err := m.projects.Card(m.ctx, ls.cardID); err == nil && isRetryText(card.DoingNow) {
		m.setDoingNow(ls, "")
	}
}

// setDoingNow writes the card's "doing now" line. A failure is logged and goes no further: the line is
// a status, and the agent's work does not depend on it.
func (m *Manager) setDoingNow(ls *liveSession, text string) {
	if _, err := m.projects.UpdateCard(m.ctx, ls.cardID, protocol.UpdateCardRequest{DoingNow: &text}); err != nil {
		m.log.Warn("could not update a card's doing now line", "card_id", ls.cardID, "error", err)
	}
}

// failedByNetwork says whether a failure the agent reported was the network: it had been waiting to
// retry for the network just before, or the words it printed say so.
func (ls *liveSession) failedByNetwork(e agents.Failed) bool {
	return !ls.isChat() && (ls.sawNetworkRetry || netwatch.HasNetworkWords(e.Message+" "+e.Detail))
}

// endNetworkFailed ends the process of a card whose turn the network killed. It runs on its own
// goroutine because stopping waits on the process, and this one is the reader of its events. The end of
// the process is what finishPump reacts to: it marks the session stopped and moves the card to Needs
// you, with the reason networkStoppedReason gives.
func (m *Manager) endNetworkFailed(ls *liveSession) {
	ls.networkStopped = true
	go func() {
		if err := ls.agent.Stop(m.ctx, ls.handle); err != nil {
			m.log.Error("could not end the session of an agent the network stopped", "card_id", ls.cardID, "error", err)
		}
	}()
}

// networkStoppedReason is what a card says when the network ended its agent's turn.
func networkStoppedReason(label string) string {
	return fmt.Sprintf("The network dropped while the agent for %s was working, so it stopped. "+
		"Once you are back online, press Resume agent to continue where it left off.", label)
}
