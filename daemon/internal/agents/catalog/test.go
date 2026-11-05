package catalog

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/proc"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Testing one agent program from the screen (docs/architecture.md section 18): it looks for the
// program again, reads its version, and checks the one thing that decides whether Marshal can drive
// it - the way it takes work. None of it sends a prompt to a model, so a test costs nothing and
// changes nothing.

const (
	// checkInstalled names the check that the agent program is there to run.
	checkInstalled = "Installed"
	// testTimeout bounds one test. Gemini CLI takes over half a minute to answer its greeting on a
	// machine with MCP servers, so the greeting is given a minute.
	testTimeout     = 75 * time.Second
	helloTimeout    = 60 * time.Second
	slowHello       = 10 * time.Second
	helpOutputBytes = 64 << 10
	acpProtocol     = 1
	// helloBytes bounds what is read while waiting for an ACP server's answer.
	helloBytes = 1 << 20
)

// ErrUnknownAgent is what testing an id Marshal does not list answers.
var ErrUnknownAgent = errors.New("unknown agent")

// target is what one test is about, whichever list the id came from.
type target struct {
	id, name, program string
	iface             protocol.AgentToolInterface
	acpArgs           []string
	// kind is set for an agent a card can use.
	kind protocol.AgentKind
}

// acpArgsFor are the arguments that start an agent kind as an ACP server, or nil.
func acpArgsFor(kind protocol.AgentKind) []string {
	if kind == protocol.AgentKindGemini {
		return []string{"--acp"}
	}
	return nil
}

func targetOf(id string) (target, bool) {
	for _, kind := range Kinds() {
		if string(kind) != id {
			continue
		}
		sp := specFor(kind)
		t := target{id: id, name: sp.name, program: sp.program, kind: kind, acpArgs: acpArgsFor(kind)}
		switch {
		case len(t.acpArgs) > 0:
			t.iface = protocol.AgentToolInterfaceACP
		case kind != protocol.AgentKindBuiltin && kind != protocol.AgentKindCodex:
			t.iface = protocol.AgentToolInterfacePrint
		}
		return t, true
	}
	if tl, ok := toolByID(id); ok {
		return target{id: tl.id, name: tl.name, program: tl.program, iface: tl.iface, acpArgs: tl.acpArgs}, true
	}
	return target{}, false
}

// Test looks at one agent again and says what it found, without sending any prompt.
func (c *Catalog) Test(ctx context.Context, id string) (protocol.TestResult, error) {
	t, ok := targetOf(id)
	if !ok {
		return protocol.TestResult{}, fmt.Errorf("%w: %s", ErrUnknownAgent, id)
	}
	if t.kind == protocol.AgentKindBuiltin {
		return protocol.NewTestResult(id, c.builtinChecks(), c.now()), nil
	}
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	return protocol.NewTestResult(id, c.checksFor(ctx, t), c.now()), nil
}

func (c *Catalog) builtinChecks() []protocol.TestCheck {
	checks := []protocol.TestCheck{{Name: checkInstalled, State: protocol.CheckStatePassed,
		Message: "The built-in agent is part of Marshal, so there is nothing to install."}}
	if c.builtinModels != nil && len(c.builtinModels()) > 0 {
		return append(checks, protocol.TestCheck{Name: "Models", State: protocol.CheckStatePassed,
			Message: "A model provider is set up, so it has models to run."})
	}
	return append(checks, protocol.TestCheck{Name: "Models", State: protocol.CheckStateWarning,
		Message: noProvidersWarning, Fix: "Add an API key for a provider, on the Connect your agents screen or in Settings."})
}

func (c *Catalog) checksFor(ctx context.Context, t target) []protocol.TestCheck {
	prober, ok := c.probe.(ProgramProber)
	if !ok {
		return []protocol.TestCheck{{Name: checkInstalled, State: protocol.CheckStateFailed,
			Message: "Marshal cannot look for programs in this mode."}}
	}
	found, err := c.probeFor(ctx, prober, t)
	if errors.Is(err, ErrNotFound) {
		return []protocol.TestCheck{{Name: checkInstalled, State: protocol.CheckStateFailed,
			Message: t.name + " is not installed on this computer.", Fix: installFix(t)}}
	}
	checks := []protocol.TestCheck{{Name: checkInstalled, State: protocol.CheckStatePassed,
		Message: t.name + " is on this computer."}}
	if err != nil {
		return append(checks, protocol.TestCheck{Name: "Version", State: protocol.CheckStateFailed,
			Message: "It did not answer \"" + t.program + " --version\".",
			Fix:     "Run \"" + t.program + " --version\" in a terminal to see what is wrong."})
	}
	checks = append(checks, protocol.TestCheck{Name: "Version", State: protocol.CheckStatePassed, Message: found.Version.String() + "."})
	if t.kind != "" {
		checks = append(checks, testedCheck(t, found.Version))
	}
	checks = append(checks, c.interfaceCheck(ctx, t, found.Path))
	if !c.startable(t) {
		checks = append(checks, protocol.TestCheck{Name: "Marshal can start it", State: protocol.CheckStateWarning,
			Message: t.name + " works on its own, but Marshal has no adapter that starts it for a card yet, so it is not offered for cards."})
	}
	return checks
}

func installFix(t target) string {
	if t.kind != "" {
		return specFor(t.kind).installHint
	}
	return "Install " + t.name + ", then scan again."
}

// startable says whether Marshal has an adapter that starts this program for a card.
func (c *Catalog) startable(t target) bool {
	return t.kind != "" && specFor(t.kind).startable
}

func testedCheck(t target, version Semver) protocol.TestCheck {
	status, warning := classify(specFor(t.kind), version)
	if status == protocol.AgentStatusSupported {
		return protocol.TestCheck{Name: "Tested version", State: protocol.CheckStatePassed,
			Message: "Marshal's tests have run against this version."}
	}
	return protocol.TestCheck{Name: "Tested version", State: protocol.CheckStateWarning, Message: warning}
}

// interfaceCheck greets server-based agents and checks help for the other program modes.
func (c *Catalog) interfaceCheck(ctx context.Context, t target, path string) protocol.TestCheck {
	switch {
	case t.kind == protocol.AgentKindCodex:
		return codexCheck(ctx, path)
	case len(t.acpArgs) > 0:
		started := time.Now()
		hello, cancel := context.WithTimeout(ctx, helloTimeout)
		defer cancel()
		agent, err := acpHello(hello, path, t.acpArgs)
		if err != nil {
			return protocol.TestCheck{Name: "Agent Client Protocol", State: protocol.CheckStateFailed,
				Message: "Started with \"" + t.program + " " + strings.Join(t.acpArgs, " ") + "\" but it did not answer the greeting: " + redact(err.Error(), path),
				Fix:     "Run \"" + t.program + " " + strings.Join(t.acpArgs, " ") + "\" in a terminal to see what it says."}
		}
		took := time.Since(started).Round(100 * time.Millisecond)
		if took > slowHello {
			return protocol.TestCheck{Name: "Agent Client Protocol", State: protocol.CheckStateWarning,
				Message: agent + fmt.Sprintf(" It took %s to answer, so a card will wait that long before it starts.", took),
				Fix:     "Turn off MCP servers or extensions in " + t.name + " that you do not use, to make it start faster."}
		}
		return protocol.TestCheck{Name: "Agent Client Protocol", State: protocol.CheckStatePassed, Message: agent + fmt.Sprintf(" (%s)", took)}
	case t.iface == protocol.AgentToolInterfaceRPC:
		return helpCheck(ctx, t, path, "rpc", "RPC mode")
	case t.iface == protocol.AgentToolInterfacePrint:
		return helpCheck(ctx, t, path, "stream-json", "Streaming JSON mode")
	}
	return protocol.TestCheck{Name: "Mode", State: protocol.CheckStateWarning, Message: "Marshal has no mode to check for it."}
}

// CodexAppServerArgs starts the stdio server used by both Settings and card sessions.
func CodexAppServerArgs() []string {
	return []string{"app-server", "--listen", "stdio://"}
}

// CodexInitializeParams identifies Marshal in the app-server greeting.
func CodexInitializeParams() map[string]any {
	return map[string]any{"clientInfo": map[string]string{"name": "marshal", "title": "Marshal", "version": "0.0.0"}}
}

func codexCheck(ctx context.Context, path string) protocol.TestCheck {
	ctx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	const name = "Codex app-server"
	if _, err := serverHello(ctx, path, CodexAppServerArgs(), CodexInitializeParams()); err != nil {
		return protocol.TestCheck{Name: name, State: protocol.CheckStateFailed,
			Message: "Codex did not complete the app-server greeting.",
			Fix:     "Run \"codex app-server --listen stdio://\" in a terminal to check startup, then test again."}
	}
	return protocol.TestCheck{Name: name, State: protocol.CheckStatePassed,
		Message: "Codex started in stdio mode and answered Marshal’s greeting."}
}

// probeFor looks for the program the test is about: the path the person chose when there is one, so
// the test checks the program a card would run, and otherwise the programs on PATH and in the folders.
func (c *Catalog) probeFor(ctx context.Context, prober ProgramProber, t target) (Found, error) {
	if t.kind != "" {
		if pref := c.preferenceOf(ctx, t.kind); pref.Path != "" {
			return c.probeOf(ctx, t.kind, pref.Path)
		}
	}
	return prober.ProbeProgram(ctx, t.program)
}

// redact replaces a program's full path in an error text with its file name, so a check's words never
// carry the place of a program the person did not choose to show.
func redact(text, path string) string {
	return strings.ReplaceAll(text, path, filepath.Base(path))
}

func helpCheck(ctx context.Context, t target, path, want, name string) protocol.TestCheck {
	help, err := readHelp(ctx, path)
	switch {
	case err != nil:
		return protocol.TestCheck{Name: name, State: protocol.CheckStateWarning, Message: "Could not read its help: " + redact(err.Error(), path)}
	case strings.Contains(strings.ToLower(help), want):
		return protocol.TestCheck{Name: name, State: protocol.CheckStatePassed, Message: "Its help lists " + want + "."}
	}
	return protocol.TestCheck{Name: name, State: protocol.CheckStateFailed,
		Message: "Its help does not list " + want + ", which Marshal would need.",
		Fix:     "Update " + t.name + " and test again."}
}

// readHelp runs the program with --help and reads its answer.
func readHelp(ctx context.Context, path string) (string, error) {
	child, err := proc.Start(ctx, proc.Spec{Path: path, Args: []string{"--help"}, Env: ProgramEnv(path)})
	if err != nil {
		return "", err
	}
	out, _ := io.ReadAll(io.LimitReader(child.Stdout, helpOutputBytes))
	_ = child.Stdout.Close()
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), versionStopTimeout)
	defer cancel()
	_ = child.Stop(stopCtx, versionStopGrace)
	child.Wait()
	if len(out) == 0 {
		// Some programs print their help on standard error.
		if tail := child.StderrTail(); tail != "" {
			return tail, nil
		}
		return "", errors.New("it printed nothing")
	}
	return string(out), nil
}

// acpHello starts the program as an ACP server, sends the initialize request, and reads its answer.
// It sends nothing else, so no model is called. It answers a sentence naming the agent and the
// protocol version it speaks.
func acpHello(ctx context.Context, path string, args []string) (string, error) {
	params := map[string]any{"protocolVersion": acpProtocol, "clientCapabilities": map[string]any{}}
	raw, err := serverHello(ctx, path, args, params)
	if err != nil {
		return "", err
	}
	var result struct {
		ProtocolVersion int `json:"protocolVersion"`
		AgentInfo       *struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"agentInfo"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("read the greeting: %w", err)
	}
	if info := result.AgentInfo; info != nil && info.Name != "" {
		text := fmt.Sprintf("%s %s answered the greeting and speaks protocol version %d.", info.Name, info.Version, result.ProtocolVersion)
		return strings.Join(strings.Fields(text), " "), nil
	}
	return fmt.Sprintf("It answered the greeting and speaks protocol version %d.", result.ProtocolVersion), nil
}

// serverHello sends only initialize, so the Settings check does not start a task or a model turn.
func serverHello(ctx context.Context, path string, args []string, params map[string]any) (json.RawMessage, error) {
	child, err := proc.Start(ctx, proc.Spec{Path: path, Args: args, Env: ProgramEnv(path), Stdin: true})
	if err != nil {
		return nil, fmt.Errorf("it would not start (%w)", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), versionStopTimeout)
		defer cancel()
		_ = child.Stdout.Close()
		_ = child.Stop(stopCtx, versionStopGrace)
		child.Wait()
	}()
	request, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": params,
	})
	if _, err := child.Stdin.Write(append(request, '\n')); err != nil {
		return nil, fmt.Errorf("it closed before it could be greeted (%w)", err)
	}
	type answer struct {
		result json.RawMessage
		err    error
	}
	got := make(chan answer, 1)
	go func() {
		scanner := bufio.NewScanner(io.LimitReader(child.Stdout, helloBytes))
		scanner.Buffer(make([]byte, 0, 64<<10), helloBytes)
		for scanner.Scan() {
			var reply struct {
				ID     json.RawMessage  `json:"id"`
				Result *json.RawMessage `json:"result"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			// Other lines before the answer, such as a banner, are not the answer.
			if json.Unmarshal(scanner.Bytes(), &reply) != nil || string(reply.ID) != "1" {
				continue
			}
			if reply.Error != nil {
				got <- answer{err: errors.New(reply.Error.Message)}
				return
			}
			if reply.Result == nil {
				got <- answer{err: errors.New("the answer holds no result")}
				return
			}
			got <- answer{result: *reply.Result}
			return
		}
		got <- answer{err: errors.New("it closed without answering")}
	}()
	select {
	case a := <-got:
		return a.result, a.err
	case <-ctx.Done():
		return nil, errors.New("it did not answer in time")
	}
}
