package integrations

// This file owns the two chat connections (B9.3, build-plan 9.5 and 9.6). They are the Trello and
// GitHub twins in every way that matters: where their settings and their secret live, what saving
// one does, and how the bot is read back. The talk-to-the-service half itself lives in
// internal/chatbot, and the bot is built here from what is stored.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// TelegramID is the Telegram connection's own id, and the id its `integrations` row, its keychain
// entry, and its saved test result are filed under.
const TelegramID = "telegram"

// DiscordID is the Discord connection's own id.
const DiscordID = "discord"

// NtfyID is the ntfy connection's own id.
const NtfyID = "ntfy"

// KindTelegram and KindDiscord are the kinds the two chat connections' rows and tests are filed
// under. They are the same words as the bot's own kinds, so a row, a bot, and a screen never have to
// translate between two names for the same service.
const (
	KindTelegram = string(chatbot.KindTelegram)
	KindDiscord  = string(chatbot.KindDiscord)
	KindNtfy     = string(chatbot.KindNtfy)
)

// chatConfig is the non-secret half of a chat connection, stored in the `integrations` row's
// config_json. Only where notices go is in it: the chat id is not a secret, and a screen shows it so
// a person can check it against the chat they made.
type chatConfig struct {
	// ChatID is the Telegram chat, the Discord channel, or the ntfy topic notices are sent to.
	ChatID string `json:"chatId"`
	// Server is the ntfy server's address; empty is ntfy's own. Only ntfy has one.
	Server string `json:"server,omitempty"`
}

// chatSecrets is the secret half, stored in the OS keychain as one JSON document under the
// connection's own id. The token is what stands for the bot to the service, and it never comes back
// out.
type chatSecrets struct {
	// Token is the bot's own token.
	Token string `json:"token"`
}

// SaveTelegram stores the Telegram connection, replacing whatever was there (B9.3). The owner
// creates the real bot and token (docs/backend-checklist.md section 3).
func (s *Service) SaveTelegram(ctx context.Context, req protocol.SaveTelegramRequest) error {
	return s.saveChat(ctx, TelegramID, KindTelegram, req.Token, req.ChatID)
}

// SaveDiscord stores the Discord connection, replacing whatever was there (B9.3).
func (s *Service) SaveDiscord(ctx context.Context, req protocol.SaveDiscordRequest) error {
	return s.saveChat(ctx, DiscordID, KindDiscord, req.Token, req.ChannelID)
}

// SaveNtfy stores the ntfy connection, replacing whatever was there. Unlike a chat bot it needs no
// token: an open topic on a public server has none, so only the topic is required.
func (s *Service) SaveNtfy(ctx context.Context, req protocol.SaveNtfyRequest) error {
	if strings.TrimSpace(req.Topic) == "" {
		return protocol.InvalidArgument("Choose the ntfy topic Marshal should send notices to.")
	}
	secrets, err := json.Marshal(chatSecrets{Token: strings.TrimSpace(req.Token)})
	if err != nil {
		return fmt.Errorf("write the ntfy connection's secrets: %w", err)
	}
	if err := s.keys.Set(NtfyID, string(secrets)); err != nil {
		return fmt.Errorf("save the ntfy connection's secret in the keychain: %w", err)
	}
	config, err := json.Marshal(chatConfig{ChatID: strings.TrimSpace(req.Topic), Server: strings.TrimSpace(req.Server)})
	if err != nil {
		return fmt.Errorf("write the ntfy connection's settings: %w", err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{
			ID: NtfyID, Kind: KindNtfy, ConfigJSON: string(config), KeychainRef: NtfyID,
		})
	})
	if err != nil {
		return fmt.Errorf("save the ntfy connection's settings: %w", err)
	}
	return nil
}

// saveChat is the write half both chat connections share. A token and a place to send are both
// required, because a bot with no token cannot talk to the service and a bot with no chat has
// nowhere to deliver a notice; either one alone is not a connection.
//
// The secret goes into the keychain before the settings go into the row, for the same reason the
// GitHub App's does: a secret with no settings is inert, while settings with no secret would be a
// connection whose every notice fails.
func (s *Service) saveChat(ctx context.Context, id, kind, token, chatID string) error {
	if strings.TrimSpace(token) == "" {
		return protocol.InvalidArgument(fmt.Sprintf("Marshal needs the %s bot's token to reach it.", kind))
	}
	if strings.TrimSpace(chatID) == "" {
		return protocol.InvalidArgument(fmt.Sprintf("Choose the %s chat or channel Marshal should send notices to.", kind))
	}
	secrets, err := json.Marshal(chatSecrets{Token: token})
	if err != nil {
		return fmt.Errorf("write the %s connection's secrets: %w", kind, err)
	}
	if err := s.keys.Set(id, string(secrets)); err != nil {
		return fmt.Errorf("save the %s connection's secret in the keychain: %w", kind, err)
	}
	config, err := json.Marshal(chatConfig{ChatID: chatID})
	if err != nil {
		return fmt.Errorf("write the %s connection's settings: %w", kind, err)
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.UpsertIntegration(ctx, db.UpsertIntegrationParams{
			ID:          id,
			Kind:        kind,
			ConfigJSON:  string(config),
			KeychainRef: id,
		})
	})
	if err != nil {
		return fmt.Errorf("save the %s connection's settings: %w", kind, err)
	}
	return nil
}

// ChatBot answers the bot for one chat connection, built from what is stored. It is what the
// notification router sends through (B9.4). It answers ErrNotConnected when nothing is stored, which
// is the ordinary state of a daemon nobody has connected that service to.
//
// Nothing is cached: a bot is a thin wrapper around the token and the chat, so building one per
// notice costs nothing and a saved token takes effect at once, without a restart.
func (s *Service) ChatBot(ctx context.Context, kind chatbot.Kind) (chatbot.Bot, error) {
	id, ok := chatConnectionID(kind)
	if !ok {
		return nil, protocol.NotFound("connection").With("id", string(kind))
	}
	return s.botFor(ctx, id, kind)
}

// botFor builds the bot for one chat connection's id.
func (s *Service) botFor(ctx context.Context, id string, kind chatbot.Kind) (chatbot.Bot, error) {
	config, secrets, err := s.readChat(ctx, id)
	if err != nil {
		return nil, err
	}
	// An ntfy topic can be open, so it is connected with a topic alone; a bot always needs a token.
	if config.ChatID == "" || (secrets.Token == "" && kind != chatbot.KindNtfy) {
		return nil, ErrNotConnected
	}
	switch kind {
	case chatbot.KindTelegram:
		bot, err := chatbot.NewTelegram(chatbot.TelegramConfig{
			Token: secrets.Token, ChatID: config.ChatID, BaseURL: s.telegramBase, Now: s.now,
		})
		if err != nil {
			return nil, fmt.Errorf("build the Telegram bot: %w", err)
		}
		return bot, nil
	case chatbot.KindDiscord:
		bot, err := chatbot.NewDiscord(chatbot.DiscordConfig{
			Token: secrets.Token, ChannelID: config.ChatID, HTTPClient: s.discordClient, Now: s.now,
		})
		if err != nil {
			return nil, fmt.Errorf("build the Discord bot: %w", err)
		}
		return bot, nil
	case chatbot.KindNtfy:
		bot, err := chatbot.NewNtfy(chatbot.NtfyConfig{
			Server: config.Server, Topic: config.ChatID, Token: secrets.Token,
			HTTPClient: s.ntfyClient, Now: s.now,
		})
		if err != nil {
			return nil, fmt.Errorf("build the ntfy publisher: %w", err)
		}
		return bot, nil
	default:
		return nil, protocol.NotFound("connection").With("id", string(kind))
	}
}

// chatConnectionID maps a bot's kind to the connection id it is stored under.
func chatConnectionID(kind chatbot.Kind) (string, bool) {
	switch kind {
	case chatbot.KindTelegram:
		return TelegramID, true
	case chatbot.KindDiscord:
		return DiscordID, true
	case chatbot.KindNtfy:
		return NtfyID, true
	default:
		return "", false
	}
}

// readChat reads the stored config and secrets. A missing row or a missing keychain entry is not an
// error: both mean "nothing is stored", which is what a connection nobody has set up looks like.
func (s *Service) readChat(ctx context.Context, id string) (chatConfig, chatSecrets, error) {
	var config chatConfig
	var found bool
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := q.GetIntegration(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("read the %s connection's settings: %w", id, err)
		}
		found = true
		if row.ConfigJSON == "" {
			return nil
		}
		return json.Unmarshal([]byte(row.ConfigJSON), &config)
	})
	if err != nil {
		return chatConfig{}, chatSecrets{}, err
	}
	if !found {
		return chatConfig{}, chatSecrets{}, nil
	}
	raw, err := s.keys.Get(id)
	if err != nil && !errors.Is(err, security.ErrNoKey) {
		return chatConfig{}, chatSecrets{}, fmt.Errorf(
			"read the %s connection's secret from the keychain: %w", id, err)
	}
	var secrets chatSecrets
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &secrets); err != nil {
			return chatConfig{}, chatSecrets{}, fmt.Errorf("read the %s connection's secrets: %w", id, err)
		}
	}
	return config, secrets, nil
}

// testChat is the real connection test for either chat service: the stored bot, and what it can
// reach. It builds the bot and asks the service itself, which is the same code a person's "Test
// connection" presses.
func (s *Service) testChat(ctx context.Context, info Info) (protocol.TestResult, error) {
	if err := ctx.Err(); err != nil {
		return protocol.TestResult{}, err
	}
	kind := chatbot.Kind(info.Kind)
	bot, err := s.botFor(ctx, info.ID, kind)
	if errors.Is(err, ErrNotConnected) {
		return protocol.NewTestResult(info.ID, []protocol.TestCheck{{
			Name:    CheckSummary,
			State:   protocol.CheckStateFailed,
			Message: fmt.Sprintf("Marshal has no %s connection saved, so it cannot use %s.", info.Kind, info.Kind),
			Fix:     fmt.Sprintf("Add the %s bot's token and its chat in Settings, under Integrations.", info.Kind),
		}}, s.now()), nil
	}
	if err != nil {
		return protocol.TestResult{}, err
	}
	defer func() { _ = bot.Close() }()
	return bot.Test(ctx)
}

// DetectTelegramChat finds the chat that most recently wrote to a bot, so a person does not have to
// look up a numeric chat id. It needs only the token, before anything is saved, and it sends nothing.
func (s *Service) DetectTelegramChat(ctx context.Context, req protocol.DetectTelegramChatRequest) (protocol.DetectTelegramChatAnswer, error) {
	chat, err := chatbot.DetectTelegramChat(ctx, nil, s.telegramBase, req.Token)
	switch {
	case err == nil:
		return protocol.DetectTelegramChatAnswer{Found: true, ChatID: chat.ID, Name: chat.Name, Kind: chat.Kind}, nil
	case errors.Is(err, chatbot.ErrNoTelegramChat):
		return protocol.DetectTelegramChatAnswer{
			Message: "Nobody has written to the bot yet. Open your bot in Telegram, press Start, send it any message, then try again.",
		}, nil
	case errors.Is(err, chatbot.ErrTelegramTokenRefused):
		return protocol.DetectTelegramChatAnswer{}, protocol.InvalidArgument("Telegram did not accept that token. Copy it again from @BotFather.")
	case errors.Is(err, chatbot.ErrTelegramBusy):
		return protocol.DetectTelegramChatAnswer{}, protocol.Conflict("Something is already reading this bot's messages, so Marshal cannot look. Type the chat id yourself, or disconnect Telegram first.")
	}
	return protocol.DetectTelegramChatAnswer{}, protocol.Unavailable("Marshal could not reach Telegram. Check the connection and try again.")
}
