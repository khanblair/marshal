package api_test

// The quality routes (docs/backend-checklist.md B5.8, build-plan 5.12 to 5.15, docs/architecture.md
// section 17, docs/ui-rules.md section 14): a card's code-smell findings, the two calls that act on
// one, and one project's smell profile. What a check finds in a diff is internal/quality's own
// business and is tested there over a real store; this file tests the wire - the shape of a list,
// what the two acting calls write down, and what a profile may not be set to.
//
// The one process these tests avoid is the linter and the agent: a finding is written straight into
// the store the way a check writes one, and the call that asks a card's agent to fix a finding is
// driven on a daemon with no session manager at all, so nothing is ever started.

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// seedFinding writes one finding for a card straight into the store, the way the checks write one
// when they run for a commit: its place, its words, and its status, which start open. An empty
// commit is the finding of uncommitted work, and it is what a card with no worktree reads.
func seedFinding(t *testing.T, st *stack, cardID, findingID, commit string, severity protocol.SmellSeverity) {
	t.Helper()
	err := st.store.Write(context.Background(), func(q *db.Queries) error {
		return q.InsertSmellFinding(context.Background(), db.InsertSmellFindingParams{
			ID: findingID, CardID: cardID, CommitSha: commit,
			Family: string(protocol.SmellFamilyBloaters), Smell: string(protocol.SmellCheckLongFunction),
			File: "src/util.js", Line: 12, Severity: string(severity),
			Message:    "This function is longer than the profile allows.",
			Suggestion: "Split it into smaller functions.",
			Status:     string(protocol.SmellStatusOpen),
		})
	})
	if err != nil {
		t.Fatalf("seed a finding for card %s: %v", cardID, err)
	}
}

// settingIn answers one check's setting out of a resolved profile, and fails the test when the
// profile does not name it: a resolved profile always carries every built-in check.
func settingIn(t *testing.T, profile protocol.SmellProfile, check protocol.SmellCheck) protocol.SmellCheckSetting {
	t.Helper()
	for _, setting := range profile.Checks {
		if setting.Check == check {
			return setting
		}
	}
	t.Fatalf("the profile does not name the %s check: %+v", check, profile.Checks)
	return protocol.SmellCheckSetting{}
}

// A card that has never been checked answers an empty list with a null checked time, and the bytes
// are the empty array and the null literal, so no client has to handle a missing list or a checked
// time of the beginning of time.
func TestACardThatWasNeverCheckedAnswersAnEmptyFindingList(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Nothing found yet")

	r := st.do(http.MethodGet, "/v1/cards/"+card.ID+"/findings", nil).want(t, http.StatusOK)
	list := decode[protocol.SmellFindingList](t, r)
	if list.CardID != card.ID || len(list.Findings) != 0 || list.Blocking != 0 {
		t.Errorf("the list = %+v, want an empty list for %s", list, card.ID)
	}
	if list.Checked != nil {
		t.Errorf("checked = %v, want null for a card that was never checked", list.Checked)
	}
	if body := string(r.Body); !strings.Contains(body, `"findings":[]`) || !strings.Contains(body, `"checked":null`) {
		t.Errorf("the answer = %s, want an empty findings array and a null checked", body)
	}
}

// A card's findings come back with the family, the rule name, and the severity a check saved, and
// the list carries the blocking count so a screen can say it without counting.
func TestACardsFindingsComeBackWithTheirFamilyAndSeverity(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Has a smell")
	seedFinding(t, st, card.ID, sampleFindingID, "", protocol.SmellSeverityBlocking)

	list := decode[protocol.SmellFindingList](t,
		st.do(http.MethodGet, "/v1/cards/"+card.ID+"/findings", nil).want(t, http.StatusOK))
	if len(list.Findings) != 1 {
		t.Fatalf("the findings = %+v, want one", list.Findings)
	}
	got := list.Findings[0]
	if got.ID != sampleFindingID || got.CardID != card.ID || got.Family != protocol.SmellFamilyBloaters ||
		got.Smell != string(protocol.SmellCheckLongFunction) || got.Severity != protocol.SmellSeverityBlocking ||
		got.Status != protocol.SmellStatusOpen {
		t.Errorf("the finding = %+v, want the seeded one", got)
	}
	if list.Blocking != 1 {
		t.Errorf("blocking = %d, want 1", list.Blocking)
	}
}

// Findings are read as of the card's current commit: a finding of an older commit is never shown
// against newer work. The list runs nothing, so it is a read as cheap as any other.
func TestFindingsAreReadAsOfTheCardsCurrentCommit(t *testing.T) {
	st := newStack(t)
	project, repo := st.addProject("small-repo")
	card := st.addCard(project.ID, "Moved on")
	dir := st.startWorktree(t, project, repo, card)
	head := headOf(t, st, dir)

	seedFinding(t, st, card.ID, sampleFindingID, head, protocol.SmellSeverityWarning)
	// A finding of uncommitted work belongs to another commit and must not be shown here.
	seedFinding(t, st, card.ID, "01M3C107JB041061050R3GG28D", "", protocol.SmellSeverityWarning)

	list := decode[protocol.SmellFindingList](t,
		st.do(http.MethodGet, "/v1/cards/"+card.ID+"/findings", nil).want(t, http.StatusOK))
	if list.Commit != head {
		t.Errorf("the list is as of %q, want %q", list.Commit, head)
	}
	if len(list.Findings) != 1 || list.Findings[0].ID != sampleFindingID {
		t.Errorf("the findings = %+v, want just the one at the current commit", list.Findings)
	}
}

// Dismissing a finding keeps the reason, writes it down rather than holding it on the client, and
// clears the finding from the blocking count.
func TestDismissingAFindingKeepsTheReason(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Dismiss me")
	seedFinding(t, st, card.ID, sampleFindingID, "", protocol.SmellSeverityBlocking)

	got := decode[protocol.SmellFinding](t, st.do(http.MethodPost,
		"/v1/cards/"+card.ID+"/findings/"+sampleFindingID+"/dismiss",
		protocol.DismissFindingRequest{Reason: "The switch is deliberate."}).want(t, http.StatusOK))
	if got.Status != protocol.SmellStatusDismissed || got.DismissReason != "The switch is deliberate." {
		t.Errorf("the finding = %+v, want it dismissed with the reason", got)
	}
	list := decode[protocol.SmellFindingList](t,
		st.do(http.MethodGet, "/v1/cards/"+card.ID+"/findings", nil).want(t, http.StatusOK))
	if len(list.Findings) != 1 || list.Findings[0].Status != protocol.SmellStatusDismissed {
		t.Errorf("the list after dismissing = %+v, want the dismissed finding", list.Findings)
	}
	if list.Blocking != 0 {
		t.Errorf("blocking after dismissing = %d, want 0", list.Blocking)
	}
}

// A dismissal with no reason is refused with the field that is wrong: a finding is waved away on
// purpose, and the reason is what a later phase's lessons learn from.
func TestDismissingAFindingNeedsAReason(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Dismiss me")
	seedFinding(t, st, card.ID, sampleFindingID, "", protocol.SmellSeverityWarning)

	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/findings/"+sampleFindingID+"/dismiss",
		protocol.DismissFindingRequest{Reason: "   "}).
		apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if got.Details["field"] != "reason" {
		t.Errorf("the refusal = %+v, want field=reason", got)
	}
}

// Asking the agent to fix a finding on a daemon with no session manager is refused with the reason
// the app names, and nothing is written down: there was nobody to ask.
func TestAskingTheAgentToFixAFindingWithoutAnAgentIsRefused(t *testing.T) {
	st := newStack(t, withoutSessions())
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "No agent here")
	seedFinding(t, st, card.ID, sampleFindingID, "", protocol.SmellSeverityWarning)

	got := st.do(http.MethodPost, "/v1/cards/"+card.ID+"/findings/"+sampleFindingID+"/fix", nil).
		apiError(t, http.StatusUnprocessableEntity, protocol.ErrorCodeRefused)
	if got.Details["reason"] != "quality_no_agent" || got.Details["cardId"] != card.ID {
		t.Errorf("the refusal = %+v, want reason=quality_no_agent for %s", got, card.ID)
	}
	// The finding is still open: the refusal did not mark a hand-over that never happened.
	list := decode[protocol.SmellFindingList](t,
		st.do(http.MethodGet, "/v1/cards/"+card.ID+"/findings", nil).want(t, http.StatusOK))
	if len(list.Findings) != 1 || list.Findings[0].Status != protocol.SmellStatusOpen {
		t.Errorf("the finding after the refusal = %+v, want it still open", list.Findings)
	}
}

// An id that cannot be the shape of an id, or a card and a finding that do not exist, are all not
// found, and the answer carries the id it could not find.
func TestAFindingRouteRefusesWhatItCannotFind(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "Some card")
	seedFinding(t, st, card.ID, sampleFindingID, "", protocol.SmellSeverityWarning)

	tests := []struct {
		name         string
		method, path string
		wantID       string
	}{
		{"a card id of the wrong shape", http.MethodGet, "/v1/cards/not-a-card/findings", "not-a-card"},
		{"a card Marshal does not have", http.MethodGet, "/v1/cards/" + sampleCardID + "/findings", sampleCardID},
		{"a finding id of the wrong shape", http.MethodPost, "/v1/cards/" + card.ID + "/findings/nope/fix", "nope"},
		{
			"a finding that is not on the card", http.MethodPost,
			"/v1/cards/" + card.ID + "/findings/01M3C107JB041061050R3GG28D/dismiss",
			"01M3C107JB041061050R3GG28D",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := any(nil)
			if tt.method == http.MethodPost && strings.HasSuffix(tt.path, "/dismiss") {
				body = protocol.DismissFindingRequest{Reason: "Fine as it is."}
			}
			got := st.do(tt.method, tt.path, body).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
			if got.Details["id"] != tt.wantID {
				t.Errorf("the refusal = %+v, want id=%s", got, tt.wantID)
			}
		})
	}
}

// The five quality routes exist only when the quality module is wired: a daemon built without it has
// no such address at all, and no token can change that.
func TestTheQualityRoutesAreGoneWithoutTheQualityModule(t *testing.T) {
	st := newStack(t, withoutQuality())
	project, _ := st.addProject("small-repo")
	card := st.addCard(project.ID, "No checks here")
	requests := []string{
		http.MethodGet + " /v1/cards/" + card.ID + "/findings",
		http.MethodPost + " /v1/cards/" + card.ID + "/findings/" + sampleFindingID + "/fix",
		http.MethodPost + " /v1/cards/" + card.ID + "/findings/" + sampleFindingID + "/dismiss",
		http.MethodGet + " /v1/projects/" + project.ID + "/smell-profile",
		http.MethodPut + " /v1/projects/" + project.ID + "/smell-profile",
	}
	for _, request := range requests {
		method, path, _ := strings.Cut(request, " ")
		r := st.do(method, path, nil)
		if r.Status != http.StatusNotFound || !strings.Contains(string(r.Body), nothingThereMsg) {
			t.Errorf("%s = %d %s, want 404 and %q", request, r.Status, r.Body, nothingThereMsg)
		}
	}
}

// A project that has never set a profile answers the defaults, with every threshold and every check
// filled in, so the screen has numbers to draw before anything is saved.
func TestASmellProfileOfAProjectThatNeverSetOneIsTheDefaults(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")

	profile := decode[protocol.SmellProfile](t,
		st.do(http.MethodGet, "/v1/projects/"+project.ID+"/smell-profile", nil).want(t, http.StatusOK))
	defaults := protocol.DefaultSmellProfile()
	if profile.ProjectID != project.ID {
		t.Errorf("projectId = %q, want %s", profile.ProjectID, project.ID)
	}
	if profile.MaxFunctionLines != defaults.MaxFunctionLines || profile.MaxFileLines != defaults.MaxFileLines ||
		profile.MaxParameters != defaults.MaxParameters || profile.MaxNesting != defaults.MaxNesting ||
		profile.MaxLineLength != defaults.MaxLineLength ||
		profile.DuplicateBlockLines != defaults.DuplicateBlockLines {
		t.Errorf("the profile = %+v, want the defaults", profile)
	}
	if len(profile.Checks) != len(defaults.Checks) {
		t.Errorf("the profile names %d checks, want %d", len(profile.Checks), len(defaults.Checks))
	}
}

// Saving a profile answers it resolved - the checks the profile did not name keep their default -
// and it is written down, so reading it back answers the same thing.
func TestSavingASmellProfileAnswersItResolved(t *testing.T) {
	st := newStack(t)
	project, _ := st.addProject("small-repo")

	saved := decode[protocol.SmellProfile](t, st.do(http.MethodPut,
		"/v1/projects/"+project.ID+"/smell-profile", protocol.SmellProfile{
			MaxFunctionLines: 120,
			MaxLineLength:    200,
			Checks: []protocol.SmellCheckSetting{
				{Check: protocol.SmellCheckMagicNumber, Enabled: false, Severity: protocol.SmellSeverityInfo},
			},
		}).want(t, http.StatusOK))
	if saved.MaxFunctionLines != 120 || saved.MaxLineLength != 200 {
		t.Errorf("the saved profile = %+v, want the thresholds that were set", saved)
	}
	if magic := settingIn(t, saved, protocol.SmellCheckMagicNumber); magic.Enabled || magic.Severity != protocol.SmellSeverityInfo {
		t.Errorf("the magic-number setting = %+v, want it off and info", magic)
	}
	if longFn := settingIn(t, saved, protocol.SmellCheckLongFunction); !longFn.Enabled || longFn.Severity != protocol.SmellSeverityBlocking {
		t.Errorf("the long-function setting = %+v, want the default", longFn)
	}
	again := decode[protocol.SmellProfile](t,
		st.do(http.MethodGet, "/v1/projects/"+project.ID+"/smell-profile", nil).want(t, http.StatusOK))
	if !reflect.DeepEqual(again, saved) {
		t.Errorf("the profile read back = %+v, want %+v", again, saved)
	}
}

// A profile that cannot be used is refused with the field that is wrong: a threshold outside its
// bounds, a check Marshal does not know, one named twice, a severity that is not one, and a linter
// with no program.
func TestSavingASmellProfileRefusesWhatCannotWork(t *testing.T) {
	tests := []struct {
		name  string
		body  protocol.SmellProfile
		field string
	}{
		{
			name:  "a threshold below its minimum",
			body:  protocol.SmellProfile{MaxFunctionLines: 1},
			field: "maxFunctionLines",
		},
		{
			name:  "a threshold above its maximum",
			body:  protocol.SmellProfile{MaxLineLength: 100_000},
			field: "maxLineLength",
		},
		{
			name: "a check Marshal does not know",
			body: protocol.SmellProfile{Checks: []protocol.SmellCheckSetting{
				{Check: protocol.SmellCheck("smells-nice"), Enabled: true, Severity: protocol.SmellSeverityWarning},
			}},
			field: "checks",
		},
		{
			name: "the same check twice",
			body: protocol.SmellProfile{Checks: []protocol.SmellCheckSetting{
				{Check: protocol.SmellCheckLargeFile, Enabled: true, Severity: protocol.SmellSeverityWarning},
				{Check: protocol.SmellCheckLargeFile, Enabled: false, Severity: protocol.SmellSeverityInfo},
			}},
			field: "checks",
		},
		{
			name: "a severity Marshal does not know",
			body: protocol.SmellProfile{Checks: []protocol.SmellCheckSetting{
				{Check: protocol.SmellCheckLongLine, Enabled: true, Severity: protocol.SmellSeverity("loud")},
			}},
			field: "checks",
		},
		{
			name: "a linter with no program",
			body: protocol.SmellProfile{Linters: []protocol.SmellLinter{
				{Name: "eslint", Family: protocol.SmellFamilyLexicalAbusers},
			}},
			field: "linters",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newStack(t)
			project, _ := st.addProject("small-repo")
			got := st.do(http.MethodPut, "/v1/projects/"+project.ID+"/smell-profile", tt.body).
				apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
			if got.Details["field"] != tt.field {
				t.Errorf("the refusal = %+v, want field=%s", got, tt.field)
			}
		})
	}
}

// The profile routes are not found for a project that does not exist, whether the id is the wrong
// shape or merely unknown.
func TestASmellProfileOfAProjectThatDoesNotExistIsNotFound(t *testing.T) {
	st := newStack(t)
	tests := []struct {
		name string
		id   string
	}{
		{"a project id of the wrong shape", "UPPER"},
		{"a project Marshal does not have", "no-such-project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := st.do(http.MethodGet, "/v1/projects/"+tt.id+"/smell-profile", nil).
				apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
			if got.Details["id"] != tt.id {
				t.Errorf("the refusal = %+v, want id=%s", got, tt.id)
			}
		})
	}
}
