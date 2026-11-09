package chatcmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// Control exposes the same card actions used by Marshal's screens.
type Control interface {
	Card(context.Context, protocol.CardKey) (protocol.Card, error)
	Cards(context.Context) ([]protocol.Card, error)
	Start(context.Context, string) (protocol.Card, error)
	Stop(context.Context, string) error
	Send(context.Context, string, string) error
	Reply(context.Context, string) (string, error)
	SetAgent(context.Context, string, protocol.AgentKind) error
	Finish(context.Context, string) error
}

// WithControl enables coding commands for a configured chat.
func (s *Service) WithControl(control Control) *Service {
	s.control = control
	return s
}

func (s *Service) controlCommand(ctx context.Context, bot chatbot.Bot, cmd chatbot.Command) bool {
	switch cmd.Verb {
	case chatbot.VerbStart, chatbot.VerbResume, chatbot.VerbStop, chatbot.VerbMessage,
		chatbot.VerbReply, chatbot.VerbAgent, chatbot.VerbFinish, chatbot.VerbStatus:
	default:
		return false
	}
	if s.control == nil {
		s.reply(ctx, bot, "Coding commands are not set up on this computer.")
		return true
	}
	if cmd.Verb == chatbot.VerbStatus && cmd.Target == "" {
		s.listStatus(ctx, bot)
		return true
	}
	key, err := protocol.ParseCardKey(cmd.Target)
	if err != nil {
		s.reply(ctx, bot, "Name the project and card together, for example web#12. Send status to list cards.")
		return true
	}
	card, err := s.control.Card(ctx, key)
	if err != nil {
		s.controlError(ctx, bot, err)
		return true
	}
	answer, err := s.runControl(ctx, card, cmd)
	if err != nil {
		s.controlError(ctx, bot, err)
		return true
	}
	s.reply(ctx, bot, answer)
	return true
}

func (s *Service) runControl(ctx context.Context, card protocol.Card, cmd chatbot.Command) (string, error) {
	switch cmd.Verb {
	case chatbot.VerbStart, chatbot.VerbResume:
		started, err := s.control.Start(ctx, card.ID)
		return cardStatus(started) + "\nSend reply " + card.Key + " to read the answer.", err
	case chatbot.VerbStop:
		return "Stopped " + card.Key + ". Send resume " + card.Key + " to continue.", s.control.Stop(ctx, card.ID)
	case chatbot.VerbMessage:
		err := s.control.Send(ctx, card.ID, cmd.Text)
		return "Sent to " + card.Key + ". Send reply " + card.Key + " to read the answer.", err
	case chatbot.VerbReply:
		text, err := s.control.Reply(ctx, card.ID)
		if text == "" {
			text = "No answer for the latest message yet. Send status " + card.Key + " to check progress."
		}
		return card.Key + "\n" + text, err
	case chatbot.VerbAgent:
		agent := protocol.AgentKind(strings.ToLower(strings.TrimSpace(cmd.Text)))
		if !agent.Valid() {
			return "", protocol.InvalidArgument("Choose claude, codex, gemini, or builtin.")
		}
		return "Agent for " + card.Key + " set to " + string(agent) + ".", s.control.SetAgent(ctx, card.ID, agent)
	case chatbot.VerbFinish:
		return "Review and completion requested for " + card.Key + ". Send status " + card.Key + " to check progress.", s.control.Finish(ctx, card.ID)
	default:
		return cardStatus(card), nil
	}
}

func cardStatus(card protocol.Card) string {
	session := "not started"
	if card.Session != nil {
		session = string(*card.Session)
	}
	text := fmt.Sprintf("%s — %s\n%s, session %s, agent %s", card.Key, card.Title, card.State, session, card.Agent)
	if card.DoingNow != "" {
		text += "\n" + card.DoingNow
	}
	if card.NeedsReason != nil {
		text += "\n" + card.NeedsReason.Text
		if card.NeedsReason.ApprovalID != "" {
			text += "\nSend approve " + card.NeedsReason.ApprovalID + " or reject " + card.NeedsReason.ApprovalID + "."
		}
	}
	return text
}

func (s *Service) listStatus(ctx context.Context, bot chatbot.Bot) {
	cards, err := s.control.Cards(ctx)
	if err != nil {
		s.controlError(ctx, bot, err)
		return
	}
	lines := []string{"Cards in Marshal"}
	const maxCards = 20
	for _, card := range cards {
		if card.State == protocol.CardStateDone {
			continue
		}
		if len(lines) > maxCards {
			lines = append(lines, "Open Marshal for more cards.")
			break
		}
		lines = append(lines, fmt.Sprintf("%s — %s (%s)", card.Key, card.Title, card.State))
	}
	if len(lines) == 1 {
		lines = append(lines, "No open cards. Send new <project> <title> to add one.")
	}
	s.reply(ctx, bot, strings.Join(lines, "\n"))
}

func (s *Service) controlError(ctx context.Context, bot chatbot.Bot, err error) {
	var message *protocol.Error
	if errors.As(err, &message) {
		s.reply(ctx, bot, message.Message)
		return
	}
	s.log.Warn("a chat coding command failed", "err", err)
	s.reply(ctx, bot, "Marshal could not complete that action. Check the card in Marshal and try again.")
}
