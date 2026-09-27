package providers

import "testing"

// designBuiltinModels are the models the design's own fixtures list for the built-in agent
// (design/store.js's AGENTS table and apps/web/src/mock/constants.ts). They are the contract the
// table has to meet: a card's model picker offers the models of the providers that are set up, so
// the providers Marshal knows must add up to exactly these, with the design's own thinking flags.
var designBuiltinModels = []struct {
	id       string
	thinking bool
}{
	{"claude-sonnet-4-5", true},
	{"gpt-5-mini", true},
	{"deepseek-chat", false},
	{"gemini-2.5-flash", true},
	{"qwen2.5-coder:32b", false},
}

// modelOwners maps every model id in the table to the provider that offers it, failing the test when
// an id is offered twice or a model carries no name.
func modelOwners(t *testing.T) map[string]Info {
	t.Helper()
	owners := map[string]Info{}
	for _, info := range Known() {
		for _, model := range info.Models {
			if model.Name == "" {
				t.Errorf("%s of %s has no name to show in a picker", model.ID, info.ID)
			}
			if other, seen := owners[model.ID]; seen {
				t.Errorf("%s is offered by both %s and %s, so a card naming it is ambiguous",
					model.ID, other.ID, info.ID)
			}
			owners[model.ID] = info
		}
	}
	return owners
}

func TestKnownProvidersAreWhole(t *testing.T) {
	infos := Known()
	if len(infos) == 0 {
		t.Fatal("Marshal knows no providers, so the built-in agent could never run")
	}
	families := map[string]bool{FamilyAnthropic: true, FamilyOpenAI: true, FamilyGemini: true}
	seen := map[string]bool{}
	for _, info := range infos {
		switch {
		case info.ID == "":
			t.Errorf("%q has no id, so no route or key could name it", info.Name)
		case seen[info.ID]:
			t.Errorf("%s is in the table twice", info.ID)
		}
		seen[info.ID] = true
		if info.Name == "" || info.ModelsPhrase == "" {
			t.Errorf("%s is missing the name or the phrase its row shows", info.ID)
		}
		if !families[info.Family] {
			t.Errorf("%s is in the family %q, which has no adapter to call it", info.ID, info.Family)
		}
		if info.AnyModel && len(info.Models) > 0 {
			t.Errorf("%s serves any model and also lists models, so its list would never be read",
				info.ID)
		}
		if info.Local && info.BaseURL == "" {
			t.Errorf("%s runs on this machine but suggests no address to enter", info.ID)
		}
	}
	modelOwners(t)
}

// TestTheTableRunsTheDesignsModels is the tie between this table and the screens: the five models
// the design gives the built-in agent, with the design's own thinking flags, are exactly what the
// providers add up to.
func TestTheTableRunsTheDesignsModels(t *testing.T) {
	owners := modelOwners(t)
	for _, want := range designBuiltinModels {
		owner, ok := owners[want.id]
		if !ok {
			t.Errorf("the design runs %s on the built-in agent, but no provider offers it", want.id)
			continue
		}
		model, ok := owner.Model(want.id)
		if !ok {
			t.Fatalf("%s is not in %s's own list", want.id, owner.ID)
		}
		if model.Thinking != want.thinking {
			t.Errorf("%s: thinking is %v, want %v", want.id, model.Thinking, want.thinking)
		}
	}
	if len(owners) != len(designBuiltinModels) {
		t.Errorf("the table offers %d models, want the design's %d: %v",
			len(owners), len(designBuiltinModels), owners)
	}
}

func TestDefaultModelIsAModelTheTableHas(t *testing.T) {
	owners := modelOwners(t)
	owner, ok := owners[DefaultModel]
	if !ok {
		t.Fatalf("the default model %s is not offered by any provider", DefaultModel)
	}
	first := Known()[0]
	if owner.ID != first.ID {
		t.Errorf("the default model belongs to %s, want the first provider %s", owner.ID, first.ID)
	}
	if first.Models[0].ID != DefaultModel {
		t.Errorf("the first provider's first model is %s, want the default %s",
			first.Models[0].ID, DefaultModel)
	}
}

// TestEveryModelIsInThePickerOrder holds the picker's order and the providers' model lists
// together: every model a provider names is offered in the picker exactly once, and the picker
// offers nothing the providers do not have. A model added to a provider without being added here
// would otherwise never reach a picker.
func TestEveryModelIsInThePickerOrder(t *testing.T) {
	owners := modelOwners(t)
	seen := map[string]bool{}
	for _, id := range builtinModelOrder {
		if seen[id] {
			t.Errorf("%s is in the picker's order twice", id)
		}
		seen[id] = true
		if _, ok := owners[id]; !ok {
			t.Errorf("the picker offers %s, which no provider names", id)
		}
	}
	for id := range owners {
		if !seen[id] {
			t.Errorf("%s is offered by a provider but is missing from the picker's order", id)
		}
	}
	if len(builtinModelOrder) == 0 {
		t.Error("the picker's order is empty")
	}
	if DefaultModel != builtinModelOrder[0] {
		t.Errorf("the default model %s is not the first the picker offers, %s",
			DefaultModel, builtinModelOrder[0])
	}
}

func TestLookupFindsKnownProvidersAndRefusesOthers(t *testing.T) {
	for _, info := range Known() {
		got, ok := Lookup(info.ID)
		if !ok {
			t.Errorf("Lookup does not find %s, which is in the table", info.ID)
			continue
		}
		if got.Name != info.Name {
			t.Errorf("Lookup returned %q for %s, want %q", got.Name, info.ID, info.Name)
		}
	}
	if _, ok := Lookup("gemini-cli"); ok {
		t.Error("Lookup found a provider Marshal has no adapter for")
	}
	if _, ok := Lookup(""); ok {
		t.Error("Lookup found a provider with an empty id")
	}
}

// TestKnownHandsOutCopies proves a caller cannot change the table through what it is given, which
// matters because the API layer fills in a status on each entry it is handed.
func TestKnownHandsOutCopies(t *testing.T) {
	first := Known()
	before := first[0].Models[0].Name
	first[0].Models[0].Name = "changed"
	first[0].Name = "changed"

	again := Known()
	if again[0].Models[0].Name != before || again[0].Name == "changed" {
		t.Error("changing a returned entry changed the table")
	}
}

func TestModelFindsOnlyWhatTheProviderOffers(t *testing.T) {
	anthropic, ok := Lookup(AnthropicID)
	if !ok {
		t.Fatal("Anthropic is not in the table")
	}
	if _, ok := anthropic.Model("claude-sonnet-4-5"); !ok {
		t.Error("Anthropic does not offer its own model")
	}
	if _, ok := anthropic.Model("gpt-5-mini"); ok {
		t.Error("Anthropic offers another provider's model")
	}
	openrouter, ok := Lookup(OpenRouterID)
	if !ok {
		t.Fatal("OpenRouter is not in the table")
	}
	if _, ok := openrouter.Model(DefaultModel); ok {
		t.Error("a provider that serves any model also claims to have a list")
	}
}
