package chatcmd_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/chatcmd"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

type fakeControl struct {
	calls []string
	fail  error
}

func (f *fakeControl) Card(_ context.Context, key protocol.CardKey) (protocol.Card, error) {
	f.calls = append(f.calls, "card "+key.String())
	return protocol.Card{ID: "opaque-id", Key: key.String(), Title: "Fix login", State: protocol.CardStateWorking}, f.fail
}
func (f *fakeControl) Cards(context.Context) ([]protocol.Card, error) {
	f.calls = append(f.calls, "cards")
	return []protocol.Card{{Key: "web#3", Title: "Fix login", State: protocol.CardStateWorking}}, f.fail
}
func (f *fakeControl) Start(_ context.Context, id string) (protocol.Card, error) {
	f.calls = append(f.calls, "start "+id)
	return protocol.Card{ID: id, Key: "web#3"}, f.fail
}
func (f *fakeControl) Stop(_ context.Context, id string) error {
	f.calls = append(f.calls, "stop "+id)
	return f.fail
}
func (f *fakeControl) Send(_ context.Context, id, text string) error {
	f.calls = append(f.calls, "message "+id+" "+text)
	return f.fail
}
func (f *fakeControl) Reply(_ context.Context, id string) (string, error) {
	f.calls = append(f.calls, "reply "+id)
	return strings.Repeat("🧩", 1200), f.fail
}
func (f *fakeControl) SetAgent(_ context.Context, id string, agent protocol.AgentKind) error {
	f.calls = append(f.calls, "agent "+id+" "+string(agent))
	return f.fail
}
func (f *fakeControl) Finish(_ context.Context, id string) error {
	f.calls = append(f.calls, "finish "+id)
	return f.fail
}

func TestCodingCommandsUseTheResolvedCard(t *testing.T) {
	for _, command := range []string{"start", "resume", "stop", "message", "reply", "agent", "finish", "status"} {
		t.Run(command, func(t *testing.T) {
			control, bot := &fakeControl{}, &fakeBot{}
			svc, err := chatcmd.New(&fakeApprover{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			svc.WithControl(control)
			if err := svc.Run(t.Context(), bot); err != nil {
				t.Fatal(err)
			}
			text := command + " web#3"
			if command == "message" {
				text += " Keep  spaces\nand lines."
			}
			if command == "agent" {
				text += " codex"
			}
			bot.start(t.Context(), chatbot.Incoming{ChatID: "1", Text: text})
			if len(control.calls) == 0 || control.calls[0] != "card web#3" {
				t.Fatalf("wrong card resolution: %v", control.calls)
			}
			if command != "status" && (len(control.calls) != 2 || !strings.Contains(control.calls[1], "opaque-id")) {
				t.Fatalf("wrong control calls: %v", control.calls)
			}
			if len(bot.sent) == 0 {
				t.Fatal("command was not acknowledged")
			}
			for _, text := range bot.sent {
				if len([]rune(text)) > 900 {
					t.Fatalf("reply exceeds the message chunk limit: %d", len([]rune(text)))
				}
			}
		})
	}
}

func TestCodingCommandsIgnoreUnconfiguredChatsAndMalformedKeys(t *testing.T) {
	control, bot := &fakeControl{}, &fakeBot{}
	svc, err := chatcmd.New(&fakeApprover{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.WithControl(control)
	if err := svc.Run(t.Context(), bot); err != nil {
		t.Fatal(err)
	}
	bot.start(t.Context(), chatbot.Incoming{ChatID: "stranger", Text: "start web#3"})
	if len(control.calls) != 0 || len(bot.sent) != 0 {
		t.Fatal("a stranger reached a coding action")
	}
	bot.start(t.Context(), chatbot.Incoming{ChatID: "1", Text: "start 3"})
	if len(control.calls) != 0 || !strings.Contains(bot.sent[0], "project and card") {
		t.Fatal("an ambiguous card number reached the controller")
	}
	control.fail = fmt.Errorf("wrapped: %w", protocol.Refused("Another coding session owns this project folder."))
	bot.start(t.Context(), chatbot.Incoming{ChatID: "1", Text: "start web#3"})
	if len(control.calls) != 1 || bot.sent[len(bot.sent)-1] != "Another coding session owns this project folder." {
		t.Fatalf("lookup refusal was lost: %v, %v", control.calls, bot.sent)
	}
}
