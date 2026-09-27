package providers

// This file is one provider's connection test (docs/architecture.md section 18, inventory B4.6,
// build-plan task 4.9): the tiny request that proves a stored key works, and what Marshal tells the
// person about it. It is the provider half of the generic framework in internal/connectiontest - the
// cooldown, the time limit, and the saving of the result all live there; all this file writes is the
// answer to "does this key work, and what does the provider say about my allowance".
//
// The test is deliberately read-only and as cheap as it can be: one call, one token asked for, to
// the cheapest model the provider offers by Marshal's own price table (Info.Cheapest). No test ever
// uses a real key or a real provider: the adapters are driven through the same Client interface a
// session uses, so this package's tests point a fake HTTP server at the adapter, and the daemon's
// own route tests build the Service with a fake client factory (Options.Build).

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The names of the checks a provider's test reports. They are the row labels a screen shows, so they
// are words rather than ids.
const (
	// CheckAPIKey is the check that the stored key is accepted.
	CheckAPIKey = "API key"
	// CheckRateLimits is the check that reads the provider's own rate-limit headers.
	CheckRateLimits = "Rate limits"
)

// testMaxTokens is how much of an answer the tiny request asks for. It is not zero - a provider sent
// no limit answers at its own default length, which costs far more than a test should - and it is
// large enough that the answer arrives in one piece.
const testMaxTokens = 8

// testPrompt is the whole of what the test asks. One word, so the call costs a token or two.
const testPrompt = "ping"

// Test runs the connection test for one provider and answers what to show the person. An id Marshal
// has no provider for is not found, and a stored value Marshal cannot read is the daemon's own
// failure; everything else - a key the provider refused, a provider that could not be reached, a
// provider that reported no rate limits - is part of the answer, not an error.
func (s *Service) Test(ctx context.Context, id string) (protocol.TestResult, error) {
	info, ok := Lookup(id)
	if !ok {
		return protocol.TestResult{}, unknownProvider(id)
	}
	secret, err := s.secret(id)
	if err != nil {
		return protocol.TestResult{}, err
	}
	if secret == "" {
		return protocol.NewTestResult(id, []protocol.TestCheck{{
			Name:  CheckAPIKey,
			State: protocol.CheckStateFailed,
			Message: fmt.Sprintf("No %s key is stored, so Marshal cannot use %s.",
				info.Name, info.Name),
			Fix: fmt.Sprintf("Add a key for %s in Settings, under Provider keys.", info.Name),
		}}, s.now()), nil
	}
	model, nameable := info.Cheapest()
	if !nameable {
		return s.testUnnameable(id, info), nil
	}
	client, err := s.client(info, secret)
	if err != nil {
		return protocol.TestResult{}, err
	}
	reply, callErr := client.Complete(ctx, Request{
		Model:     model.ID,
		Messages:  []Message{{Role: RoleUser, Parts: []Part{{Kind: PartText, Text: testPrompt}}}},
		MaxTokens: testMaxTokens,
	})
	return protocol.NewTestResult(id, []protocol.TestCheck{
		keyCheck(info, reply, callErr),
		rateLimitCheck(info, reply.RateLimit),
	}, s.now()), nil
}

// testUnnameable is the answer for a provider Marshal cannot name a model for - OpenRouter, which
// proxies other providers' models, and LM Studio, which runs whatever the person has loaded. Neither
// has a model in Marshal's table, so there is nothing to ask for one token from, and Marshal says so
// rather than sending a request that would fail for a reason that is Marshal's own gap. The stored
// value's presence is still reported, which is what the check is for.
func (s *Service) testUnnameable(id string, info Info) protocol.TestResult {
	return protocol.NewTestResult(id, []protocol.TestCheck{{
		Name:  CheckAPIKey,
		State: protocol.CheckStateWarning,
		Message: fmt.Sprintf(
			"Marshal cannot name a model to test %s with, so it did not call %s.", info.Name, info.Name),
		Fix: "The key is stored. Start a chat to try it.",
	}}, s.now())
}

// keyCheck is what the tiny request proved.
func keyCheck(info Info, reply Reply, err error) protocol.TestCheck {
	if err != nil {
		message, fix := testFailure(info, err)
		return protocol.TestCheck{Name: CheckAPIKey, State: protocol.CheckStateFailed, Message: message, Fix: fix}
	}
	return protocol.TestCheck{
		Name:    CheckAPIKey,
		State:   protocol.CheckStatePassed,
		Message: fmt.Sprintf("%s accepted the key and answered a tiny request.", info.Name),
	}
}

// rateLimitCheck is what the provider said about the caller's allowance. A provider that sent no
// rate-limit headers gets a warning rather than a failure: the key works, which is what the test is
// for, and Marshal is honest that it has no numbers to show rather than reporting a zero it made up.
func rateLimitCheck(info Info, limit *RateLimit) protocol.TestCheck {
	check := protocol.TestCheck{Name: CheckRateLimits}
	switch {
	case limit == nil || !limit.Reported():
		check.State = protocol.CheckStateWarning
		check.Message = fmt.Sprintf("%s did not say how many requests are left.", info.Name)
		return check
	case limit.Limit > 0:
		check.State = protocol.CheckStatePassed
		check.Message = fmt.Sprintf("%d of %d requests left this window.", limit.Remaining, limit.Limit)
	default:
		check.State = protocol.CheckStatePassed
		check.Message = fmt.Sprintf("%d requests left this window.", limit.Remaining)
	}
	if !limit.Reset.IsZero() {
		check.Message += " It resets at " + limit.Reset.Format(time.RFC3339) + "."
	}
	return check
}

// testFailure turns a failed call into the sentence and the fix a person reads. Every case is one the
// person can act on, which is why they are named one by one rather than passed through: an adapter's
// own error text names the API, and a screen does not.
func testFailure(info Info, err error) (message, fix string) {
	switch {
	case errors.Is(err, ErrAuth):
		return fmt.Sprintf("%s refused this key.", info.Name),
			"Check the key and save it again, under Provider keys."
	case errors.Is(err, ErrNoSuchModel):
		return fmt.Sprintf("%s does not offer the model Marshal tested with.", info.Name),
			"Update Marshal, or use another provider."
	case errors.Is(err, ErrRateLimited):
		return fmt.Sprintf("%s is rate limiting this key.", info.Name),
			"Wait a minute and test again."
	case errors.Is(err, ErrUnavailable):
		return fmt.Sprintf("Marshal could not reach %s.", info.Name),
			"Check this computer's connection, then test again."
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return fmt.Sprintf("%s did not answer in time.", info.Name),
			"Test again, or check whether the provider is having trouble."
	default:
		return fmt.Sprintf("The test failed: %s.", err), ""
	}
}
