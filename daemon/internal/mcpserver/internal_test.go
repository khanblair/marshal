package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

func TestFirstRunTakesTheFirstLineThatIsNotBlank(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"a description", "\n\nAdd a health check.\nThen a test.\n", "Add a health check."},
		{"an indented line", "   \n\tship the board  \n", "ship the board"},
		{"no description", "", ""},
		{"only blanks", "\n   \n\t\n", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := firstRun(test.text, goalRun); got != test.want {
				t.Errorf("firstRun(%q) is %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestCutRunesMarksWhatItCutAndNeverSplitsACharacter(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{"short enough", "abc", 5, "abc"},
		{"exactly the limit", "abcde", 5, "abcde"},
		{"cut", "abcdefg", 5, "abcde" + ellipsis},
		{"cut on a space", "abcd efg", 5, "abcd" + ellipsis},
		{"no limit at all", "abcdefg", 0, "abcdefg"},
		{"runes and not bytes", "日本語のテキスト", 3, "日本語" + ellipsis},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cutRunes(test.text, test.limit); got != test.want {
				t.Errorf("cutRunes(%q, %d) is %q, want %q", test.text, test.limit, got, test.want)
			}
		})
	}
}

func TestExcerptCollapsesWhitespace(t *testing.T) {
	got := excerpt("  one\n\n two   three \n", 100)
	if got != "one two three" {
		t.Errorf("excerpt is %q, want %q", got, "one two three")
	}
}

func TestBoardSummaryCountsTheBoardInBoardOrder(t *testing.T) {
	shown := []boardCard{
		{State: protocol.CardStateWorking},
		{State: protocol.CardStateNeeds},
		{State: protocol.CardStateWorking},
		{State: protocol.CardStateBacklog},
	}
	got := boardSummary(4, shown, false)
	want := "4 other cards on this project's board: 1 backlog, 2 working, 1 needs."
	if got != want {
		t.Errorf("the summary is %q, want %q", got, want)
	}
	if cut := boardSummary(9, shown, true); !strings.HasSuffix(cut, "The list below is the first 4.") {
		t.Errorf("a cut summary is %q", cut)
	}
	if none := boardSummary(0, nil, false); !strings.Contains(none, "No other cards") {
		t.Errorf("the summary of an empty board is %q", none)
	}
}

func TestClaimsByCardGroupsThem(t *testing.T) {
	got := claimsByCard([]memory.Claim{
		{CardID: "a", PathOrPackage: "one.go"},
		{CardID: "b", PathOrPackage: "two.go"},
		{CardID: "a", PathOrPackage: "three.go"},
	})
	if len(got["a"]) != 2 || len(got["b"]) != 1 {
		t.Errorf("the claims were grouped as %+v", got)
	}
	if len(got["c"]) != 0 {
		t.Errorf("a card with no claims has %+v", got["c"])
	}
}

func TestTimeTextIsEmptyForNoTime(t *testing.T) {
	if got := timeText(nil); got != "" {
		t.Errorf("no time reads as %q", got)
	}
	stamp := protocol.NewTimestamp(time.Date(2026, time.September, 27, 9, 30, 0, 0, time.UTC))
	if got, want := timeText(&stamp), "2026-09-27T09:30:00Z"; got != want {
		t.Errorf("the time reads as %q, want %q", got, want)
	}
}

func TestAQuestionSaysWhoIsAskingAndWhereTheAnswerStays(t *testing.T) {
	me := protocol.Card{Key: "small-repo#1", Title: "Add a health check"}
	got := questionFrom(me, "  Are you on src/board.ts?  ")
	for _, want := range []string{
		"card small-repo#1", `"Add a health check"`, "Are you on src/board.ts?", "does not carry",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the question %q does not say %q", got, want)
		}
	}
	if strings.Contains(got, "  Are you") {
		t.Errorf("the question was not trimmed: %q", got)
	}
}

func TestRefusalNamesTheModeAndTheTool(t *testing.T) {
	tests := []struct {
		name string
		mode protocol.PermissionMode
		out  harness.Outcome
		want string
	}{
		{
			"a plan refuses a change",
			protocol.PermissionModePlan,
			harness.Outcome{Decision: harness.DecisionDeny, Rule: harness.RuleMode},
			"only reads",
		},
		{
			"a careful mode leaves it to the owner",
			protocol.PermissionModeAutoEdits,
			harness.Outcome{Decision: harness.DecisionAsk, Rule: harness.RuleMode},
			"card's owner",
		},
		{
			"a profile refusal is not the mode's",
			protocol.PermissionModeFullAuto,
			harness.Outcome{Decision: harness.DecisionDeny, Rule: security.RuleProfile},
			"profile",
		},
		{
			"a rule of its own is named",
			protocol.PermissionModeFullAuto,
			harness.Outcome{Decision: harness.DecisionDeny, Rule: "some-rule"},
			"some-rule",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			said := refusal(test.mode, "post_note", test.out)
			for _, want := range []string{test.want, "post_note", "not done"} {
				if !strings.Contains(said, want) {
					t.Errorf("the refusal %q does not say %q", said, want)
				}
			}
		})
	}
	if said := cannotReadTheMode("post_note"); !strings.Contains(said, "could not tell") {
		t.Errorf("a session with no mode is refused as %q", said)
	}
}

func TestBoardCardOfNamesTheWork(t *testing.T) {
	role := boardCardOf(protocol.Card{Role: "Reviewer", Agent: protocol.AgentKindClaude}, nil, nil)
	if role.Owner != "Reviewer" {
		t.Errorf("a card with a role is owned by %q", role.Owner)
	}
	agent := boardCardOf(protocol.Card{Agent: protocol.AgentKindCodex}, nil, nil)
	if agent.Owner != string(protocol.AgentKindCodex) {
		t.Errorf("a card with no role is owned by %q", agent.Owner)
	}
	if agent.Claims != nil {
		t.Errorf("a card with no claims has %q", agent.Claims)
	}
}
