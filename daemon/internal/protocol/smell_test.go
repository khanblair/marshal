package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The wire shape of the code-smell checks (B5.8, architecture.md section 17): one finding in a
// card's diff, the list a card's checks panel draws, and one project's profile of which checks run
// at what thresholds and how much they matter.

// sampleFinding is one blocking finding: a function that is far longer than the profile allows.
func sampleFinding() protocol.SmellFinding {
	return protocol.SmellFinding{
		ID:         "01JD7Q4M2X8K9V0P5T3RB6NHC1",
		CardID:     sampleCardID,
		Commit:     "9f2c1ab4d5e6f708192a3b4c5d6e7f8091a2b3c4",
		Family:     protocol.SmellFamilyBloaters,
		Smell:      string(protocol.SmellCheckLongFunction),
		File:       "src/report.ts",
		Line:       118,
		Severity:   protocol.SmellSeverityBlocking,
		Message:    "This function is 143 lines, over the 80 a new function may be.",
		Suggestion: "Split it into one function per section of the report.",
		Status:     protocol.SmellStatusOpen,
	}
}

// sampleFindingList is the answer to GET /v1/cards/{id}/findings: one blocking finding, one warning
// a person waved away with a reason, and one informational one.
func sampleFindingList() protocol.SmellFindingList {
	warning := sampleFinding()
	warning.ID = "01JD7Q4M2X8K9V0P5T3RB6NHC2"
	warning.Smell = "golangci-lint:gocyclo"
	warning.Family = protocol.SmellFamilyObfuscators
	warning.File = "src/report.ts"
	warning.Line = 42
	warning.Severity = protocol.SmellSeverityWarning
	warning.Message = "This function has a cyclomatic complexity of 24."
	warning.Suggestion = "Move the two branches into their own functions."
	warning.Status = protocol.SmellStatusDismissed
	warning.DismissReason = "The report is generated once and thrown away."

	info := sampleFinding()
	info.ID = "01JD7Q4M2X8K9V0P5T3RB6NHC3"
	info.Smell = string(protocol.SmellCheckLongLine)
	info.Family = protocol.SmellFamilyLexicalAbusers
	info.Line = 7
	info.Severity = protocol.SmellSeverityInfo
	info.Message = "This line is 212 characters long, over the 160 a new line may be."
	info.Suggestion = "Break the call across lines."

	// A finding from uncommitted work carries no commit.
	info.Commit = ""
	return protocol.NewSmellFindingList(sampleCardID, sampleFinding().Commit,
		[]protocol.SmellFinding{sampleFinding(), warning, info}, providersNow, providersNow)
}

func TestSmellFindingGolden(t *testing.T) {
	testutil.Golden(t, "smell-finding", sampleFinding())
}

func TestSmellFindingListGolden(t *testing.T) {
	testutil.Golden(t, "smell-finding-list", sampleFindingList())
}

// A card with nothing wrong, or one whose checks have never run, answers [] and not null, so a
// client never has to handle both an empty list and a missing one.
func TestSmellFindingListNeverEncodesNull(t *testing.T) {
	body, err := json.Marshal(protocol.NewSmellFindingList(sampleCardID, "", nil, providersNow,
		providersNow))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); !strings.Contains(got, `"findings":[]`) {
		t.Errorf("an empty finding list encoded as %s, want []", got)
	}
}

// Only a finding that is still open and blocking counts towards Blocking: one that was dismissed
// and one that was handed to the agent are no longer stopping the card.
func TestOnlyOpenBlockingFindingsCount(t *testing.T) {
	list := sampleFindingList()
	if list.Blocking != 1 {
		t.Errorf("Blocking = %d, want 1 (one open blocking finding)", list.Blocking)
	}
	fixed := sampleFinding()
	fixed.Status = protocol.SmellStatusFixed
	if got := protocol.NewSmellFindingList(sampleCardID, "", []protocol.SmellFinding{fixed},
		providersNow, providersNow).Blocking; got != 0 {
		t.Errorf("Blocking = %d after the finding was fixed, want 0", got)
	}
}

// A card whose checks have never run carries no checked time at all, which is what the panel reads
// to say the checks have not run yet.
func TestAnUncheckedCardCarriesNoCheckedTime(t *testing.T) {
	list := protocol.NewSmellFindingList(sampleCardID, "", nil, time.Time{}, providersNow)
	if list.Checked != nil {
		t.Errorf("Checked = %v, want null", list.Checked)
	}
	body, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"checked":null`) {
		t.Errorf("an unchecked card wrote a checked time: %s", body)
	}
	if !strings.Contains(string(body), `"serverTime":"2026-09-27T09:30:00.000Z"`) {
		t.Errorf("an answer carries the daemon's time: %s", body)
	}
	if got := sampleFindingList().Checked; got == nil {
		t.Error("a checked card carries the moment its checks ran")
	}
}

func TestSmellProfileGolden(t *testing.T) {
	profile := protocol.DefaultSmellProfile()
	profile.ProjectID = sampleProjectID
	profile.MaxFunctionLines = 120
	profile.Linters = []protocol.SmellLinter{{
		Name: "golangci-lint", Command: []string{"golangci-lint", "run", "--out-format", "line-number"},
		Family: protocol.SmellFamilyObfuscators,
	}}
	testutil.Golden(t, "smell-profile", profile)
}

func TestDismissFindingRequestGolden(t *testing.T) {
	testutil.Golden(t, "dismiss-finding-request", protocol.DismissFindingRequest{
		Reason: "The report is generated once and thrown away.",
	})
}

// The nine families and the seven checks are stable strings, because a project's profile names
// them and a finding carries them, and a client groups findings by family without knowing the
// checks.
func TestTheSmellFamiliesAndChecksAreStable(t *testing.T) {
	families := []protocol.SmellFamily{
		protocol.SmellFamilyBloaters, protocol.SmellFamilyChangePreventers,
		protocol.SmellFamilyCouplers, protocol.SmellFamilyDataDealers,
		protocol.SmellFamilyDispensables, protocol.SmellFamilyFunctionalAbusers,
		protocol.SmellFamilyLexicalAbusers, protocol.SmellFamilyObfuscators,
		protocol.SmellFamilyObjectOrientedAbusers,
	}
	if got := protocol.SmellFamilyValues(); len(got) != len(families) {
		t.Fatalf("SmellFamilyValues has %d families, want the nine of product scope 15.4", len(got))
	}
	for _, family := range families {
		if !family.Valid() {
			t.Errorf("SmellFamily(%q).Valid() = false", family)
		}
	}
	if protocol.SmellFamily("bad-taste").Valid() {
		t.Error("SmellFamily.Valid accepts a family Marshal does not know")
	}
	if protocol.SmellFamily("").Valid() {
		t.Error("SmellFamily.Valid accepts the empty string")
	}
	checks := []protocol.SmellCheck{
		protocol.SmellCheckLongFunction, protocol.SmellCheckLargeFile,
		protocol.SmellCheckLongParameterList, protocol.SmellCheckDeepNesting,
		protocol.SmellCheckLongLine, protocol.SmellCheckMagicNumber,
		protocol.SmellCheckDuplicateBlock,
	}
	for _, check := range checks {
		if !check.Valid() {
			t.Errorf("SmellCheck(%q).Valid() = false", check)
		}
	}
	if protocol.SmellCheck("golangci-lint").Valid() {
		t.Error("a linter's own rule name is not one of Marshal's built-in checks")
	}
	for _, severity := range protocol.SmellSeverityValues() {
		if !severity.Valid() {
			t.Errorf("SmellSeverity(%q).Valid() = false", severity)
		}
	}
	if protocol.SmellSeverityBlocking != "blocking" || protocol.SmellSeverityWarning != "warning" ||
		protocol.SmellSeverityInfo != "info" {
		t.Errorf("severity values = %q/%q/%q, want blocking/warning/info",
			protocol.SmellSeverityBlocking, protocol.SmellSeverityWarning, protocol.SmellSeverityInfo)
	}
	for _, status := range protocol.SmellStatusValues() {
		if !status.Valid() {
			t.Errorf("SmellStatus(%q).Valid() = false", status)
		}
	}
	if protocol.SmellStatusOpen != "open" || protocol.SmellStatusFixed != "fixed" ||
		protocol.SmellStatusDismissed != "dismissed" {
		t.Errorf("status values = %q/%q/%q, want open/fixed/dismissed",
			protocol.SmellStatusOpen, protocol.SmellStatusFixed, protocol.SmellStatusDismissed)
	}
}

// The default profile only blocks on the few clear cases the product scope names (section 15.4);
// everything else warns, and every built-in check is on.
func TestTheDefaultProfileBlocksOnlyOnTheClearCases(t *testing.T) {
	profile := protocol.DefaultSmellProfile()
	blocking := map[protocol.SmellCheck]bool{}
	seen := map[protocol.SmellCheck]bool{}
	for _, setting := range profile.Checks {
		if !setting.Check.Valid() {
			t.Fatalf("the default profile names a check Marshal does not know: %q", setting.Check)
		}
		if seen[setting.Check] {
			t.Fatalf("the default profile names %q twice", setting.Check)
		}
		seen[setting.Check] = true
		blocking[setting.Check] = setting.Severity == protocol.SmellSeverityBlocking
	}
	if len(profile.Checks) != len(protocol.SmellCheckValues()) {
		t.Errorf("the default profile sets %d checks, want all %d", len(profile.Checks),
			len(protocol.SmellCheckValues()))
	}
	for _, check := range protocol.SmellCheckValues() {
		if !seen[check] {
			t.Errorf("the default profile does not set %q", check)
		}
	}
	for _, want := range []protocol.SmellCheck{
		protocol.SmellCheckLongFunction, protocol.SmellCheckLargeFile,
		protocol.SmellCheckDuplicateBlock,
	} {
		if !blocking[want] {
			t.Errorf("%q blocks by default, want true", want)
		}
	}
	if blocking[protocol.SmellCheckMagicNumber] {
		t.Error("a magic number warns by default, and must not block")
	}
}
