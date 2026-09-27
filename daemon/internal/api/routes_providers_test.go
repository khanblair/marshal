package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The model provider and limits routes (build-plan task 4.10, N18, B4.5). The rules are in
// internal/providers and are tested there; these tests prove the routes read the path and the body,
// answer in the wire type, and never send a key back.
//
// Every key here is synthetic and reaches nothing: the provider routes make no provider call, and
// the stack's provider service is built over an in-memory keychain, so no test reads or writes this
// machine's real keychain and no test could reach a provider even if it tried. The one local address
// is the port-9 discard address on loopback.
const (
	testAnthropicKey  = "sk-ant-api03-not-real-7c41"
	testLocalAddress  = "http://127.0.0.1:9/v1"
	listProvidersPath = "/v1/providers"
	limitsPath        = "/v1/limits"
)

// providerRow finds one provider in a list by its id.
func providerRow(t *testing.T, list protocol.ProviderList, id string) protocol.Provider {
	t.Helper()
	for _, row := range list.Providers {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("the list has no provider %q; it has %d rows", id, len(list.Providers))
	return protocol.Provider{}
}

// TestListProvidersCarriesEveryProviderAndNoKey is the read the Settings > Models screen makes: one
// row per provider Marshal knows, in the screens' order, all empty until a key is saved.
func TestListProvidersCarriesEveryProviderAndNoKey(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodGet, listProvidersPath, nil).want(t, http.StatusOK)
	sameShape(t, "provider-list", r.Body)

	list := decode[protocol.ProviderList](t, r)
	if len(list.Providers) == 0 {
		t.Fatal("the list is empty, want one row per provider Marshal knows")
	}
	if list.ServerTime.Time().IsZero() {
		t.Error("serverTime is zero, want the daemon's time")
	}
	for _, row := range list.Providers {
		if row.Status != protocol.ProviderStatusEmpty {
			t.Errorf("%s is %q, want empty before any key is saved", row.ID, row.Status)
		}
		if row.Masked != "" {
			t.Errorf("%s has the masked value %q, want none", row.ID, row.Masked)
		}
	}
	// The screens' order starts with Anthropic (the providers table's own order), so the first row
	// and the last are the ends of that list.
	if got := list.Providers[0].ID; got != "anthropic" {
		t.Errorf("the first provider is %q, want anthropic", got)
	}
}

// TestSavingAKeyMasksItAndNeverSendsItBack is N18's one rule: the answer shows which key is stored
// and never the key. The middle is hidden, the last four characters are kept so two keys can be
// told apart, and the key itself is nowhere in the bytes.
func TestSavingAKeyMasksItAndNeverSendsItBack(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodPut, listProvidersPath+"/anthropic",
		protocol.SaveProviderRequest{Key: testAnthropicKey}).want(t, http.StatusOK)

	if strings.Contains(string(r.Body), testAnthropicKey) {
		t.Fatalf("the answer carries the key itself: %s", r.Body)
	}
	row := providerRow(t, decode[protocol.ProviderList](t, r), "anthropic")
	if row.Status != protocol.ProviderStatusSaved {
		t.Errorf("anthropic is %q, want saved", row.Status)
	}
	if !strings.HasSuffix(row.Masked, testAnthropicKey[len(testAnthropicKey)-4:]) {
		t.Errorf("the masked value %q does not end in the key's last four characters", row.Masked)
	}
	if strings.Contains(row.Masked, testAnthropicKey[8:len(testAnthropicKey)-4]) {
		t.Errorf("the masked value %q shows the middle of the key", row.Masked)
	}

	// A read afterwards says the same thing, so the save really is stored rather than echoed back.
	list := decode[protocol.ProviderList](t, st.do(http.MethodGet, listProvidersPath, nil).want(t, http.StatusOK))
	if got := providerRow(t, list, "anthropic").Status; got != protocol.ProviderStatusSaved {
		t.Errorf("after a read, anthropic is %q, want saved", got)
	}

	st.mustNotLog(testAnthropicKey)
}

// TestSavingAKeyLeavesEveryOtherProviderAlone is the row-per-provider rule: saving one key must not
// make another provider look set up.
func TestSavingAKeyLeavesEveryOtherProviderAlone(t *testing.T) {
	st := newStack(t)
	st.do(http.MethodPut, listProvidersPath+"/openai",
		protocol.SaveProviderRequest{Key: "sk-proj-not-real-6b7c"}).want(t, http.StatusOK)

	list := decode[protocol.ProviderList](t, st.do(http.MethodGet, listProvidersPath, nil).want(t, http.StatusOK))
	if got := providerRow(t, list, "openai").Status; got != protocol.ProviderStatusSaved {
		t.Errorf("openai is %q, want saved", got)
	}
	if got := providerRow(t, list, "anthropic").Status; got != protocol.ProviderStatusEmpty {
		t.Errorf("anthropic is %q, want empty: a key for one provider is not a key for another", got)
	}
}

// TestSavingAKeyASecondTimeReplacesIt is rotating a key: the new value is what is stored, so the
// masked value follows it.
func TestSavingAKeyASecondTimeReplacesIt(t *testing.T) {
	st := newStack(t)
	first := "sk-ant-api03-not-real-1111"
	second := "sk-ant-api03-not-real-2222"

	r := st.do(http.MethodPut, listProvidersPath+"/anthropic",
		protocol.SaveProviderRequest{Key: first}).want(t, http.StatusOK)
	before := providerRow(t, decode[protocol.ProviderList](t, r), "anthropic").Masked

	r = st.do(http.MethodPut, listProvidersPath+"/anthropic",
		protocol.SaveProviderRequest{Key: second}).want(t, http.StatusOK)
	after := providerRow(t, decode[protocol.ProviderList](t, r), "anthropic").Masked

	if before == after {
		t.Errorf("the masked value did not change: %q both times", before)
	}
	if !strings.HasSuffix(after, "2222") {
		t.Errorf("the masked value %q does not follow the key that was saved last", after)
	}
}

// TestRemovingAKeyLeavesTheProviderListedAndEmpty is forgetting a key: the row stays, so the screen
// can still offer to add one.
func TestRemovingAKeyLeavesTheProviderListedAndEmpty(t *testing.T) {
	st := newStack(t)
	st.do(http.MethodPut, listProvidersPath+"/anthropic",
		protocol.SaveProviderRequest{Key: testAnthropicKey}).want(t, http.StatusOK)

	r := st.do(http.MethodDelete, listProvidersPath+"/anthropic", nil).want(t, http.StatusOK)
	row := providerRow(t, decode[protocol.ProviderList](t, r), "anthropic")
	if row.Status != protocol.ProviderStatusEmpty || row.Masked != "" {
		t.Fatalf("anthropic is %q with %q, want empty", row.Status, row.Masked)
	}
}

// TestRemovingAKeyThatWasNeverSavedIsNotAnError is removing an avatar that is not there: the answer
// is the same list, so clearing a field twice behaves the same both times.
func TestRemovingAKeyThatWasNeverSavedIsNotAnError(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodDelete, listProvidersPath+"/openai", nil).want(t, http.StatusOK)
	if got := providerRow(t, decode[protocol.ProviderList](t, r), "openai").Status; got != protocol.ProviderStatusEmpty {
		t.Errorf("openai is %q, want empty", got)
	}
}

// TestAKeyForAProviderMarshalDoesNotKnowIsNotFound keeps a typo from writing a secret nowhere: the
// answer names the address, because the address is what is wrong.
func TestAKeyForAProviderMarshalDoesNotKnowIsNotFound(t *testing.T) {
	st := newStack(t)
	got := st.do(http.MethodPut, listProvidersPath+"/closedai",
		protocol.SaveProviderRequest{Key: testAnthropicKey}).apiError(t, http.StatusNotFound, protocol.ErrorCodeNotFound)
	if !strings.Contains(got.Message, "model provider") {
		t.Errorf("message = %q, want it to name what could not be found", got.Message)
	}
	st.mustNotLog(testAnthropicKey)
}

// TestAKeyThatCouldNeverWorkIsRefusedBeforeItIsStored holds a value the provider's own check refuses
// to the words a person reads, in the form's own sentence, and stores nothing.
func TestAKeyThatCouldNeverWorkIsRefusedBeforeItIsStored(t *testing.T) {
	tests := []struct {
		name string
		path string
		key  string
		want string
	}{
		{"an empty key", "/anthropic", "", "Enter the Anthropic API key."},
		{"a key that starts with a space", "/anthropic", " " + testAnthropicKey, "cannot start or end with a space"},
		{"an empty local address", "/ollama", "", "Enter the address of the Ollama server"},
		{"a key pasted into a local address", "/ollama", "sk-ant-api03-not-real-7c41", "is not an http address"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := newStack(t)
			got := st.do(http.MethodPut, listProvidersPath+tc.path,
				protocol.SaveProviderRequest{Key: tc.key}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
			if !strings.Contains(got.Message, tc.want) {
				t.Errorf("message = %q, want it to contain %q", got.Message, tc.want)
			}
			list := decode[protocol.ProviderList](t, st.do(http.MethodGet, listProvidersPath, nil).want(t, http.StatusOK))
			if row := providerRow(t, list, strings.TrimPrefix(tc.path, "/")); row.Status != protocol.ProviderStatusEmpty {
				t.Errorf("%s is %q after a refused save, want empty", row.ID, row.Status)
			}
		})
	}
}

// TestALocalProviderStoresAnAddressAndShowsItAsItIs is the local-provider rule: the stored value is
// a server address, not a secret, so the screen is shown the whole thing rather than a mask.
func TestALocalProviderStoresAnAddressAndShowsItAsItIs(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodPut, listProvidersPath+"/ollama",
		protocol.SaveProviderRequest{Key: testLocalAddress}).want(t, http.StatusOK)

	row := providerRow(t, decode[protocol.ProviderList](t, r), "ollama")
	if row.Status != protocol.ProviderStatusSaved {
		t.Fatalf("ollama is %q, want saved", row.Status)
	}
	if row.Masked != testLocalAddress {
		t.Errorf("the stored value shows as %q, want the address %q as it is", row.Masked, testLocalAddress)
	}
	if !row.Local {
		t.Error("ollama is not marked local, so the screen would ask for a key instead of an address")
	}
}

// TestAProviderBodyWithAFieldMarshalDoesNotKnowIsRefused is the body rule every route has: a field
// the wire type does not have is refused by name, so a client that misspells `key` is told.
func TestAProviderBodyWithAFieldMarshalDoesNotKnowIsRefused(t *testing.T) {
	st := newStack(t)
	got := st.do(http.MethodPut, listProvidersPath+"/anthropic",
		`{"secret": "`+testAnthropicKey+`"}`).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if !strings.Contains(got.Message, "secret") {
		t.Errorf("message = %q, want it to name the field it will not take", got.Message)
	}
	st.mustNotLog(testAnthropicKey)
}

// TestLimitsAreEmptyOnAFreshInstall is the shipped state: nothing is set up, so nothing is limited,
// and the answer is an empty list rather than a missing field.
func TestLimitsAreEmptyOnAFreshInstall(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodGet, limitsPath, nil).want(t, http.StatusOK)
	if !strings.Contains(string(r.Body), `"limits":[]`) {
		t.Errorf("a fresh install's answer is %s, want an empty list", r.Body)
	}
	if got := decode[protocol.LimitList](t, r); len(got.Limits) != 0 {
		t.Errorf("limits = %v, want none", got.Limits)
	}
}

// TestSettingALimitAnswersWithTheWholeList is the one shape every limits call has: whatever changed,
// the screen redraws its form from the answer.
func TestSettingALimitAnswersWithTheWholeList(t *testing.T) {
	st := newStack(t)
	r := st.do(http.MethodPut, limitsPath+"/global/cost-day",
		protocol.SetLimitRequest{Value: 25_000_000}).want(t, http.StatusOK)

	got := decode[protocol.LimitList](t, r)
	if len(got.Limits) != 1 {
		t.Fatalf("limits = %v, want the one that was just set", got.Limits)
	}
	want := protocol.Limit{Scope: "global", Kind: protocol.LimitKindCostDay, Value: 25_000_000}
	if got.Limits[0] != want {
		t.Errorf("limit = %v, want %v", got.Limits[0], want)
	}

	// And a read says the same, so the ceiling is stored rather than echoed.
	list := decode[protocol.LimitList](t, st.do(http.MethodGet, limitsPath, nil).want(t, http.StatusOK))
	if len(list.Limits) != 1 || list.Limits[0] != want {
		t.Errorf("after a read, limits = %v, want %v", list.Limits, want)
	}
}

// TestTheThreeKindsAndASecondScopeKeepTheirOwnCeilings is the whole form: one scope holds its three
// ceilings side by side, and a second scope's are its own. A project's id is its own slug, which is
// what the fixture's are.
func TestTheThreeKindsAndASecondScopeKeepTheirOwnCeilings(t *testing.T) {
	st := newStack(t)
	for _, set := range []struct {
		path  string
		value int64
	}{
		{limitsPath + "/global/cost-day", 25_000_000},
		{limitsPath + "/global/cost-month", 400_000_000},
		{limitsPath + "/global/awake", 18},
		{limitsPath + "/web-dashboard/cost-day", 8_000_000},
		{limitsPath + "/web-dashboard/awake", 6},
	} {
		st.do(http.MethodPut, set.path, protocol.SetLimitRequest{Value: set.value}).want(t, http.StatusOK)
	}

	got := decode[protocol.LimitList](t, st.do(http.MethodGet, limitsPath, nil).want(t, http.StatusOK))
	want := []protocol.Limit{
		{Scope: "global", Kind: protocol.LimitKindCostDay, Value: 25_000_000},
		{Scope: "global", Kind: protocol.LimitKindCostMonth, Value: 400_000_000},
		{Scope: "global", Kind: protocol.LimitKindAwake, Value: 18},
		{Scope: "web-dashboard", Kind: protocol.LimitKindCostDay, Value: 8_000_000},
		{Scope: "web-dashboard", Kind: protocol.LimitKindAwake, Value: 6},
	}
	if len(got.Limits) != len(want) {
		t.Fatalf("limits = %v, want %v", got.Limits, want)
	}
	for i := range want {
		if got.Limits[i] != want[i] {
			// The order is the screen's: the global scope first, and within a scope the kinds in
			// the form's order rather than the alphabet's.
			t.Fatalf("limits[%d] = %v, want %v (in the form's order)", i, got.Limits[i], want[i])
		}
	}
}

// TestALimitOfZeroIsRefused is the one value rule, in the form's own sentence: zero is not a way to
// say "no limit", and nothing is written.
func TestALimitOfZeroIsRefused(t *testing.T) {
	st := newStack(t)
	got := st.do(http.MethodPut, limitsPath+"/global/cost-day",
		protocol.SetLimitRequest{Value: 0}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
	if got.Message != "Enter a limit above zero." {
		t.Errorf("message = %q, want the form's own sentence", got.Message)
	}
	if body := string(st.do(http.MethodGet, limitsPath, nil).want(t, http.StatusOK).Body); !strings.Contains(body, `"limits":[]`) {
		t.Errorf("a refused limit left %s, want nothing set", body)
	}
}

// TestALimitOfAKindMarshalDoesNotKnowIsRefused keeps a word nothing reads out of the table: the kind
// decides what a value's unit is.
func TestALimitOfAKindMarshalDoesNotKnowIsRefused(t *testing.T) {
	st := newStack(t)
	for _, kind := range []string{"spend", "cost", "awake-cards", "cost_day"} {
		got := st.do(http.MethodPut, limitsPath+"/global/"+kind,
			protocol.SetLimitRequest{Value: 1}).apiError(t, http.StatusBadRequest, protocol.ErrorCodeInvalidArgument)
		if !strings.Contains(got.Message, kind) {
			t.Errorf("message = %q, want it to name the kind %q it will not take", got.Message, kind)
		}
	}
}

// TestDeletingALimitRemovesItAndTheRestStay is clearing one field: the ceiling is gone and the others
// are not.
func TestDeletingALimitRemovesItAndTheRestStay(t *testing.T) {
	st := newStack(t)
	st.do(http.MethodPut, limitsPath+"/global/cost-day", protocol.SetLimitRequest{Value: 25_000_000}).want(t, http.StatusOK)
	st.do(http.MethodPut, limitsPath+"/global/awake", protocol.SetLimitRequest{Value: 18}).want(t, http.StatusOK)

	r := st.do(http.MethodDelete, limitsPath+"/global/cost-day", nil).want(t, http.StatusOK)
	got := decode[protocol.LimitList](t, r)
	want := []protocol.Limit{{Scope: "global", Kind: protocol.LimitKindAwake, Value: 18}}
	if len(got.Limits) != 1 || got.Limits[0] != want[0] {
		t.Fatalf("limits = %v, want %v", got.Limits, want)
	}
}

// TestDeletingALimitThatIsNotSetIsNotAnError is clearing a field that is already empty: the answer is
// the same list.
func TestDeletingALimitThatIsNotSetIsNotAnError(t *testing.T) {
	st := newStack(t)
	st.do(http.MethodDelete, limitsPath+"/global/cost-month", nil).want(t, http.StatusOK)
	st.do(http.MethodDelete, limitsPath+"/global/cost-month", nil).want(t, http.StatusOK)
}

// TestTheProviderRoutesAreNotThereWithoutTheirService is the registration rule: no keychain, no
// provider routes, and no token can change that.
func TestTheProviderRoutesAreNotThereWithoutTheirService(t *testing.T) {
	st := newStack(t, withoutProviders())
	for _, tc := range []struct {
		method string
		body   any
	}{
		{http.MethodGet, nil},
		{http.MethodPut, protocol.SaveProviderRequest{Key: testAnthropicKey}},
		{http.MethodDelete, nil},
	} {
		st.do(tc.method, listProvidersPath+"/anthropic", tc.body).want(t, http.StatusNotFound)
	}
	// The limits routes need their own service and are still there, so the two are wired apart.
	st.do(http.MethodGet, limitsPath, nil).want(t, http.StatusOK)
	st.mustNotLog(testAnthropicKey)
}

// TestTheLimitsRoutesAreNotThereWithoutTheirService is the same rule for the ceilings, and it proves
// a daemon with a key saved but no store can still list its providers.
func TestTheLimitsRoutesAreNotThereWithoutTheirService(t *testing.T) {
	st := newStack(t, withoutCostLimits())
	st.do(http.MethodGet, limitsPath, nil).want(t, http.StatusNotFound)
	st.do(http.MethodPut, limitsPath+"/global/cost-day", protocol.SetLimitRequest{Value: 1}).want(t, http.StatusNotFound)
	st.do(http.MethodGet, listProvidersPath, nil).want(t, http.StatusOK)
}
