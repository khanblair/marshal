// Package search answers the command palette and the top bar (docs/backend-checklist.md B2.11,
// docs/backend-inventory.md N23): one query, matched against every project, every card, and every
// live chat, answered as one list per kind with the best match first. Past sessions and notes join
// in Phase 7.
//
// It owns no table. The projects and the chats are read through the two small interfaces below,
// which the projects service and the chats service already satisfy, so this package never reads
// another module's tables (docs/architecture.md section 4) and a new place a card can be found
// changes one service, not this one. Nothing is indexed: a search reads what the sidebar and the
// boards read and matches it in memory, which takes about 3 ms over 3 projects with 100 cards and
// 15 chats (TestSearchIsQuickOnABigBoard in internal/api logs the figure), so no index is needed.
package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

const (
	messageQueryTooLong = "Search for 200 characters or fewer."
	messageQueryNotText = "That search is not text Marshal can read."
)

// Projects is what a search needs from the projects service: the projects, and one project's
// cards.
type Projects interface {
	// List returns every project, oldest first.
	List(ctx context.Context) (protocol.ProjectListSnapshot, error)
	// Cards returns every card of one project.
	Cards(ctx context.Context, projectID string) ([]protocol.Card, error)
}

// Chats is what a search needs from the chats service.
type Chats interface {
	// List returns one project's chats: the live ones, or the archived ones when archived is true.
	List(ctx context.Context, projectID string, archived bool) (protocol.ChatListSnapshot, error)
}

// Deps are the parts the service is built from.
type Deps struct {
	// Projects gives the projects and their cards.
	Projects Projects
	// Chats gives the chats of a project.
	Chats Chats
}

// Service answers searches. It is safe for use by many goroutines: it keeps nothing between
// searches.
type Service struct {
	projects Projects
	chats    Chats
	now      func() time.Time
}

// Option changes how New builds a Service.
type Option func(*Service)

// WithClock sets the clock that stamps an answer. The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// New builds the service. Both readers are required: an answer with a kind missing would look like
// a kind with no matches.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Projects == nil || deps.Chats == nil {
		return nil, errors.New("search: the projects and the chats are both required")
	}
	s := &Service{projects: deps.Projects, chats: deps.Chats, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// Search answers GET /v1/search?q=. The query is cleaned first (see cleanQuery); an empty one
// matches nothing and costs no read, since the palette lists its own commands until something is
// typed. A query that is too long, or not text, is refused with a sentence.
//
// A project removed between the list and the read of its cards is left out rather than failing the
// search: the answer is then the same as one made a moment later.
func (s *Service) Search(ctx context.Context, raw string) (protocol.SearchSnapshot, error) {
	query, err := cleanQuery(raw)
	if err != nil {
		return protocol.SearchSnapshot{}, err
	}
	var got matches
	if query != "" {
		if err := s.collect(ctx, strings.Fields(strings.ToLower(query)), &got); err != nil {
			return protocol.SearchSnapshot{}, err
		}
	}
	return protocol.SearchSnapshot{
		Query:    query,
		Projects: got.projects.top(),
		Cards:    got.cards.top(),
		Chats:    got.chats.top(),
		Totals: protocol.SearchTotals{
			Projects: got.projects.total(), Cards: got.cards.total(), Chats: got.chats.total(),
		},
		ServerTime: protocol.NewTimestamp(s.now()),
	}, nil
}

// matches is what a search has matched so far, one ranking for each kind.
type matches struct {
	projects ranking[protocol.ProjectHit]
	cards    ranking[protocol.CardHit]
	chats    ranking[protocol.ChatHit]
}

// collect reads the projects, then each project's cards and chats, and matches them against the
// words of the query.
func (s *Service) collect(ctx context.Context, words []string, out *matches) error {
	list, err := s.projects.List(ctx)
	if err != nil {
		return fmt.Errorf("search: list the projects: %w", err)
	}
	for _, project := range list.Projects {
		if err := ctx.Err(); err != nil {
			return err
		}
		if score := projectScore(project, words); score > 0 {
			out.projects.add(score, 0, protocol.ProjectHit{
				ProjectID: project.ID, Name: project.Name, Path: project.Path, Language: project.Language,
			})
		}
		if err := s.collectCards(ctx, project, words, &out.cards); err != nil {
			return err
		}
		if err := s.collectChats(ctx, project, words, &out.chats); err != nil {
			return err
		}
	}
	return nil
}

// collectCards matches one project's cards.
func (s *Service) collectCards(ctx context.Context, project protocol.Project, words []string, out *ranking[protocol.CardHit]) error {
	cards, err := s.projects.Cards(ctx, project.ID)
	if gone(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("search: read the cards of project %s: %w", project.ID, err)
	}
	for _, card := range cards {
		if score := cardScore(card, words); score > 0 {
			out.add(score, card.UpdatedAt.Time().UnixMilli(), protocol.CardHit{
				CardID: card.ID, Key: card.Key, Number: card.Number, Title: card.Title,
				State: card.State, ProjectID: project.ID, ProjectName: project.Name,
			})
		}
	}
	return nil
}

// collectChats matches one project's live chats. Archived chats stay behind the Archived toggle,
// so a search does not bring them back.
func (s *Service) collectChats(ctx context.Context, project protocol.Project, words []string, out *ranking[protocol.ChatHit]) error {
	list, err := s.chats.List(ctx, project.ID, false)
	if gone(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("search: read the chats of project %s: %w", project.ID, err)
	}
	for _, chat := range list.Chats {
		if score := chatScore(chat, words); score > 0 {
			out.add(score, chat.LastActiveAt.Time().UnixMilli(), protocol.ChatHit{
				ChatID: chat.ID, Title: chat.Title, ProjectID: project.ID, ProjectName: project.Name,
				LastActiveAt: chat.LastActiveAt,
			})
		}
	}
	return nil
}

// gone reports whether err is a project that was removed while the search was running.
func gone(err error) bool {
	var refusal *protocol.Error
	return errors.As(err, &refusal) && refusal.Code == protocol.ErrorCodeNotFound
}

// cleanQuery trims the query and makes each run of spaces one, which is the form the answer
// echoes. A query is text a person typed, so bytes that are not text are refused, and a query
// longer than protocol.MaxSearchQueryChars is refused rather than cut: a cut query would answer a
// question nobody asked.
func cleanQuery(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", protocol.InvalidArgument(messageQueryNotText)
	}
	query := strings.Join(strings.Fields(raw), " ")
	if utf8.RuneCountInString(query) > protocol.MaxSearchQueryChars {
		return "", protocol.InvalidArgument(messageQueryTooLong)
	}
	return query, nil
}

// ranking collects the hits of one kind with their scores, and hands back the best of them.
type ranking[T any] struct {
	entries []entry[T]
}

// entry is one hit with what orders it: its score, then how recently it was touched.
type entry[T any] struct {
	score int
	touch int64
	hit   T
}

// add records a hit. touch is when the thing last changed, in Unix milliseconds, and breaks a tie
// between two hits of the same score; two of the same touch keep the order they were added in.
func (r *ranking[T]) add(score int, touch int64, hit T) {
	r.entries = append(r.entries, entry[T]{score: score, touch: touch, hit: hit})
}

// total is how many hits were added.
func (r *ranking[T]) total() int { return len(r.entries) }

// top returns the best protocol.SearchHitsPerKind hits, best first. It is never nil.
func (r *ranking[T]) top() []T {
	slices.SortStableFunc(r.entries, func(a, b entry[T]) int {
		if a.score != b.score {
			return b.score - a.score
		}
		switch {
		case a.touch > b.touch:
			return -1
		case a.touch < b.touch:
			return 1
		}
		return 0
	})
	n := min(len(r.entries), protocol.SearchHitsPerKind)
	hits := make([]T, n)
	for i := range n {
		hits[i] = r.entries[i].hit
	}
	return hits
}
