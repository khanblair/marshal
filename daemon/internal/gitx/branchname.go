package gitx

import (
	"strconv"
	"strings"
)

const (
	// CardBranchPrefix starts the name of every branch Marshal makes for a card.
	CardBranchPrefix = "marshal/"

	maxSlugLength     = 40
	maxProjectLength  = 32
	fallbackSlug      = "card"
	fallbackProjectID = "project"
)

// CardBranchName builds the branch name for a card: marshal/<project>-<number>-<title>. The title
// becomes lower case ASCII words joined with hyphens, cut to 40 characters. Letters that are not
// ASCII, emoji, and punctuation count as gaps between words, and a title with no words left
// becomes "card", so the name is never empty. A card number below zero is treated as zero. The
// result is always a valid Git branch name.
func CardBranchName(projectID string, number int, title string) string {
	return CardBranchPrefix + slug(projectID, maxProjectLength, fallbackProjectID) +
		"-" + strconv.Itoa(max(number, 0)) + "-" + slug(title, maxSlugLength, fallbackSlug)
}

// slug turns text into lower case ASCII words joined with single hyphens.
func slug(text string, limit int, fallback string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if gap && b.Len() > 0 {
				b.WriteByte('-')
			}
			gap = false
			b.WriteRune(r)
			continue
		}
		gap = true
	}
	out := b.String()
	if len(out) > limit {
		out = out[:limit]
	}
	out = strings.TrimRight(out, "-")
	if out == "" {
		return fallback
	}
	return out
}
