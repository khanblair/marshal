package protocol

// The wire shape of the two chat connections (B9.3, build-plan 9.5 and 9.6). They are saved through
// the same route every other connection uses (PUT /v1/integrations/{id}), so each one needs only its
// own body; the connection's row, its status, and its last test ride back on the list every
// connection route already answers with (protocol.IntegrationList).

// SaveTelegramRequest is the body that saves a Telegram bot connection. The token is written to the
// keychain and never comes back from any route; the chat id is written to the connection's own row,
// because it is not a secret and a screen shows it so a person can check where notices go.
type SaveTelegramRequest struct {
	// Token is the bot's token, the one BotFather hands out.
	Token string `json:"token"`
	// ChatID is the chat Marshal sends notices to: a numeric id, or a channel name such as
	// "@my_channel".
	ChatID string `json:"chatId"`
}

// SaveDiscordRequest is the body that saves a Discord bot connection. The token is written to the
// keychain and never comes back from any route; the channel id is written to the connection's own
// row, for the same reason Telegram's chat id is.
type SaveDiscordRequest struct {
	// Token is the bot's own token, from the Discord developer portal.
	Token string `json:"token"`
	// ChannelID is the channel Marshal sends notices to.
	ChannelID string `json:"channelId"`
}

// SaveNtfyRequest is the body that saves an ntfy connection. The token is written to the keychain
// and never comes back from any route; the server and the topic are written to the connection's own
// row, because a screen shows them so a person can check where notices go.
type SaveNtfyRequest struct {
	// Server is the ntfy server's address. Empty means ntfy's own public server.
	Server string `json:"server"`
	// Topic is where notices are published.
	Topic string `json:"topic"`
	// Token is an access token for a server that needs one. Empty is fine for an open topic.
	Token string `json:"token"`
}

// DetectTelegramChatRequest is the body of POST /v1/integrations/telegram/detect-chat: the bot's
// token, before it is saved.
type DetectTelegramChatRequest struct {
	// Token is the bot's token from BotFather.
	Token string `json:"token"`
}

// DetectTelegramChatAnswer is what the detection found. Found is false, with a Message, when nobody
// has written to the bot yet.
type DetectTelegramChatAnswer struct {
	// Found says a chat was found.
	Found bool `json:"found"`
	// ChatID is the chat's numeric id, to save the connection with. Empty when not found.
	ChatID string `json:"chatId"`
	// Name is what a person calls the chat, such as a group's title or a person's name.
	Name string `json:"name"`
	// Kind is "private", "group", "supergroup", or "channel".
	Kind string `json:"kind"`
	// Message is one plain sentence for the person when nothing was found.
	Message string `json:"message"`
}
