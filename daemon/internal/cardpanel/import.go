package cardpanel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

// ImportComment adds a comment that was written somewhere else, such as on a Trello card, and answers
// its id. The comment is the person's, dated when it was written there. Unlike Post it never puts the
// text in front of the card's agent, whatever it says: text from outside must not wake an agent, and
// the agent reads it with the rest of the card's unread comments when it next runs.
func (s *Service) ImportComment(ctx context.Context, cardID, body, userID string, at time.Time) (string, error) {
	if _, err := s.card(ctx, cardID); err != nil {
		return "", err
	}
	body = strings.TrimSpace(body)
	if runes := []rune(body); len(runes) > protocol.MaxCommentChars {
		body = string(runes[:protocol.MaxCommentChars])
	}
	if body == "" {
		return "", protocol.InvalidArgument("There is no text to import.")
	}
	id, err := s.newID()
	if err != nil {
		return "", fmt.Errorf("make a comment id: %w", err)
	}
	if at.IsZero() {
		at = s.now()
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		return q.CreateComment(ctx, db.CreateCommentParams{
			ID: id, CardID: cardID, AuthorKind: string(protocol.AuthorKindPerson), AuthorID: userID,
			Body: body, MentionsJSON: "[]", CreatedAt: ms(at),
		})
	})
	if err != nil {
		return "", fmt.Errorf("import a comment on card %s: %w", cardID, err)
	}
	s.publish(cardID, protocol.EventTypeCommentCreated)
	return id, nil
}
