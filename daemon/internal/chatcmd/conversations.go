package chatcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/khanblair/marshal/daemon/internal/chatbot"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	conversationPollInterval = 2 * time.Second
	conversationCallTimeout  = 30 * time.Second
	maxConversationLinks     = 512
	maxConversationCards     = 64
)

// Answers identifies a completed answer by its history revision, including repeated identical replies.
type Answers interface {
	Answer(context.Context, string) (text, revision string, err error)
}

type conversationStore struct {
	store  *store.Store
	mu     sync.Mutex
	active map[string]*conversation
}

type conversation struct {
	mu    sync.Mutex
	Links []messageLink
	Cards []watchedCard
}

type messageLink struct {
	ChatID, MessageID, ThreadID, Key string
}

type watchedCard struct {
	ID, Key, Revision string
	To                chatbot.Incoming
}

// WithConversations stores reply links and delivered revisions in Marshal's private database.
func (s *Service) WithConversations(st *store.Store) *Service {
	s.conversations = &conversationStore{store: st, active: make(map[string]*conversation)}
	return s
}

func (s *Service) conversation(ctx context.Context, scope string) (*conversation, error) {
	cs := s.conversations
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if found := cs.active[scope]; found != nil {
		return found, nil
	}
	state := &conversation{}
	if cs.store != nil {
		value, err := cs.store.Queries().GetSetting(ctx, "chat.conversation."+scope)
		if err != nil && !store.IsNotFound(err) {
			return nil, err
		}
		if err == nil {
			if err := json.Unmarshal([]byte(value), state); err != nil {
				return nil, fmt.Errorf("read chat links: %w", err)
			}
		}
	}
	cs.active[scope] = state
	return state, nil
}

func (s *Service) saveConversation(ctx context.Context, scope string, state *conversation) error {
	if s.conversations.store == nil {
		return nil
	}
	value, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.conversations.store.Write(ctx, func(q *db.Queries) error {
		return q.SetSetting(ctx, db.SetSettingParams{Key: "chat.conversation." + scope, ValueJSON: string(value)})
	})
}

// linkedBot narrows every reply to the already trusted incoming conversation.
type linkedBot struct {
	chatbot.ConversationBot
	svc   *Service
	state *conversation
	in    chatbot.Incoming
	key   string
}

func (b *linkedBot) Notify(ctx context.Context, notice chatbot.Notice) error {
	id, err := b.SendReply(ctx, notice, b.in)
	if err != nil {
		return err
	}
	if id != "" && b.key != "" {
		b.state.Links = append(b.state.Links, messageLink{ChatID: b.in.ChatID, MessageID: id, ThreadID: b.in.ThreadID, Key: b.key})
		if len(b.state.Links) > maxConversationLinks {
			b.state.Links = b.state.Links[len(b.state.Links)-maxConversationLinks:]
		}
		return b.svc.saveConversation(ctx, b.Scope(), b.state)
	}
	return nil
}

func (b *linkedBot) resolve(cmd chatbot.Command, text string) (chatbot.Command, bool) {
	for i := len(b.state.Links) - 1; i >= 0; i-- {
		link := b.state.Links[i]
		if link.ChatID != b.in.ChatID {
			continue
		}
		thread := b.in.ThreadID != "" && link.ThreadID == b.in.ThreadID
		reply := b.in.ReplyToID != "" && link.MessageID == b.in.ReplyToID
		if !thread && !reply {
			continue
		}
		b.key = link.Key
		cmd = contextualCommand(cmd, text, link.Key)
		if thread && differentCard(cmd, link.Key) {
			return cmd, false
		}
		return cmd, true
	}
	return cmd, true
}

func differentCard(cmd chatbot.Command, key string) bool {
	if cmd.Verb == chatbot.VerbNew {
		return true
	}
	if cmd.Verb == chatbot.VerbApprove || cmd.Verb == chatbot.VerbReject {
		return false
	}
	return cmd.Target != "" && cmd.Target != key
}

func contextualCommand(cmd chatbot.Command, text, key string) chatbot.Command {
	if cmd.Verb == chatbot.VerbMessage && !strings.Contains(cmd.Target, "#") {
		words := strings.Fields(text)
		rest := strings.TrimSpace(strings.TrimSpace(text)[len(words[0]):])
		return chatbot.Command{Verb: chatbot.VerbMessage, Target: key, Text: rest}
	}
	if cmd.Verb == chatbot.VerbStatus && cmd.Target == "" {
		cmd.Target = key
	}
	if cmd.Verb != chatbot.VerbUnknown || strings.HasPrefix(strings.ToLower(strings.TrimSpace(text)), "approval:") {
		return cmd
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return cmd
	}
	verb := strings.ToLower(strings.SplitN(strings.TrimPrefix(words[0], "/"), "@", 2)[0])
	rest := strings.TrimSpace(strings.TrimSpace(text)[len(words[0]):])
	switch chatbot.Verb(verb) {
	case chatbot.VerbStart, chatbot.VerbResume, chatbot.VerbStop, chatbot.VerbReply, chatbot.VerbFinish:
		if rest == "" {
			return chatbot.Command{Verb: chatbot.Verb(verb), Target: key}
		}
	case chatbot.VerbAgent, chatbot.VerbMessage:
		if rest != "" {
			return chatbot.Command{Verb: chatbot.Verb(verb), Target: key, Text: rest}
		}
	case chatbot.VerbApprove, chatbot.VerbReject, chatbot.VerbNew, chatbot.VerbHelp, chatbot.VerbStatus:
		return cmd
	default:
		// Only ordinary text in a linked reply becomes an editing instruction.
		if !strings.HasPrefix(strings.TrimSpace(text), "/") {
			return chatbot.Command{Verb: chatbot.VerbMessage, Target: key, Text: text}
		}
	}
	return cmd
}

func (s *Service) linkCard(ctx context.Context, bot chatbot.Bot, card protocol.Card) error {
	b, ok := bot.(*linkedBot)
	if !ok {
		return nil
	}
	b.key = card.Key
	for i := range b.state.Cards {
		if b.state.Cards[i].ID == card.ID {
			b.state.Cards[i].To = replyAddress(b.in)
			return s.saveConversation(ctx, b.Scope(), b.state)
		}
	}
	var revision string
	if answers, ok := s.control.(Answers); ok {
		_, baseline, err := answers.Answer(ctx, card.ID)
		if err != nil {
			return err
		}
		revision = baseline
	}
	b.state.Cards = append(b.state.Cards, watchedCard{ID: card.ID, Key: card.Key, Revision: revision, To: replyAddress(b.in)})
	if len(b.state.Cards) > maxConversationCards {
		b.state.Cards = b.state.Cards[len(b.state.Cards)-maxConversationCards:]
	}
	return s.saveConversation(ctx, b.Scope(), b.state)
}

// Only addresses are saved. Incoming instructions and credentials do not belong in these settings.
func replyAddress(in chatbot.Incoming) chatbot.Incoming {
	return chatbot.Incoming{ChatID: in.ChatID, ChatName: in.ChatName, MessageID: in.MessageID, ThreadID: in.ThreadID}
}

func (s *Service) runConversation(ctx context.Context, bot chatbot.Bot) error {
	transport, capable := bot.(chatbot.ConversationBot)
	if !capable || s.conversations == nil {
		return bot.Start(ctx, func(callCtx context.Context, in chatbot.Incoming) { s.handle(callCtx, bot, in) })
	}
	state, err := s.conversation(ctx, transport.Scope())
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		s.followAnswers(runCtx, transport, state)
	}()
	err = bot.Start(runCtx, func(callCtx context.Context, in chatbot.Incoming) {
		if !bot.Accepts(in) {
			return
		}
		callCtx, cancel := context.WithTimeout(callCtx, conversationCallTimeout)
		defer cancel()
		state.mu.Lock()
		defer state.mu.Unlock()
		s.handle(callCtx, &linkedBot{ConversationBot: transport, svc: s, state: state, in: in}, in)
	})
	if err != nil {
		cancel()
	}
	return err
}

func (s *Service) followAnswers(ctx context.Context, bot chatbot.ConversationBot, state *conversation) {
	ticker := time.NewTicker(conversationPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAnswers(ctx, bot, state)
		}
	}
}

func (s *Service) checkAnswers(ctx context.Context, bot chatbot.ConversationBot, state *conversation) {
	answers, ok := s.control.(Answers)
	if !ok {
		return
	}
	ctx, stop := context.WithTimeout(ctx, conversationCallTimeout)
	defer stop()
	state.mu.Lock()
	defer state.mu.Unlock()
	for i := range state.Cards {
		if ctx.Err() != nil {
			return
		}
		watch := &state.Cards[i]
		if err := s.checkAnswer(ctx, bot, state, watch, answers); err != nil {
			s.log.Warn("could not deliver a card answer", "card_id", watch.ID, "error", err)
		}
	}
}

func (s *Service) checkAnswer(ctx context.Context, bot chatbot.ConversationBot, state *conversation, watch *watchedCard, answers Answers) error {
	key, err := protocol.ParseCardKey(watch.Key)
	if err != nil {
		return err
	}
	card, err := s.control.Card(ctx, key)
	if err != nil {
		return err
	}
	idle := card.Session != nil && *card.Session == protocol.SessionStateAwake
	if !idle && card.State != protocol.CardStateDone {
		return nil
	}
	text, revision, err := answers.Answer(ctx, watch.ID)
	if err != nil {
		return err
	}
	if text == "" || revision == watch.Revision {
		return nil
	}
	if !bot.Accepts(watch.To) {
		return fmt.Errorf("the card conversation is no longer available")
	}
	routed := &linkedBot{ConversationBot: bot, svc: s, state: state, in: watch.To, key: watch.Key}
	if err := sendAnswer(ctx, routed, watch.Key+" — answer\n"+text); err != nil {
		return err
	}
	watch.Revision = revision
	return s.saveConversation(ctx, bot.Scope(), state)
}

func sendAnswer(ctx context.Context, bot chatbot.Bot, text string) error {
	const chunkRunes = 900
	const maxRunes = 6000
	runes := []rune(text)
	if len(runes) > maxRunes {
		runes = append(runes[:maxRunes], []rune("\nOpen Marshal for the rest of this answer.")...)
	}
	for len(runes) > 0 {
		size := min(len(runes), chunkRunes)
		if err := bot.Notify(ctx, chatbot.Notice{Title: string(runes[:size])}); err != nil {
			return err
		}
		runes = runes[size:]
	}
	return nil
}

// Deliver routes the normal app notification, with its original approval buttons, to a linked card.
// False leaves unlinked cards on the existing notification route.
func (s *Service) Deliver(ctx context.Context, bot chatbot.Bot, notice chatbot.Notice) (bool, error) {
	transport, ok := bot.(chatbot.ConversationBot)
	if !ok || s.conversations == nil || notice.CardID == "" {
		return false, nil
	}
	state, err := s.conversation(ctx, transport.Scope())
	if err != nil {
		return true, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, card := range state.Cards {
		if card.ID != notice.CardID {
			continue
		}
		if !bot.Accepts(card.To) {
			return true, fmt.Errorf("the card conversation is no longer available")
		}
		routed := &linkedBot{ConversationBot: transport, svc: s, state: state, in: card.To, key: card.Key}
		notice.Title = card.Key + " — " + notice.Title
		return true, routed.Notify(ctx, notice)
	}
	return false, nil
}
