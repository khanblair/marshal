package providers

import (
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Providers' own ids. They are the ids the rest of the daemon and the app use, and what an
// adapter's ID method returns.
const (
	// AnthropicID is Anthropic's Messages API.
	AnthropicID = "anthropic"
	// OpenAIID is OpenAI itself.
	OpenAIID = "openai"
	// OpenRouterID is OpenRouter, which serves many models behind one OpenAI-compatible address.
	OpenRouterID = "openrouter"
	// DeepSeekID is DeepSeek.
	DeepSeekID = "deepseek"
	// OllamaID is a local Ollama server, which needs no key.
	OllamaID = "ollama"
	// LMStudioID is a local LM Studio server, which needs no key.
	LMStudioID = "lmstudio"
	// GeminiID is Google's Gemini API.
	GeminiID = "gemini"
)

// Thinking families. A family is one provider's way of saying how hard a model should think, and
// each family has exactly one table below. Several providers can share a family - the whole
// OpenAI-compatible group does, since one adapter serves them all by swapping the base URL
// (docs/library-docs.md section 2.4) - so a new provider that is compatible with an existing one
// needs no new entry here.
const (
	// FamilyAnthropic is the Messages API: thinking is a token budget.
	FamilyAnthropic = "anthropic"
	// FamilyOpenAI is the chat-completions API: thinking is an effort word.
	FamilyOpenAI = "openai"
	// FamilyGemini is Google's GenerateContent API: thinking is a level word.
	FamilyGemini = "gemini"
)

// Level is how hard a model should think, as one provider-independent step. The protocol's
// ThinkingMode is turned into a Level once, here, and each family then reads its own vocabulary
// for that Level - a token budget for Anthropic, an effort word for the OpenAI-compatible family,
// a level word for Gemini. This is the "one place" docs/library-docs.md section 2.4 asks for, so a
// provider's thinking setting is never spread through the rest of the code.
type Level int

// The thinking levels, from the least to the most.
const (
	// LevelNone means the model thinks no more than it does by default.
	LevelNone Level = iota
	LevelLow
	LevelMedium
	LevelHigh
	LevelExtraHigh
)

// levelOf turns a protocol.ThinkingMode into a Level. An empty mode, or one that is not a thinking
// mode, is LevelNone: a card that names no mode leaves the provider's own default alone.
func levelOf(mode string) Level {
	switch protocol.ThinkingMode(mode) {
	case protocol.ThinkingModeLow:
		return LevelLow
	case protocol.ThinkingModeMedium:
		return LevelMedium
	case protocol.ThinkingModeHigh:
		return LevelHigh
	case protocol.ThinkingModeExtraHigh:
		return LevelExtraHigh
	default:
		return LevelNone
	}
}

// anthropicBudgets is how many tokens Anthropic lets its model think for a Level, or 0 for
// LevelNone, which means "do not think". Anthropic's smallest budget is 1,024 tokens; the steps
// above it are the ones Marshal's card settings offer.
var anthropicBudgets = map[Level]int64{
	LevelNone:      0,
	LevelLow:       2048,
	LevelMedium:    8192,
	LevelHigh:      16384,
	LevelExtraHigh: 32768,
}

// anthropicBudget returns the thinking budget to send Anthropic for mode, or 0 when thinking should
// be left to Anthropic's own default.
func anthropicBudget(mode string) int64 {
	return anthropicBudgets[levelOf(mode)]
}

// openAIEfforts is the effort word the OpenAI-compatible family uses for a Level, or "" for
// LevelNone, which means "do not ask". The family has no word above "high", so the two most
// thinking levels both ask for high: the highest a compatible provider understands.
var openAIEfforts = map[Level]string{
	LevelNone:      "",
	LevelLow:       "low",
	LevelMedium:    "medium",
	LevelHigh:      "high",
	LevelExtraHigh: "high",
}

// openAIEffort returns the reasoning effort to send an OpenAI-compatible provider for mode, or ""
// when thinking should be left alone. Providers that do not know the field ignore it.
func openAIEffort(mode string) string {
	return openAIEfforts[levelOf(mode)]
}

// geminiLevels is the level word Gemini uses for a Level, or "" for LevelNone, which means "do not
// ask". The levels are the API's own words rather than a token budget on purpose: a budget's
// allowed range depends on the model, so a number Marshal picked could be rejected by a model it
// was never measured against, while a level is what the API takes for every model that thinks.
// Gemini has no level above "high", so the two most thinking levels both ask for high.
var geminiLevels = map[Level]string{
	LevelNone:      "",
	LevelLow:       "LOW",
	LevelMedium:    "MEDIUM",
	LevelHigh:      "HIGH",
	LevelExtraHigh: "HIGH",
}

// geminiLevel returns the thinking level to send Gemini for mode, or "" when thinking should be
// left to Gemini's own default.
func geminiLevel(mode string) string {
	return geminiLevels[levelOf(mode)]
}
