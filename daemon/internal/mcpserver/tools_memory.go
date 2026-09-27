package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// post_note, read_notes, search_memory, and search_codebase: how an agent leaves something for the
// next reader and finds what earlier work left (docs/architecture.md sections 11.2, 11.4, and 12;
// docs/backend-checklist.md B7.4 and B7.5; build-plan tasks 7.1, 7.12, and 7.10).
//
// A note is inside Marshal what it is outside it: the markdown file the vault keeps for one card, at
// `<vault>/<project>/cards/<number>-<title>.md`, which opens in Obsidian and which the Notes tab
// reads and writes. So post_note writes the same note a person does, and read_notes reads it - there
// is one note per card and not a second, private one for agents.

// noteExcerptRun is how much of a note a search answer shows. The note itself is read with
// read_notes; a search answer that carried whole notes would cost more than the reading it saves.
const noteExcerptRun = 200

// codeMatchLimits bound what search_codebase asks the map for: enough to answer "where is this", and
// few enough to stay a pointer at the file rather than a copy of it.
const (
	codeMatchDefault = 20
	codeMatchMax     = 50
)

// noteOut is a card's note as these tools answer it. It is this package's own shape rather than the
// wire type, because what a model reads is a path, a body, and a time, and the time is clearer as
// text than as anything structured.
type noteOut struct {
	// CardKey is the key of the card the note belongs to, such as "small-repo#7", so a note found
	// by a search can be asked for by name.
	CardKey string `json:"cardKey"`
	// Path is where the note lives, relative to the vault root, in the shape
	// "<project>/cards/<number>-<title>.md". It is what a person opens in Obsidian.
	Path string `json:"path"`
	// Body is the whole note, in markdown.
	Body string `json:"body"`
	// Author is who last wrote it: "person" or "agent".
	Author protocol.NoteAuthor `json:"author"`
	// UpdatedAt is when it was last saved, in RFC 3339. Empty when nothing has been saved for the
	// card yet and the body is the note Marshal would start one from.
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// postNoteInput is post_note's arguments.
type postNoteInput struct {
	// Body is the whole note in markdown. It replaces what is there rather than adding to it, which
	// is what one file per card means; read_notes first when the note is to be added to.
	Body string `json:"body"`
}

// post_note writes this card's note.
func (s *Server) postNote(ctx context.Context, _ *mcp.CallToolRequest, in postNoteInput) (*mcp.CallToolResult, noteOut, error) {
	if err := s.allow("post_note", kindWrite); err != nil {
		return nil, noteOut{}, err
	}
	note, err := s.deps.Notes.SaveNote(ctx, s.identity.CardID, in.Body, protocol.NoteAuthorAgent)
	if err != nil {
		return nil, noteOut{}, fmt.Errorf("write the note: %w", err)
	}
	return s.noteAnswer(ctx, note)
}

// readNotesInput is read_notes's arguments.
type readNotesInput struct {
	// CardKey is the card whose note to read, in the form another card is named by
	// ("small-repo#7"). Empty reads this card's own note.
	CardKey string `json:"cardKey,omitempty"`
}

// read_notes reads a card's note: another card's, or this one's own.
func (s *Server) readNotes(ctx context.Context, _ *mcp.CallToolRequest, in readNotesInput) (*mcp.CallToolResult, noteOut, error) {
	if err := s.allow("read_notes", kindRead); err != nil {
		return nil, noteOut{}, err
	}
	cardID := s.identity.CardID
	if key := strings.TrimSpace(in.CardKey); key != "" {
		parsed, err := protocol.ParseCardKey(key)
		if err != nil {
			return nil, noteOut{}, fmt.Errorf("read_notes: %w", err)
		}
		other, err := s.deps.Cards.CardByKey(ctx, parsed)
		if err != nil {
			return nil, noteOut{}, fmt.Errorf("read the card whose note was asked for: %w", err)
		}
		if other.ProjectID != s.identity.ProjectID {
			return nil, noteOut{}, fmt.Errorf("card %s is in another project, and one card may not read across projects", other.Key)
		}
		cardID = other.ID
	}
	note, err := s.deps.Notes.Note(ctx, cardID)
	if err != nil {
		return nil, noteOut{}, fmt.Errorf("read the note: %w", err)
	}
	return s.noteAnswer(ctx, note)
}

// noteAnswer names the note's card and turns its save time into text. The card is read rather than
// carried on the note because a note on the wire names its card by id, and a key is what an agent can
// say out loud.
func (s *Server) noteAnswer(ctx context.Context, note protocol.Note) (*mcp.CallToolResult, noteOut, error) {
	card, err := s.deps.Cards.Card(ctx, note.CardID)
	if err != nil {
		return nil, noteOut{}, fmt.Errorf("read the card of the note: %w", err)
	}
	return nil, noteOut{
		CardKey: card.Key, Path: note.Path, Body: note.Body, Author: note.Author,
		UpdatedAt: timeText(note.UpdatedAt),
	}, nil
}

// timeText writes a wire time as RFC 3339, or as nothing at all when there is no time. It is the one
// place a nil time becomes an empty string, which is what "nothing has been saved" means here.
func timeText(t *protocol.Timestamp) string {
	if t == nil {
		return ""
	}
	return t.Time().UTC().Format(time.RFC3339)
}

// searchMemoryInput is search_memory's arguments.
type searchMemoryInput struct {
	// Query is what to look for. Words are matched as prefixes, best match first.
	Query string `json:"query"`
	// Limit is the most notes to answer with. Zero means the daemon's default.
	Limit int `json:"limit,omitempty"`
}

// memoryHit is one note a search found, shown as an excerpt.
type memoryHit struct {
	// CardKey is the card that left the note.
	CardKey string `json:"cardKey"`
	// Path is where the note lives in the vault.
	Path string `json:"path"`
	// Excerpt is the beginning of the note, cut short. Read the whole note with read_notes.
	Excerpt string `json:"excerpt"`
	// Author is who wrote it: "person" or "agent".
	Author protocol.NoteAuthor `json:"author"`
	// UpdatedAt is when it was last saved, in RFC 3339.
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// lessonHit is one lesson a search found, shown as an excerpt, the lesson-shaped sibling of
// memoryHit. It carries a title rather than a card key, because a lesson belongs to the project and
// not to any one card.
type lessonHit struct {
	// Title is the lesson's heading.
	Title string `json:"title"`
	// Path is where the lesson lives in the vault.
	Path string `json:"path"`
	// Excerpt is the beginning of the lesson, cut short.
	Excerpt string `json:"excerpt"`
	// Author is who wrote it: "person" or "agent".
	Author protocol.NoteAuthor `json:"author"`
	// UpdatedAt is when it was last saved, in RFC 3339.
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// searchMemoryOut is search_memory's answer.
type searchMemoryOut struct {
	// Query is what was searched for.
	Query string `json:"query"`
	// Notes are the card notes that matched, best match first. Never null.
	Notes []memoryHit `json:"notes"`
	// Lessons are the lessons that matched, best match first. Never null.
	Lessons []lessonHit `json:"lessons"`
	// Searched is what was looked in: the cards' notes and the lessons of this project. The
	// summaries of past sessions join it when those are indexed for an agent's own search.
	Searched string `json:"searched"`
}

// search_memory searches what the cards of this project have left in their notes, and the lessons
// the project has learned.
func (s *Server) searchMemory(ctx context.Context, _ *mcp.CallToolRequest, in searchMemoryInput) (*mcp.CallToolResult, searchMemoryOut, error) {
	if err := s.allow("search_memory", kindRead); err != nil {
		return nil, searchMemoryOut{}, err
	}
	query := strings.TrimSpace(in.Query)
	out := searchMemoryOut{
		Query: query, Notes: []memoryHit{}, Lessons: []lessonHit{},
		Searched: "the notes the cards of this project have left, and the project's lessons",
	}
	if query == "" {
		return nil, out, nil
	}
	notes, err := s.deps.Notes.SearchNotes(ctx, s.identity.ProjectID, query, in.Limit)
	if err != nil {
		return nil, searchMemoryOut{}, fmt.Errorf("search the notes: %w", err)
	}
	for _, note := range notes {
		card, err := s.deps.Cards.Card(ctx, note.CardID)
		if err != nil {
			// A note whose card cannot be read cannot be named, and a nameless hit is not worth
			// failing the search over. The note rows are keyed to their card, so this is a card
			// deleted underneath a search.
			continue
		}
		out.Notes = append(out.Notes, memoryHit{
			CardKey: card.Key, Path: note.Path, Excerpt: excerpt(note.Body, noteExcerptRun),
			Author: note.Author, UpdatedAt: timeText(note.UpdatedAt),
		})
	}
	lessons, err := s.deps.Notes.SearchLessons(ctx, s.identity.ProjectID, query, in.Limit)
	if err != nil {
		return nil, searchMemoryOut{}, fmt.Errorf("search the lessons: %w", err)
	}
	for _, lesson := range lessons {
		out.Lessons = append(out.Lessons, lessonHit{
			Title: lesson.Title, Path: lesson.Path, Excerpt: excerpt(lesson.Body, noteExcerptRun),
			Author: lesson.Author, UpdatedAt: timeText(&lesson.UpdatedAt),
		})
	}
	return nil, out, nil
}

// searchCodebaseInput is search_codebase's arguments.
type searchCodebaseInput struct {
	// Query is the name to look for: a function, a type, a method.
	Query string `json:"query"`
	// Limit is the most matches to answer with. Zero means the daemon's default.
	Limit int `json:"limit,omitempty"`
}

// searchCodebaseOut is search_codebase's answer.
type searchCodebaseOut struct {
	// Query is what was looked for.
	Query string `json:"query"`
	// Matches are the places the name is written, most relevant first. Never null.
	Matches []CodeMatch `json:"matches"`
	// Notice is what the map has to say about itself when it cannot read symbols - universal ctags
	// not being installed - and answered from file names alone. Empty is the ordinary answer, and it
	// is worth saying: this is the difference between "that name is not here" and "this machine
	// cannot see names".
	Notice string `json:"notice,omitempty"`
}

// search_codebase asks the codebase map where a name is, so an agent can go to the file instead of
// reading the tree to find it.
func (s *Server) searchCodebase(ctx context.Context, _ *mcp.CallToolRequest, in searchCodebaseInput) (*mcp.CallToolResult, searchCodebaseOut, error) {
	if err := s.allow("search_codebase", kindRead); err != nil {
		return nil, searchCodebaseOut{}, err
	}
	query := strings.TrimSpace(in.Query)
	out := searchCodebaseOut{Query: query, Matches: []CodeMatch{}}
	if query == "" {
		return nil, out, nil
	}
	if s.deps.Codebase == nil {
		// A server built without a map (the daemon gives every session the real one). The agent is
		// told plainly rather than being answered with no matches, which would read as "that name is
		// not in this project".
		return nil, searchCodebaseOut{}, notBuilt(noCodebase)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = codeMatchDefault
	}
	if limit > codeMatchMax {
		limit = codeMatchMax
	}
	matches, err := s.deps.Codebase.Search(ctx, s.identity.ProjectID, query, limit)
	if err != nil {
		return nil, searchCodebaseOut{}, fmt.Errorf("search the codebase map: %w", err)
	}
	if matches != nil {
		out.Matches = matches
	}
	// The map says so itself when it is answering from file names alone, so the agent reads the
	// difference between an empty answer and a blind one.
	out.Notice = s.deps.Codebase.Notice()
	return nil, out, nil
}
