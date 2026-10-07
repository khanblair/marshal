package integrations

import "testing"

func TestDiscordChannelIDReadsAnIDOrAChannelLink(t *testing.T) {
	cases := map[string]string{
		"1234567890123456789":     "1234567890123456789",
		"  1234567890123456789  ": "1234567890123456789",
		"https://discord.com/channels/111111111111111111/222222222222222222":     "222222222222222222",
		"https://discord.com/channels/111111111111111111/222222222222222222/333": "222222222222222222",
		"https://ptb.discord.com/channels/1/2/3":                                 "2",
		"https://discordapp.com/channels/1/2":                                    "2",
	}
	for input, want := range cases {
		got, err := discordChannelID(input)
		if err != nil || got != want {
			t.Errorf("discordChannelID(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestDiscordChannelIDRefusesWhatCannotBeAChannel(t *testing.T) {
	for _, input := range []string{
		"#general", "general", "12ab", "-100123456",
		"https://discord.com/channels/111111111111111111",
		"https://discord.com/channels/111111111111111111/",
		"https://discord.com/channels/@me/222",
		"https://example.com/channels/1/2",
	} {
		if got, err := discordChannelID(input); err == nil {
			t.Errorf("discordChannelID(%q) = %q, want a refusal", input, got)
		}
	}
}

func TestDiscordTokenDropsTheSpacesAndTheWordBot(t *testing.T) {
	cases := map[string]string{
		"abc.def.ghi": "abc.def.ghi", "  abc.def.ghi \n": "abc.def.ghi",
		"Bot abc.def.ghi": "abc.def.ghi", "bot   abc.def.ghi": "abc.def.ghi", "": "", "Bot ": "Bot",
	}
	for input, want := range cases {
		if got := discordToken(input); got != want {
			t.Errorf("discordToken(%q) = %q, want %q", input, got, want)
		}
	}
}
