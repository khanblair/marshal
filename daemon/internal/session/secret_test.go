package session_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The secret scanner in a session (docs/backend-checklist.md B3.5, docs/marshal-product-scope.md
// section 14.5): a commit an agent makes that holds something looking like a credential stops the
// card, and the credential itself is never written down. The message queued behind that commit
// waits for a person rather than letting the agent build on top of it.

// awsExampleKey is Amazon's own public documentation example, not a real credential. It is the same
// key the secrets package's own tests use, because a rule that fires for one caller fires for both.
const awsExampleKey = "AKIAQRSTUVWXYZ234567"

// commitSecret writes a file holding a synthetic credential into a card's worktree and commits it,
// the way an agent would on its own branch.
func commitSecret(t *testing.T, e *env, cardID string) {
	t.Helper()
	ctx := context.Background()
	row, err := e.store.Queries().GetCard(ctx, cardID)
	if err != nil {
		t.Fatalf("read the card: %v", err)
	}
	if row.WorktreePath == "" || row.Branch == "" {
		t.Fatalf("the card has no worktree and branch to commit into: %+v", row)
	}
	writeFile(t, filepath.Join(row.WorktreePath, "deploy.sh"), "export AWS_ACCESS_KEY_ID="+awsExampleKey+"\n")
	for _, args := range [][]string{
		{"add", "-A"},
		{"-c", "commit.gpgsign=false", "commit", "--quiet", "--message", "add a deploy script"},
	} {
		if _, err := e.git.Run(ctx, row.WorktreePath, args...); err != nil {
			t.Fatalf("git %v in the card's worktree: %v", args, err)
		}
	}
}

// secretStopped waits for the card to be stopped over a credential and returns the card it read.
func secretStopped(t *testing.T, e *env, cardID string) protocol.Card {
	t.Helper()
	var card protocol.Card
	waitFor(t, "the card to move to needs you", func() bool {
		c, err := e.proj.Card(context.Background(), cardID)
		if err != nil {
			return false
		}
		card = c
		return c.State == protocol.CardStateNeeds
	})
	if card.NeedsReason == nil || card.NeedsReason.Kind != protocol.NeedsReasonKindSecret {
		t.Fatalf("the card's needs reason = %+v, want a secret-detected reason", card.NeedsReason)
	}
	return card
}

// secretRows are the audit rows that record the daemon stopping a card over a credential.
func secretRows(t *testing.T, e *env) []db.AuditLog {
	t.Helper()
	var out []db.AuditLog
	for _, row := range e.auditRows(t) {
		if row.Action == audit.ActionCommitBlocked {
			out = append(out, row)
		}
	}
	return out
}

// systemNotes are the notes the daemon wrote about the card's own session, in order.
func systemNotes(t *testing.T, e *env, cardID string) []history.Event {
	t.Helper()
	store, err := history.New(e.store)
	if err != nil {
		t.Fatalf("make the history store: %v", err)
	}
	notes, err := store.PageByKind(context.Background(), cardID, history.KindSystem, 0, 10)
	if err != nil {
		t.Fatalf("read the card's system notes: %v", err)
	}
	return notes.Events
}

func TestACommitWithACredentialStopsTheCard(t *testing.T) {
	e := newEnv(t)
	t.Cleanup(func() { _ = e.mgr.Close() })
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a deploy script")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	commitSecret(t, e, card.ID)

	if err := e.mgr.Send(context.Background(), card.ID, "carry on"); err != nil {
		t.Fatalf("send a message: %v", err)
	}

	stopped := secretStopped(t, e, card.ID)
	if !strings.Contains(stopped.NeedsReason.Text, "credential") {
		t.Errorf("the reason a person reads = %q, want it to name the credential", stopped.NeedsReason.Text)
	}

	// The block is on the trail: the daemon, the commit, and what was found in it, but never the
	// value itself.
	waitFor(t, "the commit.blocked audit row", func() bool { return len(secretRows(t, e)) == 1 })
	row := secretRows(t, e)[0]
	if row.Actor != audit.ActorDaemon {
		t.Errorf("the audit row actor = %q, want daemon", row.Actor)
	}
	if !contains(row.DetailJSON, `"rule":"aws-access-token"`) {
		t.Errorf("the audit detail = %s, want the rule that fired", row.DetailJSON)
	}
	if !contains(row.DetailJSON, `"file":"deploy.sh"`) {
		t.Errorf("the audit detail = %s, want the file the credential is in", row.DetailJSON)
	}
	if contains(row.DetailJSON, awsExampleKey) {
		t.Errorf("the audit row holds the credential itself: %s", row.DetailJSON)
	}

	// The person is told in the card's own history, in the daemon's own words, and again without
	// the value.
	waitFor(t, "the system note", func() bool { return len(systemNotes(t, e, card.ID)) == 1 })
	note := systemNotes(t, e, card.ID)[0]
	if !strings.Contains(note.Summary, "credential") {
		t.Errorf("the system note = %q, want it to say a credential was found", note.Summary)
	}
	if contains(note.Summary, awsExampleKey) {
		t.Errorf("the system note holds the credential itself: %q", note.Summary)
	}
}

// TestTheMessageQueuedBehindTheCommitWaitsForAPerson is the other half of the block: the turn that
// made the commit is the last one that runs. The message already queued behind it is not delivered,
// so the agent does not build on top of the offending commit.
func TestTheMessageQueuedBehindTheCommitWaitsForAPerson(t *testing.T) {
	e := newEnv(t)
	e.agent.hold = make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(e.agent.hold) }) }
	t.Cleanup(func() {
		release()
		_ = e.mgr.Close()
	})
	project := e.project(t, "small-repo")
	card := e.card(t, project.ID, "Add a deploy script")
	if _, err := e.mgr.Start(context.Background(), card.ID); err != nil {
		t.Fatalf("start the card: %v", err)
	}
	commitSecret(t, e, card.ID)

	if err := e.mgr.Send(context.Background(), card.ID, "first"); err != nil {
		t.Fatalf("send the first message: %v", err)
	}
	e.untilState(t, card.ID, protocol.SessionStateWorking)
	if err := e.mgr.Send(context.Background(), card.ID, "second"); err != nil {
		t.Fatalf("send a message behind the running turn: %v", err)
	}

	// The turn ends, the commit is read, and the card is stopped. The queued message waits.
	release()
	secretStopped(t, e, card.ID)

	waitFor(t, "the card's history to settle", func() bool { return len(systemNotes(t, e, card.ID)) == 1 })
	for _, ev := range e.historyOf(t, card.ID) {
		if ev.Kind == history.KindUser && contains(ev.Summary, "second") {
			t.Errorf("the message queued behind the offending commit was delivered anyway: %+v", ev)
		}
	}
}
