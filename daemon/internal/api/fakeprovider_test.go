package api_test

import (
	"context"
	"errors"

	"github.com/khanblair/marshal/daemon/internal/providers"
)

// This file is the fake provider the route tests are built over. Saving a key tests it and pressing
// Test tests it again (routes_providers.go), so a stack built with the real client factory would dial
// the provider named in the request body. Handing the provider service a factory that answers from
// memory is what keeps every test here off the network - the same rule the providers package keeps
// with its own fake HTTP server, one layer up.

// fakeProviderBuild is the client factory a stack's provider service is built with. It answers a fake
// for every provider, so no test in this package can reach a real one. The stack's own option decides
// whether that fake succeeds or fails (withProviderFailure).
func fakeProviderBuild(failure error) func(info providers.Info, secret string) (providers.Client, error) {
	return func(info providers.Info, secret string) (providers.Client, error) {
		return fakeProviderClient{id: info.ID, secret: secret, fail: failure}, nil
	}
}

// fakeProviderClient answers every call the way a working provider would, or with the failure it was
// built with.
type fakeProviderClient struct {
	id     string
	secret string
	// fail, when set, is the error every call answers with: it stands in for the adapter's own
	// wrapping of what a provider said, so the route's fix sentences are the ones a real failure
	// produces.
	fail error
}

// ID is the provider's own id.
func (c fakeProviderClient) ID() string { return c.id }

// Complete answers with one word and a rate limit a test can read back.
func (c fakeProviderClient) Complete(_ context.Context, req providers.Request) (providers.Reply, error) {
	if c.fail != nil {
		return providers.Reply{}, c.fail
	}
	text := "ok"
	for _, message := range req.Messages {
		for _, part := range message.Parts {
			if part.Kind == providers.PartText {
				text = part.Text
			}
		}
	}
	return providers.Reply{
		Parts:      []providers.Part{{Kind: providers.PartText, Text: text}},
		StopReason: providers.StopEndTurn,
		Usage:      providers.Usage{InputTokens: 1, OutputTokens: 1},
		RateLimit:  &providers.RateLimit{Limit: 100, Remaining: 99},
	}, nil
}

// Stream is never used by a route, so it says so rather than pretending to have an answer.
func (c fakeProviderClient) Stream(context.Context, providers.Request) (providers.Stream, error) {
	return nil, errors.New("the fake provider does not stream")
}
