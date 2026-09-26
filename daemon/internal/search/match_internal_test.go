package search

import "testing"

// A word is in a text at the best tier any of its places gives, not the first place it is found:
// "cache" is inside "uncached" and also starts the word after it.
func TestMatchTierUsesTheBestPlaceInTheText(t *testing.T) {
	for _, tc := range []struct {
		text, word string
		want       tier
	}{
		{"cache", "cache", tierWhole},
		{"cache miss", "cache", tierStart},
		{"uncached then cache", "cache", tierWordStart},
		{"uncached reads", "cache", tierWithin},
		{"a-cache", "cache", tierWordStart},
		{"a_cache", "cache", tierWordStart},
		{"2cache", "cache", tierWithin},
		{"über prüfung", "prüf", tierWordStart},
		{"überprüfung", "prüf", tierWithin},
		{"nothing here", "cache", tierNone},
		{"", "cache", tierNone},
	} {
		if got := matchTier(tc.text, tc.word); got != tc.want {
			t.Errorf("matchTier(%q, %q) = %d, want %d", tc.text, tc.word, got, tc.want)
		}
	}
}

// A word is a number only when it is digits, and only ASCII ones: a full-width digit is text.
func TestIsDigits(t *testing.T) {
	for text, want := range map[string]bool{
		"41": true, "0": true, "": false, "4a": false, "-4": false, "４１": false, " 4": false,
	} {
		if got := isDigits(text); got != want {
			t.Errorf("isDigits(%q) = %v, want %v", text, got, want)
		}
	}
}
