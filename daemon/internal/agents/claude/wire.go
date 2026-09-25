package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// lineHead is read from every line Claude Code writes to standard output, to decide which shape
// to decode the rest of the line into. See docs/architecture.md section 4.2 for where this fits.
type lineHead struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
}

// systemLine is a "system" message. The one this adapter waits for is subtype "init", which
// carries the session id and arrives once near the start of the process's output, ahead of any
// input this adapter has to send. Other subtypes, such as "api_retry" and "plugin_install", are
// read as far as lineHead and then skipped: Marshal has no event for them yet.
type systemLine struct {
	SessionID string `json:"session_id"`
}

// streamEventLine is a "stream_event" message: a raw Claude API streaming event, present only
// because Start passes --include-partial-messages. Only content_block_delta events carry text or
// thinking chunks; the rest (message_start, content_block_start and _stop, message_delta,
// message_stop) are read this far and dropped.
type streamEventLine struct {
	Event struct {
		Type  string `json:"type"`
		Delta struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"delta"`
	} `json:"event"`
}

// messageLine is an "assistant" or a "user" message: one content block of the model's answer, or
// the echo of a tool's result. Claude Code emits one assistant message per finished content
// block, so only its tool_use blocks are new information here; its text and thinking already
// arrived as stream_event deltas.
type messageLine struct {
	Message struct {
		Content []contentBlock `json:"content"`
	} `json:"message"`
}

// contentBlock is one block of an assistant or user message. Which fields are set depends on
// Type: "tool_use" carries ID, Name, and Input; "tool_result" carries ToolUseID, Content, and
// IsError; "text" carries Text, and is only read here to build a tool result's content.
type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// resultLine is the "result" message that ends a turn. Subtype "success" is a normal end;
// "error_max_turns" is documented; every other non-empty subtype is read as a generic failure.
// Claude Code also reports duration_ms, total_cost_usd, and usage on this line, which are left
// undecoded here: nothing reads them yet, and a future usage-tracking task is the place to add
// them, not this adapter.
type resultLine struct {
	Subtype    string `json:"subtype"`
	IsError    bool   `json:"is_error"`
	Result     string `json:"result"`
	StopReason string `json:"stop_reason"`
}

// controlResponseLine answers a control_request this adapter sent, such as the interrupt request.
// Its own outcome is not acted on (see turn.go): the request is a best-effort nudge, and the
// interrupt grace timer is what actually bounds how long Interrupt waits.
type controlResponseLine struct {
	Response struct {
		RequestID string `json:"request_id"`
		Subtype   string `json:"subtype"`
	} `json:"response"`
}

// outUserMessage is one line of stream-json input: a user's turn. Its shape matches what the
// Claude Agent SDK's streaming input mode sends on the wire (confirmed from Anthropic's own docs,
// see the report's Ruling on the input schema), since the CLI's --input-format stream-json is
// that same wire format.
type outUserMessage struct {
	Type    string `json:"type"`
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	ParentToolUseID *string `json:"parent_tool_use_id"`
}

// encodeUserMessage builds one line of stream-json input carrying a user's message text.
func encodeUserMessage(text string) ([]byte, error) {
	var line outUserMessage
	line.Type = "user"
	line.Message.Role = "user"
	line.Message.Content = text
	return encodeLine(line)
}

// outControlRequest is a control_request line: the documented way to ask a running claude -p
// process to do something other than answer a prompt. This adapter only ever asks for "interrupt".
type outControlRequest struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
	Request   struct {
		Subtype string `json:"subtype"`
	} `json:"request"`
}

// encodeInterruptRequest builds a control_request line that asks Claude Code to stop the turn in
// flight. id is echoed back on the matching control_response.
func encodeInterruptRequest(id string) ([]byte, error) {
	var line outControlRequest
	line.Type = "control_request"
	line.RequestID = id
	line.Request.Subtype = "interrupt"
	return encodeLine(line)
}

// encodeLine marshals v and adds the newline that stream-json input needs, one JSON value per
// line.
func encodeLine(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("build a message for claude code: %w", err)
	}
	return append(b, '\n'), nil
}

// toolResultText reads the text of a tool_result block's content, which Claude Code sends either
// as a plain JSON string or as an array of content blocks (only the text ones are kept).
func toolResultText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s
		}
	case '[':
		var blocks []contentBlock
		if err := json.Unmarshal(trimmed, &blocks); err == nil {
			return joinText(blocks)
		}
	}
	return ""
}

// joinText joins the text of every text block, in order.
func joinText(blocks []contentBlock) string {
	var out []byte
	for _, b := range blocks {
		if b.Type != "text" || b.Text == "" {
			continue
		}
		if len(out) > 0 {
			out = append(out, '\n')
		}
		out = append(out, b.Text...)
	}
	return string(out)
}
