package search

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The excerpt is what a search answer shows of a note: the beginning of it, cut at the run and never
// past it. The whole note goes to the caller that scores a hit, so this cut is only about what the
// answer carries.

func TestTheExcerptIsTheBeginningOfANote(t *testing.T) {
	// Whitespace around the note is not part of it, and a short note is shown whole.
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"a short note", "a short note", "a short note"},
		{"padding trimmed", "\n\n  a short note  \n", "a short note"},
		{"empty", "", ""},
		{"only whitespace", "  \n\t ", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := excerpt(tc.body); got != tc.want {
				t.Errorf("excerpt(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// A note exactly the run long is shown whole; one character more is cut at the run and says so with
// an ellipsis, so the reader knows there is more and the answer stays the length it promised.
func TestTheExcerptIsCutAtTheRunAndSaysSo(t *testing.T) {
	atRun := strings.Repeat("a", noteExcerptRun)
	if got := excerpt(atRun); got != atRun {
		t.Errorf("a note at the run came out %d bytes long, want it whole (%d)", len(got), len(atRun))
	}

	over := strings.Repeat("a", noteExcerptRun) + "b"
	got := excerpt(over)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("a note over the run came out %q, want it to end with an ellipsis", got)
	}
	if body := strings.TrimSuffix(got, "…"); body != atRun {
		t.Errorf("the cut note kept %d bytes, want the first %d", len(body), noteExcerptRun)
	}
}

// A note written in characters wider than a byte is cut at a character boundary, so the answer never
// carries half a character: that would be text nothing can read.
func TestTheExcerptCutNeverSplitsACharacter(t *testing.T) {
	// Two bytes a character, so the run lands on a boundary at every other byte and the cut has to
	// walk back to one when the note is an odd length.
	for _, note := range []string{
		strings.Repeat("é", noteExcerptRun), // exactly the run in characters, twice the run in bytes
		strings.Repeat("é", noteExcerptRun/2) + "a",
		strings.Repeat("é", 3),
		strings.Repeat("日", noteExcerptRun) + "x",
	} {
		got := excerpt(note)
		if !utf8.ValidString(got) {
			t.Errorf("the excerpt of a wide note is not valid text: %q", got)
		}
		if len(got) > noteExcerptRun+len("…") {
			t.Errorf("the excerpt is %d bytes, want at most the run plus the ellipsis", len(got))
		}
	}
	// A four-byte character sitting across the run is walked back over rather than halved.
	wide := strings.Repeat("a", noteExcerptRun-2) + "😀" + strings.Repeat("b", 10)
	got := excerpt(wide)
	if !utf8.ValidString(got) || strings.ContainsRune(got, '\uFFFD') {
		t.Errorf("the excerpt of a note with an emoji at the cut is not valid text: %q", got)
	}
}

// The run is the length the MCP server's search_memory tool shows of the same note, so a person and
// an agent reading one search see the same amount of the same note (internal/mcpserver's own
// noteExcerptRun). The two are stated in two places, so this is written down as a fact and not left
// to be noticed.
func TestTheExcerptRunMatchesTheMCPTools(t *testing.T) {
	if noteExcerptRun != 200 {
		t.Errorf("noteExcerptRun = %d, want the 200 internal/mcpserver uses", noteExcerptRun)
	}
}
