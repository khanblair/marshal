package providers

// This file reads what a provider says about the caller's allowance on the call it just answered.
// It exists for the connection test (docs/architecture.md section 18, "Model providers | Key valid,
// with a tiny request to the cheapest model, and rate-limit headers read"): a person who presses
// Test should be told both that the key works and how much of the minute is left.
//
// Every provider spells its headers its own way, so the spelling is read here, once, next to the
// adapters that hand it the response - rather than in each adapter or, worse, in the test itself.
// A provider that sends nothing (Ollama, LM Studio) leaves Reply.RateLimit nil, which the test
// reports as a warning rather than inventing a number.

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RateLimit is what a provider reported about the caller's allowance. Every field is what the
// provider itself said: a field it left out is zero, and a provider that said nothing at all leaves
// Reply.RateLimit nil rather than a RateLimit full of zeroes (use Reported to tell the two apart
// when one is in hand).
type RateLimit struct {
	// Limit is how many requests the window allows. Zero when the provider did not say.
	Limit int64
	// Remaining is how many requests are left in the window. Zero when the provider did not say.
	Remaining int64
	// Reset is when the window starts again. The zero Time when the provider did not say.
	Reset time.Time
}

// Reported says whether the provider told Marshal anything at all about its allowance. A provider
// that answered with none of the headers gives false, which is what the connection test turns into
// a warning instead of a number nobody sent.
func (r RateLimit) Reported() bool {
	return r.Limit != 0 || r.Remaining != 0 || !r.Reset.IsZero()
}

// Header spellings, by provider family. Anthropic and OpenAI each use a prefix of their own, and
// the OpenAI-compatible family is not uniform: OpenAI itself says "requests", while OpenRouter says
// just "limit"/"remaining"/"reset". Both are tried, because one adapter serves the whole family.
const (
	anthropicLimitHeader     = "anthropic-ratelimit-requests-limit"
	anthropicRemainingHeader = "anthropic-ratelimit-requests-remaining"
	anthropicResetHeader     = "anthropic-ratelimit-requests-reset"

	openAILimitHeader      = "x-ratelimit-limit-requests"
	openAIRemainingHeader  = "x-ratelimit-remaining-requests"
	openAIResetHeader      = "x-ratelimit-reset-requests"
	openRouterLimitHeader  = "x-ratelimit-limit"
	openRouterRemainHeader = "x-ratelimit-remaining"
	openRouterResetHeader  = "x-ratelimit-reset"
)

// anthropicRateLimit reads Anthropic's rate-limit headers off a response. It answers nil when the
// response is nil (a fake or a transport that handed none back) or when Anthropic said nothing.
func anthropicRateLimit(resp *http.Response, now time.Time) *RateLimit {
	return rateLimitFrom(resp, now,
		anthropicLimitHeader, anthropicRemainingHeader, anthropicResetHeader)
}

// openAIRateLimit reads the rate-limit headers of the OpenAI-compatible family off a response,
// trying OpenAI's own spelling first and OpenRouter's second. It answers nil when neither was sent.
func openAIRateLimit(resp *http.Response, now time.Time) *RateLimit {
	if out := rateLimitFrom(resp, now, openAILimitHeader, openAIRemainingHeader, openAIResetHeader); out != nil {
		return out
	}
	return rateLimitFrom(resp, now, openRouterLimitHeader, openRouterRemainHeader, openRouterResetHeader)
}

// rateLimitFrom reads one trio of headers. It answers nil when nothing was reported, so a caller
// never has to check whether a RateLimit it was handed means anything.
func rateLimitFrom(resp *http.Response, now time.Time, limit, remaining, reset string) *RateLimit {
	if resp == nil {
		return nil
	}
	out := RateLimit{
		Limit:     headerCount(resp.Header, limit),
		Remaining: headerCount(resp.Header, remaining),
		Reset:     headerReset(resp.Header, reset, now),
	}
	if !out.Reported() {
		return nil
	}
	return &out
}

// headerCount reads one header as a whole number. A value that is not a number is treated as absent:
// a provider that sends something Marshal cannot read has not said how many requests are left, and
// a zero is the honest answer to that.
func headerCount(h http.Header, name string) int64 {
	value := strings.TrimSpace(h.Get(name))
	if value == "" {
		return 0
	}
	count, err := strconv.ParseInt(value, 10, 64)
	if err != nil || count < 0 {
		return 0
	}
	return count
}

// headerReset reads one header as the moment a window starts again. Providers write this two ways -
// Anthropic an RFC 3339 time, OpenAI a duration from now ("6m0s") - so both are tried, and a value
// that is neither leaves the time zero, which reads as "did not say".
func headerReset(h http.Header, name string, now time.Time) time.Time {
	value := strings.TrimSpace(h.Get(name))
	if value == "" {
		return time.Time{}
	}
	if at, err := time.Parse(time.RFC3339, value); err == nil {
		return at
	}
	if after, err := time.ParseDuration(value); err == nil {
		return now.Add(after)
	}
	return time.Time{}
}
