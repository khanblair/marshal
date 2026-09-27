package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The audit-log wire shapes (docs/backend-checklist.md B3.5). One entry, one paged list of them,
// and one export, each with a golden file the web client's mapper tests read too.

func auditNow() time.Time {
	return time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
}

func sampleAuditEntry() protocol.AuditEntry {
	return protocol.AuditEntry{
		ID:        "aud_01JQZ0000000000000000000AA",
		SessionID: "ses_01JQZ0000000000000000000AB",
		Actor:     "daemon",
		Action:    "commit.blocked",
		Target:    "9f1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c",
		Detail: map[string]any{
			"cardId": "crd_01JQZ0000000000000000000AC",
			"commit": "9f1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c",
			"file":   "deploy/settings.yaml",
			"line":   float64(3),
			"rule":   "aws-access-token",
		},
		At: protocol.NewTimestamp(auditNow()),
	}
}

func TestAuditEntryGolden(t *testing.T) {
	testutil.Golden(t, "audit-entry", sampleAuditEntry())
}

func TestAuditEntryListGolden(t *testing.T) {
	page := protocol.NewPage([]protocol.AuditEntry{sampleAuditEntry()},
		"eyJyb3dpZCI6MTJ9", auditNow())
	testutil.Golden(t, "audit-entry-list", page)
}

func TestAuditExportGolden(t *testing.T) {
	testutil.Golden(t, "audit-export", protocol.AuditExport{
		Query:      "commit.blocked",
		Entries:    []protocol.AuditEntry{sampleAuditEntry()},
		Total:      1,
		ServerTime: protocol.NewTimestamp(auditNow()),
	})
}
