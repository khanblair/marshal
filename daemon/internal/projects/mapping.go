package projects

import (
	"encoding/json"
	"fmt"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The database keeps a yes or no as 0 or 1, because that is how SQLite holds it.
func boolFromInt(v int64) bool { return v != 0 }

func intFromBool(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

// toProject builds the wire project from a row and its badge counts. Packages is never nil, so it
// is [] on the wire and never null.
func toProject(row db.Project, badges protocol.ProjectBadges) (protocol.Project, error) {
	packages := []string{}
	if err := json.Unmarshal([]byte(row.PackagesJSON), &packages); err != nil {
		return protocol.Project{}, fmt.Errorf("read the packages of project %s: %w", row.ID, err)
	}
	if packages == nil {
		packages = []string{}
	}
	return protocol.Project{
		ID:            row.ID,
		Name:          row.Name,
		Path:          row.RepoPath,
		Language:      row.Language,
		DefaultBranch: row.DefaultBranch,
		DevCommand:    row.DevCommand,
		BypassLocked:  boolFromInt(row.BypassLocked),
		IsMonorepo:    boolFromInt(row.IsMonorepo),
		Packages:      packages,
		CreatedAt:     store.Timestamp(row.CreatedAt),
		Badges:        badges,
	}, nil
}

// toCard builds the wire card from a row. The worktree folder stays server side: it is a path on
// the daemon's machine, and clients have no use for it.
func toCard(row db.Card) protocol.Card {
	card := protocol.Card{
		ID:             row.ID,
		ProjectID:      row.ProjectID,
		Number:         int(row.Number),
		Key:            protocol.CardKey{ProjectID: row.ProjectID, Number: int(row.Number)}.String(),
		Title:          row.Title,
		Body:           row.Body,
		State:          protocol.CardState(row.State),
		Agent:          protocol.AgentKind(row.AgentKind),
		Model:          row.Model,
		PermissionMode: protocol.PermissionMode(row.PermissionMode),
		Branch:         row.Branch,
		CreatedAt:      store.Timestamp(row.CreatedAt),
		UpdatedAt:      store.Timestamp(row.UpdatedAt),
	}
	if row.Thinking != "" {
		thinking := protocol.ThinkingMode(row.Thinking)
		card.Thinking = &thinking
	}
	return card
}

func toCards(rows []db.Card) []protocol.Card {
	cards := make([]protocol.Card, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, toCard(row))
	}
	return cards
}
