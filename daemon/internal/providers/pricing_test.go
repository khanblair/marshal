package providers

import "testing"

// TestEveryModelMarshalShipsHasAPrice holds the price table to the catalog: a model a card can pick
// must be one Marshal can price, or its calls would silently be recorded at zero cost and the cost
// meter would be wrong in a way nobody notices.
func TestEveryModelMarshalShipsHasAPrice(t *testing.T) {
	for _, model := range builtinModelOrder {
		if _, ok := PriceOf(model); !ok {
			t.Errorf("%s can be picked but has no price, so its calls would be recorded as free", model)
		}
	}
}

// TestEveryPriceIsForAModelMarshalShips is the other direction: a price row for a model no provider
// offers is dead weight that would drift, and a model Marshal cannot name is one it cannot price
// (a model reached through a catch-all like OpenRouter is recorded at zero cost and logged).
func TestEveryPriceIsForAModelMarshalShips(t *testing.T) {
	shipped := map[string]bool{}
	for _, model := range builtinModelOrder {
		shipped[model] = true
	}
	for model := range prices {
		if !shipped[model] {
			t.Errorf("the price table prices %s, which no provider in the catalog offers", model)
		}
	}
}

// TestPricesArePositiveOrAnExplicitZero refuses a negative rate, which would turn a call into a
// credit and make the day's total move backwards.
func TestPricesArePositiveOrAnExplicitZero(t *testing.T) {
	for model, price := range prices {
		if price.InputMicros < 0 || price.OutputMicros < 0 {
			t.Errorf("%s is priced at %+v, and a price is never negative", model, price)
		}
	}
}

// TestPriceOfRefusesAModelItDoesNotKnow is the "unknown, not free" rule: a model with no row answers
// false with the zero Price, so a caller can tell "nobody knows" from "nothing".
func TestPriceOfRefusesAModelItDoesNotKnow(t *testing.T) {
	price, ok := PriceOf("some-model-nobody-priced")
	if ok {
		t.Errorf("an unknown model was reported as priced: %+v", price)
	}
	if price != (Price{}) {
		t.Errorf("an unknown model priced as %+v, want the zero Price", price)
	}
}

// TestCostIsMicroDollarsRoundedToTheNearest pins the arithmetic: a token count times a price per
// million, rounded to the nearest micro-dollar rather than truncated, so many small calls do not
// drift below what the provider billed.
func TestCostIsMicroDollarsRoundedToTheNearest(t *testing.T) {
	cases := []struct {
		name   string
		model  string
		usage  Usage
		cost   int64
		reason string
	}{
		{
			name: "a million input tokens of the Sonnet tier", model: "claude-sonnet-4-5",
			usage: Usage{InputTokens: 1_000_000}, cost: 3_000_000,
			reason: "3 micro-dollars per million tokens times a million tokens",
		},
		{
			name: "one token each way", model: "claude-sonnet-4-5",
			usage: Usage{InputTokens: 1, OutputTokens: 1}, cost: 18,
			reason: "3 + 15 micro-dollars, rounded",
		},
		{
			name: "two input tokens where truncation would lose the cost", model: "gpt-5-mini",
			usage: Usage{InputTokens: 2}, cost: 1,
			reason: "half a micro-dollar rounds up, where truncation would answer zero",
		},
		{
			name: "one input token where rounding goes down", model: "gpt-5-mini",
			usage: Usage{InputTokens: 1}, cost: 0,
			reason: "a quarter of a micro-dollar rounds down to nothing",
		},
		{
			name: "a million output tokens of the Gemini Flash tier", model: "gemini-2.5-flash",
			usage: Usage{OutputTokens: 1_000_000}, cost: 2_500_000,
			reason: "the output rate is the one that applies to what was written",
		},
		{
			name: "a big call", model: "claude-sonnet-4-5",
			usage: Usage{InputTokens: 250_000, OutputTokens: 40_000}, cost: 1_350_000,
			reason: "250000*3 + 40000*15 micro-dollars per million, which is exact",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cost, ok := Cost(tc.model, tc.usage)
			if !ok {
				t.Fatalf("%s is not priced, so the test cannot check its arithmetic", tc.model)
			}
			if cost != tc.cost {
				t.Errorf("Cost(%s, %+v) = %d, want %d (%s)", tc.model, tc.usage, cost, tc.cost, tc.reason)
			}
		})
	}
}

// TestCostOfAModelWithNoPriceIsUnknownNotFree is the distinction that keeps the meter honest: an
// unpriced model answers 0 with false, so a caller records the tokens and says plainly that it could
// not price them, instead of reporting a call as free.
func TestCostOfAModelWithNoPriceIsUnknownNotFree(t *testing.T) {
	cost, ok := Cost("some-model-nobody-priced", Usage{InputTokens: 100_000, OutputTokens: 100_000})
	if ok {
		t.Errorf("an unpriced model answered ok, so its call would look free")
	}
	if cost != 0 {
		t.Errorf("an unpriced model cost %d, want zero and false", cost)
	}
}

// TestCostOfAFreeModelIsZeroAndKnown is the other side: a local model really does cost nothing, and
// that is a price Marshal knows rather than one it lacks.
func TestCostOfAFreeModelIsZeroAndKnown(t *testing.T) {
	cost, ok := Cost("qwen2.5-coder:32b", Usage{InputTokens: 5_000_000, OutputTokens: 9_000_000})
	if !ok {
		t.Error("a local model was reported as unpriced, want a known price of nothing")
	}
	if cost != 0 {
		t.Errorf("a local model cost %d, want zero", cost)
	}
}

// TestCostOfNoTokensIsZeroAndKnown means a call a provider billed nothing for adds nothing to the
// day, whatever model it was.
func TestCostOfNoTokensIsZeroAndKnown(t *testing.T) {
	for _, model := range builtinModelOrder {
		cost, ok := Cost(model, Usage{})
		if !ok {
			t.Errorf("%s answered ok=false for no tokens", model)
		}
		if cost != 0 {
			t.Errorf("%s cost %d for no tokens, want zero", model, cost)
		}
	}
}
