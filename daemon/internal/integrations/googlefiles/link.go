package googlefiles

import (
	"net/url"
	"regexp"
	"strings"
)

// fileID is what a Google file's id is made of. Checking it keeps anything else out of an address.
var fileID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

// linkKind is what the first part of a Google file's address says the file is.
func linkKind(part string) (Kind, bool) {
	switch part {
	case "document":
		return KindDoc, true
	case "spreadsheets":
		return KindSheet, true
	case "presentation":
		return KindSlides, true
	}
	return "", false
}

// ParseLink takes the kind and the file id out of the address of a Google Doc, Sheet or Slides
// presentation, as copied from a browser. Only the id is taken: Marshal never opens the address, it
// asks the API for the file by id. ok is false for any other address.
func ParseLink(raw string) (kind Kind, id string, ok bool) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Port() != "" {
		return "", "", false
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" || !strings.EqualFold(parsed.Hostname(), "docs.google.com") {
		return "", "", false
	}
	// The path is /document/d/<id>/edit, or /document/u/0/d/<id>/edit for a person signed in to
	// more than one account.
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	kind, found := linkKind(parts[0])
	if !found {
		return "", "", false
	}
	rest := parts[1:]
	if len(rest) >= 2 && rest[0] == "u" {
		rest = rest[2:]
	}
	if len(rest) < 2 || rest[0] != "d" {
		return "", "", false
	}
	id = rest[1]
	// "d/e/<key>" is a published page, which has no id the API can read.
	if id == "e" || !fileID.MatchString(id) {
		return "", "", false
	}
	return kind, id, true
}
