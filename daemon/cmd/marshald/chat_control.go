package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/khanblair/marshal/daemon/internal/cardhistory"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/session"
)

// chatControl uses the same services as the HTTP card controls.
type chatControl struct {
	projects *projects.Service
	sessions *session.Manager
	history  *cardhistory.Service
	finisher interface {
		SendToMerge(context.Context, string) (protocol.Card, error)
	}
}

func (c chatControl) Card(ctx context.Context, key protocol.CardKey) (protocol.Card, error) {
	return c.projects.CardByKey(ctx, key)
}

func (c chatControl) Cards(ctx context.Context) ([]protocol.Card, error) {
	list, err := c.projects.List(ctx)
	if err != nil {
		return nil, err
	}
	var cards []protocol.Card
	for _, project := range list.Projects {
		found, err := c.projects.Cards(ctx, project.ID)
		if err != nil {
			return nil, err
		}
		cards = append(cards, found...)
	}
	return cards, nil
}

func (c chatControl) Start(ctx context.Context, id string) (protocol.Card, error) {
	return c.sessions.Start(ctx, id)
}

func (c chatControl) Stop(ctx context.Context, id string) error {
	return c.sessions.Stop(ctx, id)
}

func (c chatControl) Send(ctx context.Context, id, text string) error {
	return c.sessions.Send(ctx, id, text)
}

func (c chatControl) SetAgent(ctx context.Context, id string, agent protocol.AgentKind) error {
	card, err := c.projects.Card(ctx, id)
	if err != nil {
		return err
	}
	if card.Session != nil {
		return protocol.Refused("Choose the agent before starting the card. Create a new card to use a different agent.")
	}
	// A model chosen for the old agent must not be passed to the newly selected one.
	model := ""
	_, err = c.projects.UpdateCard(ctx, id, protocol.UpdateCardRequest{Agent: &agent, Model: &model})
	return err
}

func (c chatControl) Finish(ctx context.Context, id string) error {
	if c.finisher == nil {
		return protocol.Unavailable("Card completion is not set up on this computer.")
	}
	_, err := c.finisher.SendToMerge(ctx, id)
	return err
}

func (c chatControl) Reply(ctx context.Context, id string) (string, error) {
	text, _, err := c.Answer(ctx, id)
	return text, err
}

func (c chatControl) Answer(ctx context.Context, id string) (string, string, error) {
	const maxPages = 4
	const maxBytes = 64 << 10
	const maxRunes = 6000
	var cursor int64
	var chunks []string
	var identity strings.Builder
	bytes := 0
	done := false
	for range maxPages {
		page, err := c.history.Messages(ctx, id, cursor, cardhistory.MaxPageSize)
		if err != nil {
			return "", "", err
		}
		for _, message := range page.Items {
			if message.Kind == protocol.ChatMessageKindUser {
				done = true
				break
			}
			if message.Kind == protocol.ChatMessageKindAgent {
				fmt.Fprintf(&identity, "%s:%d;", message.ID, message.Seq)
				chunks = append(chunks, message.Text)
				bytes += len(message.Text)
				if bytes >= maxBytes {
					done = true
					break
				}
			}
		}
		if done || !page.More {
			break
		}
		cursor = page.Cursor
	}
	var answer strings.Builder
	for i := len(chunks) - 1; i >= 0; i-- {
		answer.WriteString(chunks[i])
	}
	revision := fmt.Sprintf("%x", sha256.Sum256([]byte(identity.String()+answer.String())))
	text := []rune(answer.String())
	if len(text) > maxRunes {
		return "(Latest answer excerpt)\n" + string(text[len(text)-maxRunes:]), revision, nil
	}
	return string(text), revision, nil
}
