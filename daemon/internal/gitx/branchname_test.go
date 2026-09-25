package gitx_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/gitx"
)

func TestCardBranchName(t *testing.T) {
	tests := []struct {
		name    string
		project string
		number  int
		title   string
		want    string
	}{
		{"a plain title", "acme", 42, "Fix token refresh", "marshal/acme-42-fix-token-refresh"},
		{"upper case and punctuation", "acme", 7, "  Add OAuth2 -- (login)!  ", "marshal/acme-7-add-oauth2-login"},
		{"an empty title", "acme", 1, "", "marshal/acme-1-card"},
		{"only spaces and symbols", "acme", 1, " --- !!! ", "marshal/acme-1-card"},
		{"non-ASCII letters are gaps", "acme", 3, "Café au lait", "marshal/acme-3-caf-au-lait"},
		{"a title with no ASCII at all", "acme", 4, "日本語のタイトル", "marshal/acme-4-card"},
		{"emoji", "acme", 5, "Ship it 🚀🚀 now", "marshal/acme-5-ship-it-now"},
		{"only emoji", "acme", 6, "🚀🔥", "marshal/acme-6-card"},
		{"digits stay", "acme", 8, "2024 plan v2", "marshal/acme-8-2024-plan-v2"},
		{"a project id with symbols", "prj_01H.X Y", 9, "x", "marshal/prj-01h-x-y-9-x"},
		{"an empty project id", "", 10, "x", "marshal/project-10-x"},
		{"zero", "acme", 0, "start", "marshal/acme-0-start"},
		{"a number below zero", "acme", -5, "start", "marshal/acme-0-start"},
		{"the cut lands on a hyphen", "acme", 11, strings.Repeat("a", 39) + " b", "marshal/acme-11-" + strings.Repeat("a", 39)},
		{"exactly 40 characters", "acme", 12, strings.Repeat("b", 40), "marshal/acme-12-" + strings.Repeat("b", 40)},
		{"a very long title", "acme", 13, strings.Repeat("word ", 100), "marshal/acme-13-" + strings.TrimRight(strings.Repeat("word-", 8), "-")},
		{"a very long project id", strings.Repeat("p", 100), 14, "x", "marshal/" + strings.Repeat("p", 32) + "-14-x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := gitx.CardBranchName(tc.project, tc.number, tc.title)
			if got != tc.want {
				t.Errorf("CardBranchName(%q, %d, %q) = %q, want %q", tc.project, tc.number, tc.title, got, tc.want)
			}
		})
	}
}

// slugShape is what the slug part of a card branch may look like.
var slugShape = regexp.MustCompile(`^marshal/[a-z0-9]+(-[a-z0-9]+)*-\d+-[a-z0-9]+(-[a-z0-9]+)*$`)

func TestCardBranchNamesAreValidForGit(t *testing.T) {
	titles := []string{
		"", " ", "Fix token refresh", "日本語", "🚀", "Café", "a/b/c", "..", "x.lock", "-leading", "trailing-",
		"HEAD", "@{-1}", "a~^:?*[\\b", "tab\tand\nnewline", "\x00\x01", strings.Repeat("é", 200),
		strings.Repeat("a-", 100), "ǅ Ⅷ ﬁ K",
	}
	projects := []string{"acme", "", "p_1", "a/b", "..", "x.lock", "日本", "-", "HEAD"}
	for _, project := range projects {
		for _, title := range titles {
			name := gitx.CardBranchName(project, 12, title)
			if err := testGit().ValidBranchName(context.Background(), name); err != nil {
				t.Errorf("CardBranchName(%q, 12, %q) = %q, which Git refuses: %v", project, title, name, err)
			}
			if !slugShape.MatchString(name) {
				t.Errorf("CardBranchName(%q, 12, %q) = %q, which does not have the expected shape", project, title, name)
			}
			rest := strings.TrimPrefix(name, gitx.CardBranchPrefix)
			if slug := rest[strings.LastIndex(rest, "-12-")+len("-12-"):]; len(slug) > 40 {
				t.Errorf("the slug in %q is %d characters, want at most 40", name, len(slug))
			}
		}
	}
}
