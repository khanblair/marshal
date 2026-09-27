package mcpserver

import (
	"strings"
	"unicode/utf8"
)

// The two ways a longer text is cut down for an answer. Both measure in runes, so a cut never splits
// a character, and both mark a cut with an ellipsis so an agent can tell a shortened text from a
// short one.

// ellipsis marks a text that was cut. It is one rune and it is what tells an agent that there is
// more to read in the file itself.
const ellipsis = "…"

// firstRun returns the first non-blank line of a text, cut to at most limit runes. It is how a card's
// body becomes a "goal" in the board summary: the body's own first line is the sentence whoever wrote
// the card put first, which is as close to a goal as a description has.
func firstRun(text string, limit int) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return cutRunes(line, limit)
	}
	return ""
}

// excerpt returns the beginning of a text with its runs of whitespace collapsed to one space, cut to
// at most limit runes. It is how a note found by a search is shown without the whole note: the note
// itself is read with read_notes, and a search answer that carried twenty whole notes would cost more
// than the reading it is meant to save.
func excerpt(text string, limit int) string {
	return cutRunes(strings.Join(strings.Fields(text), " "), limit)
}

// cutRunes returns at most limit runes of a text, with an ellipsis when it had to cut.
func cutRunes(text string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(text) <= limit {
		return text
	}
	return strings.TrimRight(string([]rune(text)[:limit]), " ") + ellipsis
}
