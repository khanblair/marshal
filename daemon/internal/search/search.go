// Package search answers the command palette and the top bar (docs/backend-checklist.md B2.11 and
// B7.4, docs/backend-inventory.md N23, build-plan task 7.10): one query, matched against every
// project, every card, every live chat, the stored summaries of past sessions, and the notes the
// cards have left, answered as one list per kind with the best match first.
//
// It owns no table. The projects and the chats are read through two small interfaces below, which
// the projects service and the chats service already satisfy, and the past sessions and the notes
// through two more, which the memory module satisfies, so this package never reads another module's
// tables (docs/architecture.md section 4) and a new place a card can be found changes one service,
// not this one. Nothing is indexed here: a search reads what the sidebar and the boards read and
// matches it in memory, which takes about 3 ms over 3 projects with 100 cards and 15 chats
// (TestSearchIsQuickOnABigBoard in internal/api logs the figure), so no index is needed. The two
// kinds that are not read whole - a project's past sessions and its notes - are searched through the
// full-text indexes docs/architecture.md section 10 asks for, one bounded read per kind per project.
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

// Sessions is what a search needs from the memory module: the past session events of one project
// whose stored summaries match a query, best match first. The memory service answers it
// (internal/memory.SearchSessions), reading the full-text index over `session_events.summary`.
type Sessions interface {
	SearchSessions(ctx context.Context, projectID, query string, limit int) ([]protocol.SessionHit, error)
}

// Notes is what a search needs from the memory module: the card notes of one project whose text
// matches a query, best match first. The memory service answers it (internal/memory.SearchNotes),
// reading the full-text index over the notes.
type Notes interface {
	SearchNotes(ctx context.Context, projectID, query string, limit int) ([]protocol.Note, error)
}

// Deps are the parts the service is built from.
type Deps struct {
	// Projects gives the projects and their cards.
	Projects Projects
	// Chats gives the chats of a project.
	Chats Chats
	// Sessions gives the past session events of a project.
	Sessions Sessions
	// Notes gives the notes of a project.
	Notes Notes
}

// Service answers searches. It is safe for use by many goroutines: it keeps nothing between
// searches.
type Service struct {
	projects Projects
	chats    Chats
	sessions Sessions
	notes    Notes
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

// New builds the service. Every reader is required: an answer with a kind missing would look like a
// kind with no matches.
func New(deps Deps, opts ...Option) (*Service, error) {
	if deps.Projects == nil || deps.Chats == nil || deps.Sessions == nil || deps.Notes == nil {
		return nil, errors.New("search: the projects, the chats, the sessions, and the notes are all required")
	}
	s := &Service{
		projects: deps.Projects, chats: deps.Chats, sessions: deps.Sessions, notes: deps.Notes,
		now: time.Now,
	}
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
		Sessions: got.sessions.top(),
		Notes:    got.notes.top(),
		Totals: protocol.SearchTotals{
			Projects: got.projects.total(), Cards: got.cards.total(), Chats: got.chats.total(),
			Sessions: got.sessions.total(), Notes: got.notes.total(),
		},
		ServerTime: protocol.NewTimestamp(s.now()),
	}, nil
}

// matches is what a search has matched so far, one ranking for each kind.
type matches struct {
	projects ranking[protocol.ProjectHit]
	cards    ranking[protocol.CardHit]
	chats    ranking[protocol.ChatHit]
	sessions ranking[protocol.SessionHit]
	notes    ranking[protocol.NoteHit]
}

// collect reads the projects, then each project's cards, chats, past sessions, and notes, and
// matches them against the words of the query.
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
		cards, err := s.collectCards(ctx, project, words, &out.cards)
		if err != nil {
			return err
		}
		if err := s.collectChats(ctx, project, words, &out.chats); err != nil {
			return err
		}
		if err := s.collectSessions(ctx, project, cards, words, &out.sessions); err != nil {
			return err
		}
		if err := s.collectNotes(ctx, project, cards, words, &out.notes); err != nil {
			return err
		}
	}
	return nil
}

// collectCards matches one project's cards, and answers them so the two kinds that are named by
// their card - a past session and a note - are named without reading the cards a second time.
func (s *Service) collectCards(ctx context.Context, project protocol.Project, words []string, out *ranking[protocol.CardHit]) ([]protocol.Card, error) {
	cards, err := s.projects.Cards(ctx, project.ID)
	if gone(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("search: read the cards of project %s: %w", project.ID, err)
	}
	for _, card := range cards {
		if score := cardScore(card, words); score > 0 {
			out.add(score, card.UpdatedAt.Time().UnixMilli(), protocol.CardHit{
				CardID: card.ID, Key: card.Key, Number: card.Number, Title: card.Title,
				State: card.State, ProjectID: project.ID, ProjectName: project.Name,
			})
		}
	}
	return cards, nil
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

// collectSessions matches one project's past session events. The index is read with a limit and not
// whole: it holds every event a card's sessions ever stored, and a palette needs a pointer at the
// work rather than a census of it. The limit is the memory module's own default (it caps at its
// MaxSearchLimit whatever it is asked for), so the sessions total counts what the index answered
// with, at most that many per project.
func (s *Service) collectSessions(ctx context.Context, project protocol.Project, cards []protocol.Card, words []string, out *ranking[protocol.SessionHit]) error {
	hits, err := s.sessions.SearchSessions(ctx, project.ID, strings.Join(words, " "), 0)
	if gone(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("search: read the past sessions of project %s: %w", project.ID, err)
	}
	byID := cardIndex(cards)
	for _, hit := range hits {
		card, ok := byID[hit.CardID]
		if !ok {
			// The event names a card this project's read did not return, which is a card deleted
			// between the two reads. A hit that cannot be named is skipped rather than shown as a
			// dead link.
			continue
		}
		hit.ProjectID, hit.ProjectName = project.ID, project.Name
		if hit.Key == "" {
			hit.Key, hit.Title = card.Key, card.Title
		}
		if score := sessionScore(card.Title, hit.Excerpt, words); score > 0 {
			out.add(score, hit.At.Time().UnixMilli(), hit)
		}
	}
	return nil
}

// collectNotes matches one project's card notes. Like the sessions above it, the index is read with
// a limit: a note's whole body is in the index, and the palette shows the beginning of it.
func (s *Service) collectNotes(ctx context.Context, project protocol.Project, cards []protocol.Card, words []string, out *ranking[protocol.NoteHit]) error {
	notes, err := s.notes.SearchNotes(ctx, project.ID, strings.Join(words, " "), 0)
	if gone(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("search: read the notes of project %s: %w", project.ID, err)
	}
	byID := cardIndex(cards)
	for _, note := range notes {
		card, ok := byID[note.CardID]
		if !ok {
			continue
		}
		hit := protocol.NoteHit{
			CardID: note.CardID, Key: card.Key, Title: card.Title, Path: note.Path,
			Excerpt: excerpt(note.Body), Author: note.Author, ProjectID: project.ID,
			ProjectName: project.Name,
		}
		if note.UpdatedAt != nil {
			hit.At = *note.UpdatedAt
		}
		if score := noteScore(card.Title, note.Body, words); score > 0 {
			out.add(score, hit.At.Time().UnixMilli(), hit)
		}
	}
	return nil
}

// cardIndex keys a project's cards by id, so a session or a note is named from the read that was
// already made for the cards kind instead of reading each card again.
func cardIndex(cards []protocol.Card) map[string]protocol.Card {
	out := make(map[string]protocol.Card, len(cards))
	for _, card := range cards {
		out[card.ID] = card
	}
	return out
}

// noteExcerptRun is how much of a note a search answer shows. It is the same length the MCP server's
// `search_memory` tool shows (its noteExcerptRun), so a person and an agent reading one search see
// the same amount of the same note.
const noteExcerptRun = 200

// excerpt is the beginning of a note: its first noteExcerptRun bytes, cut at a character boundary.
// The whole note is read at its own route; an answer that carried it would cost more than the
// reading it saves.
func excerpt(body string) string {
	text := strings.TrimSpace(body)
	if len(text) <= noteExcerptRun {
		return text
	}
	cut := noteExcerptRun
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return strings.TrimRight(text[:cut], " ") + "…"
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
