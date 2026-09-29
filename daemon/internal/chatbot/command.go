package chatbot

import "strings"

// This file is the reading half of a chat connection: turning what a person types into the action
// it means. It is deliberately a pure function over a string, with no bot, no clock, and no store,
// so every rule about what a message means is proved by a plain unit test rather than by a fake
// server. The service is a phone in a pocket, so the grammar is small and forgiving.

// Verb is what a chat message asks Marshal to do.
type Verb string

const (
	// VerbApprove approves one thing waiting on the person, by the id the notice carried.
	VerbApprove Verb = "approve"
	// VerbReject rejects one thing waiting on the person, by the id the notice carried.
	VerbReject Verb = "reject"
	// VerbNew creates a card. Its Text is the title and its Target names the project.
	VerbNew Verb = "new"
	// VerbStatus asks what is happening, and is answered with a short summary.
	VerbStatus Verb = "status"
	// VerbHelp asks what Marshal understands.
	VerbHelp Verb = "help"
	// VerbUnknown is anything else. It is answered with the help text rather than silently dropped,
	// because a message that was ignored looks exactly like a bot that is broken.
	VerbUnknown Verb = "unknown"
)

// Command is one message read as an instruction.
type Command struct {
	// Verb is what was asked for.
	Verb Verb
	// Target is the id a decision is about, or the project a new card goes in. Empty when the verb
	// needs neither.
	Target string
	// Text is the rest of the message: a new card's title. Empty otherwise.
	Text string
}

// Parse reads one message as an instruction. It is case-insensitive and splits on the first run of
// whitespace, so "Approve abc123" and "approve abc123" are the same, and a bot mention in front of
// the word ("@marshal approve abc123") is skipped rather than turning the message into an unknown.
func Parse(message string) Command {
	words := strings.Fields(strings.TrimSpace(message))
	if len(words) == 0 {
		return Command{Verb: VerbUnknown}
	}
	// A leading @mention is the service naming the bot, not a person's instruction.
	if strings.HasPrefix(words[0], "@") {
		words = words[1:]
	}
	if len(words) == 0 {
		return Command{Verb: VerbUnknown}
	}
	verb := Verb(strings.ToLower(words[0]))
	rest := words[1:]
	switch verb {
	case VerbApprove, VerbReject:
		if len(rest) == 0 {
			return Command{Verb: VerbUnknown}
		}
		return Command{Verb: verb, Target: rest[0]}
	case VerbNew:
		return parseNew(rest)
	case VerbStatus, VerbHelp:
		return Command{Verb: verb}
	default:
		return Command{Verb: VerbUnknown}
	}
}

// parseNew reads the rest of a "new" message into a project and a title. The project is the first
// word, and the title is everything after it, so a title with spaces in it works without quoting.
// A message with no project, or no title after it, is unknown rather than half a card.
func parseNew(rest []string) Command {
	if len(rest) < 2 {
		return Command{Verb: VerbUnknown}
	}
	project := strings.TrimSuffix(rest[0], ":")
	title := strings.TrimSpace(strings.TrimPrefix(strings.Join(rest[1:], " "), ":"))
	if title == "" {
		return Command{Verb: VerbUnknown}
	}
	return Command{Verb: VerbNew, Target: project, Text: title}
}

// Help says what Marshal understands from a chat, in one message.
func Help() string {
	return "Send Marshal one of these:\n" +
		"approve <id> - approve what a notice is asking about\n" +
		"reject <id> - reject it\n" +
		"new <project> <title> - add a card\n" +
		"status - what is happening right now\n" +
		"help - this list"
}
