package audit_test

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// testTime is the clock every test gives the recorder, so a stored time is exact.
var testTime = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

// testClock is the recorder's clock, which a test moves by hand: two rows written in the same
// millisecond would sort by their random ids, so a test that cares about order moves it on.
type testClock struct{ at time.Time }

func (c *testClock) now() time.Time { return c.at }

// newRecorder opens a store and an audit recorder on it, with a clock the test holds.
func newRecorder(t *testing.T, log *slog.Logger) (*store.Store, *audit.Recorder, *testClock) {
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
	clock := &testClock{at: testTime}
	rec, err := audit.New(st, clock.now, nil, log)
	if err != nil {
		t.Fatalf("audit.New: %v", err)
	}
	return st, rec, clock
}

func TestRecordWritesTheRow(t *testing.T) {
	st, rec, clock := newRecorder(t, nil)
	ctx := context.Background()

	err := rec.Record(ctx, audit.Entry{
		Actor: audit.ActorPerson, Action: audit.ActionBypassOn, Target: "card-1",
		Detail: map[string]any{"changed": true, "permissionMode": "bypass"},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	rows, err := st.Queries().ListAuditLog(ctx, 10)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("the audit log = %+v, want one row", rows)
	}
	row := rows[0]
	if row.Actor != audit.ActorPerson || row.Action != audit.ActionBypassOn || row.Target != "card-1" {
		t.Errorf("the row = %+v, want the action that was recorded", row)
	}
	if row.SessionID != "" {
		t.Errorf("session_id = %q, want empty for an action that belongs to no session", row.SessionID)
	}
	if row.CreatedAt != testTime.UnixMilli() {
		t.Errorf("created_at = %d, want %d", row.CreatedAt, testTime.UnixMilli())
	}
	// The detail is stored as JSON, so a reader gets back what the writer meant.
	if !strings.Contains(row.DetailJSON, `"changed":true`) ||
		!strings.Contains(row.DetailJSON, `"permissionMode":"bypass"`) {
		t.Errorf("detail_json = %s, want the payload as JSON", row.DetailJSON)
	}

	// An entry with no detail is a row with an empty detail, not a broken one.
	clock.at = testTime.Add(time.Second)
	if err := rec.Record(ctx, audit.Entry{Actor: audit.ActorDaemon, Action: audit.ActionDeny}); err != nil {
		t.Fatalf("Record with no detail: %v", err)
	}
	rows, err = st.Queries().ListAuditLog(ctx, 10)
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	if len(rows) != 2 || rows[0].DetailJSON != "" || rows[0].Action != audit.ActionDeny {
		t.Errorf("the audit log = %+v, want the later row with no detail first", rows)
	}
}

// TestRecordRefusesDetailItCannotEncode keeps the trail honest: a row whose payload cannot be
// written is not written at all, rather than stored without the part that explains it.
func TestRecordRefusesDetailItCannotEncode(t *testing.T) {
	st, rec, _ := newRecorder(t, nil)
	ctx := context.Background()

	err := rec.Record(ctx, audit.Entry{
		Actor: audit.ActorPerson, Action: audit.ActionBypassOn, Detail: map[string]any{"bad": func() {}},
	})
	if err == nil {
		t.Fatal("a detail that cannot be encoded was recorded anyway")
	}
	if rows, err := st.Queries().ListAuditLog(ctx, 10); err != nil || len(rows) != 0 {
		t.Errorf("the audit log = %+v (%v), want nothing written", rows, err)
	}
}

// TestLogAndForgetSwallowsAFailure is the other half of Record's contract: the paths that cannot
// return an error (the pump reacting to an agent) must go on, with the failure in the log.
func TestLogAndForgetSwallowsAFailure(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	st, rec, _ := newRecorder(t, log)

	if err := st.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}
	rec.LogAndForget(context.Background(), audit.Entry{Actor: audit.ActorDaemon, Action: audit.ActionApprove, Target: "call-1"})

	if text := buf.String(); !strings.Contains(text, audit.ActionApprove) {
		t.Errorf("the log = %q, want the action that could not be written", text)
	}
}

func TestNewNeedsAStore(t *testing.T) {
	if _, err := audit.New(nil, nil, nil, nil); err == nil {
		t.Error("a recorder was built with no store")
	}
}
