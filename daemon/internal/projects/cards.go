package projects

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/store/db"
)

const (
	// maxTitleChars is the longest card title.
	maxTitleChars = 200
	// maxBodyBytes is the longest card description.
	maxBodyBytes = 100 * 1024
	// maxModelChars is the longest model name.
	maxModelChars = 100
)

// CardOption changes one call to CreateCard.
type CardOption func(*cardConfig)

type cardConfig struct {
	createdBy string
}

// WithCreatedBy records which user made the card. The API layer knows the user, and the request
// body does not carry it, so a client cannot claim to be someone else.
func WithCreatedBy(userID string) CardOption {
	return func(c *cardConfig) { c.createdBy = userID }
}

// CreateCard adds a card to the backlog of a project. Its number comes from the project's own
// counter, which moves on in the same transaction, so numbers never repeat and never skip, even
// when many cards are made at once. card.created is published after the commit.
func (s *Service) CreateCard(ctx context.Context, projectID string, in CardInput, opts ...CardOption) (protocol.Card, error) {
	var cfg cardConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	params, err := s.newCardParams(projectID, in, cfg)
	if err != nil {
		return protocol.Card{}, err
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		board, err := q.GetBoardByProject(ctx, projectID)
		if err != nil {
			return notFound(fmt.Errorf("read the board of project %s: %w", projectID, err), notFoundProject(projectID))
		}
		if params.Number, err = q.ReserveCardNumber(ctx, projectID); err != nil {
			return fmt.Errorf("take the next card number of project %s: %w", projectID, err)
		}
		params.BoardID = board.ID
		if err := q.CreateCard(ctx, params); err != nil {
			return fmt.Errorf("insert the card: %w", err)
		}
		return nil
	})
	if err != nil {
		return protocol.Card{}, fmt.Errorf("add a card to project %s: %w", projectID, err)
	}
	card := toCard(db.Card{
		ID: params.ID, ProjectID: projectID, Number: params.Number, BoardID: params.BoardID,
		Title: params.Title, Body: params.Body, State: params.State, AgentKind: params.AgentKind,
		Model: params.Model, Thinking: params.Thinking, PermissionMode: params.PermissionMode,
		CreatedBy: params.CreatedBy, CreatedAt: params.CreatedAt, UpdatedAt: params.UpdatedAt,
	})
	s.log.Info("added a card", "project_id", projectID, "card_id", card.ID, "number", card.Number)
	s.publish(protocol.ProjectTopic(projectID), protocol.EventTypeCardCreated, protocol.CardEventData{Card: card}, true)
	return card, nil
}

// newCardParams checks a request and fills in its defaults: Claude Code, auto-accept edits, and no
// thinking setting. The number and the board are filled in inside the transaction.
func (s *Service) newCardParams(projectID string, in CardInput, cfg cardConfig) (db.CreateCardParams, error) {
	title := strings.TrimSpace(in.Title)
	if err := checkCardInput(title, in); err != nil {
		return db.CreateCardParams{}, err
	}
	agent, mode := in.Agent, in.PermissionMode
	if agent == "" {
		agent = protocol.AgentKindClaude
	}
	if mode == "" {
		mode = protocol.PermissionModeAutoEdits
	}
	id, err := s.newID()
	if err != nil {
		return db.CreateCardParams{}, fmt.Errorf("make a card id: %w", err)
	}
	now := s.now().UnixMilli()
	return db.CreateCardParams{
		ID: id, ProjectID: projectID, Title: title, Body: in.Body,
		State: string(protocol.CardStateBacklog), AgentKind: string(agent), Model: strings.TrimSpace(in.Model),
		Thinking: string(in.Thinking), PermissionMode: string(mode), CreatedBy: cfg.createdBy,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// checkCardInput refuses a card request that is not allowed. An empty agent, thinking, or
// permission mode is fine: it means the default.
func checkCardInput(title string, in CardInput) error {
	switch {
	case title == "":
		return protocol.InvalidArgument("Give the card a title.")
	case utf8.RuneCountInString(title) > maxTitleChars:
		return protocol.InvalidArgument(fmt.Sprintf("Card titles can have at most %d characters.", maxTitleChars))
	case len(in.Body) > maxBodyBytes:
		return protocol.InvalidArgument("That description is too long. Shorten it, or attach a file instead.")
	case utf8.RuneCountInString(in.Model) > maxModelChars:
		return protocol.InvalidArgument("That model name is too long.")
	case in.Agent != "" && !in.Agent.Valid():
		return protocol.InvalidArgument("Marshal does not know that agent.").With("agent", string(in.Agent))
	case in.Thinking != "" && !in.Thinking.Valid():
		return protocol.InvalidArgument("That is not a thinking setting Marshal knows.").With("thinking", string(in.Thinking))
	case in.PermissionMode != "" && !in.PermissionMode.Valid():
		return protocol.InvalidArgument("That is not a permission mode Marshal knows.").With("permissionMode", string(in.PermissionMode))
	}
	return nil
}

// Card returns one card by its opaque id.
func (s *Service) Card(ctx context.Context, id string) (protocol.Card, error) {
	row, err := s.store.Queries().GetCard(ctx, id)
	if err != nil {
		return protocol.Card{}, notFound(fmt.Errorf("read card %s: %w", id, err), notFoundCard(id))
	}
	return toCard(row), nil
}

// CardByKey returns the card with this project and number. Two projects can each have a card 12,
// so the number alone means nothing.
func (s *Service) CardByKey(ctx context.Context, key protocol.CardKey) (protocol.Card, error) {
	row, err := s.store.Queries().GetCardByKey(ctx, db.GetCardByKeyParams{ProjectID: key.ProjectID, Number: int64(key.Number)})
	if err != nil {
		return protocol.Card{}, notFound(fmt.Errorf("read card %s: %w", key, err), protocol.NotFound("card").With("key", key.String()))
	}
	return toCard(row), nil
}

// Cards returns all the cards of a project in number order.
func (s *Service) Cards(ctx context.Context, projectID string) ([]protocol.Card, error) {
	var rows []db.Card
	err := s.store.Read(ctx, func(q *db.Queries) error {
		if _, err := q.GetProject(ctx, projectID); err != nil {
			return notFound(fmt.Errorf("read project %s: %w", projectID, err), notFoundProject(projectID))
		}
		var err error
		if rows, err = q.ListCardsByProject(ctx, projectID); err != nil {
			return fmt.Errorf("list the cards of project %s: %w", projectID, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return toCards(rows), nil
}
