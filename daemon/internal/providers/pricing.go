package providers

// This file turns a call's token counts into money. It is the "pricing" half of
// docs/project-structure.md's "internal/providers/ - Model providers, rate limits, usage, pricing",
// and it is the one place a price appears: nothing else in Marshal multiplies a token by a rate.
//
// Money is in micro-dollars (one millionth of a dollar) everywhere in the daemon - the usage table,
// daily_stats, and the cost limits are all integers of micro-dollars - so a price here is what a
// million tokens cost, in that same unit.

// Price is what a model costs, in micro-dollars per million tokens. A model that is free has a zero
// Price, which is a price known to be nothing rather than a price nobody knows (see PriceOf).
type Price struct {
	// InputMicros is the cost of a million tokens read.
	InputMicros int64
	// OutputMicros is the cost of a million tokens written.
	OutputMicros int64
}

// prices is what each model costs, in micro-dollars per million tokens: the providers' own list
// prices for the models Marshal ships (catalog.go). Three things are worth saying about it.
//
// It is a table because prices change and nothing here fetches one: a provider can move a price at
// any time, and this is the one place to correct when the cost meter looks wrong. Refresh it from
// the provider's own pricing page, which is also where the numbers below came from.
//
// It is per model and not per provider, because one provider's models are priced differently, and a
// model served through OpenRouter is billed by OpenRouter.
//
// A model that is not in it is not guessed at: the call is still recorded, with its tokens and a
// cost of zero, and the daemon logs that the model has no price (see Recorder). A missing price
// shows up as an unpriced model rather than as a made-up number.
func prices() map[string]Price {
	return map[string]Price{
		// Anthropic's Claude Sonnet tier.
		"claude-sonnet-4-5": {InputMicros: 3_000_000, OutputMicros: 15_000_000},
		// OpenAI's small GPT-5 tier.
		"gpt-5-mini": {InputMicros: 250_000, OutputMicros: 2_000_000},
		// DeepSeek's chat model, at its cache-miss price. A cache hit costs a fraction of this, and
		// Marshal does not ask which it was, so this is the upper bound of what a turn costs rather
		// than an exact figure.
		"deepseek-chat": {InputMicros: 280_000, OutputMicros: 420_000},
		// Gemini 2.5 Flash's standard tier.
		"gemini-2.5-flash": {InputMicros: 300_000, OutputMicros: 2_500_000},
		// A local model on the person's own machine: no provider bill, so no cost. It is here rather
		// than left out, because it is a price Marshal knows - nothing - and not one it does not.
		"qwen2.5-coder:32b": {},
	}
}

// microsPerMillion is the unit prices are given in. Money is stored as whole micro-dollars, so a
// cost is rounded to the nearest one rather than truncated: a truncating division would lose up to
// a micro-dollar on every call and, over a day of small calls, drift visibly against the provider's
// own total.
const (
	microsPerMillion = 1_000_000
	halfMillion      = microsPerMillion / 2
)

// PriceOf returns what a model costs per million tokens, and whether Marshal knows. A model that is
// not in the table answers false, which is "unknown", not "free".
func PriceOf(model string) (Price, bool) {
	price, ok := prices()[model]
	return price, ok
}

// Cost returns what one call cost, in micro-dollars, and whether the model has a known price. A
// model with no price answers 0 with false, so a caller can record the tokens and say plainly that
// it could not price them.
//
// The arithmetic is integers only: a token count times a price per million, rounded to the nearest
// micro-dollar.
func Cost(model string, usage Usage) (int64, bool) {
	price, ok := PriceOf(model)
	if !ok {
		return 0, false
	}
	micros := usage.InputTokens*price.InputMicros + usage.OutputTokens*price.OutputMicros
	if micros <= 0 {
		return 0, true
	}
	return (micros + halfMillion) / microsPerMillion, true
}
