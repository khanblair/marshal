package integrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// Progress is where a card's merge is.
type Progress struct {
	// Phase is the phase the merge is in. Empty means no merge is in progress.
	Phase protocol.MergePhase
	// Note is one plain sentence about the merge. A merge that stopped leaves its reason here.
	Note string
	// DoingNow is the card's "doing now" line.
	DoingNow string
}

// Settings are how a project's merges are run.
type Settings struct {
	// Paused is true while the owner has paused merging.
	Paused bool
	// AutoMerge is true when a card that becomes ready is merged without being asked.
	AutoMerge bool
	// PendingTip is the commit on the integrator branch that passed its tests and waits to be
	// delivered. Empty when nothing waits.
	PendingTip string
}

// CardRow is a card as the queue reads it.
type CardRow struct {
	ID        string
	ProjectID string
	// Key is the project and the number, such as "web#12".
	Key       string
	Title     string
	State     protocol.CardState
	Branch    string
	Phase     protocol.MergePhase
	Note      string
	NeedsText string
	UpdatedAt time.Time
}

// Delivery is one card the Integrator delivered, as the history keeps it.
type Delivery struct {
	ID        string
	ProjectID string
	CardID    string
	// CardKey and CardTitle are read with the history and never written.
	CardKey      string
	CardTitle    string
	Target       string
	MergedAt     time.Time
	Commit       string
	PrevTip      string
	Resolved     int
	Summary      string
	BackupBranch string
	WIPRef       string
	FolderTree   string
	// UndoneAt is zero until the delivery is undone.
	UndoneAt time.Time
}

// Ledger is where the queue keeps what it has to remember between merges: the progress it writes on
// cards, how each project is set, and what it delivered. The SQL ledger is the real one.
type Ledger interface {
	// SetProgress writes a card's merge phase, note, and "doing now" line.
	SetProgress(ctx context.Context, cardID string, p Progress) error
	// Settings reads a project's settings. A project with none has the defaults.
	Settings(ctx context.Context, projectID string) (Settings, error)
	// SetPaused pauses or resumes a project's merging.
	SetPaused(ctx context.Context, projectID string, paused bool) error
	// SetAutoMerge turns a project's automatic merging on or off.
	SetAutoMerge(ctx context.Context, projectID string, on bool) error
	// SetPendingTip records the tested commit that waits for delivery, or clears it with "".
	SetPendingTip(ctx context.Context, projectID, tip string) error
	// AddDelivery writes one delivery to the history.
	AddDelivery(ctx context.Context, d Delivery) error
	// Deliveries reads the newest deliveries of a project.
	Deliveries(ctx context.Context, projectID string, limit int) ([]Delivery, error)
	// LatestDelivery reads the newest delivery of a card.
	LatestDelivery(ctx context.Context, cardID string) (Delivery, bool, error)
	// MarkUndone marks a delivery undone. It reports false when it already was.
	MarkUndone(ctx context.Context, id string, at time.Time) (bool, error)
	// Queue reads the project's cards that wait for a merge or are being merged, the one being
	// merged first.
	Queue(ctx context.Context, projectID string) ([]CardRow, error)
	// Stalled reads the project's cards that a merge stopped, newest first.
	Stalled(ctx context.Context, projectID string) ([]CardRow, error)
	// Branches reads the project's cards that have a branch and are not done.
	Branches(ctx context.Context, projectID string) ([]CardRow, error)
	// Merging reads every card that is marked as being merged, in any project.
	Merging(ctx context.Context) ([]CardRow, error)
}

// SQLLedger is the Ledger over the database.
type SQLLedger struct {
	store *store.Store
	now   func() time.Time
}

var _ Ledger = (*SQLLedger)(nil)

// NewLedger builds the ledger over the open database.
func NewLedger(st *store.Store) *SQLLedger { return &SQLLedger{store: st, now: time.Now} }

// SetProgress writes a card's merge progress.
func (l *SQLLedger) SetProgress(ctx context.Context, cardID string, p Progress) error {
	return l.store.Write(ctx, func(q *db.Queries) error {
		n, err := q.SetCardMergeProgress(ctx, db.SetCardMergeProgressParams{
			MergePhase: string(p.Phase), MergeNote: p.Note, DoingNow: p.DoingNow, ID: cardID,
		})
		if err != nil {
			return fmt.Errorf("write the merge progress of card %s: %w", cardID, err)
		}
		if n == 0 {
			return fmt.Errorf("write the merge progress of card %s: no such card", cardID)
		}
		return nil
	})
}

// Settings reads a project's settings.
func (l *SQLLedger) Settings(ctx context.Context, projectID string) (Settings, error) {
	row, err := l.store.Queries().GetMergeSettings(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{AutoMerge: true}, nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("read the merge settings of project %s: %w", projectID, err)
	}
	return Settings{Paused: row.Paused != 0, AutoMerge: row.AutoMerge != 0, PendingTip: row.PendingTip}, nil
}

// SetPaused pauses or resumes a project's merging.
func (l *SQLLedger) SetPaused(ctx context.Context, projectID string, paused bool) error {
	return l.store.Write(ctx, func(q *db.Queries) error {
		return q.SetMergePaused(ctx, db.SetMergePausedParams{
			ProjectID: projectID, Paused: boolInt(paused), UpdatedAt: l.now().UnixMilli(),
		})
	})
}

// SetAutoMerge turns a project's automatic merging on or off.
func (l *SQLLedger) SetAutoMerge(ctx context.Context, projectID string, on bool) error {
	return l.store.Write(ctx, func(q *db.Queries) error {
		return q.SetMergeAutoMerge(ctx, db.SetMergeAutoMergeParams{
			ProjectID: projectID, AutoMerge: boolInt(on), UpdatedAt: l.now().UnixMilli(),
		})
	})
}

// SetPendingTip records the commit that waits for delivery.
func (l *SQLLedger) SetPendingTip(ctx context.Context, projectID, tip string) error {
	return l.store.Write(ctx, func(q *db.Queries) error {
		return q.SetMergePendingTip(ctx, db.SetMergePendingTipParams{
			ProjectID: projectID, PendingTip: tip, UpdatedAt: l.now().UnixMilli(),
		})
	})
}

// AddDelivery writes a delivery to the history.
func (l *SQLLedger) AddDelivery(ctx context.Context, d Delivery) error {
	return l.store.Write(ctx, func(q *db.Queries) error {
		return q.InsertMergeHistory(ctx, db.InsertMergeHistoryParams{
			ID: d.ID, ProjectID: d.ProjectID, CardID: d.CardID, Target: d.Target,
			MergedAt: d.MergedAt.UnixMilli(), CommitSha: d.Commit, PrevTip: d.PrevTip,
			Resolved: int64(d.Resolved), Summary: d.Summary, BackupBranch: d.BackupBranch,
			WipRef: d.WIPRef, FolderTree: d.FolderTree,
		})
	})
}

// Deliveries reads the newest deliveries of a project.
func (l *SQLLedger) Deliveries(ctx context.Context, projectID string, limit int) ([]Delivery, error) {
	rows, err := l.store.Queries().ListMergeHistory(ctx, db.ListMergeHistoryParams{ProjectID: projectID, Limit: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("read the merge history of project %s: %w", projectID, err)
	}
	out := make([]Delivery, 0, len(rows))
	for _, row := range rows {
		out = append(out, Delivery{
			ID: row.ID, ProjectID: row.ProjectID, CardID: row.CardID,
			CardKey: cardKey(row.ProjectID, row.CardNumber), CardTitle: row.CardTitle,
			Target: row.Target, MergedAt: time.UnixMilli(row.MergedAt).UTC(), Commit: row.CommitSha,
			PrevTip: row.PrevTip, Resolved: int(row.Resolved), Summary: row.Summary,
			BackupBranch: row.BackupBranch, WIPRef: row.WipRef, FolderTree: row.FolderTree,
			UndoneAt: optionalTime(row.UndoneAt),
		})
	}
	return out, nil
}

// LatestDelivery reads the newest delivery of a card.
func (l *SQLLedger) LatestDelivery(ctx context.Context, cardID string) (Delivery, bool, error) {
	row, err := l.store.Queries().GetLatestMergeHistoryForCard(ctx, cardID)
	if errors.Is(err, sql.ErrNoRows) {
		return Delivery{}, false, nil
	}
	if err != nil {
		return Delivery{}, false, fmt.Errorf("read the newest delivery of card %s: %w", cardID, err)
	}
	return Delivery{
		ID: row.ID, ProjectID: row.ProjectID, CardID: row.CardID, Target: row.Target,
		MergedAt: time.UnixMilli(row.MergedAt).UTC(), Commit: row.CommitSha, PrevTip: row.PrevTip,
		Resolved: int(row.Resolved), Summary: row.Summary, BackupBranch: row.BackupBranch,
		WIPRef: row.WipRef, FolderTree: row.FolderTree, UndoneAt: optionalTime(row.UndoneAt),
	}, true, nil
}

// MarkUndone marks a delivery undone.
func (l *SQLLedger) MarkUndone(ctx context.Context, id string, at time.Time) (bool, error) {
	var changed int64
	err := l.store.Write(ctx, func(q *db.Queries) error {
		n, err := q.MarkMergeHistoryUndone(ctx, db.MarkMergeHistoryUndoneParams{UndoneAt: at.UnixMilli(), ID: id})
		changed = n
		return err
	})
	return changed > 0, err
}

// Queue reads the cards that wait for a merge or are being merged.
func (l *SQLLedger) Queue(ctx context.Context, projectID string) ([]CardRow, error) {
	rows, err := l.store.Queries().ListMergeQueue(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("read the merge queue of project %s: %w", projectID, err)
	}
	out := make([]CardRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, CardRow{
			ID: r.ID, ProjectID: r.ProjectID, Key: cardKey(r.ProjectID, r.Number), Title: r.Title,
			State: protocol.CardState(r.State), Branch: r.Branch, Phase: protocol.MergePhase(r.MergePhase),
			Note: r.MergeNote, UpdatedAt: time.UnixMilli(r.UpdatedAt).UTC(),
		})
	}
	return out, nil
}

// Stalled reads the cards that a merge stopped.
func (l *SQLLedger) Stalled(ctx context.Context, projectID string) ([]CardRow, error) {
	rows, err := l.store.Queries().ListMergeStalled(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("read the cards a merge stopped in project %s: %w", projectID, err)
	}
	out := make([]CardRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, CardRow{
			ID: r.ID, ProjectID: r.ProjectID, Key: cardKey(r.ProjectID, r.Number), Title: r.Title,
			State: protocol.CardState(r.State), Branch: r.Branch, Phase: protocol.MergePhase(r.MergePhase),
			Note: r.MergeNote, NeedsText: r.NeedsReasonText, UpdatedAt: time.UnixMilli(r.UpdatedAt).UTC(),
		})
	}
	return out, nil
}

// Branches reads the cards of a project that have a branch and are not done.
func (l *SQLLedger) Branches(ctx context.Context, projectID string) ([]CardRow, error) {
	rows, err := l.store.Queries().ListMergeBranches(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("read the card branches of project %s: %w", projectID, err)
	}
	out := make([]CardRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, CardRow{
			ID: r.ID, ProjectID: projectID, State: protocol.CardState(r.State), Branch: r.Branch,
			Phase: protocol.MergePhase(r.MergePhase),
		})
	}
	return out, nil
}

// Merging reads every card that is marked as being merged.
func (l *SQLLedger) Merging(ctx context.Context) ([]CardRow, error) {
	rows, err := l.store.Queries().ListMergingCards(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the cards marked as merging: %w", err)
	}
	out := make([]CardRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, CardRow{ID: r.ID, ProjectID: r.ProjectID, State: protocol.CardStateMerging})
	}
	return out, nil
}

// cardKey is the project and the number, the way a person reads a card.
func cardKey(projectID string, number int64) string { return fmt.Sprintf("%s#%d", projectID, number) }

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

// optionalTime reads a stored time where 0 means "not set".
func optionalTime(millis int64) time.Time {
	if millis == 0 {
		return time.Time{}
	}
	return time.UnixMilli(millis).UTC()
}

// memoryLedger is the Ledger a queue has when it is given none: settings and history in memory,
// and no cards to read, so it writes no progress and its queue is empty.
type memoryLedger struct {
	mu         sync.Mutex
	settings   map[string]Settings
	deliveries []Delivery
}

func newMemoryLedger() *memoryLedger { return &memoryLedger{settings: map[string]Settings{}} }

func (m *memoryLedger) SetProgress(context.Context, string, Progress) error { return nil }

func (m *memoryLedger) Settings(_ context.Context, projectID string) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settingsLocked(projectID), nil
}

func (m *memoryLedger) settingsLocked(projectID string) Settings {
	if s, ok := m.settings[projectID]; ok {
		return s
	}
	return Settings{AutoMerge: true}
}

func (m *memoryLedger) update(projectID string, change func(*Settings)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.settingsLocked(projectID)
	change(&s)
	m.settings[projectID] = s
	return nil
}

func (m *memoryLedger) SetPaused(_ context.Context, projectID string, paused bool) error {
	return m.update(projectID, func(s *Settings) { s.Paused = paused })
}

func (m *memoryLedger) SetAutoMerge(_ context.Context, projectID string, on bool) error {
	return m.update(projectID, func(s *Settings) { s.AutoMerge = on })
}

func (m *memoryLedger) SetPendingTip(_ context.Context, projectID, tip string) error {
	return m.update(projectID, func(s *Settings) { s.PendingTip = tip })
}

func (m *memoryLedger) AddDelivery(_ context.Context, d Delivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deliveries = append(m.deliveries, d)
	return nil
}

func (m *memoryLedger) Deliveries(_ context.Context, projectID string, limit int) ([]Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Delivery
	for i := len(m.deliveries) - 1; i >= 0 && len(out) < limit; i-- {
		if m.deliveries[i].ProjectID == projectID {
			out = append(out, m.deliveries[i])
		}
	}
	return out, nil
}

func (m *memoryLedger) LatestDelivery(_ context.Context, cardID string) (Delivery, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.deliveries) - 1; i >= 0; i-- {
		if m.deliveries[i].CardID == cardID {
			return m.deliveries[i], true, nil
		}
	}
	return Delivery{}, false, nil
}

func (m *memoryLedger) MarkUndone(_ context.Context, id string, at time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.deliveries {
		if m.deliveries[i].ID == id && m.deliveries[i].UndoneAt.IsZero() {
			m.deliveries[i].UndoneAt = at
			return true, nil
		}
	}
	return false, nil
}

func (m *memoryLedger) Queue(context.Context, string) ([]CardRow, error)    { return nil, nil }
func (m *memoryLedger) Stalled(context.Context, string) ([]CardRow, error)  { return nil, nil }
func (m *memoryLedger) Branches(context.Context, string) ([]CardRow, error) { return nil, nil }
func (m *memoryLedger) Merging(context.Context) ([]CardRow, error)          { return nil, nil }
