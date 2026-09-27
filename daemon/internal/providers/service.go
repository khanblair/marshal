package providers

// This file owns what is true of one install rather than of a provider: which providers have a key,
// what the settings screen is told about each, and which models the built-in agent may therefore
// run. The table of providers themselves is catalog.go; turning a model into a client is
// resolver.go; reading and writing a key is keys.go.
//
// One Service is built once, when the daemon starts, and is safe for concurrent use: the session
// manager resolves models on it while the API layer saves keys.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// Options sets up a Service. Every field is optional.
type Options struct {
	// Logger receives the lines about a provider that could not be read. Nil uses slog.Default().
	Logger *slog.Logger
	// Recorder files what each call cost (usage.go). Nil records nothing, which is what a test
	// wants and what a daemon with no store wired yet gets.
	Recorder UsageRecorder
	// Notices is told when Marshal does something on its own the person should be told about -
	// today, only a model fallback (scope 7.5, "notifies you"). Nil logs the same sentence instead.
	Notices func(ctx context.Context, text string)
	// Now returns the current time, which a call's receipt is stamped with. Nil uses time.Now.
	Now func() time.Time
	// Sleep waits between retries. Nil waits for real; a test passes one that returns at once.
	Sleep func(ctx context.Context, d time.Duration) error
	// RetryAttempts is how many times one request is tried, first try included. Below one means the
	// default; a test uses one to see a single request.
	RetryAttempts int
	// RetryBackoff is the wait before the second try; it doubles from there. Zero means the default.
	RetryBackoff time.Duration
	// MaxConcurrent caps how many requests one provider runs at once (scope 7.6). Below one means
	// the default.
	MaxConcurrent int
	// Build makes the client for a provider, replacing the real adapters. Nil uses buildClient
	// (resolver.go). It exists so that a caller that must never reach a provider can say so - a test
	// of the daemon's own routes, which would otherwise dial the provider named in the request body.
	// A test passes a factory that returns its own fake; nothing else sets it.
	Build func(info Info, secret string) (Client, error)
}

// Service is the one place that knows which model providers Marshal can talk to, which of them the
// owner has set up, and how to make a client for one. It holds the keychain, so nothing else in
// Marshal reads a provider key.
//
// It deliberately holds no context and no long-lived call: making a client for Gemini needs a
// context, and a client outlives the call that asked for it, so clients are built on the daemon's
// own lifetime rather than on a request's (see buildClient in resolver.go).
//
// The clients it hands out are not the adapters themselves but the managed wrappers around them
// (managed.go), so every call a caller makes is queued, retried, able to fall back, and receipted
// without the caller knowing. That is why the clients here are *provider and not Client.
type Service struct {
	keys  security.Keychain
	log   *slog.Logger
	build func(info Info, secret string) (Client, error)

	recorder UsageRecorder
	notices  func(ctx context.Context, text string)
	now      func() time.Time
	sleep    sleepFunc
	retry    retryPolicy
	maxOpen  int

	mu      sync.Mutex
	clients map[string]*provider
}

// New returns a service over a keychain. The keychain is required and is never nil: a service that
// quietly stored keys nowhere would report every provider as set up and then fail on the call.
func New(keys security.Keychain, opts Options) (*Service, error) {
	if keys == nil {
		return nil, errors.New("providers: a keychain is required")
	}
	s := &Service{
		keys:     keys,
		log:      opts.Logger,
		build:    buildClient,
		recorder: opts.Recorder,
		notices:  opts.Notices,
		now:      opts.Now,
		sleep:    opts.Sleep,
		retry:    retryPolicy{attempts: opts.RetryAttempts, backoff: opts.RetryBackoff},
		maxOpen:  opts.MaxConcurrent,
		clients:  map[string]*provider{},
	}
	if opts.Build != nil {
		s.build = opts.Build
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.sleep == nil {
		s.sleep = sleepFor
	}
	if s.retry.attempts < 1 {
		s.retry.attempts = defaultAttempts
	}
	if s.retry.backoff <= 0 {
		s.retry.backoff = defaultBackoff
	}
	if s.maxOpen < 1 {
		s.maxOpen = defaultMaxConcurrent
	}
	return s, nil
}

// List returns one row per provider Marshal knows, in the order the screens show them: what it is
// called, whether a value is stored for it, and that value masked. The stored value itself is never
// in the answer - the masked form is all a client ever has (docs/backend-inventory.md N18).
//
// A provider nobody has set up is still listed, as an empty row, so the screen can offer to add it.
func (s *Service) List() ([]protocol.Provider, error) {
	out := make([]protocol.Provider, 0, len(known))
	for _, info := range known {
		secret, err := s.secret(info.ID)
		if err != nil {
			return nil, err
		}
		row := protocol.Provider{
			ID: info.ID, Name: info.Name, Models: info.ModelsPhrase, Local: info.Local,
			Status: protocol.ProviderStatusEmpty,
		}
		if secret != "" {
			row.Status = protocol.ProviderStatusSaved
			row.Masked = info.MaskedValue(secret)
		}
		out = append(out, row)
	}
	return out, nil
}

// Models lists the models of the providers that are set up, in the order the built-in agent's
// picker shows them, so a card's model picker offers exactly what has a key behind it. It is what
// the agent catalog asks for on every list (catalog.BuiltinModels), so a key saved in Settings
// shows up the next time a picker opens.
//
// There is no error to report and none to swallow silently: the caller is a plain list of models,
// so a keychain that cannot be read is logged and treated as "that provider is not set up". A
// provider with no key is left out rather than offered and failing on the first call.
func (s *Service) Models() []protocol.AgentModel {
	out := []protocol.AgentModel{}
	for _, id := range builtinModelOrder {
		owner, model, ok := modelOwner(id)
		if !ok {
			// The order list and the table are held together by a test
			// (TestEveryModelIsInThePickerOrder). This is here so that a mistake is a missing
			// row in a picker rather than a crash in a shipped build.
			s.log.Error("a model in the picker's order is offered by no provider", "model", id)
			continue
		}
		setUp, err := s.setUp(owner.ID)
		if err != nil {
			s.log.Error("could not read a provider key, so its models are left out",
				"provider", owner.ID, "error", err)
			continue
		}
		if setUp {
			out = append(out, model)
		}
	}
	return out
}

// Has reports whether a provider is set up: a key, or a server address for a provider that runs on
// this machine, is stored for it.
func (s *Service) Has(id string) (bool, error) {
	if _, ok := Lookup(id); !ok {
		return false, unknownProvider(id)
	}
	return s.setUp(id)
}

// setUp is Has without the id check, for callers that already hold an entry from the table.
func (s *Service) setUp(id string) (bool, error) {
	secret, err := s.secret(id)
	if err != nil {
		return false, err
	}
	return secret != "", nil
}

// secret returns the value stored for a provider, or "" when nothing is stored. ErrNoKey is the
// expected answer for a provider nobody set up, so it is turned into "" here; any other failure is
// returned, because a keychain that cannot be read is not the same as a provider with no key.
func (s *Service) secret(id string) (string, error) {
	value, err := s.keys.Get(id)
	switch {
	case errors.Is(err, security.ErrNoKey):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("read the %s key from the keychain: %w", id, err)
	}
	return value, nil
}

// unknownProvider is the answer for an id Marshal has no adapter for. It is an answer the client
// sees, not the daemon's own failure: the id came from the request.
func unknownProvider(id string) error {
	return protocol.NotFound("model provider").With("providerId", id)
}

// forgetClient drops the kept client for a provider, so the next call builds one over the value
// that was just stored or removed.
func (s *Service) forgetClient(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, id)
}

// client returns the managed client for a provider, making it the first time and keeping it: a
// Client is safe for concurrent use, and building one per call would throw away the adapter's
// connection pool - and, for Gemini, repeat a set-up step that can fail. The queue it is given is
// the provider's own, so it is the same queue every caller shares (queue.go).
func (s *Service) client(info Info, secret string) (*provider, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.clients[info.ID]; ok {
		return p, nil
	}
	inner, err := s.build(info, secret)
	if err != nil {
		return nil, fmt.Errorf("set up the %s provider: %w", info.ID, err)
	}
	p := &provider{svc: s, info: info, inner: inner, gate: newQueue(s.maxOpen)}
	s.clients[info.ID] = p
	return p, nil
}
