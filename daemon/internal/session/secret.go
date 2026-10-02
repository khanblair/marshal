package session

import (
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/gitx"
	"github.com/khanblair/marshal/daemon/internal/history"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/secrets"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The secret scanner's place in a session (docs/backend-checklist.md B3.5,
// docs/marshal-product-scope.md section 14.5): every commit an agent makes is read after the turn
// that made it, and a commit that holds something looking like a credential stops the card.
//
// The daemon does not intercept the commit, because it has no way to: an agent runs `git commit`
// in its own worktree, and the harness only speaks to a tool call an agent chooses to ask about.
// What the daemon can do is read the branch it gave the card - whose commits are the agent's, since
// the worktree was cut from the project's default branch - and stop the card the moment a commit
// holds a credential. That is what "blocked" means here, and it is recorded as a ruling.

// scanTurnForSecrets reads the commits the agent made since base and stops the card on the first
// one that holds something that looks like a credential. It reports whether the card was stopped,
// so its caller can hold the next queued message rather than carry on.
//
// It never returns an error: a card whose commits cannot be read is left alone and the reason is
// logged, because a Git failure must not stop the session the way a found secret does.
func (m *Manager) scanTurnForSecrets(ls *liveSession) bool {
	if ls.isChat() || m.cfg.Secrets == nil {
		return false
	}
	row, err := m.store.Queries().GetCard(m.ctx, ls.cardID)
	if err != nil {
		return false
	}
	if row.WorktreePath == "" || row.Branch == "" {
		// A card that never started has no branch and no commits of its own.
		return false
	}
	project, err := m.store.Queries().GetProject(m.ctx, row.ProjectID)
	if err != nil {
		m.log.Error("could not read a project to scan a card's commits", "card_id", ls.cardID, "error", err)
		return false
	}
	base := project.IntegrationBranch
	if base == "" {
		base = project.DefaultBranch
	}
	if base == "" {
		// Without a base there is no way to tell the agent's commits from the project's history.
		return false
	}
	commits, err := m.git.CommitsOnBranch(m.ctx, row.WorktreePath, base)
	if err != nil {
		m.log.Error("could not read a card's commits to scan them", "card_id", ls.cardID, "error", err)
		return false
	}
	for _, commit := range unscanned(commits, ls.scannedThrough) {
		ls.scannedThrough = commit.SHA
		finding, found, err := m.scanCommit(row.WorktreePath, commit)
		if err != nil {
			m.log.Error("could not scan a card's commit", "card_id", ls.cardID, "commit", commit.SHA, "error", err)
			continue
		}
		if found {
			m.blockCommitForSecret(ls, row, commit, finding)
			return true
		}
	}
	return false
}

// unscanned returns the commits after the last one already read. A session that has read nothing
// reads them all; a branch that was rewritten so the last one read is gone reads them all again,
// because a commit that cannot be found is not a commit that was read.
func unscanned(commits []gitx.Commit, through string) []gitx.Commit {
	if through == "" {
		return commits
	}
	for i, commit := range commits {
		if commit.SHA == through {
			return commits[i+1:]
		}
	}
	return commits
}

// scanCommit reads the files a commit changed, as they are in that commit, and returns the first
// credential it finds. A path the commit no longer holds, and a file that is not text, are left
// alone: there is nothing a person could read there.
func (m *Manager) scanCommit(dir string, commit gitx.Commit) (secrets.Finding, bool, error) {
	paths, err := m.git.ChangedPaths(m.ctx, dir, commit.SHA)
	if err != nil {
		return secrets.Finding{}, false, err
	}
	for _, path := range paths {
		content, err := m.git.FileAtCommit(m.ctx, dir, commit.SHA, path)
		if err != nil {
			continue
		}
		if strings.ContainsRune(content, 0) {
			// A binary file is not something a credential rule reads.
			continue
		}
		findings, err := m.cfg.Secrets.Scan(path, content)
		if err != nil {
			return secrets.Finding{}, false, err
		}
		if len(findings) > 0 {
			return findings[0], true, nil
		}
	}
	return secrets.Finding{}, false, nil
}

// blockCommitForSecret records what was found, moves the card to Needs you with the reason a person
// reads, and writes a system note in the card's own history. It never writes the credential: the
// rule, the file, and the line are what a person needs in order to look for themselves.
func (m *Manager) blockCommitForSecret(ls *liveSession, row db.Card, commit gitx.Commit, finding secrets.Finding) {
	short := commit.SHA
	if len(short) > 8 {
		short = short[:8]
	}
	m.audit.LogAndForget(m.ctx, audit.Entry{
		Actor: audit.ActorDaemon, Action: audit.ActionCommitBlocked, Target: commit.SHA,
		SessionID: ls.sessionRowID,
		Detail: map[string]any{
			cardIDDetailKey: row.ID, "commit": commit.SHA, "subject": commit.Subject,
			"rule": finding.Rule, "file": finding.File, "line": finding.Line,
		},
	})
	text := fmt.Sprintf("A commit looks like it holds a credential: %s in %s line %d.",
		finding.Rule, finding.File, finding.Line)
	reason := protocol.NeedsReason{Kind: protocol.NeedsReasonKindSecret, Text: text}
	if _, err := m.projects.SetNeeds(m.ctx, row.ID, reason); err != nil {
		m.log.Error("could not move a card to needs you after its commit held a credential", "card_id", row.ID, "error", err)
	}
	summary := fmt.Sprintf(
		"Marshal stopped this card: commit %s holds what looks like a credential (%s in %s, line %d). Nothing was rewritten; remove the value from the commit and resume the card.",
		short, finding.Rule, finding.File, finding.Line)
	m.appendRecords(ls, []history.Record{{Kind: history.KindSystem, Summary: summary}})
	m.log.Warn("stopped a card whose commit held a credential",
		"card_id", row.ID, "commit", commit.SHA, "rule", finding.Rule, "file", finding.File)
}
