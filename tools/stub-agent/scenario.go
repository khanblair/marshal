package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/coder/acp-go-sdk"
)

// builtinScenarios holds the scenario files that ship inside the program, so a bare binary works
// without any files next to it. go:embed can only fill a package-level variable.
//
//go:embed scenarios/*.json
var builtinScenarios embed.FS

const (
	scenarioDir  = "scenarios"
	scenarioExt  = ".json"
	maxPauseMs   = 3_600_000
	maxRepeats   = 1000
	statusFailed = "failed"
	statusOK     = "completed"
	stopEndTurn  = "end_turn"
)

// stepType is the kind of one scenario step.
type stepType string

const (
	stepSay        stepType = "say"
	stepPause      stepType = "pause"
	stepTool       stepType = "tool"
	stepPermission stepType = "permission"
	stepRepeat     stepType = "repeat"
	stepEnd        stepType = "end"
)

// scenario is one scripted behavior. The file name, without its extension, is the scenario name.
type scenario struct {
	Name        string `json:"-"`
	Description string `json:"description"`
	Steps       []step `json:"steps"`
}

// step is one thing the agent does. Which fields count depends on Type, and validate checks that
// the right ones are set, so a typo in a scenario file fails at start instead of during a turn.
type step struct {
	Type stepType `json:"type"`

	// Text is what a say step says.
	Text string `json:"text"`
	// Ms is how long a pause step waits, before the speed setting scales it.
	Ms int `json:"ms"`

	// A tool or permission step describes one tool call.
	toolCall

	// OnAllow and OnDeny are the steps a permission step follows after the answer.
	OnAllow []step `json:"onAllow"`
	OnDeny  []step `json:"onDeny"`

	// Count and Steps make a repeat step.
	Count int    `json:"count"`
	Steps []step `json:"steps"`

	// Reason is the stop reason of an end step.
	Reason string `json:"reason"`
}

// toolCall describes a tool call that the agent shows to the client.
type toolCall struct {
	// ID names the call inside the scenario. The wire id adds the turn and a counter, because a
	// repeated call must not reuse an id.
	ID      string `json:"id"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Command string `json:"command"`
	// Write really writes a file under the session's working folder, but only when the call
	// completes: a failed call changes nothing.
	Write  *fileWrite `json:"write"`
	Status string     `json:"status"`
	Result string     `json:"result"`
}

// fileWrite is a file that a tool call writes. Content is the text, or Lines is the same text one
// line per entry, which keeps a long file readable inside a JSON scenario.
type fileWrite struct {
	Path    string   `json:"path"`
	Content string   `json:"content"`
	Lines   []string `json:"lines"`
}

// text returns the whole content of the file.
func (w fileWrite) text() string {
	if len(w.Lines) == 0 {
		return w.Content
	}
	return strings.Join(w.Lines, "\n") + "\n"
}

// loadBuiltinScenarios reads the scenarios that ship inside the program.
func loadBuiltinScenarios() (map[string]scenario, error) {
	sub, err := fs.Sub(builtinScenarios, scenarioDir)
	if err != nil {
		return nil, fmt.Errorf("open built-in scenarios: %w", err)
	}
	return loadScenarios(sub)
}

// loadScenarios reads and checks every scenario file in the root of fsys. One bad file fails the
// whole load, with the file name in the message, because a half-loaded script set hides mistakes.
func loadScenarios(fsys fs.FS) (map[string]scenario, error) {
	files, err := fs.Glob(fsys, "*"+scenarioExt)
	if err != nil {
		return nil, fmt.Errorf("list scenarios: %w", err)
	}
	found := make(map[string]scenario, len(files))
	for _, file := range files {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("read scenario %s: %w", file, err)
		}
		sc, err := parseScenario(data)
		if err != nil {
			return nil, fmt.Errorf("scenario %s: %w", file, err)
		}
		sc.Name = strings.TrimSuffix(path.Base(file), scenarioExt)
		found[sc.Name] = sc
	}
	return found, nil
}

// parseScenario decodes one scenario file. Unknown fields are an error, so a misspelled key does
// not silently turn into a step that does nothing.
func parseScenario(data []byte) (scenario, error) {
	var sc scenario
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sc); err != nil {
		return scenario{}, fmt.Errorf("decode: %w", err)
	}
	if dec.More() {
		return scenario{}, errors.New("decode: extra data after the scenario")
	}
	if len(sc.Steps) == 0 {
		return scenario{}, errors.New("no steps")
	}
	if err := validateSteps(sc.Steps); err != nil {
		return scenario{}, err
	}
	return sc, nil
}

// isNameRune says whether c may appear in a scenario name or a session id: letters, digits, dash
// and underscore, which are safe in a file name and cannot end a marker early by accident.
func isNameRune(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '-' || c == '_'
}

// scenarioNames lists the scenario names in a stable order, for messages.
func scenarioNames(all map[string]scenario) string {
	return strings.Join(slices.Sorted(maps.Keys(all)), ", ")
}

// validateSteps checks a list of steps and the lists nested inside them.
func validateSteps(steps []step) error {
	for i, s := range steps {
		if err := s.validate(); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	return nil
}

// validate checks one step. Nested lists are checked through validateSteps.
func (s step) validate() error {
	switch s.Type {
	case stepSay:
		if s.Text == "" {
			return errors.New("say needs text")
		}
	case stepPause:
		if s.Ms < 0 || s.Ms > maxPauseMs {
			return fmt.Errorf("pause of %d ms is out of range", s.Ms)
		}
	case stepTool:
		return s.toolCall.validate()
	case stepPermission:
		return s.validatePermission()
	case stepRepeat:
		return s.validateRepeat()
	case stepEnd:
		if _, ok := stopReasonOf(s.Reason); !ok {
			return fmt.Errorf("unknown stop reason %q", s.Reason)
		}
	default:
		return fmt.Errorf("unknown step type %q", s.Type)
	}
	return nil
}

// validatePermission checks the tool call and both answer branches.
func (s step) validatePermission() error {
	if err := s.toolCall.validate(); err != nil {
		return err
	}
	if err := validateSteps(s.OnAllow); err != nil {
		return fmt.Errorf("onAllow: %w", err)
	}
	if err := validateSteps(s.OnDeny); err != nil {
		return fmt.Errorf("onDeny: %w", err)
	}
	return nil
}

// validateRepeat checks the loop count and the steps it repeats.
func (s step) validateRepeat() error {
	if s.Count < 1 || s.Count > maxRepeats {
		return fmt.Errorf("repeat count %d is out of range", s.Count)
	}
	if len(s.Steps) == 0 {
		return errors.New("repeat needs steps")
	}
	if err := validateSteps(s.Steps); err != nil {
		return fmt.Errorf("repeat: %w", err)
	}
	return nil
}

// validate checks the fields that every tool call needs.
func (t toolCall) validate() error {
	if t.ID == "" || t.Title == "" {
		return errors.New("a tool call needs an id and a title")
	}
	if _, ok := toolKindOf(t.Kind); !ok {
		return fmt.Errorf("unknown tool kind %q", t.Kind)
	}
	if _, ok := toolStatusOf(t.Status); !ok {
		return fmt.Errorf("unknown tool status %q", t.Status)
	}
	if t.Write == nil {
		return nil
	}
	if t.Kind != "edit" {
		return errors.New("only an edit tool call can write a file")
	}
	if t.Write.Path == "" {
		return errors.New("write needs a path")
	}
	if t.Write.Content != "" && len(t.Write.Lines) > 0 {
		return errors.New("write takes content or lines, not both")
	}
	return nil
}

// toolKindOf maps a scenario kind to the protocol kind.
func toolKindOf(kind string) (acp.ToolKind, bool) {
	switch kind {
	case "read":
		return acp.ToolKindRead, true
	case "edit":
		return acp.ToolKindEdit, true
	case "execute":
		return acp.ToolKindExecute, true
	}
	return "", false
}

// toolStatusOf maps a scenario status to the protocol status. An empty status means completed.
func toolStatusOf(status string) (acp.ToolCallStatus, bool) {
	switch status {
	case "", statusOK:
		return acp.ToolCallStatusCompleted, true
	case statusFailed:
		return acp.ToolCallStatusFailed, true
	}
	return "", false
}

// stopReasonOf maps a scenario stop reason to the protocol value. An empty reason means the turn
// ended normally.
func stopReasonOf(reason string) (acp.StopReason, bool) {
	switch reason {
	case "", stopEndTurn:
		return acp.StopReasonEndTurn, true
	case "cancelled":
		return acp.StopReasonCancelled, true
	case "refusal":
		return acp.StopReasonRefusal, true
	case "max_tokens":
		return acp.StopReasonMaxTokens, true
	case "max_turn_requests":
		return acp.StopReasonMaxTurnRequests, true
	}
	return "", false
}
