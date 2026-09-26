package projects

import (
	"context"
	"fmt"
	"strings"
	"time"
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
	createdBy  string
	forkedFrom string
	doingNow   string
	fields     *CardFields
}

// CardFields are the fields a fixture sets as it creates a card: the ones a person's create
// request does not carry, because they are not choices a person makes. Only the dev fixture and
// tests use it. A zero field is left at its default.
type CardFields struct {
	// Number is the card's number in its project. Zero means the project's next number.
	Number int
	// State is the column the card is created in. Empty means the backlog.
	State protocol.CardState
	// CreatedAt and UpdatedAt are when the card was made and last changed. Zero means now.
	CreatedAt time.Time
	UpdatedAt time.Time
	// Role, Package, and DoingNow are the card's role name, monorepo package, and "doing now" line.
	Role, Package, DoingNow string
	// Needs is why the card waits on a person, and NeedsSince is when it started waiting.
	Needs      *protocol.NeedsReason
	NeedsSince time.Time
	// ContextUsed is how full the agent's context window is, as a percentage.
	ContextUsed int
	// PlannedStart, PlannedEnd, Due, ActualStart, and ActualEnd are the card's dates. A zero time
	// leaves the date unset.
	PlannedStart, PlannedEnd, Due, ActualStart, ActualEnd time.Time
	// PullRequest and CI are the card's pull request and CI state.
	PullRequest *protocol.PullRequest
	CI          *protocol.CIState
	// Pinned and Paused are the card's keep-awake and pause flags.
	Pinned, Paused bool
	// Labels are the ids of the labels to put on the card. They must belong to its project.
	Labels []string
}

// WithFields sets the fields a fixture writes as it creates a card. It is for the dev fixture and
// tests: a person's card starts with none of them.
func WithFields(fields CardFields) CardOption {
	return func(c *cardConfig) { c.fields = &fields }
}

// WithCreatedBy records which user made the card. The API layer knows the user, and the request
// body does not carry it, so a client cannot claim to be someone else.
func WithCreatedBy(userID string) CardOption {
	return func(c *cardConfig) { c.createdBy = userID }
}

// WithForkedFrom records the card this one was forked from, so its branch starts from that card's
// latest commit (see ForkCard).
func WithForkedFrom(cardID string) CardOption {
	return func(c *cardConfig) { c.forkedFrom = cardID }
}

// WithDoingNow sets the card's "doing now" line as it is created. A fork uses it; a person's card
// starts with no line.
func WithDoingNow(text string) CardOption {
	return func(c *cardConfig) { c.doingNow = text }
}

// CreateCard adds a card to the backlog of a project. Its number comes from the project's own
// counter, which moves on in the same transaction, so numbers never repeat and never skip, even
// when many cards are made at once. card.created is published after the commit.
func (s *Service) CreateCard(ctx context.Context, projectID string, in CardInput, opts ...CardOption) (protocol.Card, error) {
	var cfg cardConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	var labels []protocol.Label
	params, err := s.newCardParams(projectID, in, cfg)
	if err != nil {
		return protocol.Card{}, err
	}
	err = s.store.Write(ctx, func(q *db.Queries) error {
		var err error
		if params, labels, err = insertCard(ctx, q, projectID, params, cfg); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return protocol.Card{}, fmt.Errorf("add a card to project %s: %w", projectID, err)
	}
	// A card made by a person has no labels, and toCard sends [] rather than null for an empty
	// list. A fixture's card carries the labels it was given.
	card := toCard(db.Card{
		ID: params.ID, ProjectID: projectID, Number: params.Number, BoardID: params.BoardID,
		Title: params.Title, Body: params.Body, State: params.State, AgentKind: params.AgentKind,
		Model: params.Model, Thinking: params.Thinking, PermissionMode: params.PermissionMode,
		Role: params.Role, Package: params.Package, DoingNow: cfg.doingNow,
		CreatedBy: params.CreatedBy, CreatedAt: params.CreatedAt, UpdatedAt: params.UpdatedAt,
	}, labels)
	s.log.Info("added a card", "project_id", projectID, "card_id", card.ID, "number", card.Number)
	s.publish(protocol.ProjectTopic(projectID), protocol.EventTypeCardCreated, protocol.CardEventData{Card: card}, true)
	return card, nil
}

// insertCard writes one card inside the caller's transaction: the row, then what a fixture asked
// for that the plain insert does not carry, then its labels. It answers with the labels that ended
// up on the card.
func insertCard(ctx context.Context, q *db.Queries, projectID string, params db.CreateCardParams, cfg cardConfig) (db.CreateCardParams, []protocol.Label, error) {
	board, err := q.GetBoardByProject(ctx, projectID)
	if err != nil {
		return params, nil, notFound(fmt.Errorf("read the board of project %s: %w", projectID, err), notFoundProject(projectID))
	}
	if number, ok := fixtureNumber(cfg); ok {
		// A fixture writes the prototype's own numbers. Taking one from the counter and then
		// overwriting it would creep the counter upwards, so the next card a person adds would be
		// numbered past the highest the fixture wrote rather than just after it.
		params.Number = int64(number)
	} else if params.Number, err = q.ReserveCardNumber(ctx, projectID); err != nil {
		return params, nil, fmt.Errorf("take the next card number of project %s: %w", projectID, err)
	}
	params.BoardID = board.ID
	if err := q.CreateCard(ctx, params); err != nil {
		return params, nil, fmt.Errorf("insert the card: %w", err)
	}
	if cfg.forkedFrom != "" || cfg.doingNow != "" {
		if _, err := q.SetCardForkFields(ctx, db.SetCardForkFieldsParams{
			ForkedFrom: cfg.forkedFrom, DoingNow: cfg.doingNow,
			UpdatedAt: params.UpdatedAt, ID: params.ID,
		}); err != nil {
			return params, nil, fmt.Errorf("record where the card came from: %w", err)
		}
	}
	if cfg.fields == nil {
		return params, nil, nil
	}
	if err := seedCardFields(ctx, q, params, *cfg.fields); err != nil {
		return params, nil, err
	}
	if len(cfg.fields.Labels) == 0 {
		return params, nil, nil
	}
	labels, err := replaceCardLabels(ctx, q, projectID, params.ID, cfg.fields.Labels)
	return params, labels, err
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
	state := protocol.CardStateBacklog
	created, updated := now, now
	if f := cfg.fields; f != nil {
		if f.State != "" {
			state = f.State
		}
		if !f.CreatedAt.IsZero() {
			created = f.CreatedAt.UnixMilli()
		}
		if !f.UpdatedAt.IsZero() {
			updated = f.UpdatedAt.UnixMilli()
		}
	}
	return db.CreateCardParams{
		ID: id, ProjectID: projectID, Title: title, Body: in.Body,
		State: string(state), AgentKind: string(agent), Model: strings.TrimSpace(in.Model),
		Thinking: string(in.Thinking), PermissionMode: string(mode),
		Role: strings.TrimSpace(in.Role), Package: strings.TrimSpace(in.Package),
		CreatedBy: cfg.createdBy, CreatedAt: created, UpdatedAt: updated,
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
	case in.StartState != "" && !startStateAllowed(in.StartState):
		return protocol.InvalidArgument(
			"A card can be added to the backlog, planning, or working. The other columns come from what happens to the card.").
			With("startState", string(in.StartState))
	}
	return nil
}

// startStateAllowed reports whether a card may be created in a state. Only the three columns a
// person can add a card in are allowed (architecture.md section 6).
func startStateAllowed(state protocol.CardState) bool {
	switch state {
	case protocol.CardStateBacklog, protocol.CardStatePlanning, protocol.CardStateWorking:
		return true
	}
	return false
}

// seedCardFields writes the fields a fixture sets that the plain insert does not carry. It reads
// the row back and writes it whole through updateCardRow, so there is one place that knows what
// the card columns mean.
func seedCardFields(ctx context.Context, q *db.Queries, params db.CreateCardParams, f CardFields) error {
	row, err := q.GetCard(ctx, params.ID)
	if err != nil {
		return fmt.Errorf("read the card back to seed it: %w", err)
	}
	row.DoingNow = f.DoingNow
	row.ContextUsed, row.Pinned, row.Paused = int64(f.ContextUsed), intFromBool(f.Pinned), intFromBool(f.Paused)
	row.PlannedStart, row.PlannedEnd = timePointerOrNil(f.PlannedStart), timePointerOrNil(f.PlannedEnd)
	row.Due, row.ActualStart, row.ActualEnd = timePointerOrNil(f.Due), timePointerOrNil(f.ActualStart), timePointerOrNil(f.ActualEnd)
	if f.PullRequest != nil {
		number := int64(f.PullRequest.Number)
		row.PullRequestNumber, row.PullRequestUrl = &number, f.PullRequest.URL
	}
	if f.CI != nil {
		row.CiState = string(*f.CI)
	}
	if f.Needs != nil {
		row.NeedsReasonKind, row.NeedsReasonText = string(f.Needs.Kind), f.Needs.Text
	}
	row.NeedsSince = timePointerOrNil(f.NeedsSince)
	if err := updateCardRow(ctx, q, row); err != nil {
		return err
	}
	if f.Number > 0 {
		if err := q.SetNextCardNumber(ctx, db.SetNextCardNumberParams{
			NextCardNumber: int64(f.Number + 1), ID: params.ProjectID, NextCardNumber_2: int64(f.Number + 1),
		}); err != nil {
			return fmt.Errorf("move the card counter past the fixture's number: %w", err)
		}
	}
	return nil
}

// fixtureNumber is the number a fixture gave a card, and whether it gave one.
func fixtureNumber(cfg cardConfig) (int, bool) {
	if cfg.fields == nil || cfg.fields.Number <= 0 {
		return 0, false
	}
	return cfg.fields.Number, true
}

// timePointerOrNil is a wire pointer for a time that may be unset.
func timePointerOrNil(t time.Time) *int64 {
	if t.IsZero() {
		return nil
	}
	ms := t.UnixMilli()
	return &ms
}

// Card returns one card by its opaque id.
func (s *Service) Card(ctx context.Context, id string) (protocol.Card, error) {
	return s.oneCard(ctx, notFoundCard(id), func(q *db.Queries) (db.Card, error) {
		return q.GetCard(ctx, id)
	})
}

// CardByKey returns the card with this project and number. Two projects can each have a card 12,
// so the number alone means nothing.
func (s *Service) CardByKey(ctx context.Context, key protocol.CardKey) (protocol.Card, error) {
	missing := protocol.NotFound("card").With("key", key.String())
	return s.oneCard(ctx, missing, func(q *db.Queries) (db.Card, error) {
		return q.GetCardByKey(ctx, db.GetCardByKeyParams{ProjectID: key.ProjectID, Number: int64(key.Number)})
	})
}

// oneCard reads one card with the lookup the caller gives, with its labels, in one read.
func (s *Service) oneCard(ctx context.Context, missing *protocol.Error, load func(*db.Queries) (db.Card, error)) (protocol.Card, error) {
	var card protocol.Card
	err := s.store.Read(ctx, func(q *db.Queries) error {
		row, err := load(q)
		if err != nil {
			return notFound(err, missing)
		}
		labels, err := q.ListLabelsForCard(ctx, row.ID)
		if err != nil {
			return fmt.Errorf("read the labels of card %s: %w", row.ID, err)
		}
		card = toCard(row, toLabels(labels))
		return nil
	})
	if err != nil {
		return protocol.Card{}, err
	}
	return s.withSession(ctx, card)
}

// Cards returns all the cards of a project in number order.
func (s *Service) Cards(ctx context.Context, projectID string) ([]protocol.Card, error) {
	var (
		rows         []db.Card
		labelsByCard map[string][]protocol.Label
	)
	err := s.store.Read(ctx, func(q *db.Queries) error {
		if _, err := q.GetProject(ctx, projectID); err != nil {
			return notFound(fmt.Errorf("read project %s: %w", projectID, err), notFoundProject(projectID))
		}
		var err error
		if rows, err = q.ListCardsByProject(ctx, projectID); err != nil {
			return fmt.Errorf("list the cards of project %s: %w", projectID, err)
		}
		if labelsByCard, err = cardLabelsByCard(ctx, q, projectID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.withSessions(ctx, projectID, toCards(rows, labelsByCard))
}
