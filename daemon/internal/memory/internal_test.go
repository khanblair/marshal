package memory

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The pieces a note's path and a search's match expression are made of, tested where they are
// written: they are the rules the daemon and the app have to agree on, and a change to one of them
// that the other does not follow shows up as a note the person cannot find.

func TestNoteSlugKeepsOnlyTheLettersAndDigitsOfATitle(t *testing.T) {
	for _, tc := range []struct {
		name  string
		title string
		want  string
	}{
		{"an ordinary title", "Add a health check", "add-a-health-check"},
		{"punctuation and a backtick", "Fix: the `foo` bar!", "fix-the-foo-bar"},
		{"runs of spaces and symbols", "Two   --  words", "two-words"},
		{"already short and lower", "small-repo", "small-repo"},
		{"digits are kept", "Bump Go to 1.27", "bump-go-to-1-27"},
		{"a title with nothing we keep", "日本語", noteSlugFallback},
		{"a title of only punctuation", "***", noteSlugFallback},
		{"an empty title", "", noteSlugFallback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := noteSlug(tc.title); got != tc.want {
				t.Fatalf("noteSlug(%q) = %q, want %q", tc.title, got, tc.want)
			}
		})
	}
}

func TestNoteSlugCutsALongTitleAndNeverEndsOnAHyphen(t *testing.T) {
	long := strings.Repeat("a", 40) + " " + strings.Repeat("b", 40)
	got := noteSlug(long)
	if len(got) > noteSlugMax {
		t.Fatalf("noteSlug left %d bytes, want at most %d", len(got), noteSlugMax)
	}
	if strings.HasSuffix(got, "-") {
		t.Fatalf("noteSlug(%q) = %q, which ends on a hyphen", long, got)
	}
	// The cut lands inside the second word, so the first word and the hyphen between them survive.
	if !strings.HasPrefix(got, strings.Repeat("a", 40)+"-bbb") {
		t.Fatalf("noteSlug(%q) = %q, want it to keep the first word", long, got)
	}
}

func TestNoteFileAndRelPathAreTheNumberThenTheTitle(t *testing.T) {
	card := protocol.Card{
		ID: "card-7", ProjectID: "small-repo", Number: 7, Title: "Add a health check",
	}
	if got, want := noteFileName(card), "7-add-a-health-check.md"; got != want {
		t.Fatalf("noteFileName = %q, want %q", got, want)
	}
	if got, want := noteRelPath(card), "small-repo/cards/7-add-a-health-check.md"; got != want {
		t.Fatalf("noteRelPath = %q, want %q", got, want)
	}
}

func TestNoteNumberReadsTheCardNumberAFileNameStartsWith(t *testing.T) {
	// The number in front of a note's file name is what ties a file in the vault back to a card
	// without reading the file's body, which is what the vault watcher (task 7.8) turns a file back
	// into a card with. A name it cannot read is a file it leaves alone.
	for _, tc := range []struct {
		name string
		file string
		want int
		ok   bool
	}{
		{"an ordinary name", "7-add-a-health-check.md", 7, true},
		{"a card number past a hundred", "1042-trim-the-bundle.md", 1042, true},
		{"a number with nothing after it", "7-.md", 7, true},
		{"a name with no number in it", "readme.md", 0, false},
		{"a number written as a word", "seven-things.md", 0, false},
		{"zero is not a card number", "0-a-card.md", 0, false},
		{"a negative number", "-7-a-card.md", 0, false},
		{"the extension alone", ".md", 0, false},
		{"a name that is only digits and hyphens", "7-9-10.md", 7, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := noteNumber(tc.file)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("noteNumber(%q) = %d, %v, want %d, %v", tc.file, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestNotePathKeepsEveryNoteInsideTheVaultRoot(t *testing.T) {
	s := &Service{root: filepath.Join("/tmp", "a-vault")}
	full, err := s.notePath("small-repo/cards/7-add-a-health-check.md")
	if err != nil {
		t.Fatalf("a note inside the vault was refused: %v", err)
	}
	if want := filepath.Join("/tmp", "a-vault", "small-repo", "cards", "7-add-a-health-check.md"); full != want {
		t.Fatalf("notePath = %q, want %q", full, want)
	}
	for _, rel := range []string{"", ".", "../outside.md", "small-repo/../../outside.md"} {
		if _, err := s.notePath(rel); err == nil {
			t.Fatalf("notePath(%q) was allowed out of the vault", rel)
		}
	}
	if _, err := s.notePath("/etc/passwd"); err == nil {
		t.Fatal("notePath allowed an absolute path")
	}
}

func TestPlaceholderNoteNamesTheCardAndItsProject(t *testing.T) {
	card := protocol.Card{ID: "card-7", ProjectID: "small-repo", Number: 7, Title: "Add a health check"}
	project := protocol.Project{ID: "small-repo", Name: "Small Repo"}
	body := placeholderNote(card, project)
	for _, want := range []string{"Add a health check", "add a health check", "[[small-repo]]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the placeholder note does not mention %q:\n%s", want, body)
		}
	}
}

func TestMatchExpressionIsOnePrefixTermPerWord(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		want  string
	}{
		{"two words", "health check", "health* check*"},
		{"one word with space around it", "   heal   ", "heal*"},
		{"punctuation between words", "a-b", "a* b*"},
		{"a stray FTS operator", `foo" OR bar`, "foo* OR* bar*"},
		{"a hyphen FTS reads as NOT", "auth-not", "auth* not*"},
		{"nothing but punctuation", "-- ** ?!", ""},
		{"nothing at all", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchExpression(tc.query); got != tc.want {
				t.Fatalf("matchExpression(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestMatchExpressionCutsAWordNobodyTyped(t *testing.T) {
	got := matchExpression(strings.Repeat("a", 200))
	if want := strings.Repeat("a", searchTermMax) + "*"; got != want {
		t.Fatalf("matchExpression of a 200-letter word = %q, want it cut to %d letters", got, searchTermMax)
	}
}

func TestCleanClaimPathsTidiesWhatACallNames(t *testing.T) {
	got, err := cleanClaimPaths([]string{"internal/api/routes.go", " internal/api/routes.go ", "internal/projects"})
	if err != nil {
		t.Fatalf("an ordinary claim was refused: %v", err)
	}
	if want := []string{"internal/api/routes.go", "internal/projects"}; len(got) != len(want) ||
		got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("cleanClaimPaths = %v, want %v (the duplicate dropped, the order kept)", got, want)
	}
}

func TestCleanClaimPathsRefusesWhatIsNotAPathInTheRepository(t *testing.T) {
	tooMany := make([]string, maxClaimsPerCall+1)
	for i := range tooMany {
		tooMany[i] = "file" + string(rune('a'+i%26)) + ".go"
	}
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{"nothing named", nil},
		{"a blank path", []string{"   "}},
		{"an absolute path", []string{"/etc/passwd"}},
		{"a path above the repository", []string{".."}},
		{"a path climbing above the repository", []string{"../secrets"}},
		{"a path climbing through the repository", []string{"internal/../../etc/passwd"}},
		{"a path ending above the repository", []string{"internal/.."}},
		{"a path too long to be real", []string{strings.Repeat("a", maxClaimLength+1)}},
		{"more paths than a call may name", tooMany},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cleanClaimPaths(tc.paths); err == nil {
				t.Fatalf("cleanClaimPaths accepted %v", tc.paths)
			}
		})
	}
}
