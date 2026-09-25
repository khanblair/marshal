// Package catalog finds out which coding agents are installed on this machine, and tells the rest
// of Marshal about them: the versions, whether each one is tested, the models it runs, and what it
// can do. It answers GET /v1/agents, and it gives the session manager the place of each program so
// that the adapters in agents/claude, agents/gemini, and agents/codex can be started.
//
// Detection is slow (it starts each program to ask for its version), so it runs on the first
// request and the answer is kept for a few minutes. It never runs when the daemon starts.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	defaultTTL = 5 * time.Minute
	// defaultTimeout bounds a whole detection run, all agents together.
	defaultTimeout = 8 * time.Second
)

// Detected is what detection found for one kind of agent. The catalog sent to clients is made from
// it, without Path: the place of a program names the person's folders.
type Detected struct {
	Kind protocol.AgentKind
	// Path is where the program is. It is empty when the agent is missing.
	Path string
	// Version is the installed version, or empty when the agent is missing.
	Version string
	Status  protocol.AgentStatus
	// Warning is a plain sentence, or empty.
	Warning string
	// Startable says that the agent is installed and Marshal has an adapter that can start it.
	Startable bool
}

// Source is what the API layer and the session manager need from a catalog. Both the real Catalog
// and the Stub of dev mode are one.
type Source interface {
	// List returns the catalog, from the kept answer when it is recent.
	List(ctx context.Context) (protocol.AgentCatalog, error)
	// Refresh looks again and returns the new catalog.
	Refresh(ctx context.Context) (protocol.AgentCatalog, error)
	// Detect returns what List is made from, with the place of each program, for the session
	// manager. The order is the order of Kinds.
	Detect(ctx context.Context) ([]Detected, error)
}

// Options sets up a Catalog. Every field is optional.
type Options struct {
	// Probe looks for the programs. The default is DefaultProbe.
	Probe Probe
	// Now is the clock. The default is time.Now.
	Now func() time.Time
	// TTL is how long an answer is kept. The default is 5 minutes.
	TTL time.Duration
	// Timeout bounds one detection run, all agents together. The default is 8 seconds.
	Timeout time.Duration
	// Logger receives one line per agent for each run. The default is slog.Default().
	Logger *slog.Logger
}

// Catalog detects the installed agents and keeps the answer. It is safe for concurrent use.
type Catalog struct {
	probe   Probe
	now     func() time.Time
	ttl     time.Duration
	timeout time.Duration
	log     *slog.Logger

	mu     sync.Mutex
	cached *snapshot
	flight *flight
}

var _ Source = (*Catalog)(nil)

// snapshot is one detection run and when it finished.
type snapshot struct {
	at    time.Time
	found []Detected
}

// flight is a detection run in progress. Callers that arrive while it runs wait for it instead of
// starting another.
type flight struct {
	done chan struct{}
	snap snapshot
}

// New returns a catalog. It starts nothing: the first List or Detect starts the first run.
func New(opts Options) *Catalog {
	c := &Catalog{
		probe: opts.Probe, now: opts.Now, ttl: opts.TTL, timeout: opts.Timeout, log: opts.Logger,
	}
	if c.probe == nil {
		c.probe = DefaultProbe()
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.ttl <= 0 {
		c.ttl = defaultTTL
	}
	if c.timeout <= 0 {
		c.timeout = defaultTimeout
	}
	if c.log == nil {
		c.log = slog.Default()
	}
	return c
}

// List returns the catalog. It uses the kept answer when that is younger than the TTL, and
// otherwise looks again. The context only bounds this caller's wait: a run that was started keeps
// going, within the timeout, so the next caller has an answer.
func (c *Catalog) List(ctx context.Context) (protocol.AgentCatalog, error) {
	snap, err := c.get(ctx, false)
	if err != nil {
		return protocol.AgentCatalog{}, err
	}
	return catalogOf(snap, c.now()), nil
}

// Refresh looks again, whatever is kept, and returns the new catalog.
func (c *Catalog) Refresh(ctx context.Context) (protocol.AgentCatalog, error) {
	snap, err := c.get(ctx, true)
	if err != nil {
		return protocol.AgentCatalog{}, err
	}
	return catalogOf(snap, c.now()), nil
}

// Detect returns what detection found, with the place of each program.
func (c *Catalog) Detect(ctx context.Context) ([]Detected, error) {
	snap, err := c.get(ctx, false)
	if err != nil {
		return nil, err
	}
	return slices.Clone(snap.found), nil
}

// get returns a snapshot that is fresh enough, or a new one when force is set.
func (c *Catalog) get(ctx context.Context, force bool) (snapshot, error) {
	c.mu.Lock()
	if !force && c.cached != nil && c.now().Sub(c.cached.at) < c.ttl {
		snap := *c.cached
		c.mu.Unlock()
		return snap, nil
	}
	f := c.flight
	if f == nil {
		f = &flight{done: make(chan struct{})}
		c.flight = f
		go c.run(f)
	}
	c.mu.Unlock()

	select {
	case <-f.done:
		return f.snap, nil
	case <-ctx.Done():
		return snapshot{}, fmt.Errorf("wait for the agent check: %w", ctx.Err())
	}
}

// run does one detection run for a flight. It belongs to the catalog, not to the caller that
// started it, and the timeout is what stops it.
func (c *Catalog) run(f *flight) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	snap := snapshot{found: c.detectAll(ctx)}
	snap.at = c.now()
	c.mu.Lock()
	c.cached = &snap
	c.flight = nil
	c.mu.Unlock()
	f.snap = snap
	close(f.done)
}

// detectAll asks the probe about every kind at the same time.
func (c *Catalog) detectAll(ctx context.Context) []Detected {
	kinds := Kinds()
	out := make([]Detected, len(kinds))
	var wg sync.WaitGroup
	for i, kind := range kinds {
		wg.Go(func() { out[i] = c.detectOne(ctx, kind) })
	}
	wg.Wait()
	return out
}

// detectOne turns what the probe says about one kind into a Detected.
func (c *Catalog) detectOne(ctx context.Context, kind protocol.AgentKind) Detected {
	sp := specFor(kind)
	found, err := c.probe.Probe(ctx, kind)
	d := Detected{Kind: kind, Status: protocol.AgentStatusMissing}
	switch {
	case err == nil:
		d.Path, d.Version = found.Path, found.Version.String()
		d.Status, d.Warning = classify(sp, found.Version)
		d.Startable = sp.startable
	case errors.Is(err, ErrNotFound):
		// Not installed is the usual reason, and it needs no warning: the install hint says it.
	default:
		d.Warning = unreadableWarning(sp)
		// The reason is for the log. It can name a folder, so only the daemon's own log gets it.
		c.log.Info("could not read the version of an agent", "kind", string(kind), "err", err)
	}
	c.log.Info("agent checked", "kind", string(kind), "status", string(d.Status), "version", d.Version)
	return d
}

// classify decides the status of an installed agent from its version.
func classify(sp spec, version Semver) (protocol.AgentStatus, string) {
	if !sp.startable {
		return protocol.AgentStatusUntested,
			"Marshal cannot start " + sp.name + " sessions yet, so it is not offered for cards."
	}
	tested := testedVersions(sp.kind)
	if slices.Contains(tested, version.String()) {
		return protocol.AgentStatusSupported, ""
	}
	return protocol.AgentStatusUntested, untestedWarning(sp.name, version.String(), tested)
}

// untestedWarning is the sentence for an installed version that has not been tested.
func untestedWarning(name, version string, tested []string) string {
	text := "Marshal has not been tested with " + name + " " + version + ". It usually works, but if something looks wrong,"
	if len(tested) == 0 {
		return text + " tell us."
	}
	return text + " try version " + tested[len(tested)-1] + "."
}

// unreadableWarning is the sentence for a program that is there but did not give its version.
func unreadableWarning(sp spec) string {
	return "Marshal found " + sp.name + " but could not read its version, so it is not using it. " +
		"Try running \"" + sp.program + " --version\" in a terminal."
}

// catalogOf makes the wire catalog from a snapshot. It is stamped with the time of the answer, not
// with the time of the check, like every other answer that shows state.
func catalogOf(snap snapshot, now time.Time) protocol.AgentCatalog {
	agents := make([]protocol.Agent, 0, len(snap.found))
	for _, d := range snap.found {
		agents = append(agents, agentOf(d))
	}
	return protocol.NewAgentCatalog(agents, now)
}

// agentOf makes the wire form of one detected agent. The place of the program is left out.
func agentOf(d Detected) protocol.Agent {
	sp := specFor(d.Kind)
	agent := protocol.Agent{
		Kind: d.Kind, Name: sp.name, Version: d.Version, Status: d.Status, Warning: d.Warning,
		Models: modelsFor(d.Kind), Capabilities: sp.capabilities,
	}
	if d.Status == protocol.AgentStatusMissing {
		agent.InstallHint = sp.installHint
	}
	return agent
}
