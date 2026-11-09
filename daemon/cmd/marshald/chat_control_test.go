package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/chatcmd"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

type commandBot struct {
	kind    chatbot.Kind
	handler chatbot.Handler
	mu      sync.Mutex
	replies []string
}

func (b *commandBot) Kind() chatbot.Kind               { return b.kind }
func (b *commandBot) Accepts(in chatbot.Incoming) bool { return in.ChatID == "owner" }
func (b *commandBot) Close() error                     { return nil }
func (b *commandBot) Test(context.Context) (protocol.TestResult, error) {
	return protocol.TestResult{}, nil
}
func (b *commandBot) Start(_ context.Context, handler chatbot.Handler) error {
	b.handler = handler
	return nil
}
func (b *commandBot) Notify(_ context.Context, notice chatbot.Notice) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.replies = append(b.replies, notice.Title)
	return nil
}
func (b *commandBot) command(ctx context.Context, text string) string {
	b.handler(ctx, chatbot.Incoming{ChatID: "owner", Text: text})
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.replies[len(b.replies)-1]
}

func TestChatControlsRunARealStubCard(t *testing.T) {
	for _, kind := range []chatbot.Kind{chatbot.KindTelegram, chatbot.KindDiscord} {
		t.Run(string(kind), func(t *testing.T) {
			mods, _, _, _ := buildTestModules(t, testutil.StubAgent(t))
			ctx := t.Context()
			project, err := mods.proj.Create(ctx, protocol.CreateProjectRequest{
				Source: protocol.ProjectSourceFolder, Path: t.TempDir(), Name: "web",
			})
			if err != nil {
				t.Fatal(err)
			}
			no := false
			if _, err := mods.integrator.UpdateSettings(ctx, project.ID, protocol.IntegrationSettingsPatch{
				Review: &no, Checks: &no, AutoShip: &no,
			}); err != nil {
				t.Fatal(err)
			}
			control := chatControl{projects: mods.proj, sessions: mods.sessions, history: mods.history, finisher: mods.integrator}
			svc, err := chatcmd.New(mods.sessions, nil)
			if err != nil {
				t.Fatal(err)
			}
			svc.WithCards(chatCards{svc: mods.proj}).WithControl(control)
			bot := &commandBot{kind: kind}
			if err := svc.Run(ctx, bot); err != nil {
				t.Fatal(err)
			}
			if answer := bot.command(ctx, "new web Fix login"); !strings.Contains(answer, "start web#1") {
				t.Fatalf("creation did not return the card key: %s", answer)
			}
			bot.command(ctx, "agent web#1 codex")
			bot.command(ctx, "start web#1")
			card, err := mods.proj.CardByKey(ctx, protocol.CardKey{ProjectID: project.ID, Number: 1})
			if err != nil {
				t.Fatal(err)
			}
			waitCardIdle(t, mods, card.ID)
			if answer := bot.command(ctx, "reply web#1"); !strings.Contains(answer, "plan.md") {
				t.Fatalf("stored agent answer did not reach the bot: %s", answer)
			}
			bot.command(ctx, "new web Second task")
			if answer := bot.command(ctx, "start web#2"); !strings.Contains(answer, "Another coding session owns") {
				t.Fatalf("chat bypassed the project editing guard: %s", answer)
			}
			if answer := bot.command(ctx, "status"); !strings.Contains(answer, "web#1") {
				t.Fatalf("status lost the card: %s", answer)
			}
			bot.command(ctx, "message web#1 Review the plan once more.")
			waitCardIdle(t, mods, card.ID)
			bot.command(ctx, "stop web#1")
			stopped, err := mods.proj.Card(ctx, card.ID)
			if err != nil || stopped.Session == nil || *stopped.Session != protocol.SessionStateStopped {
				t.Fatalf("stop failed: %+v, %v", stopped.Session, err)
			}
			bot.command(ctx, "resume web#1")
			waitCardIdle(t, mods, card.ID)
			if answer := bot.command(ctx, "status web#1"); !strings.Contains(answer, "agent codex") {
				t.Fatalf("resume lost agent settings: %s", answer)
			}
			if answer := bot.command(ctx, "finish web#1"); !strings.Contains(answer, "completion requested") {
				t.Fatalf("completion request failed: %s", answer)
			}
			deadline := time.Now().Add(15 * time.Second)
			for time.Now().Before(deadline) {
				current, err := mods.proj.Card(ctx, card.ID)
				if err == nil && current.State == protocol.CardStateDone {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			current, err := mods.proj.Card(ctx, card.ID)
			if err != nil || current.State != protocol.CardStateDone {
				t.Fatalf("completion did not reach Done: %+v, %v", current, err)
			}
			if _, err := os.Stat(filepath.Join(project.Path, "notes", "plan.md")); err != nil {
				t.Fatalf("agent changes were lost: %v", err)
			}
		})
	}
}

func waitCardIdle(t *testing.T, mods daemonModules, id string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		card, err := mods.proj.Card(t.Context(), id)
		if err == nil && card.Session != nil && *card.Session == protocol.SessionStateAwake {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("card did not finish its turn")
}
