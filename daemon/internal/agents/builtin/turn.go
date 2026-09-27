package builtin

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/agents"
	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/providers"
)

// turn is one model call and everything that follows from it.
type turn struct {
	parts      []providers.Part
	stopReason string
}

// runTurn is the loop. It appends the person's message, calls the model, runs whatever tools the
// model asked for, feeds the results back, and repeats until the model answers without asking for a
// tool, a limit stops it, or the turn is interrupted. It runs on the session's own goroutine and
// always ends by reporting one TurnEnded.
func (s *session) runTurn(ctx context.Context, instructions, userText string) {
	cfg := s.harnessConfig()
	s.mu.Lock()
	s.history = append(s.history, providers.Message{
		Role: providers.RoleUser, Parts: []providers.Part{{Kind: providers.PartText, Text: userText}},
	})
	history := slices.Clone(s.history)
	s.mu.Unlock()

	for range s.cfg.MaxTurns {
		if ctx.Err() != nil {
			s.endTurn(agents.TurnCancelled)
			return
		}
		result, err := s.callProvider(ctx, instructions, history)
		if err != nil {
			if ctx.Err() != nil {
				s.endTurn(agents.TurnCancelled)
				return
			}
			s.failedTurn(failureMessage(err), err)
			return
		}
		history = append(history, providers.Message{Role: providers.RoleAssistant, Parts: result.parts})
		s.recordAssistant(result.parts)

		calls := toolUses(result.parts)
		if len(calls) == 0 {
			s.endTurn(turnReason(result.stopReason))
			return
		}
		results := s.runTools(ctx, cfg, calls)
		if ctx.Err() != nil {
			s.endTurn(agents.TurnCancelled)
			return
		}
		history = append(history, providers.Message{Role: providers.RoleUser, Parts: results})
	}
	s.endTurn(agents.TurnMaxRequests)
}

// callProvider makes one streamed call and collects the model's whole answer, emitting the text and
// reasoning as it arrives.
func (s *session) callProvider(ctx context.Context, instructions string, history []providers.Message) (turn, error) {
	stream, err := s.resolved.Client.Stream(ctx, providers.Request{
		Model: s.resolved.Model, System: s.systemPrompt(instructions), Messages: history,
		Tools: s.toolSpecs(), Thinking: s.thinking, MaxTokens: s.cfg.MaxTokens,
	})
	if err != nil {
		return turn{}, err
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			s.log.Debug("closing a provider stream", "err", closeErr)
		}
	}()

	var out turn
	var text, thinking strings.Builder
	var thinkingSig string
	flushText := func() {
		if text.Len() > 0 {
			out.parts = append(out.parts, providers.Part{Kind: providers.PartText, Text: text.String()})
			text.Reset()
		}
	}
	flushThinking := func() {
		if thinking.Len() > 0 {
			out.parts = append(out.parts, providers.Part{Kind: providers.PartThinking, Text: thinking.String(), Signature: thinkingSig})
			thinking.Reset()
			thinkingSig = ""
		}
	}

	for {
		ev, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return turn{}, recvErr
		}
		switch ev.Kind {
		case providers.EventText:
			if ev.Text != "" {
				flushThinking()
				text.WriteString(ev.Text)
				s.emit(agents.MessageChunk{Text: ev.Text})
			}
		case providers.EventThinking:
			if ev.Text != "" {
				flushText()
				thinking.WriteString(ev.Text)
				if ev.Signature != "" {
					thinkingSig = ev.Signature
				}
				s.emit(agents.ThoughtChunk{Text: ev.Text})
			}
		case providers.EventToolUse:
			flushText()
			flushThinking()
			out.parts = append(out.parts, providers.Part{
				Kind: providers.PartToolUse, ToolUseID: ev.ToolUseID, ToolName: ev.ToolName, ToolInput: ev.ToolInput,
			})
		case providers.EventDone:
			out.stopReason = ev.StopReason
		}
	}
	flushText()
	flushThinking()

	// A stream that ends without a stop reason still ends the model's answer.
	if out.stopReason == "" {
		out.stopReason = providers.StopEndTurn
	}
	return out, nil
}

// toolUse is one tool the model asked for.
type toolUse struct {
	ID    string
	Name  string
	Input []byte
}

// toolUses pulls the tool calls out of an answer's parts, in order.
func toolUses(parts []providers.Part) []toolUse {
	var out []toolUse
	for _, p := range parts {
		if p.Kind == providers.PartToolUse {
			out = append(out, toolUse{ID: p.ToolUseID, Name: p.ToolName, Input: p.ToolInput})
		}
	}
	return out
}

// recordAssistant keeps the model's answer in the session's history.
func (s *session) recordAssistant(parts []providers.Part) {
	s.mu.Lock()
	s.history = append(s.history, providers.Message{Role: providers.RoleAssistant, Parts: parts})
	s.mu.Unlock()
}

// runTools runs every tool call the model asked for and returns the results, in the same order, as
// one user message's parts. The caller records that message in the history.
func (s *session) runTools(ctx context.Context, cfg harness.Config, calls []toolUse) []providers.Part {
	out := make([]providers.Part, 0, len(calls))
	for _, call := range calls {
		out = append(out, s.runTool(ctx, cfg, call))
	}
	return out
}

// runTool decides one tool call and runs it when it is allowed. It always emits a ToolCall and a
// closing ToolCallUpdate, and returns the part the model is told.
func (s *session) runTool(ctx context.Context, cfg harness.Config, call toolUse) providers.Part {
	t := s.tool(call.Name)
	if t == nil {
		s.emit(agents.ToolCall{ID: call.ID, Title: call.Name, Kind: "other", Status: agents.StatusFailed})
		s.emit(agents.ToolCallUpdate{ID: call.ID, Status: agents.StatusFailed, Content: "the agent has no tool named " + call.Name})
		return refused(call, "there is no tool named "+call.Name)
	}
	args := parseArgs(call.Input)
	path, command := t.probe(s, args)
	// The permission rules are asked about the real path; the chat view and the model see the path
	// the way the working folder shows it.
	shown := ""
	if path != "" {
		shown = s.shownPath(path)
	}
	title := toolTitle(t.name, shown, command)

	req := harness.Request{Kind: t.kind, Path: path, Command: command}
	outcome := s.decideCall(cfg, t.name, req)

	s.emit(agents.ToolCall{
		ID: call.ID, Title: title, Kind: t.kind, Status: agents.StatusInProgress, Path: shown, Command: command,
	})

	switch outcome.Decision {
	case harness.DecisionDeny:
		text := refusalText(outcome.Rule)
		s.emit(agents.ToolCallUpdate{ID: call.ID, Status: agents.StatusFailed, Content: text})
		return refused(call, text)
	case harness.DecisionAsk:
		answer, ok := s.askPermission(ctx, call, t, title, shown, command)
		if !ok {
			s.emit(agents.ToolCallUpdate{ID: call.ID, Status: agents.StatusFailed, Content: "the person did not allow it"})
			return refused(call, "the person did not allow this tool call")
		}
		if answer == answerReject {
			s.emit(agents.ToolCallUpdate{ID: call.ID, Status: agents.StatusFailed, Content: "the person refused it"})
			return refused(call, "the person refused this tool call")
		}
	}

	result := t.run(ctx, s, args)
	status := agents.StatusCompleted
	if result.IsError {
		status = agents.StatusFailed
	}
	content, truncated := agents.Truncate(result.Text, agents.MaxContentBytes)
	s.emit(agents.ToolCallUpdate{ID: call.ID, Status: status, Content: content, Truncated: truncated})
	return providers.Part{
		Kind: providers.PartToolResult, ToolUseID: call.ID, ToolName: t.name,
		Text: result.Text, IsError: result.IsError,
	}
}

// decideCall answers one tool call's permission question, first checking what the person has already
// said about this tool in this session.
func (s *session) decideCall(cfg harness.Config, name string, req harness.Request) harness.Outcome {
	s.mu.Lock()
	allow, deny := s.willAllow[name], s.willDeny[name]
	s.mu.Unlock()
	if deny {
		return harness.Outcome{Decision: harness.DecisionDeny, Rule: "the person refused this tool for this session"}
	}
	if allow {
		return harness.Outcome{Decision: harness.DecisionAllow, Rule: "the person allowed this tool for this session"}
	}
	return cfg.Decide(req)
}

// answerChoice is what a person's answer to a permission request means for this call.
type answerChoice int

const (
	answerAllow answerChoice = iota
	answerReject
)

// askPermission shows a request to the caller and waits for the answer. It reports false when
// nobody will answer (the request was withdrawn because the turn ended), and true with the choice
// otherwise. A lasting answer is remembered for the session.
func (s *session) askPermission(
	ctx context.Context, call toolUse, t *tool, title, path, command string,
) (answerChoice, bool) {
	p := &pendingPermission{options: permissionOptions(t.name), answer: make(chan agents.ApprovalResponse, 1)}
	if !s.registerPermission(p) {
		return answerReject, false
	}
	s.emit(agents.PermissionRequested{
		RequestID: p.id, ToolCallID: call.ID, Title: title, Kind: t.kind,
		Path: path, Command: command, Options: p.options,
	})
	var answer agents.ApprovalResponse
	select {
	case answer = <-p.answer:
	case <-ctx.Done():
		s.dropPending(p.id)
		return answerReject, false
	case <-s.life.Done():
		s.dropPending(p.id)
		return answerReject, false
	}
	if answer.Cancelled {
		return answerReject, false
	}
	switch answer.OptionID {
	case optionAllowOnce:
		return answerAllow, true
	case optionAllowAlways:
		s.setLasting(t.name, true)
		return answerAllow, true
	case optionRejectAlways:
		s.setLasting(t.name, false)
		return answerReject, true
	default:
		return answerReject, true
	}
}

// setLasting remembers a person's "always" answer for the session, by tool name.
func (s *session) setLasting(name string, allow bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if allow {
		s.willAllow[name] = true
		delete(s.willDeny, name)
		return
	}
	s.willDeny[name] = true
	delete(s.willAllow, name)
}

// ids of the permission options this agent offers. They are stable, so a caller can name one
// without reading the request.
const (
	optionAllowOnce    = "allow_once"
	optionAllowAlways  = "allow_always"
	optionRejectOnce   = "reject_once"
	optionRejectAlways = "reject_always"
)

// permissionOptions is the set of answers a permission request offers for a tool.
func permissionOptions(name string) []agents.PermissionOption {
	return []agents.PermissionOption{
		{ID: optionAllowOnce, Name: "Allow once", Kind: agents.OptionAllowOnce},
		{ID: optionAllowAlways, Name: "Always allow " + name, Kind: agents.OptionAllowAlways},
		{ID: optionRejectOnce, Name: "Reject once", Kind: agents.OptionRejectOnce},
		{ID: optionRejectAlways, Name: "Always reject " + name, Kind: agents.OptionRejectAlways},
	}
}

// tool finds a tool by name, or nil when the agent has none.
func (s *session) tool(name string) *tool {
	for i := range s.tools {
		if s.tools[i].name == name {
			return &s.tools[i]
		}
	}
	return nil
}

// toolSpecs is the tool set in the shape a provider request takes.
func (s *session) toolSpecs() []providers.Tool {
	out := make([]providers.Tool, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, providers.Tool{Name: t.name, Description: t.description, InputSchema: t.schema})
	}
	return out
}

// harnessConfig builds the permission rules for this session.
func (s *session) harnessConfig() harness.Config {
	containment := s.cfg.Containment(s.cwd)
	return harness.Config{
		Mode: s.permMode, Profile: s.cfg.Profile, Blocklist: s.cfg.Blocklist, Containment: &containment,
	}
}

// refused is the part the model is told when a tool call did not run.
func refused(call toolUse, why string) providers.Part {
	return providers.Part{Kind: providers.PartToolResult, ToolUseID: call.ID, ToolName: call.Name, Text: why, IsError: true}
}

// refusalText is the sentence for a call the harness refused itself.
func refusalText(rule string) string {
	if rule == "" {
		return "Marshal refused this tool call."
	}
	return "Marshal refused this tool call (" + rule + ")."
}

// toolTitle is the short label the chat view shows for a tool call.
func toolTitle(name, path, command string) string {
	switch {
	case path != "":
		return name + " " + path
	case command != "":
		text, _ := agents.Truncate(command, maxTitleCommandBytes)
		return name + ": " + text
	default:
		return name
	}
}

// maxTitleCommandBytes bounds how much of a command goes into a tool call's title.
const maxTitleCommandBytes = 80

// turnReason maps a provider stop reason to an agents.TurnEnded reason.
func turnReason(stop string) string {
	switch stop {
	case providers.StopMaxTokens:
		return agents.TurnMaxTokens
	case providers.StopRefusal:
		return agents.TurnRefusal
	default:
		return agents.TurnEndTurn
	}
}

// failureMessage is the plain sentence for a turn that could not call the provider. A missing key
// and a refused key get their own sentence, since those are things a person can fix.
func failureMessage(err error) string {
	switch {
	case errors.Is(err, providers.ErrAuth):
		return "Marshal's key for this provider was refused. Check it in Settings, then try again."
	case errors.Is(err, providers.ErrRateLimited):
		return "The provider is rate limiting Marshal. Wait a moment, then try again."
	case errors.Is(err, providers.ErrUnavailable):
		return "The provider could not be reached. Check your connection, then try again."
	case errors.Is(err, providers.ErrNoSuchModel):
		return "The provider does not offer this model. Choose another model in the card's settings."
	default:
		return "Marshal could not reach the model. Try again."
	}
}
