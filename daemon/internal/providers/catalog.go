package providers

// This file is the one list of the model providers Marshal knows. Everything else in Marshal reads
// it: the settings screen's rows, the models the built-in agent's picker offers, and the resolver
// that turns a card's model id into a provider to call. A provider is added here once, and every
// reader picks it up.
//
// The list is fixed and short on purpose. Marshal talks to a provider only when it has an adapter
// for it (this package's Client), so the set is not "every provider that exists" but "every
// provider Marshal can call" - which is exactly the set a settings screen should show, and exactly
// the set a card's model can resolve to.

import (
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// DefaultModel is the model a session runs when the caller names none. It is the first model of the
// first provider in the list, which is also the model the design's built-in agent starts on.
const DefaultModel = "claude-sonnet-4-5"

// Info is one model provider as Marshal knows it before anything about this install is known: what
// it is called, how to reach it, which adapter serves it, and which models Marshal can ask it for.
//
// Nothing here is secret and nothing here is per-install. Whether a provider is set up, and with
// what key, is the Service's business (service.go).
type Info struct {
	// ID is the provider's own id, such as "anthropic". It is the id the keychain files the secret
	// under, the id a route names, and what an adapter's ID method returns.
	ID string
	// Name is the words shown to people, such as "Google Gemini".
	Name string
	// ModelsPhrase is one plain phrase naming what the provider offers, such as "Claude models". It
	// is a description for the provider's row on the settings screen - the wire's `models` field -
	// not a list; the list of what a person can pick is Models below.
	ModelsPhrase string
	// Local is true for a provider that runs on this machine and needs a server address rather than
	// a key (Ollama, LM Studio). The screens swap the key field for a URL field, and the stored
	// value is that address.
	Local bool
	// Family says which adapter serves the provider. It is one of the Family constants in
	// thinking.go, and it is what decides how a thinking mode is sent.
	Family string
	// BaseURL is where the provider answers. Empty means the adapter's own default, which is right
	// for the providers whose SDK knows its own address. For a local provider it is the address a
	// person is expected to enter, since what is stored replaces it.
	BaseURL string
	// AnyModel is true for a provider that serves model ids Marshal has no table for - OpenRouter,
	// which proxies many providers' models, and LM Studio, which runs whatever the person has
	// loaded. Such a provider is never the first choice for a model that a table names; it is what
	// answers when nothing else can (see Service.Resolve), and it adds no models of its own to a
	// picker, because Marshal cannot know them without asking the provider.
	AnyModel bool
	// Models are the models Marshal knows this provider serves, in the order to show them. A
	// provider whose list Marshal cannot know has none (see AnyModel).
	Models []protocol.AgentModel
}

// known is every provider Marshal knows, in the order the settings screen lists them
// (design/store.js's providers, plus LM Studio). Adding a provider here is what makes it appear on
// that screen, resolve a card's model, and count as a provider the built-in agent can run.
var known = []Info{
	{
		ID: AnthropicID, Name: "Anthropic", ModelsPhrase: "Claude models",
		Family: FamilyAnthropic,
		Models: []protocol.AgentModel{
			{ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5", Thinking: true},
		},
	},
	{
		ID: OpenAIID, Name: "OpenAI", ModelsPhrase: "GPT models",
		Family: FamilyOpenAI,
		Models: []protocol.AgentModel{
			{ID: "gpt-5-mini", Name: "GPT-5 mini", Thinking: true},
		},
	},
	{
		ID: GeminiID, Name: "Google Gemini", ModelsPhrase: "Gemini models",
		Family: FamilyGemini,
		Models: []protocol.AgentModel{
			{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Thinking: true},
		},
	},
	{
		ID: DeepSeekID, Name: "DeepSeek", ModelsPhrase: "DeepSeek models",
		Family: FamilyOpenAI, BaseURL: "https://api.deepseek.com/v1",
		Models: []protocol.AgentModel{
			// DeepSeek's chat model has no reasoning-effort setting, so a card cannot ask it to
			// think harder; its Thinking flag is false for that reason and not because Marshal
			// cannot set one.
			{ID: "deepseek-chat", Name: "DeepSeek Chat", Thinking: false},
		},
	},
	{
		ID: OpenRouterID, Name: "OpenRouter", ModelsPhrase: "Any model on OpenRouter",
		Family: FamilyOpenAI, BaseURL: "https://openrouter.ai/api/v1", AnyModel: true,
	},
	{
		ID: OllamaID, Name: "Ollama", ModelsPhrase: "Local models",
		Family: FamilyOpenAI, Local: true, BaseURL: "http://127.0.0.1:11434/v1",
		Models: []protocol.AgentModel{
			{ID: "qwen2.5-coder:32b", Name: "Qwen2.5 Coder 32B", Thinking: false},
		},
	},
	{
		ID: LMStudioID, Name: "LM Studio", ModelsPhrase: "Local models",
		Family: FamilyOpenAI, Local: true, BaseURL: "http://127.0.0.1:1234/v1", AnyModel: true,
	},
}

// builtinModelOrder is the order the built-in agent's model picker offers models in, first thing
// first: the default model is the first entry, which is what a card runs when it names none.
//
// It is written out rather than derived from the provider rows' order because it is the design's
// own list for that agent (design/store.js's AGENTS table) and that list interleaves the providers
// - Claude, then GPT, then DeepSeek, then Gemini, then the local model - while the settings screen
// lists providers in its own order. Every model the providers name appears here exactly once, and
// the tests hold the two lists to that (TestEveryModelIsInThePickerOrder), so a model added to a
// provider cannot silently fall out of a picker.
var builtinModelOrder = []string{
	"claude-sonnet-4-5",
	"gpt-5-mini",
	"deepseek-chat",
	"gemini-2.5-flash",
	"qwen2.5-coder:32b",
}

// Known returns every provider Marshal knows, in the order the screens list them. The entries are
// copies, so a caller that changes one - a route filling in a status - cannot change the table.
func Known() []Info {
	out := make([]Info, len(known))
	for i, info := range known {
		info.Models = append([]protocol.AgentModel(nil), info.Models...)
		out[i] = info
	}
	return out
}

// Lookup returns what Marshal knows about a provider id, and whether it knows it at all.
func Lookup(id string) (Info, bool) {
	for _, info := range known {
		if info.ID == id {
			return info, true
		}
	}
	return Info{}, false
}

// modelOwner finds the provider that names a model, and its entry for it. It is how a model id with
// no provider beside it - which is every model a card holds - finds the provider to call.
func modelOwner(id string) (Info, protocol.AgentModel, bool) {
	for _, info := range known {
		if model, ok := info.Model(id); ok {
			return info, model, true
		}
	}
	return Info{}, protocol.AgentModel{}, false
}

// Model returns the provider's entry for a model id, and whether it has one. A provider that serves
// models Marshal cannot list (see AnyModel) has no entries, so it answers false for every id.
func (i Info) Model(id string) (protocol.AgentModel, bool) {
	for _, model := range i.Models {
		if model.ID == id {
			return model, true
		}
	}
	return protocol.AgentModel{}, false
}

// Cheapest returns the model of this provider Marshal would spend the least on, and whether it can
// name one. It is what a provider's connection test asks for one token from (test.go): the test has
// to cost something, and the point of it is to prove the key, not to spend anything.
//
// "Cheapest" is judged by Marshal's own price table (pricing.go), which is the only price it knows.
// A model the table does not hold is not a candidate (PriceOf: a price nobody knows is not a price of
// nothing), but a provider whose models are all unpriced still answers with its first model rather
// than reporting that it cannot be tested at all. A provider whose models Marshal cannot list
// (Info.AnyModel: OpenRouter, LM Studio) has nothing to choose from and answers false.
func (i Info) Cheapest() (protocol.AgentModel, bool) {
	if model, _, priced := i.cheapestPriced(); priced {
		return model, true
	}
	if len(i.Models) == 0 {
		return protocol.AgentModel{}, false
	}
	return i.Models[0], true
}

// cheapestPriced is the half of Cheapest that only answers when a price is known: this provider's
// cheapest priced model and what it costs, in micro-dollars per million tokens read and written, or
// false when the provider has no priced model at all. It is what makes one provider comparable with
// another - Service.cheapest (title.go) uses it to pick the cheapest provider that is set up - and it
// is deliberately stricter than Cheapest, because Marshal may not hand work to a provider on the
// strength of a price it guessed.
func (i Info) cheapestPriced() (protocol.AgentModel, int64, bool) {
	var best protocol.AgentModel
	var bestCost int64
	found := false
	for _, model := range i.Models {
		price, ok := PriceOf(model.ID)
		if !ok {
			continue
		}
		cost := price.InputMicros + price.OutputMicros
		if !found || cost < bestCost {
			best, bestCost, found = model, cost, true
		}
	}
	return best, bestCost, found
}
