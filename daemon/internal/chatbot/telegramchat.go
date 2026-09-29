package chatbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	telegramAPI        = "https://api.telegram.org"
	detectTimeout      = 10 * time.Second
	detectUpdatesLimit = 20
	detectMaxBodyBytes = 1 << 20
)

// ErrTelegramTokenRefused means Telegram did not accept the token.
var ErrTelegramTokenRefused = errors.New("telegram refused the token")

// ErrTelegramBusy means the bot's updates are already being read by something else, such as a
// running Marshal connection or a webhook, so they cannot be read a second time.
var ErrTelegramBusy = errors.New("the bot's updates are being read elsewhere")

// ErrNoTelegramChat means nobody has written to the bot yet.
var ErrNoTelegramChat = errors.New("no chat has written to the bot yet")

// TelegramChat is a chat that has written to a bot.
type TelegramChat struct {
	// ID is the chat's numeric id, which is what a connection is saved with.
	ID string
	// Name is what a person would call it: a group's title, or a person's name.
	Name string
	// Kind is "private", "group", "supergroup", or "channel".
	Kind string
}

// DetectTelegramChat finds the chat that most recently wrote to the bot, from the bot's waiting
// updates, so a person does not have to look up a numeric chat id. It reads updates without
// confirming them and sends nothing. baseURL is empty for Telegram itself, and client is nil for a
// default one.
func DetectTelegramChat(ctx context.Context, client *http.Client, baseURL, token string) (TelegramChat, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return TelegramChat{}, ErrTelegramTokenRefused
	}
	if baseURL == "" {
		baseURL = telegramAPI
	}
	if client == nil {
		client = &http.Client{Timeout: detectTimeout}
	}
	ctx, cancel := context.WithTimeout(ctx, detectTimeout)
	defer cancel()
	query := url.Values{"limit": {strconv.Itoa(detectUpdatesLimit)}, "timeout": {"0"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(baseURL, "/")+"/bot"+token+"/getUpdates?"+query.Encode(), nil)
	if err != nil {
		return TelegramChat{}, fmt.Errorf("build the request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return TelegramChat{}, fmt.Errorf("ask Telegram for the bot's updates: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, detectMaxBodyBytes))
	if err != nil {
		return TelegramChat{}, fmt.Errorf("read Telegram's answer: %w", err)
	}
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusNotFound:
		return TelegramChat{}, ErrTelegramTokenRefused
	case http.StatusConflict:
		return TelegramChat{}, ErrTelegramBusy
	}
	return newestChat(body)
}

// newestChat reads a getUpdates answer and picks the chat of the newest update that has one.
func newestChat(body []byte) (TelegramChat, error) {
	type chat struct {
		ID        int64  `json:"id"`
		Type      string `json:"type"`
		Title     string `json:"title"`
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	type message struct {
		Chat *chat `json:"chat"`
	}
	var answer struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      []struct {
			UpdateID      int64    `json:"update_id"`
			Message       *message `json:"message"`
			EditedMessage *message `json:"edited_message"`
			ChannelPost   *message `json:"channel_post"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return TelegramChat{}, fmt.Errorf("read Telegram's answer: %w", err)
	}
	if !answer.OK {
		return TelegramChat{}, fmt.Errorf("telegram said: %s", answer.Description)
	}
	var newest *chat
	var newestID int64 = -1
	for _, update := range answer.Result {
		for _, m := range []*message{update.Message, update.EditedMessage, update.ChannelPost} {
			if m != nil && m.Chat != nil && update.UpdateID > newestID {
				newest, newestID = m.Chat, update.UpdateID
			}
		}
	}
	if newest == nil {
		return TelegramChat{}, ErrNoTelegramChat
	}
	name := strings.TrimSpace(newest.Title)
	if name == "" {
		name = strings.TrimSpace(newest.FirstName + " " + newest.LastName)
	}
	if name == "" && newest.Username != "" {
		name = "@" + newest.Username
	}
	return TelegramChat{ID: strconv.FormatInt(newest.ID, 10), Name: name, Kind: newest.Type}, nil
}
