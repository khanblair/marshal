package protocol_test

import (
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

const sampleSessionID = "01M3C107JF041061050R3GG28B"

func TestSessionOutputEventDataGolden(t *testing.T) {
	testutil.Golden(t, "session-output", protocol.SessionOutputEventData{
		CardID: sampleCardID,
		Kind:   "message",
		Text:   "I read the code and found the health check route.",
	})
}

func TestSessionOutputEventDataForAPlanGolden(t *testing.T) {
	testutil.Golden(t, "session-output-plan", protocol.SessionOutputEventData{
		CardID: sampleCardID,
		Kind:   "plan",
		Plan: []protocol.PlanStep{
			{Text: "Read the router", Status: "completed"},
			{Text: "Add the health check route", Status: "in_progress"},
			{Text: "Write a test", Status: "pending"},
		},
	})
}

func TestSessionToolCallEventDataGolden(t *testing.T) {
	testutil.Golden(t, "session-tool-call", protocol.SessionToolCallEventData{
		CardID: sampleCardID,
		Kind:   "tool_call",
		ToolCall: protocol.AgentToolCall{
			ID: "call-1", Title: "Read main.go", ToolKind: "read", Status: "in_progress",
			Path: "/home/ada/code/api/main.go",
		},
	})
}

func TestSessionToolCallUpdateEventDataGolden(t *testing.T) {
	testutil.Golden(t, "session-tool-call-update", protocol.SessionToolCallEventData{
		CardID: sampleCardID,
		Kind:   "tool_call_update",
		ToolCall: protocol.AgentToolCall{
			ID: "call-1", Status: "completed", Content: "package main\n",
			Diffs: []protocol.FileDiff{{Path: "main.go", OldText: "", NewText: "package main\n"}},
		},
	})
}

func TestSessionStateChangedEventDataGolden(t *testing.T) {
	testutil.Golden(t, "session-state-changed", protocol.SessionStateChangedEventData{
		CardID: sampleCardID, SessionID: sampleSessionID, State: protocol.SessionStateAwake,
	})
}

func TestSessionStateChangedEventDataWithReasonGolden(t *testing.T) {
	testutil.Golden(t, "session-state-changed-reason", protocol.SessionStateChangedEventData{
		CardID: sampleCardID, SessionID: sampleSessionID, State: protocol.SessionStateStopped,
		Reason: "Marshal could not pick this session back up. Start a fresh session from the card.",
	})
}

// The batch holds one event of each kind the session manager sends, on the card's own topic.
func TestSessionEventsGolden(t *testing.T) {
	event := func(seq uint64, typ protocol.EventType, data any) protocol.Event {
		return protocol.Event{
			Seq: seq, Topic: protocol.CardTopic(sampleCardID), Type: typ,
			At:   protocol.NewTimestamp(sampleTime.Add(time.Duration(seq) * 20 * time.Millisecond)),
			Data: encodeData(t, data),
		}
	}
	testutil.Golden(t, "session-events", protocol.EventBatch{
		Epoch: "01M3C0ZZZZ000000000000000A",
		Events: []protocol.Event{
			event(1, protocol.EventTypeSessionStateChanged, protocol.SessionStateChangedEventData{
				CardID: sampleCardID, SessionID: sampleSessionID, State: protocol.SessionStateAwake,
			}),
			event(2, protocol.EventTypeSessionOutput, protocol.SessionOutputEventData{
				CardID: sampleCardID, Kind: "message", Text: "Looking at the code now.",
			}),
			event(3, protocol.EventTypeSessionToolCall, protocol.SessionToolCallEventData{
				CardID: sampleCardID, Kind: "tool_call",
				ToolCall: protocol.AgentToolCall{ID: "call-1", Title: "Read main.go", ToolKind: "read", Status: "in_progress"},
			}),
			event(4, protocol.EventTypeSessionToolCall, protocol.SessionToolCallEventData{
				CardID: sampleCardID, Kind: "tool_call_update",
				ToolCall: protocol.AgentToolCall{ID: "call-1", Status: "completed"},
			}),
			event(5, protocol.EventTypeSessionStateChanged, protocol.SessionStateChangedEventData{
				CardID: sampleCardID, SessionID: sampleSessionID, State: protocol.SessionStateStopped,
			}),
		},
	})
}

// A project chat's session sends the same three events on chat:<id>, each carrying chatId where a
// card's carries cardId. cardId stays in the payload as an empty string, so its type does not
// become optional for the clients that read a card's events.
func TestChatSessionEventsGolden(t *testing.T) {
	chatID := sampleChat().ID
	event := func(seq uint64, typ protocol.EventType, data any) protocol.Event {
		return protocol.Event{
			Seq: seq, Topic: protocol.ChatTopic(chatID), Type: typ,
			At:   protocol.NewTimestamp(sampleTime.Add(time.Duration(seq) * 20 * time.Millisecond)),
			Data: encodeData(t, data),
		}
	}
	testutil.Golden(t, "chat-session-events", protocol.EventBatch{
		Epoch: "01M3C0ZZZZ000000000000000A",
		Events: []protocol.Event{
			event(1, protocol.EventTypeSessionStateChanged, protocol.SessionStateChangedEventData{
				ChatID: chatID, SessionID: sampleSessionID, State: protocol.SessionStateWorking,
			}),
			event(2, protocol.EventTypeSessionOutput, protocol.SessionOutputEventData{
				ChatID: chatID, Kind: "message", Text: "Nothing is blocked right now.",
			}),
			event(3, protocol.EventTypeSessionToolCall, protocol.SessionToolCallEventData{
				ChatID: chatID, Kind: "tool_call",
				ToolCall: protocol.AgentToolCall{ID: "call-1", Title: "List the cards", ToolKind: "other", Status: "in_progress"},
			}),
			event(4, protocol.EventTypeSessionStateChanged, protocol.SessionStateChangedEventData{
				ChatID: chatID, SessionID: sampleSessionID, State: protocol.SessionStateAsleep,
			}),
		},
	})
}
