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

// toCard builds the wire card from a row and the labels on it. The worktree folder stays server
// side: it is a path on the daemon's machine, and clients have no use for it.
//
// Labels is never nil, so a card with no labels sends [] and never null: a list on the wire is
// never null, the same rule the agent catalog and a page of results follow.
func toCard(row db.Card, labels []protocol.Label) protocol.Card {
	if labels == nil {
		labels = []protocol.Label{}
	}
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
		Role:           row.Role,
		Labels:         labels,
		// A card is in the chat view until the session module says its session is in the terminal
		// (withSession), so a card that never had a session, and a card built without one, agree.
		ViewMode:     protocol.CardViewModeChat,
		Package:      row.Package,
		PlannedStart: timePointer(row.PlannedStart),
		PlannedEnd:   timePointer(row.PlannedEnd),
		Due:          timePointer(row.Due),
		ActualStart:  timePointer(row.ActualStart),
		ActualEnd:    timePointer(row.ActualEnd),
		PullRequest:  pullRequestOf(row),
		CI:           ciOf(row),
		ContextUsed:  int(row.ContextUsed),
		NeedsReason:  needsReasonOf(row),
		DoingNow:     row.DoingNow,
		Paused:       boolFromInt(row.Paused),
		Pinned:       boolFromInt(row.Pinned),
		Branch:       row.Branch,
		CreatedAt:    store.Timestamp(row.CreatedAt),
		UpdatedAt:    store.Timestamp(row.UpdatedAt),
	}
	if row.Thinking != "" {
		thinking := protocol.ThinkingMode(row.Thinking)
		card.Thinking = &thinking
	}
	return card
}

// timePointer turns a nullable millisecond column into the wire's nullable timestamp.
func timePointer(ms *int64) *protocol.Timestamp {
	if ms == nil {
		return nil
	}
	ts := store.Timestamp(*ms)
	return &ts
}

// pullRequestOf is the card's pull request, or nil when the card has none. A number without an
// address is still a pull request: the address is only known once a forge integration exists.
func pullRequestOf(row db.Card) *protocol.PullRequest {
	if row.PullRequestNumber == nil && row.PullRequestUrl == "" {
		return nil
	}
	pr := protocol.PullRequest{URL: row.PullRequestUrl}
	if row.PullRequestNumber != nil {
		pr.Number = int(*row.PullRequestNumber)
	}
	return &pr
}

// ciOf is the card's CI state, or nil when there is no CI data. An empty column means "no data",
// never a made-up state (the design-port deviations list).
func ciOf(row db.Card) *protocol.CIState {
	if row.CiState == "" {
		return nil
	}
	state := protocol.CIState(row.CiState)
	return &state
}

// needsReasonOf is why the card waits on a person, or nil when it does not.
func needsReasonOf(row db.Card) *protocol.NeedsReason {
	if row.NeedsReasonKind == "" {
		return nil
	}
	return &protocol.NeedsReason{
		Kind: protocol.NeedsReasonKind(row.NeedsReasonKind),
		Text: row.NeedsReasonText,
	}
}

// toCards builds the wire cards of one project with the labels of each. labelsByCard groups what
// ListCardLabelsByProject returned, so one query serves the whole board.
func toCards(rows []db.Card, labelsByCard map[string][]protocol.Label) []protocol.Card {
	cards := make([]protocol.Card, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, toCard(row, labelsByCard[row.ID]))
	}
	return cards
}

// groupLabelsByCard turns the rows of ListCardLabelsByProject into the labels of each card.
func groupLabelsByCard(rows []db.ListCardLabelsByProjectRow) map[string][]protocol.Label {
	byCard := make(map[string][]protocol.Label, len(rows))
	for _, row := range rows {
		byCard[row.CardID] = append(byCard[row.CardID], toLabel(db.Label{
			ID: row.LabelID, ProjectID: row.ProjectID, Name: row.Name,
			Color: row.Color, CreatedAt: row.CreatedAt,
		}))
	}
	return byCard
}

// toLabel builds the wire label from a row.
func toLabel(row db.Label) protocol.Label {
	return protocol.Label{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		Name:      row.Name,
		Color:     protocol.LabelColor(row.Color),
		CreatedAt: store.Timestamp(row.CreatedAt),
	}
}

// toLabels builds the wire labels of a project, by name.
func toLabels(rows []db.Label) []protocol.Label {
	labels := make([]protocol.Label, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, toLabel(row))
	}
	return labels
}
