package integrator_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// work makes a card with a branch of its own holding the given files, and a state, the way a card
// is when its agent has committed: not yet in Ready to merge.
func (e *env) work(title string, state protocol.CardState, files map[string]string) protocol.Card {
	e.t.Helper()
	card, err := e.proj.CreateCard(e.ctx, e.project.ID, protocol.CreateCardRequest{Title: title})
	if err != nil {
		e.t.Fatalf("make a card: %v", err)
	}
	branch := "marshal/" + strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	if files == nil {
		e.run(e.repo, "branch", "--no-track", branch, e.project.Target())
	} else {
		e.commitOn(branch, e.project.Target(), files)
	}
	if _, err := e.proj.SetWorktree(e.ctx, card.ID, "", branch); err != nil {
		e.t.Fatalf("record the card's branch: %v", err)
	}
	if state != protocol.CardStateBacklog {
		if card, err = e.proj.SetState(e.ctx, card.ID, state); err != nil {
			e.t.Fatalf("move the card to %s: %v", state, err)
		}
	}
	return card
}

// refusedWith checks that a send was refused for a stable reason, and left the card where it was.
func (e *env) refusedWith(card protocol.Card, reason string) {
	e.t.Helper()
	_, err := e.svc.SendToMerge(e.ctx, card.ID)
	var perr *protocol.Error
	if !errors.As(err, &perr) || perr.Code != protocol.ErrorCodeRefused || perr.Details["reason"] != reason {
		e.t.Fatalf("SendToMerge = %v, want a refusal with reason %q", err, reason)
	}
	if got := e.state(card.ID).State; got != card.State {
		e.t.Errorf("a refused send moved the card to %s", got)
	}
}

func TestSendingACardWithCommittedWorkMergesItIntoTheBranchAndTheFolder(t *testing.T) {
	e := newEnv(t)
	e.wireHook()
	card := e.work("Add retry", protocol.CardStateWorking, map[string]string{"b.txt": "b by the card\n"})

	sent, err := e.svc.SendToMerge(e.ctx, card.ID)
	if err != nil {
		t.Fatalf("SendToMerge: %v", err)
	}
	if sent.State != protocol.CardStateReady && sent.State != protocol.CardStateMerging && sent.State != protocol.CardStateDone {
		t.Errorf("the sent card is %s, want it on its way to the merge", sent.State)
	}
	e.svc.Wait()

	if got := e.state(card.ID).State; got != protocol.CardStateDone {
		t.Errorf("the card is %s after the queue ran, want done", got)
	}
	if got := e.read(e.repo, "b.txt"); got != "b by the card\n" {
		t.Errorf("the owner's b.txt = %q, want the card's change", got)
	}
}

func TestSendingACardThatCannotBeMergedYetIsRefusedWithTheReason(t *testing.T) {
	e := newEnv(t)
	e.refusedWith(e.work("Idea only", protocol.CardStateBacklog, map[string]string{"b.txt": "x\n"}), "send_not_started")
	e.refusedWith(e.work("No commits", protocol.CardStateWorking, nil), "send_no_commits")
}

func TestSendingACardOfAProjectOnGitHubIsRefused(t *testing.T) {
	e := newEnv(t)
	e.run(e.repo, "remote", "add", "origin", "https://github.com/acme/widgets.git")
	e.refusedWith(e.work("Add retry", protocol.CardStateWorking, map[string]string{"b.txt": "b\n"}), "send_has_github")
}

func TestSendingAFolderOriginThatIsNotOnGitHubIsAllowed(t *testing.T) {
	e := newEnv(t)
	e.wireHook()
	e.run(e.repo, "remote", "add", "origin", "https://git.example.com/acme/widgets.git")
	card := e.work("Add retry", protocol.CardStateWorking, map[string]string{"b.txt": "b\n"})
	if _, err := e.svc.SendToMerge(e.ctx, card.ID); err != nil {
		t.Fatalf("SendToMerge: %v", err)
	}
	e.svc.Wait()
	if got := e.state(card.ID).State; got != protocol.CardStateDone {
		t.Errorf("the card is %s, want done: an origin that is not GitHub has no pull request to wait for", got)
	}
}

// A card that is already waiting is not moved again, and a paused queue still holds it.
func TestSendingACardThatIsAlreadyReadyMovesNothingWhileMergingIsPaused(t *testing.T) {
	e := newEnv(t)
	e.wireHook()
	if _, err := e.svc.Pause(e.ctx, e.project.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	card := e.card("Add retry", map[string]string{"b.txt": "b by the card\n"})

	sent, err := e.svc.SendToMerge(e.ctx, card.ID)
	if err != nil {
		t.Fatalf("SendToMerge: %v", err)
	}
	e.svc.Wait()
	if sent.State != protocol.CardStateReady || e.state(card.ID).State != protocol.CardStateReady {
		t.Errorf("the card is %s, want it still waiting in ready while merging is paused", e.state(card.ID).State)
	}
}
