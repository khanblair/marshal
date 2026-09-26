package protocol_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// The messages the app sends are plain structs, and the golden files are what the app's own
// tests read, so a change to one side shows up on the other.
func TestTerminalInputGolden(t *testing.T) {
	testutil.Golden(t, "terminal-input", protocol.TerminalInput{
		Type: protocol.FrameTypeTerminalInput, CardID: sampleCardID, Data: "ls -la\r",
	})
}

func TestTerminalInputWithANamedKeyGolden(t *testing.T) {
	testutil.Golden(t, "terminal-input-key", protocol.TerminalInput{
		Type: protocol.FrameTypeTerminalInput, CardID: sampleCardID, Key: protocol.TerminalKeyCtrlC,
	})
}

func TestTerminalResizeGolden(t *testing.T) {
	testutil.Golden(t, "terminal-resize", protocol.TerminalResize{
		Type: protocol.FrameTypeTerminalResize, CardID: sampleCardID, Cols: 96, Rows: 28,
	})
}

func TestTerminalSnapshotRequestGolden(t *testing.T) {
	testutil.Golden(t, "terminal-snapshot-request", protocol.TerminalSnapshotRequest{
		Type: protocol.FrameTypeTerminalSnapshot, CardID: sampleCardID,
	})
}

// The frames from the daemon set their own type, so a frame built without it is still right.
func TestTerminalScreenGolden(t *testing.T) {
	testutil.Golden(t, "terminal-screen", protocol.TerminalScreen{
		CardID: sampleCardID, Cols: 120, Rows: 32, ThroughSeq: 412,
		Data: base64.StdEncoding.EncodeToString([]byte("\x1b[2J\x1b[Hready\r\n")),
	})
}

func TestTerminalScreenOfAQuietTerminalGolden(t *testing.T) {
	testutil.Golden(t, "terminal-screen-empty", protocol.TerminalScreen{
		CardID: sampleCardID, Cols: 120, Rows: 32,
	})
}

func TestTerminalRefusalGolden(t *testing.T) {
	testutil.Golden(t, "terminal-refused", protocol.TerminalRefusal{
		CardID: sampleCardID,
		Error: *protocol.Refused("This card has no terminal running. Switch it to terminal view first.").
			With("cardId", sampleCardID).With("reason", string(protocol.TerminalRefusalReasonNotActive)),
	})
}

func TestTerminalOutputEventDataGolden(t *testing.T) {
	testutil.Golden(t, "session-terminal-output", protocol.TerminalOutputEventData{
		CardID: sampleCardID, Data: base64.StdEncoding.EncodeToString([]byte("echo: hi\r\n")),
	})
}

// A batch on the card's topic that has an output event in it, as the pump sends it.
func TestTerminalEventsGolden(t *testing.T) {
	event := func(seq uint64, typ protocol.EventType, data any) protocol.Event {
		return protocol.Event{
			Seq: seq, Topic: protocol.CardTopic(sampleCardID), Type: typ,
			At:   protocol.NewTimestamp(sampleTime.Add(time.Duration(seq) * 20 * time.Millisecond)),
			Data: encodeData(t, data),
		}
	}
	testutil.Golden(t, "terminal-events", protocol.EventBatch{
		Epoch: "01M3C0ZZZZ000000000000000A",
		Events: []protocol.Event{
			event(41, protocol.EventTypeSessionStateChanged, protocol.SessionStateChangedEventData{
				CardID: sampleCardID, SessionID: sampleSessionID, State: protocol.SessionStateAwake,
			}),
			event(42, protocol.EventTypeSessionTerminalOutput, protocol.TerminalOutputEventData{
				CardID: sampleCardID, Data: base64.StdEncoding.EncodeToString([]byte("ready\r\n")),
			}),
		},
	})
}

func TestSetViewRequestGolden(t *testing.T) {
	testutil.Golden(t, "set-view-request", protocol.SetViewRequest{Mode: protocol.CardViewModeTerminal})
}

func TestCardViewGolden(t *testing.T) {
	testutil.Golden(t, "card-view", protocol.CardView{
		CardID: sampleCardID, Mode: protocol.CardViewModeTerminal, Session: protocol.SessionStateAwake,
		ServerTime: protocol.NewTimestamp(sampleTime.Add(time.Hour)),
	})
}

// A card in the terminal view has a session that runs, and says so.
func TestCardInTheTerminalViewGolden(t *testing.T) {
	card := sampleCard()
	awake := protocol.SessionStateAwake
	card.State, card.Session, card.ViewMode = protocol.CardStateWorking, &awake, protocol.CardViewModeTerminal
	testutil.Golden(t, "card-terminal", card)
}

// The frames that come from the daemon carry their type, and a frame built without one is written
// with the right one.
func TestTerminalFramesSetTheirOwnType(t *testing.T) {
	tests := []struct {
		name  string
		frame any
		want  protocol.FrameType
	}{
		{"screen", protocol.TerminalScreen{CardID: sampleCardID}, protocol.FrameTypeTerminalScreen},
		{"refusal", protocol.TerminalRefusal{CardID: sampleCardID}, protocol.FrameTypeTerminalRefused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.frame)
			if err != nil {
				t.Fatal(err)
			}
			var head struct {
				Type protocol.FrameType `json:"type"`
			}
			if err := json.Unmarshal(data, &head); err != nil || head.Type != tt.want {
				t.Errorf("type = %q (%v), want %q", head.Type, err, tt.want)
			}
		})
	}
}

// Every named key has the bytes an xterm sends for it, and a name that is not a key has none.
func TestTerminalKeysAreTheBytesATerminalSends(t *testing.T) {
	want := map[protocol.TerminalKey]string{
		protocol.TerminalKeyEnter:     "\r",
		protocol.TerminalKeyEsc:       "\x1b",
		protocol.TerminalKeyTab:       "\t",
		protocol.TerminalKeyShiftTab:  "\x1b[Z",
		protocol.TerminalKeyBackspace: "\x7f",
		protocol.TerminalKeyDelete:    "\x1b[3~",
		protocol.TerminalKeyUp:        "\x1b[A",
		protocol.TerminalKeyDown:      "\x1b[B",
		protocol.TerminalKeyRight:     "\x1b[C",
		protocol.TerminalKeyLeft:      "\x1b[D",
		protocol.TerminalKeyHome:      "\x1b[H",
		protocol.TerminalKeyEnd:       "\x1b[F",
		protocol.TerminalKeyPageUp:    "\x1b[5~",
		protocol.TerminalKeyPageDown:  "\x1b[6~",
		protocol.TerminalKeyCtrlC:     "\x03",
		protocol.TerminalKeyCtrlD:     "\x04",
		protocol.TerminalKeyCtrlL:     "\x0c",
		protocol.TerminalKeyCtrlZ:     "\x1a",
	}
	for _, key := range protocol.TerminalKeyValues() {
		bytesOfKey := key.Bytes()
		if !key.Valid() || len(bytesOfKey) == 0 {
			t.Errorf("%s is listed but is not a usable key", key)
		}
		if got := want[key]; !bytes.Equal(bytesOfKey, []byte(got)) {
			t.Errorf("%s sends %q, want %q", key, bytesOfKey, got)
		}
	}
	if len(want) != len(protocol.TerminalKeyValues()) {
		t.Errorf("the table has %d keys and the list has %d", len(want), len(protocol.TerminalKeyValues()))
	}
	if protocol.TerminalKey("f13").Valid() || protocol.TerminalKey("f13").Bytes() != nil {
		t.Error("a name that is not a key must have no bytes")
	}
}

func TestTerminalReasonsAreValidWordsWithNoSpaces(t *testing.T) {
	for _, r := range protocol.ViewRefusalReasonValues() {
		if !r.Valid() || strings.ContainsAny(string(r), " -") {
			t.Errorf("view refusal reason %q", r)
		}
	}
	for _, r := range protocol.TerminalRefusalReasonValues() {
		if !r.Valid() || strings.ContainsAny(string(r), " -") {
			t.Errorf("terminal refusal reason %q", r)
		}
	}
	if protocol.ViewRefusalReason("nope").Valid() || protocol.TerminalRefusalReason("nope").Valid() {
		t.Error("an unknown reason must not be valid")
	}
}

// The limits are what the docs say, and a message of nothing but control characters, which JSON
// writes as six bytes each, still fits in the 64 KiB that the stream reads.
func TestTerminalLimits(t *testing.T) {
	if worst := protocol.MaxTerminalInputBytes * len(`\u001b`); worst >= 64<<10 {
		t.Errorf("a full message of control characters is %d bytes in JSON, over the stream's read limit", worst)
	}
	if protocol.MaxTerminalInputBytes != 8<<10 || protocol.MaxTerminalCols != 500 || protocol.MaxTerminalRows != 200 {
		t.Errorf("limits are %d, %d, %d", protocol.MaxTerminalInputBytes, protocol.MaxTerminalCols, protocol.MaxTerminalRows)
	}
}
