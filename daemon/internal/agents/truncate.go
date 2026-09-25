package agents

import "unicode/utf8"

// Truncate cuts text to at most limit bytes without splitting a character, and says whether it
// cut anything. Adapters use it to keep events within MaxContentBytes.
func Truncate(text string, limit int) (string, bool) {
	if limit < 0 {
		limit = 0
	}
	if len(text) <= limit {
		return text, false
	}
	cut := limit
	// Back up to the start of a character, so the result is still valid text.
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut], true
}
