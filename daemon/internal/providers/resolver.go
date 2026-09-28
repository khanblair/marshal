package providers

// This file turns a model id into the provider and client that run it. It is the seam the built-in
// agent asks through (agents/builtin's Resolver), and the reason a card only ever names a model:
// which provider serves it is Marshal's business, not the card's.

import (
	"context"
	"fmt"
)

// Resolved is one provider and model a caller asked for, with the client that talks to it. The ids
// are what the caller reports in a usage row; the client is what it calls.
type Resolved struct {
	// ProviderID is the provider's own id, such as "anthropic".
	ProviderID string
	// Model is the provider's own model name, as the provider's API takes it.
	Model string
	// Client talks to the provider. It is shared with every other caller of the same provider, so
	// an adapter must be safe for concurrent use (which the Client interface requires anyway).
	Client Client
}

// buildClient makes the real client for a provider. It is the Service's own default, replaced in
// tests by a factory that returns a fake, so no test can reach a provider.
func buildClient(info Info, secret string) (Client, error) {
	cfg := Config{APIKey: secret, BaseURL: info.BaseURL}
	if info.Local {
		// A local provider stores its server address where a hosted one stores its key, so the
		// address is the base URL and there is no key to send.
		cfg.APIKey, cfg.BaseURL = "", secret
	}
	switch info.Family {
	case FamilyAnthropic:
		return NewAnthropic(cfg), nil
	case FamilyOpenAI:
		return NewOpenAICompatible(info.ID, cfg), nil
	case FamilyGemini:
		// This one adapter can fail here rather than on the first call, because its SDK builds
		// eagerly and refuses a key it cannot use. The context is the daemon's own lifetime, not a
		// request's: the client is kept and outlives whoever asked for it first.
		client, err := NewGemini(context.Background(), cfg)
		if err != nil {
			return nil, err
		}
		return client, nil
	default:
		return nil, fmt.Errorf("%s has no adapter: it is in the %q family, which Marshal cannot call",
			info.ID, info.Family)
	}
}

// Resolve returns the provider, model, and client a model id runs on. An empty model id resolves to
// DefaultModel, which is what a session with no model of its own runs.
//
// Providers are considered in the order the settings screen lists them, and the first one that is
// set up and either names the model or serves any model (see Info.AnyModel) answers. So a model two
// providers could run goes to the earlier one - a direct key before OpenRouter - and a model no
// table names goes to whichever catch-all provider is set up. A card names a model and not a
// provider, so this is the whole of the choice, and it is stable: the order is Marshal's own list.
//
// When nothing can run the model the answer wraps ErrNoSuchModel, which is the error a caller acts
// on: a different model, or a key saved for one that can run it.
func (s *Service) Resolve(model string) (Resolved, error) {
	p, named, err := s.resolve(model)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{ProviderID: p.info.ID, Model: named, Client: p}, nil
}

// resolve is Resolve's own body, and it answers with the managed provider rather than the interface,
// so the call path that needs the queue and the adapter behind it (managed.go) can reach them. It is
// what a fallback uses to find the provider a backup model runs on.
func (s *Service) resolve(model string) (*provider, string, error) {
	if model == "" {
		model = DefaultModel
	}
	for _, info := range known() {
		named, names := info.Model(model)
		if !names && !info.AnyModel {
			continue
		}
		secret, err := s.secret(info.ID)
		if err != nil {
			return nil, "", err
		}
		if secret == "" {
			// Set up or not is the whole question: an unset provider is skipped rather than
			// failing, so the next one that can run the model gets its turn.
			continue
		}
		p, err := s.client(info, secret)
		if err != nil {
			return nil, "", err
		}
		if names {
			model = named.ID
		}
		return p, model, nil
	}
	return nil, "", fmt.Errorf("%w: no provider is set up that can run %q "+
		"(save a key for it in Settings, under Provider keys)", ErrNoSuchModel, model)
}
