package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/khanblair/marshal/daemon/internal/config"
	"github.com/khanblair/marshal/daemon/internal/events"
	"github.com/khanblair/marshal/daemon/internal/platform"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/testutil"
)

// TestMain removes the stub agent the tests below build, once they are done.
func TestMain(m *testing.M) {
	code := m.Run()
	testutil.CleanStubAgent()
	os.Exit(code)
}

// buildTestModules opens a store and a bus in a temporary data folder and builds the daemon's modules
// over them, with the stub agent at the path given. Everything is closed in the order the daemon
// closes it in when the test ends: the session manager first, then the bus, and the store last.
func buildTestModules(t *testing.T, stub string) (daemonModules, *store.Store, *events.Bus, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "marshal.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	bus, err := events.New()
	if err != nil {
		t.Fatalf("make the bus: %v", err)
	}
	t.Cleanup(bus.Close)
	env := platform.Env{GOOS: runtime.GOOS, Getenv: func(key string) string {
		if key == stubAgentEnv {
			return stub
		}
		return ""
	}}
	settings := config.Settings{Mode: platform.ModeDev, DataDir: dir, Agent: config.AgentStub}
	mods, err := buildModules(st, bus, settings, env, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("build the daemon's modules: %v", err)
	}
	t.Cleanup(func() { _ = mods.sessions.Close() })
	return mods, st, bus, dir
}

// The daemon builds each module once and hands them to the API server. This checks that they build
// together over a real store and bus, and that the card diff service the Diff tab reads is among
// them: nothing else exercises the composition, and a module that cannot be built would otherwise
// only show up when a person runs the daemon.
func TestBuildModulesBuildsTheDiffService(t *testing.T) {
	ctx := context.Background()
	// The stub agent's path is only recorded here; nothing starts it in this test.
	stub := filepath.Join(t.TempDir(), "stub-agent")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	mods, _, _, _ := buildTestModules(t, stub)
	if mods.diff == nil || mods.proj == nil || mods.catalog == nil || mods.dashboard == nil || mods.history == nil {
		t.Fatalf("a module is missing: %+v", mods)
	}
	// The diff service reads a card through the projects module, so an unknown card gets the answer
	// the route would send.
	if _, err := mods.diff.Files(ctx, "01M3C107JB041061050R3GG28A"); err == nil {
		t.Error("the diff service answered for a card that is not in the store")
	}
}

// untilChatState reads the chat's topic until its session reports a state.
func untilChatState(t *testing.T, sub *events.Subscription, chatID string, state protocol.SessionState) {
	t.Helper()
	timeout := time.After(15 * time.Second)
	for {
		select {
		case ev, ok := <-sub.C():
			if !ok {
				t.Fatalf("the subscription closed while waiting for %s", state)
			}
			if data, isState := ev.Data.(protocol.SessionStateChangedEventData); isState && data.ChatID == chatID && data.State == state {
				return
			}
		case <-timeout:
			t.Fatalf("chat %s never reported %s", chatID, state)
		}
	}
}

// The chats module is given the session manager, so archiving a chat puts its running agent to
// sleep, deleting it stops the agent and removes its logs, and the daemon stopping ends the process
// of every chat that is still talking. This is the composition that internal/chats and
// internal/session are each tested against a fake of, run for real with the stub agent.
func TestTheChatsModuleStopsARunningChatSession(t *testing.T) {
	ctx := context.Background()
	mods, st, bus, dir := buildTestModules(t, testutil.StubAgent(t))
	project, err := mods.proj.Create(ctx, protocol.CreateProjectRequest{
		Source: protocol.ProjectSourceFolder, Path: testutil.Fixture(t, "small-repo"),
	})
	if err != nil {
		t.Fatalf("create a project: %v", err)
	}
	talk := func(chatID, text string, sub *events.Subscription) {
		t.Helper()
		if err := mods.chats.Send(ctx, chatID, text); err != nil {
			t.Fatalf("send %q to chat %s: %v", text, chatID, err)
		}
		untilChatState(t, sub, chatID, protocol.SessionStateWorking)
		untilChatState(t, sub, chatID, protocol.SessionStateAwake)
	}
	newChat := func() (protocol.Chat, *events.Subscription) {
		t.Helper()
		chat, err := mods.chats.Create(ctx, project.ID, protocol.CreateChatRequest{})
		if err != nil {
			t.Fatalf("create a chat: %v", err)
		}
		return chat, bus.Subscribe(events.Topics(string(protocol.ChatTopic(chat.ID))))
	}

	// Archive puts a running chat to sleep, and the next message after a restore wakes it.
	chat, sub := newChat()
	talk(chat.ID, "first question", sub)
	if mods.sessions.RecentOutput(chat.ID) == nil {
		t.Fatal("the chat's agent is not running after its first message")
	}
	if _, err := mods.chats.Archive(ctx, chat.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	untilChatState(t, sub, chat.ID, protocol.SessionStateAsleep)
	if mods.sessions.RecentOutput(chat.ID) != nil {
		t.Error("archiving the chat left its agent running")
	}
	if _, err := mods.chats.Restore(ctx, chat.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	talk(chat.ID, "second question", sub)
	row, err := st.Queries().GetSessionByChat(ctx, &chat.ID)
	if err != nil {
		t.Fatalf("read the chat's session: %v", err)
	}
	logs := filepath.Join(dir, "logs", "sessions", row.ID)
	if _, err := os.Stat(logs); err != nil {
		t.Fatalf("the running chat has no log folder: %v", err)
	}

	// Delete stops it, and removes the folder its session logged in.
	if err := mods.chats.Remove(ctx, chat.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if mods.sessions.RecentOutput(chat.ID) != nil {
		t.Error("deleting the chat left its agent running")
	}
	if _, err := os.Stat(logs); !os.IsNotExist(err) {
		t.Errorf("deleting the chat left its log folder behind: %v", err)
	}

	// The daemon stopping ends the process of a chat that is still talking, and leaves its row
	// asleep, because nothing starts a chat on its own.
	last, lastSub := newChat()
	talk(last.ID, "still here", lastSub)
	if err := mods.sessions.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if mods.sessions.RecentOutput(last.ID) != nil {
		t.Error("the daemon stopped and the chat's agent is still running")
	}
	row, err = st.Queries().GetSessionByChat(ctx, &last.ID)
	if err != nil || row.State != string(protocol.SessionStateAsleep) {
		t.Errorf("the chat's session after the daemon stopped = %+v, %v, want asleep", row, err)
	}
}
