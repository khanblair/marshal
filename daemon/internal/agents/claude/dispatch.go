package claude

import (
	"encoding/json"

	"github.com/khanblair/marshal/daemon/internal/agents"
)

// handleSystem reads a "system" line. The only subtype this adapter waits for is "init", which
// carries the session id and unblocks awaitReady; every other subtype ("api_retry",
// "plugin_install", and so on) is read this far and dropped, since Marshal has no event for them.
func (s *session) handleSystem(subtype string, line []byte, closeReady func()) {
	if subtype != "init" {
		s.log.Debug("skipped a system line from claude code", "subtype", subtype)
		return
	}
	var m systemLine
	if err := json.Unmarshal(line, &m); err != nil {
		s.log.Warn("could not read claude code's init line", "err", err)
	} else if m.SessionID != "" && m.SessionID != s.id {
		s.log.Warn("claude code reported a different session id than asked for", "want", s.id, "got", m.SessionID)
	}
	closeReady()
}

// handleStreamEvent reads a "stream_event" line. Only a content_block_delta with a text or a
// thinking delta becomes an event; the rest of the raw API stream (message_start,
// content_block_start and _stop, message_delta, message_stop) carries nothing this adapter uses.
func (s *session) handleStreamEvent(line []byte) {
	var m streamEventLine
	if err := json.Unmarshal(line, &m); err != nil {
		s.log.Warn("could not read a streaming event from claude code", "err", err)
		return
	}
	if m.Event.Type != "content_block_delta" {
		return
	}
	switch m.Event.Delta.Type {
	case "text_delta":
		s.emit(agents.MessageChunk{Text: m.Event.Delta.Text})
	case "thinking_delta":
		s.emit(agents.ThoughtChunk{Text: m.Event.Delta.Thinking})
	}
}

// handleAssistant reads an "assistant" line: one finished content block of the model's answer.
// Only its tool_use blocks are new information here; text and thinking already arrived as
// stream_event deltas, so taking them again here would show every answer twice.
func (s *session) handleAssistant(line []byte) {
	s.forEachBlock(line, "an assistant message", "tool_use", func(b contentBlock) agents.AgentEvent {
		return toolCallEvent(b)
	})
}

// handleUser reads a "user" line. In stream-json output this is Claude Code echoing a tool's
// result back, not a real person's message, so only its tool_result blocks matter here.
func (s *session) handleUser(line []byte) {
	s.forEachBlock(line, "a tool result", "tool_result", func(b contentBlock) agents.AgentEvent {
		return toolResultEvent(b)
	})
}

// forEachBlock reads a message line and emits the event that toEvent builds, for every content
// block of the given type. what names the line for the log message on a line that will not parse.
func (s *session) forEachBlock(line []byte, what, blockType string, toEvent func(contentBlock) agents.AgentEvent) {
	var m messageLine
	if err := json.Unmarshal(line, &m); err != nil {
		s.log.Warn("could not read "+what+" from claude code", "err", err)
		return
	}
	for _, b := range m.Message.Content {
		if b.Type == blockType {
			s.emit(toEvent(b))
		}
	}
}

// handleControlResponse reads a control_response line: Claude Code's answer to a control_request
// this adapter sent, such as the interrupt request in turn.go. The answer is not acted on; it is
// only logged, because the interrupt grace timer, not this response, is what actually bounds how
// long Interrupt waits.
func (s *session) handleControlResponse(line []byte) {
	var m controlResponseLine
	if err := json.Unmarshal(line, &m); err != nil {
		s.log.Debug("could not read a control response from claude code", "err", err)
		return
	}
	s.log.Debug("claude code answered a control request", "request_id", m.Response.RequestID, "subtype", m.Response.Subtype)
}

// handleResult reads a "result" line, which ends a turn, and passes it to endTurn.
func (s *session) handleResult(line []byte) {
	var m resultLine
	if err := json.Unmarshal(line, &m); err != nil {
		s.log.Warn("could not read a result from claude code", "err", err)
		return
	}
	s.endTurn(m)
}
