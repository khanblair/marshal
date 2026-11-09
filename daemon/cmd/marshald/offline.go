package main

// What the daemon does with a message when the internet is down: save it in the outbox, and send it
// when the internet is back (docs/architecture.md section 19.5).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/chatcmd"
	"github.com/khanblair/marshal/daemon/internal/integrations"
	"github.com/khanblair/marshal/daemon/internal/netwatch"
	"github.com/khanblair/marshal/daemon/internal/notify"
	"github.com/khanblair/marshal/daemon/internal/outbox"
)

// offline is what a sender needs when the internet may be down: the monitor that confirms it and the
// outbox that holds what could not go. Either may be nil, as in a test.
type offline struct {
	network *netwatch.Monitor
	box     *outbox.Service
}

// saveIfOffline saves a message to the outbox when err is the network failing and the internet really
// is down. A network-looking error is not enough: a bad address, a certificate nobody trusts, or one
// server that is down all look like it, and saving those would only make the outbox retry them for a
// day. So the monitor is asked to confirm. It answers saved=true when it saved, with the error of
// saving, if any, joined to the network error. Anything else, including no error, is not its to handle:
// it answers false and the caller reports err as it is. A nil outbox never saves, and with no monitor a
// network-looking error is taken at its word.
func saveIfOffline(ctx context.Context, off offline, kind, target string, payload any, err error) (bool, error) {
	if err == nil || off.box == nil || !netwatch.IsNetworkError(err) {
		return false, nil
	}
	if off.network != nil && off.network.Check(ctx) {
		return false, nil
	}
	if saveErr := off.box.Enqueue(ctx, kind, target, payload); saveErr != nil {
		return true, errors.Join(err, saveErr)
	}
	return true, nil
}

// sendNotice sends one notice through the channel's connection. A channel nobody connected is not an
// error: the notice goes nowhere, which is how a daemon with no chat set up behaves.
func sendNotice(ctx context.Context, links *integrations.Service, channel notify.Channel, notice chatbot.Notice, commands ...*chatcmd.Service) error {
	if links == nil {
		return nil
	}
	bot, err := links.ChatBot(ctx, chatbot.Kind(channel))
	if err != nil {
		if errors.Is(err, integrations.ErrNotConnected) {
			return nil
		}
		return err
	}
	defer func() { _ = bot.Close() }()
	if len(commands) > 0 && commands[0] != nil {
		if handled, err := commands[0].Deliver(ctx, bot, notice); handled {
			return err
		}
	}
	return bot.Notify(ctx, notice)
}

// registerOutbox says how each saved message is sent. A notice goes through the channel's chat again,
// and a brief through the same delivery a schedule uses. A message that waited is marked as late, so
// a person who reads it an hour after it happened is not misled about when.
func registerOutbox(box *outbox.Service, links *integrations.Service, commands ...*chatcmd.Service) {
	box.Register(outbox.KindNotice, func(ctx context.Context, item outbox.Item) error {
		var notice chatbot.Notice
		if err := json.Unmarshal(item.Payload, &notice); err != nil {
			return fmt.Errorf("read a saved notice: %w", err)
		}
		if _, late := item.Late(time.Now()); late {
			notice.Title = lateTitle(notice.Title)
		}
		return sendNotice(ctx, links, notify.Channel(item.Target), notice, commands...)
	})
	box.Register(outbox.KindBrief, func(ctx context.Context, item outbox.Item) error {
		var brief briefPayload
		if err := json.Unmarshal(item.Payload, &brief); err != nil {
			return fmt.Errorf("read a saved brief: %w", err)
		}
		if _, late := item.Late(time.Now()); late {
			brief.Title = lateTitle(brief.Title)
		}
		return chatDeliverer{links: links}.deliverNow(ctx, item.Target, brief.Title, brief.Body)
	})
}

// lateTitle marks a message that was held back because the internet was down. It goes on the title, not
// the body, because a body can already be as long as its channel allows and a title never is.
func lateTitle(title string) string {
	return title + " (sent late)"
}
