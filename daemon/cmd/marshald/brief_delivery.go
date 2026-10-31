package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/briefs"
	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/store"
)

// chatDeliverer sends a brief through whichever chat connection a person saved. A chat nobody set up
// answers briefs.ErrNotConnected, which a brief reports as skipped and not as a failure.
type chatDeliverer struct{ links *integrations.Service }

// Deliver sends one message to the channel's chat, or one email when the channel is email.
func (d chatDeliverer) Deliver(ctx context.Context, channel, title, body string) error {
	if d.links == nil {
		return briefs.ErrNotConnected
	}
	if channel == briefs.ChannelEmail {
		return d.email(ctx, title, body)
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

// email sends a brief by email through the Resend connection, to the address in the owner's profile.
// A missing connection is not connected, and a missing address is a skip with its reason.
func (d chatDeliverer) email(ctx context.Context, title, body string) error {
	err := d.links.SendEmail(ctx, title, body)
	switch {
	case errors.Is(err, integrations.ErrNotConnected):
		return briefs.ErrNotConnected
	case errors.Is(err, integrations.ErrNoRecipient):
		return fmt.Errorf("%w (add your email in Settings, under Profile)", briefs.ErrSkipped)
	default:
		return err
	}
}

// ownerEmail is the address in the owner's profile, where email from Marshal goes. Solo use has one
// person, the owner, the way the devices and Trello find them. It answers empty when the profile has no address.
func ownerEmail(ctx context.Context, st *store.Store) (string, error) {
	owner, err := st.Queries().GetOwner(ctx)
	if err != nil {
		if store.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("read the owner's email: %w", err)
	}
	return strings.TrimSpace(owner.Email), nil
}
