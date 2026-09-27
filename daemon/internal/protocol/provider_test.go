package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

var providersNow = time.Date(2026, time.September, 27, 9, 30, 0, 0, time.UTC)

// sampleProviders has one provider of each status and one local provider, so the golden file shows
// every field in use: a saved key, a provider nobody has set up, a refused key with the sentence
// that says what to do, and a local provider whose "key" is a server address.
func sampleProviders() []protocol.Provider {
	return []protocol.Provider{
		{
			ID: "anthropic", Name: "Anthropic", Status: protocol.ProviderStatusSaved,
			Masked: "sk-ant-…4f2a", Models: "Claude models",
		},
		{
			ID: "deepseek", Name: "DeepSeek", Status: protocol.ProviderStatusEmpty,
			Masked: "", Models: "DeepSeek models",
		},
		{
			ID: "openrouter", Name: "OpenRouter", Status: protocol.ProviderStatusInvalid,
			Masked: "sk-or-…0b33", Models: "Any model on OpenRouter",
			Error: "OpenRouter rejected this key. Create a new key at openrouter.ai/keys and paste it here.",
		},
		{
			ID: "ollama", Name: "Ollama", Status: protocol.ProviderStatusSaved,
			Masked: "http://localhost:11434", Models: "Local models", Local: true,
		},
	}
}

func TestProviderListGolden(t *testing.T) {
	testutil.Golden(t, "provider-list", protocol.NewProviderList(sampleProviders(), providersNow))
}

func TestLimitListGolden(t *testing.T) {
	limits := []protocol.Limit{
		{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindCostDay, Value: 25_000_000},
		{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindCostMonth, Value: 400_000_000},
		{Scope: protocol.LimitScopeGlobal, Kind: protocol.LimitKindAwake, Value: 18},
		{Scope: "01JD7Q4M2X8K9V0P5T3RB6NHAE", Kind: protocol.LimitKindCostDay, Value: 5_000_000},
	}
	testutil.Golden(t, "limit-list", protocol.NewLimitList(limits))
}

func TestProviderListsNeverEncodeNull(t *testing.T) {
	list, err := json.Marshal(protocol.NewProviderList(nil, providersNow))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(list); !strings.Contains(got, `"providers":[]`) {
		t.Errorf("an empty provider list encoded as %s, want []", got)
	}

	limits, err := json.Marshal(protocol.NewLimitList(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(limits); !strings.Contains(got, `"limits":[]`) {
		t.Errorf("an empty limit list encoded as %s, want []", got)
	}
}

func TestProviderStatusAndLimitKindValidateTheirWords(t *testing.T) {
	if !protocol.ProviderStatusSaved.Valid() || protocol.ProviderStatus("saved!").Valid() {
		t.Error("ProviderStatus.Valid accepts the wrong words")
	}
	if !protocol.LimitKindCostDay.Valid() || protocol.LimitKind("spend").Valid() {
		t.Error("LimitKind.Valid accepts the wrong words")
	}
}
