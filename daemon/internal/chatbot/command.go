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
	// VerbStart starts a card's coding session.
	VerbStart Verb = "start"
	// VerbResume continues a stopped session.
	VerbResume Verb = "resume"
	// VerbStop stops a coding session.
	VerbStop Verb = "stop"
	// VerbMessage sends instructions to a card.
	VerbMessage Verb = "message"
	// VerbReply reads a card's latest answer.
	VerbReply Verb = "reply"
	// VerbAgent selects a card's coding agent.
	VerbAgent Verb = "agent"
	// VerbFinish requests the project's normal review and completion checks.
	VerbFinish Verb = "finish"
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
	// Text is a new card's title, instructions, or an agent name.
	Text string
}

// Parse reads one message as an instruction. It is case-insensitive and splits on the first run of
// whitespace, so "Approve abc123" and "approve abc123" are the same, and a bot mention in front of
// the word ("@marshal approve abc123") is skipped rather than turning the message into an unknown.
func Parse(message string) Command {
	message = strings.TrimSpace(message)
	words := strings.Fields(message)
	if len(words) == 0 {
		return Command{Verb: VerbUnknown}
	}
	// A leading @mention is the service naming the bot, not a person's instruction.
	if strings.HasPrefix(words[0], "@") {
		message = strings.TrimSpace(message[len(words[0]):])
		words = words[1:]
	}
	if len(words) == 0 {
		return Command{Verb: VerbUnknown}
	}
	word := strings.SplitN(strings.TrimPrefix(words[0], "/"), "@", 2)[0]
	verb := Verb(strings.ToLower(word))
	rest := words[1:]
	switch verb {
	case VerbApprove, VerbReject:
		if len(rest) == 0 {
			return Command{Verb: VerbUnknown}
		}
		return Command{Verb: verb, Target: rest[0]}
	case VerbNew:
		return parseNew(rest)
	case VerbMessage, VerbAgent:
		if len(rest) < 2 {
			return Command{Verb: VerbUnknown}
		}
		body := strings.TrimSpace(message[len(words[0]):])
		return Command{Verb: verb, Target: rest[0], Text: strings.TrimSpace(body[len(rest[0]):])}
	case VerbStart, VerbResume, VerbStop, VerbReply, VerbFinish:
		if len(rest) != 1 {
			return Command{Verb: VerbUnknown}
		}
		return Command{Verb: verb, Target: rest[0]}
	case VerbStatus:
		if len(rest) == 1 {
			return Command{Verb: verb, Target: rest[0]}
		}
		if len(rest) != 0 {
			return Command{Verb: VerbUnknown}
		}
		return Command{Verb: verb}
	case VerbHelp:
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
		"agent <project#card> <claude|codex|gemini|builtin> - choose the agent\n" +
		"start <project#card> - start coding\n" +
		"message <project#card> <instructions> - send a follow-up\n" +
		"reply <project#card> - read the latest answer\n" +
		"stop <project#card> - stop the session\n" +
		"resume <project#card> - continue the session\n" +
		"finish <project#card> - request review and completion\n" +
		"status [project#card] - see progress\n" +
		"help - this list"
}
