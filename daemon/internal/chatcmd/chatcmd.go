// Package chatcmd turns what a person types at a chat bot into the one thing the daemon lets a chat
// do today: answering an approval (B9.3, build-plan 9.5 and 9.6).
//
// It sits between internal/chatbot (which knows how to talk to Telegram and Discord) and the session
// manager (which knows what an approval is), and it owns neither's rules. What it owns is the
// translation - a word from a pocket-sized keyboard into a decision the agent is waiting on - and
// the promise that no message is ever silently dropped, because a bot that ignores you looks exactly
// like a bot that is broken.
//
// Nothing here can be reached by a test through a real service: the Approver and the bot are both
// handed in.
package chatcmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/audit"
	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Approver answers an approval the agent is waiting on. It is the session manager's own `Respond`,
// narrowed to what a chat needs, so this package never has to learn how a session works - and a
// test hands in one that just remembers what it was asked to do.
type Approver interface {
	// Respond decides an approval. optionID may be empty, which lets the daemon pick the plain
	// option for the decision (allow_once to approve, reject_once to deny).
	Respond(ctx context.Context, approvalID string, decision protocol.ApprovalDecision, optionID, actor string) error
}

// Project is one project a chat can add a card to.
type Project struct {
	// ID is the project's id, which a card is created under.
	ID string
	// Name is what the person calls it, and what they type to choose it.
	Name string
}

// Cards adds a card on behalf of a chat. It is the projects service, narrowed to what a message
// needs, so this package learns nothing about boards and a test hands in a recorder.
type Cards interface {
	// Projects lists the projects a card can be added to.
	Projects(ctx context.Context) ([]Project, error)
	// CreateCard adds a card to a project's backlog.
	CreateCard(ctx context.Context, projectID string, in protocol.CreateCardRequest) (protocol.Card, error)
}

// Service runs a bot's receive loop against the daemon.
type Service struct {
	approver Approver
	log      *slog.Logger
	cards    Cards
}

// WithCards lets a chat add cards ("new <project> <title>"). Without it the service still answers
// approvals, and says adding a card is not set up rather than staying silent.
func (s *Service) WithCards(cards Cards) *Service {
	s.cards = cards
	return s
}

// New builds the service. The approver is required: a receive loop that cannot act on what it reads
// would only ever answer, and it would answer people's decisions with "I cannot do that".
func New(approver Approver, log *slog.Logger) (*Service, error) {
	if approver == nil {
		return nil, fmt.Errorf("chat commands need something to answer approvals with")
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{approver: approver, log: log}, nil
}

// Run starts one bot's receive loop. A Telegram bot does not return until ctx ends, while a Discord
// one returns as soon as its gateway is open and keeps receiving on its own, so the caller runs this
// in a goroutine either way and treats a return as "started".
func (s *Service) Run(ctx context.Context, bot chatbot.Bot) error {
	return bot.Start(ctx, func(callCtx context.Context, in chatbot.Incoming) {
		s.handle(callCtx, bot, in)
	})
}

// handle reads one message, acts on it, and always answers it. Every path out of here says
// something, because a message Marshal did not understand should be told so rather than left in
// silence.
func (s *Service) handle(ctx context.Context, bot chatbot.Bot, in chatbot.Incoming) {
	if in.Voice {
		// A voice note is passed on with no text on purpose: Marshal does not transcribe, so a
		// person is asked to type rather than having their words guessed at (hard rule 3).
		s.reply(ctx, bot, "Marshal heard a voice note but does not read them. Please type your answer.")
		return
	}
	if cmd := chatbot.Parse(in.Text); cmd.Verb == chatbot.VerbNew {
		s.newCard(ctx, bot, cmd)
		return
	}
	target, decision, asked := s.read(in.Text)
	if !asked {
		s.reply(ctx, bot, chatbot.Help())
		return
	}
	if target == "" {
		s.reply(ctx, bot, "Send the approval's id from the notice, like: approve 01H1234567890ABCDEFGHJKMNPQ")
		return
	}
	id, option := splitTarget(target)
	if err := s.approver.Respond(ctx, id, decision, option, audit.ActorPerson); err != nil {
		// The daemon's own sentence is what a screen would show, so the chat says the same words
		// rather than inventing one; anything that is not a protocol error is the daemon's own
		// failure and is logged rather than read out loud.
		var perr *protocol.Error
		switch {
		case errors.As(err, &perr) && perr.Message != "":
			s.reply(ctx, bot, perr.Message)
		default:
			s.log.Warn("a chat could not answer an approval", "approval_id", id, "err", err)
			s.reply(ctx, bot, "Marshal could not answer that. Check the approval's id and try again.")
		}
		return
	}
	s.log.Info("answered an approval from a chat", "approval_id", id, "decision", decision)
	s.reply(ctx, bot, answerFor(decision, id))
}

// read turns a message into what it asks for and the decision it means. The second result is false
// for a message that is not asking about an approval at all, and the third for one that asked about
// it without naming one.
//
// Two shapes are accepted, because there are two ways to reach it: what a person types ("approve
// 01H1..."), and the data a notice's own button carries back ("approval:01H1...:allow_once").
func (s *Service) read(text string) (target string, decision protocol.ApprovalDecision, asked bool) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(strings.ToLower(trimmed), "approval:") {
		// A pressed button. The whole thing after the prefix is "<id>" or "<id>:<option>".
		return strings.TrimSpace(trimmed[len("approval:"):]), protocol.ApprovalDecisionApproved, true
	}
	cmd := chatbot.Parse(text)
	switch cmd.Verb {
	case chatbot.VerbApprove:
		if cmd.Target == "" {
			return "", protocol.ApprovalDecisionApproved, true
		}
		return cmd.Target, protocol.ApprovalDecisionApproved, true
	case chatbot.VerbReject:
		if cmd.Target == "" {
			return "", protocol.ApprovalDecisionDenied, true
		}
		return cmd.Target, protocol.ApprovalDecisionDenied, true
	default:
		return "", "", false
	}
}

// splitTarget reads an approval id and, if it was sent, the exact option the agent offered. The id
// comes first because that is how a person reads it out: the option is the part that chooses between
// allow_once and allow_always, and nobody says that out loud before the id.
func splitTarget(target string) (id, option string) {
	id, option, _ = strings.Cut(target, ":")
	return strings.TrimSpace(id), strings.TrimSpace(option)
}

// answerFor is what the chat says once the daemon has decided. It names the approval so several
// decisions in one conversation can be told apart.
func answerFor(decision protocol.ApprovalDecision, id string) string {
	if decision == protocol.ApprovalDecisionDenied {
		return "Denied - " + id
	}
	return "Approved - " + id
}

// reply sends one message back. It goes to the channel notices are sent to rather than to the
// chat the command came from: those are the same place in every setup Marshal writes down - a
// person talks to the bot in the chat Marshal talks back in - and a Notice carries no address of
// its own. If someone ever chats with one bot in a dozen places, this is the line that has to
// learn the difference.
//
// A reply that fails is logged and dropped: the decision has already been made, and a person who
// cannot hear back is still better off than one whose approval never landed.
func (s *Service) reply(ctx context.Context, bot chatbot.Bot, text string) {
	if err := bot.Notify(ctx, chatbot.Notice{Title: text}); err != nil {
		s.log.Warn("a chat reply could not be sent", "err", err)
	}
}

// newCard adds the card a message asked for and says where it went. The first word is the project;
// when it names none and there is only one project, it is the start of the title, because a person
// with one project should not have to name it.
func (s *Service) newCard(ctx context.Context, bot chatbot.Bot, cmd chatbot.Command) {
	if s.cards == nil {
		s.reply(ctx, bot, "Adding a card from a chat is not set up on this computer.")
		return
	}
	projects, err := s.cards.Projects(ctx)
	if err != nil {
		s.log.Warn("a chat could not list the projects", "err", err)
		s.reply(ctx, bot, "Marshal could not read its projects just now. Try again in a moment.")
		return
	}
	project, title, ok := chooseProject(projects, cmd)
	if !ok {
		s.reply(ctx, bot, noSuchProject(cmd.Target, projects))
		return
	}
	card, err := s.cards.CreateCard(ctx, project.ID, protocol.CreateCardRequest{
		Title: title, Body: "Added from a " + string(bot.Kind()) + " chat.",
	})
	if err != nil {
		var perr *protocol.Error
		if errors.As(err, &perr) && perr.Message != "" {
			s.reply(ctx, bot, perr.Message)
			return
		}
		s.log.Warn("a chat could not add a card", "project", project.ID, "err", err)
		s.reply(ctx, bot, "Marshal could not add that card. Try again in a moment.")
		return
	}
	s.log.Info("added a card from a chat", "card", card.ID, "project", project.ID)
	s.reply(ctx, bot, fmt.Sprintf("Added %q to %s.", card.Title, project.Name))
}

// chooseProject finds the project a message names, by id or name, ignoring case. A word that is only
// the start of one name still counts when it is unique. With one project and a first word that names
// nothing, the whole message is the title.
func chooseProject(projects []Project, cmd chatbot.Command) (Project, string, bool) {
	word := strings.ToLower(cmd.Target)
	for _, project := range projects {
		if strings.ToLower(project.ID) == word || strings.ToLower(project.Name) == word {
			return project, cmd.Text, true
		}
	}
	var starts []Project
	for _, project := range projects {
		if strings.HasPrefix(strings.ToLower(project.Name), word) || strings.HasPrefix(strings.ToLower(project.ID), word) {
			starts = append(starts, project)
		}
	}
	if len(starts) == 1 {
		return starts[0], cmd.Text, true
	}
	if len(projects) == 1 {
		return projects[0], cmd.Target + " " + cmd.Text, true
	}
	return Project{}, "", false
}

// noSuchProject says which projects a card can go to, so the person can send the message again.
func noSuchProject(word string, projects []Project) string {
	if len(projects) == 0 {
		return "Marshal has no projects yet, so there is nowhere to add a card."
	}
	names := make([]string, len(projects))
	for i, project := range projects {
		names[i] = project.Name
	}
	return fmt.Sprintf("Marshal has no project called %q. Choose one of: %s.", word, strings.Join(names, ", "))
}
