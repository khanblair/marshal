package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/auditlog"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The audit log, read only (docs/backend-checklist.md B3.5, docs/marshal-product-scope.md section
// 14.7): the list, the search, and the export. Nothing here writes a row; the daemon writes rows as
// things happen (internal/audit), and these routes only read them back.
//
// The screen that shows this log is not built here: docs/backend-inventory.md section 6 lists it as
// needing design work first, so this slice builds the routes behind it and nothing above them.

// listAudit is GET /v1/audit: the audit log, newest first, paged with `limit` and `cursor`, and
// narrowed by `q` when it is given.
func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	s.serveAudit(w, r, false)
}

// searchAudit is GET /v1/audit/search: the same page, but `q` is required, so a search with nothing
// to look for is refused rather than quietly listing everything.
func (s *Server) searchAudit(w http.ResponseWriter, r *http.Request) {
	s.serveAudit(w, r, true)
}

// serveAudit is the body of both read routes.
func (s *Server) serveAudit(w http.ResponseWriter, r *http.Request, requireQuery bool) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if requireQuery && query == "" {
		s.writeError(w, protocol.InvalidArgument("A search needs something to look for: send `q`."))
		return
	}
	limit, cursor, err := readAuditPage(r)
	if err != nil {
		s.writeError(w, err)
		return
	}
	page, err := s.auditlog.Entries(r.Context(), query, cursor, limit)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	answer, err := auditPage(page, s.now())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, answer)
}

// exportAudit is GET /v1/audit/export: everything `q` matched, newest first, as the JSON export
// document by default or as a CSV download when `format=csv` is asked for.
func (s *Server) exportAudit(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	switch format := r.URL.Query().Get("format"); format {
	case "", "json":
		export, err := s.auditlog.Export(r.Context(), query)
		if err != nil {
			s.writeError(w, translate(err))
			return
		}
		s.writeJSON(w, http.StatusOK, export)
	case "csv":
		s.writeAuditCSV(w, r, query)
	default:
		s.writeError(w, protocol.InvalidArgument("An export is either `json` or `csv`.").With("format", format))
	}
}

// writeAuditCSV streams the export as CSV. The header is set before the first row is written,
// because once a byte is written the status is already sent.
func (s *Server) writeAuditCSV(w http.ResponseWriter, r *http.Request, query string) {
	export, err := s.auditlog.Export(r.Context(), query)
	if err != nil {
		s.writeError(w, translate(err))
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="marshal-audit.csv"`)
	w.WriteHeader(http.StatusOK)
	out := csv.NewWriter(w)
	_ = out.Write([]string{"id", "at", "actor", "action", "target", "sessionId", "detail"})
	for _, entry := range export.Entries {
		detail := ""
		if len(entry.Detail) > 0 {
			if encoded, err := json.Marshal(entry.Detail); err == nil {
				detail = string(encoded)
			}
		}
		_ = out.Write([]string{
			csvField(entry.ID), entry.At.Time().UTC().Format(time.RFC3339), csvField(entry.Actor),
			csvField(entry.Action), csvField(entry.Target), csvField(entry.SessionID), csvField(detail),
		})
	}
	out.Flush()
}

// csvField keeps a value from being read as a formula by a spreadsheet. A cell whose text begins
// with `=`, `+`, `-`, or `@` is treated as a formula by Excel and Sheets, so a value that looks like
// one is prefixed with an apostrophe, which those programs show as a plain value. An audit entry's
// target and detail come from an agent, so this is untrusted text on its way to a person's machine.
func csvField(value string) string {
	if value == "" {
		return ""
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

// readAuditPage reads the paging parameters of an audit route. The rules are the ones every list
// route follows (architecture.md 11.5): how many rows a page holds, and where the page before it
// ended.
func readAuditPage(r *http.Request) (int, auditlog.Cursor, error) {
	limit, raw, err := ParsePage(r)
	if err != nil {
		return 0, auditlog.Cursor{}, err
	}
	cursor, _, err := DecodeCursor[auditlog.Cursor](raw)
	if err != nil {
		return 0, auditlog.Cursor{}, err
	}
	return limit, cursor, nil
}

// auditPage builds the answer of a paged audit route. The next cursor is encoded only when another
// page follows, so a client stops at the end instead of asking once more for nothing.
func auditPage(page auditlog.Page, now time.Time) (protocol.Page[protocol.AuditEntry], error) {
	next := ""
	if page.More {
		encoded, err := EncodeCursor(page.Next)
		if err != nil {
			return protocol.Page[protocol.AuditEntry]{}, err
		}
		next = encoded
	}
	return protocol.NewPage(page.Items, next, now), nil
}
