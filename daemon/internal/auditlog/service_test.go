package auditlog_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/auditlog"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// newStore opens a fresh database for one test.
func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})
	return st
}

// auditWrite is the part of one seeded row that is not its actor or its time - bundled so seed
// stays under this codebase's argument limit.
type auditWrite struct {
	action, target, detailJSON string
}

// seed writes one audit row, at, and returns its id.
func seed(t *testing.T, st *store.Store, at int64, actor string, w auditWrite) string {
	t.Helper()
	id, err := protocol.NewID(time.UnixMilli(at).UTC(), rand.Reader)
	if err != nil {
		t.Fatalf("make an id: %v", err)
	}
	detailJSON := w.detailJSON
	if detailJSON == "" {
		detailJSON = "{}"
	}
	if err := st.Write(context.Background(), func(q *db.Queries) error {
		return q.InsertAuditLog(context.Background(), db.InsertAuditLogParams{
			ID: id, SessionID: "ses_test", Actor: actor, Action: w.action, Target: w.target,
			DetailJSON: detailJSON, CreatedAt: at,
		})
	}); err != nil {
		t.Fatalf("seed an audit row: %v", err)
	}
	return id
}

func TestNewRefusesAMissingStore(t *testing.T) {
	if _, err := auditlog.New(auditlog.Deps{}); err == nil {
		t.Fatal("New with no store = nil error, want one")
	}
}

func TestEntriesReadsNewestFirst(t *testing.T) {
	st := newStore(t)
	older := seed(t, st, 1_000, "person", auditWrite{action: "approve", target: "appr_old", detailJSON: ""})
	newest := seed(t, st, 3_000, "daemon", auditWrite{action: "commit.blocked", target: "sha_new", detailJSON: `{"file":"deploy/settings.yaml"}`})
	middle := seed(t, st, 2_000, "person", auditWrite{action: "bypass.on", target: "crd_mid", detailJSON: ""})

	svc, err := auditlog.New(auditlog.Deps{Store: st})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	page, err := svc.Entries(context.Background(), "", auditlog.Cursor{}, 0)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(page.Items) != 3 || page.More {
		t.Fatalf("page = %+v, want 3 items and no more", page)
	}
	want := []string{newest, middle, older}
	for i, entry := range page.Items {
		if entry.ID != want[i] {
			t.Errorf("entry %d = %s, want %s", i, entry.ID, want[i])
		}
	}
	if page.Items[0].Detail["file"] != "deploy/settings.yaml" {
		t.Errorf("the detail did not come back: %+v", page.Items[0].Detail)
	}
}

func TestEntriesPagesWithoutRepeatingARow(t *testing.T) {
	st := newStore(t)
	seed(t, st, 1_000, "person", auditWrite{action: "approve", target: "a", detailJSON: ""})
	seed(t, st, 2_000, "person", auditWrite{action: "approve", target: "b", detailJSON: ""})
	seed(t, st, 3_000, "person", auditWrite{action: "approve", target: "c", detailJSON: ""})

	svc, err := auditlog.New(auditlog.Deps{Store: st})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	seen := map[string]bool{}
	cursor := auditlog.Cursor{}
	for i := 0; i < 5; i++ {
		page, err := svc.Entries(context.Background(), "", cursor, 1)
		if err != nil {
			t.Fatalf("Entries: %v", err)
		}
		if len(page.Items) != 1 {
			t.Fatalf("page %d has %d items, want 1", i, len(page.Items))
		}
		if seen[page.Items[0].ID] {
			t.Fatalf("page %d repeated a row: %s", i, page.Items[0].ID)
		}
		seen[page.Items[0].ID] = true
		if !page.More {
			break
		}
		cursor = page.Next
	}
	if len(seen) != 3 {
		t.Errorf("walking the pages found %d rows, want 3", len(seen))
	}
}

func TestEntriesFiltersByQuery(t *testing.T) {
	st := newStore(t)
	seed(t, st, 1_000, "person", auditWrite{action: "approve", target: "appr_1", detailJSON: ""})
	seed(t, st, 2_000, "daemon", auditWrite{action: "commit.blocked", target: "sha_1", detailJSON: `{"file":"deploy/settings.yaml"}`})

	svc, err := auditlog.New(auditlog.Deps{Store: st})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	page, err := svc.Entries(context.Background(), "commit.blocked", auditlog.Cursor{}, 0)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Action != "commit.blocked" {
		t.Errorf("filtered page = %+v", page.Items)
	}
}

func TestExportCarriesEverythingAQueryMatched(t *testing.T) {
	st := newStore(t)
	seed(t, st, 1_000, "person", auditWrite{action: "approve", target: "appr_1", detailJSON: ""})
	seed(t, st, 2_000, "daemon", auditWrite{action: "commit.blocked", target: "sha_1", detailJSON: ""})

	svc, err := auditlog.New(auditlog.Deps{Store: st}, auditlog.WithClock(func() time.Time { return time.UnixMilli(9_000).UTC() }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	all, err := svc.Export(context.Background(), "")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if all.Total != 2 || len(all.Entries) != 2 {
		t.Errorf("export = %+v, want 2 entries", all)
	}
	if all.ServerTime.Time().UnixMilli() != 9_000 {
		t.Errorf("server time = %v, want the injected clock", all.ServerTime)
	}

	searched, err := svc.Export(context.Background(), "commit.blocked")
	if err != nil {
		t.Fatalf("Export with a query: %v", err)
	}
	if searched.Query != "commit.blocked" || searched.Total != 1 {
		t.Errorf("searched export = %+v", searched)
	}
}

func TestEntryOfLeavesOutADetailThatIsNotUsableJSON(t *testing.T) {
	st := newStore(t)
	seed(t, st, 1_000, "daemon", auditWrite{action: "commit.blocked", target: "sha_1", detailJSON: "not json"})
	seed(t, st, 2_000, "daemon", auditWrite{action: "commit.blocked", target: "sha_2", detailJSON: "{}"})

	svc, err := auditlog.New(auditlog.Deps{Store: st})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	page, err := svc.Entries(context.Background(), "", auditlog.Cursor{}, 0)
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	for _, entry := range page.Items {
		if entry.Detail != nil {
			t.Errorf("entry %s has a detail %v, want none for unusable or empty JSON", entry.ID, entry.Detail)
		}
	}
	// The round trip through JSON marshaling must still succeed with Detail left as nil.
	if _, err := json.Marshal(page.Items); err != nil {
		t.Errorf("marshal the page: %v", err)
	}
}
