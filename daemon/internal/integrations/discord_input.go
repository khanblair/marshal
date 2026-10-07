package integrations

import (
	"net/url"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// discordChannelID reads what a person pasted for the Discord channel: the channel's own id, or the
// link Discord copies for a channel, which is discord.com/channels/<server>/<channel>. A link is
// accepted because the first long number in it is the server's, and people copy the wrong one from
// a bare list of numbers, while the last is always the channel's. Anything else is refused in words,
// because a channel name or a server's link would only fail later, at the first notice.
func discordChannelID(input string) (string, error) {
	input = strings.TrimSpace(input)
	if strings.Contains(input, "/") {
		return channelFromLink(input)
	}
	if !allDigits(input) {
		return "", protocol.InvalidArgument(
			"A Discord channel id is a long number, like 1234567890123456789. " +
				"Open the channel, then copy its id (right-click it, or long-press it on a phone, and choose Copy Channel ID). " +
				"A channel's name, such as #general, will not work.")
	}
	return input, nil
}

// channelFromLink takes the channel's id out of a Discord channel link.
func channelFromLink(link string) (string, error) {
	parsed, err := url.Parse(link)
	if err != nil || !isDiscordHost(parsed.Host) {
		return "", protocol.InvalidArgument("That link is not a Discord channel link. Copy the channel's id instead.")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	// The path is channels/<server>/<channel>, with a message id after it for a link to a message.
	if len(parts) >= 2 && parts[1] == "@me" {
		return "", protocol.InvalidArgument("That link is a direct message. Use a channel in the server the bot is in.")
	}
	if len(parts) < 3 || parts[0] != "channels" || !allDigits(parts[2]) {
		return "", protocol.InvalidArgument(
			"That link names a server, not a channel. Open the channel itself, then copy its id or its link.")
	}
	return parts[2], nil
}

// isDiscordHost is true for the addresses Discord's own links use.
func isDiscordHost(host string) bool {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	for _, own := range []string{"discord.com", "ptb.discord.com", "canary.discord.com", "discordapp.com"} {
		if host == own {
			return true
		}
	}
	return false
}

// allDigits is true for a non-empty run of the digits 0 to 9.
func allDigits(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// discordToken reads what a person pasted for the bot token: the token itself, without the spaces
// around it or the word "Bot" some people copy in front of it, which the API adds on its own.
func discordToken(input string) string {
	token := strings.TrimSpace(input)
	if len(token) > len("bot ") && strings.EqualFold(token[:len("bot ")], "bot ") {
		token = strings.TrimSpace(token[len("bot "):])
	}
	return token
}
