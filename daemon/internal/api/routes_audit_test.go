package api_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// The audit log's read routes (B3.5): the list, the search, and the export, all read only. The rows
// are written straight into the store here, because how a row is written is internal/audit's job
// and is covered where it happens; what these tests pin down is what a person can read back.

// seedAudit writes one row and returns its id. The time is passed in so the order a page comes back
// in is the order the rows were written, not the order a machine happened to make them.
func seedAudit(t *testing.T, st *stack, at int64, actor, action, target string, detail map[string]any) string {
	t.Helper()
	id, err := protocol.NewID(time.UnixMilli(at).UTC(), rand.Reader)
	if err != nil {
		t.Fatalf("make an id: %v", err)
	}
	detailJSON := "{}"
	if len(detail) > 0 {
		encoded, err := json.Marshal(detail)
		if err != nil {
			t.Fatalf("encode the detail: %v", err)
		}
		detailJSON = string(encoded)
	}
	if err := st.store.Write(context.Background(), func(q *db.Queries) error {
		return q.InsertAuditLog(context.Background(), db.InsertAuditLogParams{
			ID: id, SessionID: "ses_test", Actor: actor, Action: action, Target: target,
			DetailJSON: detailJSON, CreatedAt: at,
		})
	}); err != nil {
		t.Fatalf("write an audit row: %v", err)
	}
	return id
}

// secretDetail is the detail a scanner block records, which is also the detail the golden file
// carries, so the shape check on the list and the export compares like with like.
func secretDetail() map[string]any {
	return map[string]any{
		"cardId": "crd_1", "commit": "sha_1", "file": "deploy/settings.yaml",
		"line": 3, "rule": "aws-access-token",
	}
}

func TestTheAuditListReadsNewestFirst(t *testing.T) {
	st := newStack(t)
	older := seedAudit(t, st, 1_000, "person", "approve", "appr_old", nil)
	newest := seedAudit(t, st, 3_000, "daemon", "commit.blocked", "sha_new", secretDetail())
	middle := seedAudit(t, st, 2_000, "person", "bypass.on", "crd_mid", nil)

	got := st.do(http.MethodGet, "/v1/audit", nil).want(t, http.StatusOK)
	page := decode[protocol.Page[protocol.AuditEntry]](t, got)
	if len(page.Items) != 3 {
		t.Fatalf("the list has %d entries, want 3: %s", len(page.Items), got.Body)
	}
	wantOrder := []string{newest, middle, older}
	for i, entry := range page.Items {
		if entry.ID != wantOrder[i] {
			t.Errorf("entry %d is %s, want %s", i, entry.ID, wantOrder[i])
		}
	}
	if page.NextCursor != "" {
		t.Errorf("a list that fits one page still offers another: %q", page.NextCursor)
	}
	if page.Items[0].Action != "commit.blocked" || page.Items[0].Actor != "daemon" {
		t.Errorf("the newest entry = %+v", page.Items[0])
	}
	if page.Items[0].Detail["rule"] != "aws-access-token" {
		t.Errorf("the detail did not come back whole: %+v", page.Items[0].Detail)
	}
	if page.Items[0].At.Time().UnixMilli() != 3_000 {
		t.Errorf("the time = %v, want 3000ms", page.Items[0].At)
	}
	sameShape(t, "audit-entry-list", got.Body)
}

func TestTheAuditListPagesWithoutRepeatingARow(t *testing.T) {
	st := newStack(t)
	seedAudit(t, st, 1_000, "person", "approve", "a", nil)
	seedAudit(t, st, 2_000, "person", "approve", "b", nil)
	seedAudit(t, st, 3_000, "person", "approve", "c", nil)

	seen := map[string]bool{}
	path := "/v1/audit?limit=1"
	for page := 0; page < 5; page++ {
		got := st.do(http.MethodGet, path, nil).want(t, http.StatusOK)
		answer := decode[protocol.Page[protocol.AuditEntry]](t, got)
		if len(answer.Items) != 1 {
			t.Fatalf("page %d has %d items, want 1: %s", page, len(answer.Items), got.Body)
		}
		if seen[answer.Items[0].ID] {
			t.Fatalf("page %d repeated a row: %s", page, answer.Items[0].ID)
		}
		seen[answer.Items[0].ID] = true
		if answer.NextCursor == "" {
			break
		}
		path = "/v1/audit?limit=1&cursor=" + url.QueryEscape(answer.NextCursor)
	}
	if len(seen) != 3 {
		t.Errorf("walking the pages found %d rows, want 3", len(seen))
	}
}

func TestTheAuditSearchNarrowsTheListAndNeedsAQuery(t *testing.T) {
	st := newStack(t)
	seedAudit(t, st, 1_000, "person", "approve", "appr_1", nil)
	seedAudit(t, st, 2_000, "daemon", "commit.blocked", "sha_1", map[string]any{"file": "deploy/settings.yaml"})
	seedAudit(t, st, 3_000, "person", "bypass.on", "crd_1", nil)

	// A search with nothing to look for is refused rather than quietly listing everything.
	got := st.do(http.MethodGet, "/v1/audit/search", nil)
	if got.Status != http.StatusBadRequest {
		t.Errorf("an empty search answered %d, want 400: %s", got.Status, got.Body)
	}

	// The word is looked for across the actor, the action, the target, and the recorded detail.
	for _, tt := range []struct {
		query string
		want  int
	}{
		{"commit.blocked", 1},
		{"daemon", 1},
		{"settings.yaml", 1},
		{"approve", 1},
		{"nothing-like-this", 0},
	} {
		got := st.do(http.MethodGet, "/v1/audit/search?q="+url.QueryEscape(tt.query), nil).want(t, http.StatusOK)
		page := decode[protocol.Page[protocol.AuditEntry]](t, got)
		if len(page.Items) != tt.want {
			t.Errorf("searching %q found %d rows, want %d: %s", tt.query, len(page.Items), tt.want, got.Body)
		}
	}

	// The list takes the same query, so it narrows without a second address.
	got = st.do(http.MethodGet, "/v1/audit?q=bypass.on", nil).want(t, http.StatusOK)
	page := decode[protocol.Page[protocol.AuditEntry]](t, got)
	if len(page.Items) != 1 || page.Items[0].Action != "bypass.on" {
		t.Errorf("the list with a query = %s", got.Body)
	}
}

func TestTheAuditExportCarriesEverythingTheQueryMatched(t *testing.T) {
	st := newStack(t)
	seedAudit(t, st, 1_000, "person", "approve", "appr_1", nil)
	seedAudit(t, st, 2_000, "daemon", "commit.blocked", "sha_1", secretDetail())

	got := st.do(http.MethodGet, "/v1/audit/export", nil).want(t, http.StatusOK)
	export := decode[protocol.AuditExport](t, got)
	if export.Total != 2 || len(export.Entries) != 2 {
		t.Errorf("the export holds %d entries (total %d), want 2: %s", len(export.Entries), export.Total, got.Body)
	}

	got = st.do(http.MethodGet, "/v1/audit/export?q=commit.blocked", nil).want(t, http.StatusOK)
	export = decode[protocol.AuditExport](t, got)
	if export.Query != "commit.blocked" || export.Total != 1 {
		t.Errorf("a searched export = %+v", export)
	}
	sameShape(t, "audit-export", got.Body)

	got = st.do(http.MethodGet, "/v1/audit/export?format=xml", nil)
	if got.Status != http.StatusBadRequest {
		t.Errorf("an export in an unknown format answered %d, want 400: %s", got.Status, got.Body)
	}
}

func TestTheAuditExportAsCSVIsADownload(t *testing.T) {
	st := newStack(t)
	seedAudit(t, st, 1_000, "person", "approve", "appr_1", nil)

	got := st.do(http.MethodGet, "/v1/audit/export?format=csv", nil).want(t, http.StatusOK)
	if ct := got.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
	if cd := got.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment", cd)
	}
	body := string(got.Body)
	if !strings.HasPrefix(body, "id,at,actor,action,target,sessionId,detail\n") {
		t.Errorf("the CSV header is not the one advertised:\n%s", body)
	}
	if !strings.Contains(body, "person,approve,appr_1") {
		t.Errorf("the row did not come through:\n%s", body)
	}
	if value := csvText(t, body, "appr_1"); value != "appr_1" {
		t.Errorf("the target cell = %q", value)
	}
}

// TestTheAuditExportDoesNotHandASpreadsheetAFormula covers the one way untrusted text becomes
// dangerous on its way out: a cell that begins with `=` is a formula to Excel and Sheets, and an
// audit entry's target is an agent's own words. The export prefixes such a cell with an apostrophe.
func TestTheAuditExportDoesNotHandASpreadsheetAFormula(t *testing.T) {
	st := newStack(t)
	seedAudit(t, st, 1_000, "person", "approve", "=cmd|'/C calc'!A0", nil)

	got := st.do(http.MethodGet, "/v1/audit/export?format=csv", nil).want(t, http.StatusOK)
	if cell := csvText(t, string(got.Body), "=cmd"); cell != "'=cmd|'/C calc'!A0" {
		t.Errorf("the formula cell = %q, want it prefixed with an apostrophe", cell)
	}
}

// csvText finds the cell that contains `needle` in a decoded CSV body, for a readable assertion.
func csvText(t *testing.T, body, needle string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, needle) {
			continue
		}
		var row []string
		if err := json.Unmarshal([]byte("["+quoteCSV(line)+"]"), &row); err != nil {
			continue
		}
		for _, cell := range row {
			if strings.Contains(cell, needle) {
				return cell
			}
		}
	}
	return ""
}

// quoteCSV turns one CSV line into a JSON-ish list of its cells, so the test reads cells rather
// than substrings and the assertion cannot pass on a prefix by accident.
func quoteCSV(line string) string {
	cells := strings.Split(line, ",")
	quoted := make([]string, 0, len(cells))
	for _, cell := range cells {
		cell = strings.ReplaceAll(cell, `"`, `\"`)
		quoted = append(quoted, `"`+cell+`"`)
	}
	return strings.Join(quoted, ",")
}
