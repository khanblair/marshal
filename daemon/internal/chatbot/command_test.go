package chatbot_test

import (
	"strings"
	"testing"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
)

// Reading a chat message as an instruction (B9.3, build-plan 9.5 and 9.6). It is a pure function, so
// it is proved here rather than through a fake service.

func TestAMessageIsReadAsTheInstructionItMeans(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    chatbot.Command
	}{
		{"approve", "approve abc123", chatbot.Command{Verb: chatbot.VerbApprove, Target: "abc123"}},
		{"approve is case-insensitive", "Approve ABC123", chatbot.Command{Verb: chatbot.VerbApprove, Target: "ABC123"}},
		{"reject", "reject abc123", chatbot.Command{Verb: chatbot.VerbReject, Target: "abc123"}},
		{"a bot mention in front is skipped", "@marshal approve abc123", chatbot.Command{Verb: chatbot.VerbApprove, Target: "abc123"}},
		{"new", "new api Fix the flaky test", chatbot.Command{Verb: chatbot.VerbNew, Target: "api", Text: "Fix the flaky test"}},
		{"new allows a colon", "new api: Fix the flaky test", chatbot.Command{Verb: chatbot.VerbNew, Target: "api", Text: "Fix the flaky test"}},
		{"new keeps a title with spaces", "new api fix the flaky test", chatbot.Command{Verb: chatbot.VerbNew, Target: "api", Text: "fix the flaky test"}},
		{"status", "status", chatbot.Command{Verb: chatbot.VerbStatus}},
		{"card status", "status api#3", chatbot.Command{Verb: chatbot.VerbStatus, Target: "api#3"}},
		{"start", "start api#3", chatbot.Command{Verb: chatbot.VerbStart, Target: "api#3"}},
		{"telegram command", "/resume@marshal_bot api#3", chatbot.Command{Verb: chatbot.VerbResume, Target: "api#3"}},
		{"stop", "stop api#3", chatbot.Command{Verb: chatbot.VerbStop, Target: "api#3"}},
		{"reply", "reply api#3", chatbot.Command{Verb: chatbot.VerbReply, Target: "api#3"}},
		{"agent", "agent api#3 codex", chatbot.Command{Verb: chatbot.VerbAgent, Target: "api#3", Text: "codex"}},
		{"finish", "finish api#3", chatbot.Command{Verb: chatbot.VerbFinish, Target: "api#3"}},
		{"multiline instructions", "/message api#3 Keep  two spaces\nand this line.", chatbot.Command{Verb: chatbot.VerbMessage, Target: "api#3", Text: "Keep  two spaces\nand this line."}},
		{"missing start card", "start", chatbot.Command{Verb: chatbot.VerbUnknown}},
		{"missing instructions", "message api#3", chatbot.Command{Verb: chatbot.VerbUnknown}},
		{"extra target", "stop api#3 api#4", chatbot.Command{Verb: chatbot.VerbUnknown}},
		{"help", "help", chatbot.Command{Verb: chatbot.VerbHelp}},
		{"empty", "   ", chatbot.Command{Verb: chatbot.VerbUnknown}},
		{"only a mention", "@marshal", chatbot.Command{Verb: chatbot.VerbUnknown}},
		{"something else", "what is for lunch", chatbot.Command{Verb: chatbot.VerbUnknown}},
		{"approve with no id", "approve", chatbot.Command{Verb: chatbot.VerbUnknown}},
		{"new with no title", "new api", chatbot.Command{Verb: chatbot.VerbUnknown}},
		{"new with only a colon", "new api :", chatbot.Command{Verb: chatbot.VerbUnknown}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chatbot.Parse(tc.message); got != tc.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tc.message, got, tc.want)
			}
		})
	}
}

func TestHelpNamesEveryVerbTheParserKnows(t *testing.T) {
	help := chatbot.Help()
	for _, word := range []string{"approve", "reject", "new", "status", "start", "resume", "stop", "message", "reply", "agent", "finish", "help"} {
		if !strings.Contains(help, word) {
			t.Errorf("the help text does not mention %q:\n%s", word, help)
		}
	}
}
