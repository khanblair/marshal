package main

import (
	"context"
	"errors"

	"github.com/khanblair/marshal/daemon/internal/briefs"
	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/integrations"
)

// chatDeliverer sends a brief through whichever chat connection a person saved. A chat nobody set up
// answers briefs.ErrNotConnected, which a brief reports as skipped and not as a failure.
type chatDeliverer struct{ links *integrations.Service }

// Deliver sends one message to the channel's chat.
func (d chatDeliverer) Deliver(ctx context.Context, channel, title, body string) error {
	if d.links == nil {
		return briefs.ErrNotConnected
	}
	bot, err := d.links.ChatBot(ctx, chatbot.Kind(channel))
	if err != nil {
		if errors.Is(err, integrations.ErrNotConnected) {
			return briefs.ErrNotConnected
		}
		return err
	}
	defer func() { _ = bot.Close() }()
	return bot.Notify(ctx, chatbot.Notice{Title: title, Body: body})
}
