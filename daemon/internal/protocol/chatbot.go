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
