package protocol_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

const (
	cardID = "01M3C107JB041061050R3GG28A"
	chatID = "01M3C107JC0R3GG28A04106105"
)

func TestEventBatchGolden(t *testing.T) {
	at := time.Date(2026, time.September, 25, 10, 15, 30, 123_000_000, time.UTC)
	batch := protocol.EventBatch{
		Epoch: "01M3C0ZZZZ000000000000000A",
		Events: []protocol.Event{
			{
				Seq: 41, Topic: protocol.ProjectTopic("web-dashboard"), Type: protocol.EventTypeCardCreated,
				At: protocol.NewTimestamp(at), Data: json.RawMessage(`{"cardId":"` + cardID + `"}`),
			},
			{
				Seq: 42, Topic: protocol.CardTopic(cardID), Type: protocol.EventTypeSessionStateChanged,
				At: protocol.NewTimestamp(at.Add(20 * time.Millisecond)), Data: json.RawMessage(`{"state":"working"}`),
			},
		},
	}
	testutil.Golden(t, "event-batch", batch)
}

func TestHelloGolden(t *testing.T) {
	testutil.Golden(t, "hello", protocol.Hello{
		Type:      protocol.FrameTypeHello,
		Subscribe: []protocol.Topic{protocol.HomeTopic, protocol.ProjectTopic("web-dashboard"), protocol.CardTopic(cardID)},
		SinceSeq:  41,
		Epoch:     "01M3C0ZZZZ000000000000000A",
	})
}

func TestResyncGolden(t *testing.T) {
	testutil.Golden(t, "resync", protocol.Resync{
		Epoch:  "01M3C0ZZZZ000000000000000B",
		Reason: protocol.ResyncReasonEpochChanged,
		Seq:    0,
	})
}

func TestErrorFrameGolden(t *testing.T) {
	err := protocol.InvalidArgument("That topic is not one Marshal knows. Check the topic and connect again.").With("topic", "project:Bad_ID")
	testutil.Golden(t, "error-frame", protocol.ErrorFrame{Error: *err})
}

// A frame always says what it is, even when the code that built it forgot to set the type, and
// the type is the first field so a reader sees it first.
func TestServerFramesAlwaysCarryTheirType(t *testing.T) {
	tests := []struct {
		name  string
		frame any
		want  string
	}{
		{"batch", protocol.EventBatch{Epoch: "e", Events: []protocol.Event{}}, `{"type":"events","epoch":"e","events":[]}`},
		{"batch with a wrong type", protocol.EventBatch{Type: protocol.FrameTypeResync, Epoch: "e", Events: []protocol.Event{}}, `{"type":"events","epoch":"e","events":[]}`},
		{"resync", protocol.Resync{Epoch: "e", Reason: protocol.ResyncReasonTooFarBehind, Seq: 7}, `{"type":"resync","epoch":"e","reason":"too-far-behind","seq":7}`},
		{"error", protocol.ErrorFrame{Error: *protocol.InvalidArgument("Bad.")}, `{"type":"error","error":{"code":"invalid_argument","message":"Bad."}}`},
		{"pointer to a batch", &protocol.EventBatch{Epoch: "e", Events: []protocol.Event{}}, `{"type":"events","epoch":"e","events":[]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.frame)
			if err != nil || string(got) != tc.want {
				t.Errorf("frame = %s, %v; want %s", got, err, tc.want)
			}
		})
	}
}

func TestFrameTypes(t *testing.T) {
	for _, typ := range protocol.FrameTypeValues() {
		if !typ.Valid() {
			t.Errorf("%q is listed but not valid", typ)
		}
	}
	if protocol.FrameType("ping").Valid() {
		t.Error("FrameType accepts a word that is not a frame")
	}
}

func TestTopicConstructorsAndParser(t *testing.T) {
	tests := []struct {
		name     string
		topic    protocol.Topic
		wantText string
		kind     protocol.TopicKind
		id       string
	}{
		{"home", protocol.HomeTopic, "home", protocol.TopicKindHome, ""},
		{"project", protocol.ProjectTopic("web-dashboard"), "project:web-dashboard", protocol.TopicKindProject, "web-dashboard"},
		{"project id with digits", protocol.ProjectTopic("app-2-go"), "project:app-2-go", protocol.TopicKindProject, "app-2-go"},
		{"card", protocol.CardTopic(cardID), "card:" + cardID, protocol.TopicKindCard, cardID},
		{"chat", protocol.ChatTopic(chatID), "chat:" + chatID, protocol.TopicKindChat, chatID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.topic) != tc.wantText {
				t.Fatalf("topic = %q, want %q", tc.topic, tc.wantText)
			}
			kind, id, err := protocol.ParseTopic(tc.topic)
			if err != nil || kind != tc.kind || id != tc.id {
				t.Errorf("ParseTopic(%q) = %q, %q, %v; want %q, %q", tc.topic, kind, id, err, tc.kind, tc.id)
			}
		})
	}
}

func TestParseTopicRejectsJunk(t *testing.T) {
	bad := []protocol.Topic{
		"", "home:", "home:x", "project", "project:", "project:Bad_ID", "project:1abc",
		"card:", "card:api", "card:" + cardID + "x", "chat:not-an-id", "session:" + cardID,
		"Project:api", " home", "*",
	}
	for _, topic := range bad {
		if kind, id, err := protocol.ParseTopic(topic); err == nil {
			t.Errorf("ParseTopic(%q) = %q, %q, want an error", topic, kind, id)
		}
	}
}

func TestEventTypesAreDottedNames(t *testing.T) {
	seen := map[protocol.EventType]bool{}
	for _, typ := range protocol.EventTypeValues() {
		if seen[typ] {
			t.Errorf("event type %q is listed twice", typ)
		}
		seen[typ] = true
		if !bytes.Contains([]byte(typ), []byte(".")) {
			t.Errorf("event type %q is not <thing>.<what happened>", typ)
		}
	}
	for _, phase1 := range []protocol.EventType{
		protocol.EventTypeProjectCreated, protocol.EventTypeProjectUpdated, protocol.EventTypeProjectRemoved,
		protocol.EventTypeCardCreated, protocol.EventTypeCardUpdated, protocol.EventTypeCardMoved,
		protocol.EventTypeSessionStateChanged, protocol.EventTypeSessionOutput, protocol.EventTypeSessionToolCall,
	} {
		if !phase1.Valid() {
			t.Errorf("phase 1 event type %q is missing from the list", phase1)
		}
	}
}

func TestSubprotocolNamesAndReplaySize(t *testing.T) {
	if protocol.WebSocketSubprotocol != "marshal.v1" || protocol.BearerSubprotocolPrefix != "bearer." {
		t.Errorf("subprotocols are %q and %q", protocol.WebSocketSubprotocol, protocol.BearerSubprotocolPrefix)
	}
	if protocol.ReplayBufferSize != 2000 {
		t.Errorf("ReplayBufferSize = %d, docs/architecture.md section 11.5 says 2,000", protocol.ReplayBufferSize)
	}
}
