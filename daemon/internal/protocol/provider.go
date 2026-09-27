package protocol

import "time"

// ProviderStatus says whether a provider has a usable key.
type ProviderStatus string

const (
	// ProviderStatusSaved means a key (or a server URL, for a local provider) is stored and
	// nothing is known to be wrong with it.
	ProviderStatusSaved ProviderStatus = "saved"
	// ProviderStatusEmpty means no key is stored, so the provider cannot be used yet.
	ProviderStatusEmpty ProviderStatus = "empty"
	// ProviderStatusInvalid means a key is stored and the last check refused it.
	ProviderStatusInvalid ProviderStatus = "invalid"
)

// ProviderStatusValues lists every provider status.
func ProviderStatusValues() []ProviderStatus {
	return []ProviderStatus{ProviderStatusSaved, ProviderStatusEmpty, ProviderStatusInvalid}
}

// Valid reports whether s is a provider status.
func (s ProviderStatus) Valid() bool {
	for _, v := range ProviderStatusValues() {
		if v == s {
			return true
		}
	}
	return false
}

// LimitKind says what a limit measures: how much money a scope may spend in a day or in a month, or
// how many cards it may keep awake at once. There are three kinds rather than one cost kind because
// the settings screen has three fields per scope - a daily cost limit, a monthly cost limit, and an
// awake card limit (apps/web/src/views/settings/limit-rows.ts) - and each has its own ceiling.
type LimitKind string

const (
	// LimitKindCostDay is a ceiling on one day's spending, in micro-dollars.
	LimitKindCostDay LimitKind = "cost-day"
	// LimitKindCostMonth is a ceiling on one month's spending, in micro-dollars.
	LimitKindCostMonth LimitKind = "cost-month"
	// LimitKindAwake is a ceiling on how many cards a scope may keep awake at once, counted in
	// cards, not in milliseconds. It is a count of awake cards, so it fills when a new card must
	// wake and the limit is already full (docs/architecture.md 5.2, "when a new card must wake and
	// the limit is full, the oldest idle awake card gets a sleep warning").
	LimitKindAwake LimitKind = "awake"
)

// LimitKindValues lists every kind of limit, in the order the settings screen shows them.
func LimitKindValues() []LimitKind {
	return []LimitKind{LimitKindCostDay, LimitKindCostMonth, LimitKindAwake}
}

// Valid reports whether k is a kind of limit.
func (k LimitKind) Valid() bool {
	for _, v := range LimitKindValues() {
		if v == k {
			return true
		}
	}
	return false
}

// LimitScopeGlobal is the scope a limit that covers the whole install has. Any other scope is a
// project id. It is the same word the limits table stores (migration 0014).
const LimitScopeGlobal = "global"

// Provider is one model provider as a screen sees it: what it is called, whether a key is stored,
// and enough to show the masked key without ever sending the key itself. The key never leaves the
// daemon: Masked is made on the server from the stored key, and is the only form a client ever has
// (docs/backend-inventory.md N18).
type Provider struct {
	// ID is the provider's own id, such as "anthropic". It is the key the keychain stores the
	// secret under, so it is also what a save or a remove names.
	ID string `json:"id"`
	// Name is the words shown to people, such as "Google Gemini".
	Name string `json:"name"`
	// Status says whether a key is stored and usable. The field is called Status in Go and `st`
	// on the wire because `st` is the name the screens already read for this row.
	Status ProviderStatus `json:"st"`
	// Masked is the key with its middle hidden, such as "sk-ant-…4f2a". For a local provider it
	// is the server URL, which is not a secret. It is empty when no key is stored.
	Masked string `json:"masked"`
	// Models is one plain phrase naming what the provider offers, such as "Claude models". It is
	// a description for the row, not a list: the models a person can pick are the agent
	// catalog's, and they come from the providers that are set up.
	Models string `json:"models"`
	// Error is one plain sentence saying what to do about a key the last check refused. It is
	// empty unless Status is invalid.
	Error string `json:"error"`
	// Local is true for a provider that runs on this machine and needs a server address rather
	// than a key (Ollama, LM Studio). The screens swap the key field for a URL field.
	Local bool `json:"local"`
	// LastTest is the result of the last connection test of this provider, or nil when it has
	// never been tested. It is the same shape every connection's test answers with
	// (connection.go), so a screen shows a provider's test and an integration's test the same way.
	// It is stored, not rebuilt: it is the last test's own answer, and it is what makes Status
	// invalid when the last test's key check failed.
	LastTest *TestResult `json:"lastTest,omitempty"`
}

// ProviderList is the answer to GET /v1/providers and to a change that returns the new list: every
// provider Marshal knows, in the order the screens show them.
type ProviderList struct {
	// Providers has one entry for every provider Marshal knows, whether or not a key is stored
	// for it, so the screen can show the ones that are not set up yet.
	Providers []Provider `json:"providers"`
	// ServerTime is the daemon's time when the answer was made.
	ServerTime Timestamp `json:"serverTime"`
}

// NewProviderList makes an answer stamped with the daemon's time. A nil list becomes an empty one,
// so the JSON has [] and never null.
func NewProviderList(providers []Provider, now time.Time) ProviderList {
	out := make([]Provider, len(providers))
	copy(out, providers)
	return ProviderList{Providers: out, ServerTime: NewTimestamp(now)}
}

// SaveProviderRequest is the body of a call that stores a provider's secret. The same field carries
// an API key and, for a local provider, the server URL: the person types one thing either way, and
// which it is depends on the provider, not on the request.
type SaveProviderRequest struct {
	// Key is the API key to store, or the server URL for a local provider.
	Key string `json:"key"`
}

// Limit is one ceiling: the most a scope may spend in a day or a month, or the most cards it may
// keep awake at once. Value is in the unit its kind measures - micro-dollars for a cost limit, a
// count of cards for the awake limit.
type Limit struct {
	// Scope is LimitScopeGlobal for the whole install, or a project id for one project.
	Scope string `json:"scope"`
	// Kind says what the ceiling measures.
	Kind LimitKind `json:"kind"`
	// Value is the ceiling in the unit Kind measures.
	Value int64 `json:"value"`
}

// LimitList is the answer to every limits call: the ceilings that are set, global ones first. It is
// the same shape whatever changed, so a screen redraws its form from one answer.
type LimitList struct {
	// Limits has one entry per ceiling that is set. A scope with no ceiling has no entry, so
	// this is empty on a fresh install.
	Limits []Limit `json:"limits"`
}

// NewLimitList makes a list. A nil list becomes an empty one, so the JSON has [] and never null.
func NewLimitList(limits []Limit) LimitList {
	out := make([]Limit, len(limits))
	copy(out, limits)
	return LimitList{Limits: out}
}

// SetLimitRequest is the body of a call that sets one ceiling. Which scope and kind it belongs to
// comes from the path, so the body carries only the number.
type SetLimitRequest struct {
	// Value is the new ceiling, in the unit the path's kind measures.
	Value int64 `json:"value"`
}
