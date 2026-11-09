package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/chatcmd"
	"github.com/khanblair/marshal/daemon/internal/integrations"
)

type runningChat struct {
	scope  string
	bot    chatbot.Bot
	cancel context.CancelFunc
	failed chan struct{}
}

type chatConnections struct {
	links    *integrations.Service
	commands *chatcmd.Service
	log      *slog.Logger
	running  map[chatbot.Kind]runningChat
}

// watchChats also revokes old receive loops and answer watchers after a connection changes.
func watchChats(ctx context.Context, log *slog.Logger, links *integrations.Service, commands *chatcmd.Service) {
	if links == nil {
		return
	}
	watch := chatConnections{links: links, commands: commands, log: log, running: make(map[chatbot.Kind]runningChat)}
	defer watch.close()
	ticker := time.NewTicker(chatRecheckInterval)
	defer ticker.Stop()
	for {
		for _, kind := range []chatbot.Kind{chatbot.KindTelegram, chatbot.KindDiscord} {
			watch.refresh(ctx, kind)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *chatConnections) close() {
	for _, item := range w.running {
		item.cancel()
		_ = item.bot.Close()
	}
}

func (w *chatConnections) refresh(ctx context.Context, kind chatbot.Kind) {
	bot, err := w.links.ChatBot(ctx, kind)
	scope := ""
	if capable, ok := bot.(chatbot.ConversationBot); ok {
		scope = capable.Scope()
	}
	if old, ok := w.running[kind]; ok {
		failed := false
		select {
		case <-old.failed:
			failed = true
		default:
		}
		if err == nil && old.scope == scope && !failed {
			_ = bot.Close()
			return
		}
		old.cancel()
		_ = old.bot.Close()
		delete(w.running, kind)
	}
	if err != nil {
		return
	}
	botCtx, cancel := context.WithCancel(ctx)
	failed := make(chan struct{})
	w.running[kind] = runningChat{scope: scope, bot: bot, cancel: cancel, failed: failed}
	w.log.Info("chat commands are on", "service", kind)
	go func() {
		if err := w.commands.Run(botCtx, bot); err != nil {
			cancel()
			close(failed)
			if ctx.Err() == nil {
				w.log.Warn("a chat bot stopped receiving", "service", bot.Kind(), "error", err)
			}
		}
	}()
}
