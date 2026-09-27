package providers

import (
	"net/http"
	"testing"
	"time"
)

// This file checks that what a provider says about the caller's allowance is read, in every spelling
// the providers Marshal calls actually use, and that a provider which says nothing is reported as
// having said nothing rather than as a zero.

var rateLimitNow = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

// rateHeaders builds a response's headers the way net/http does, with the names canonicalized: a
// hand-written map with lower-case keys is not what a response ever carries, and http.Header.Get
// would not find it.
func rateHeaders(pairs ...string) http.Header {
	out := http.Header{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out.Set(pairs[i], pairs[i+1])
	}
	return out
}

// TestAnthropicsRateLimitsAreRead is the wiring: a whole answer carries the headers Anthropic sent
// with it, as the SDK hands them over, and nothing about them is guessed.
func TestAnthropicRateLimitsAreRead(t *testing.T) {
	f := newFakeAnthropic(t)
	f.answerJSONWith(map[string]string{
		anthropicLimitHeader:     "4000",
		anthropicRemainingHeader: "3999",
		anthropicResetHeader:     "2026-09-27T10:00:30Z",
	}, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5",
		"content":[{"type":"text","text":"pong"}],"stop_reason":"end_turn",
		"usage":{"input_tokens":3,"output_tokens":1}}`)
	reply, err := f.client().Complete(t.Context(), Request{Model: "claude-sonnet-4-5"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if reply.RateLimit == nil {
		t.Fatal("the answer carried no rate limit, want the ones Anthropic sent")
	}
	if reply.RateLimit.Limit != 4000 || reply.RateLimit.Remaining != 3999 {
		t.Errorf("rate limit = %+v, want 3999 of 4000", *reply.RateLimit)
	}
	want := time.Date(2026, 9, 27, 10, 0, 30, 0, time.UTC)
	if !reply.RateLimit.Reset.Equal(want) {
		t.Errorf("reset = %s, want %s", reply.RateLimit.Reset, want)
	}
}

// TestAnOpenAICompatibleRateLimitIsRead is the same wiring for the family: OpenAI's own spelling,
// where the reset is a duration from now rather than a moment.
func TestOpenAICompatibleRateLimitsAreRead(t *testing.T) {
	f := newFakeProvider(t)
	f.answerJSONWith(map[string]string{
		openAILimitHeader:     "500",
		openAIRemainingHeader: "499",
		openAIResetHeader:     "1s",
	}, `{"id":"chatcmpl_1","object":"chat.completion","created":1,"model":"gpt-5-mini",
		"choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`)
	before := time.Now()
	reply, err := openAITestClient(f, "openai").Complete(t.Context(), Request{Model: "gpt-5-mini"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if reply.RateLimit == nil {
		t.Fatal("the answer carried no rate limit, want the ones the provider sent")
	}
	if reply.RateLimit.Limit != 500 || reply.RateLimit.Remaining != 499 {
		t.Errorf("rate limit = %+v, want 499 of 500", *reply.RateLimit)
	}
	if !reply.RateLimit.Reset.After(before) {
		t.Errorf("reset = %s, want a moment after the call at %s", reply.RateLimit.Reset, before)
	}
}

// TestAProviderThatSaysNothingLeavesNoRateLimit is the honest half: a provider that sends none of the
// headers leaves nil rather than a RateLimit of zeroes, which is what the connection test reads as
// "it did not say" and reports as a warning rather than as "0 requests left".
func TestAProviderThatSaysNothingLeavesNoRateLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  *RateLimit
	}{
		{"a response with no rate-limit headers", rateLimitFrom(
			&http.Response{Header: http.Header{}}, rateLimitNow, "a", "b", "c")},
		{"no response at all", rateLimitFrom(nil, rateLimitNow, "a", "b", "c")},
		{"headers that are not numbers", rateLimitFrom(
			&http.Response{Header: rateHeaders("a", "plenty", "b", "lots", "c", "soon")},
			rateLimitNow, "a", "b", "c")},
		{"a negative count", rateLimitFrom(
			&http.Response{Header: rateHeaders("a", "-1")}, rateLimitNow, "a", "b", "c")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != nil {
				t.Errorf("rate limit = %+v, want none", *tc.got)
			}
			var zero RateLimit
			if zero.Reported() {
				t.Error("an empty rate limit reported itself as reported")
			}
		})
	}
}

// TestOpenRoutersOwnSpellingIsRead covers the second spelling the compatible family uses: OpenRouter
// says limit/remaining/reset with no "requests" in the name, and one adapter serves both.
func TestOpenRoutersOwnSpellingIsRead(t *testing.T) {
	resp := &http.Response{Header: rateHeaders(
		openRouterLimitHeader, "20",
		openRouterRemainHeader, "19",
		openRouterResetHeader, "2026-09-27T10:00:30Z")}
	got := openAIRateLimit(resp, rateLimitNow)
	if got == nil {
		t.Fatal("OpenRouter's own spelling was not read")
	}
	if got.Limit != 20 || got.Remaining != 19 {
		t.Errorf("rate limit = %+v, want 19 of 20", *got)
	}
}

// TestASpellingMarshalCannotReadIsNotANumber: a header that is neither a moment nor a duration leaves
// the reset unset while the counts are still read, because half an answer is better than none and the
// reset is the least important half.
func TestASpellingMarshalCannotReadIsNotANumber(t *testing.T) {
	resp := &http.Response{Header: rateHeaders(
		openAILimitHeader, "60",
		openAIRemainingHeader, "59",
		openAIResetHeader, "in a bit")}
	got := openAIRateLimit(resp, rateLimitNow)
	if got == nil {
		t.Fatal("the counts were not read")
	}
	if !got.Reset.IsZero() {
		t.Errorf("reset = %s, want unset", got.Reset)
	}
	if !got.Reported() {
		t.Error("a rate limit with counts did not report itself as reported")
	}
}
