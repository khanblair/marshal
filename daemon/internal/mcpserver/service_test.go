package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// The test fixture: one server over fakes, and the helpers that talk to it.
//
// The tests in this package are in package mcpserver rather than mcpserver_test because the tool
// arguments and answers are unexported types, and an external test could only reach them by
// describing them a second time - which would test the description rather than the code. Every test
// below drives the server over a real in-memory MCP transport, through the same handshake, the same
// tool list, and the same marshalling a real agent uses; a test that reached into the handler
// directly would not cover the schemas the SDK generates, which is where a mistake in an argument
// type shows up.

const (
	// projectID is the fixture's project, and otherProject a second one, for the tools that refuse to
	// reach across projects.
	projectID    = "small-repo"
	otherProject = "other-repo"
)

// fixtureNoteTime is the time the fakes stamp a note with. It is fixed so a test can say what it
// expects the answer's time text to be, and it is UTC to the millisecond like every other time on the
// wire.
var fixtureNoteTime = time.Date(2026, time.September, 27, 9, 30, 0, 0, time.UTC)

// fixture is a server over fake services and the mode a test puts the card in.
type fixture struct {
	cards  *fakeCards
	notes  *fakeNotes
	claims *fakeClaims
	agents *fakeAgents
	code   *fakeCodebase

	// mode is the card's permission mode. It is read from the fixture on every call, so a test can
	// change it between two calls and watch the second one be answered differently.
	mode protocol.PermissionMode
	// noMode makes the permission rules unreadable, which is a session whose row cannot be read.
	noMode bool

	logs   *bytes.Buffer
	server *Server
}

// newFixture builds a server for card 1 of the fixture project, with two other cards to see.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		cards:  newFakeCards(),
		notes:  &fakeNotes{},
		claims: &fakeClaims{held: map[string][]memory.Claim{}},
		agents: &fakeAgents{},
		code:   &fakeCodebase{},
		mode:   protocol.PermissionModeAutoEdits,
		logs:   &bytes.Buffer{},
	}
	f.cards.add(protocol.Card{
		ID: "card-1", ProjectID: projectID, Number: 1, Key: projectID + "#1",
		Title: "Add a health check", Body: "Add a /healthz route and a test for it.\nSecond line.",
		State: protocol.CardStateWorking, Agent: protocol.AgentKindClaude, Role: "Implementer",
		DoingNow: "writing the route",
	})
	f.cards.add(protocol.Card{
		ID: "card-2", ProjectID: projectID, Number: 2, Key: projectID + "#2",
		Title: "Ship the board", Body: "The board should show claims.",
		State: protocol.CardStateWorking, Agent: protocol.AgentKindCodex, Role: "Reviewer",
		DoingNow: "reading the diff",
	})
	f.cards.add(protocol.Card{
		ID: "card-3", ProjectID: projectID, Number: 3, Key: projectID + "#3",
		Title: "Write the docs", State: protocol.CardStateBacklog, Agent: protocol.AgentKindGemini,
	})
	f.cards.add(protocol.Card{
		ID: "card-9", ProjectID: otherProject, Number: 1, Key: otherProject + "#1",
		Title: "Somewhere else", State: protocol.CardStateBacklog, Agent: protocol.AgentKindClaude,
	})
	f.claims.held["card-2"] = []memory.Claim{{
		CardID: "card-2", ProjectID: projectID, PathOrPackage: "src/board.ts",
	}}
	server, err := New(
		Deps{
			Cards: f.cards, Notes: f.notes, Claims: f.claims, Agents: f.agents, Codebase: f.code,
			Harness: f.rules,
			Logger:  slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		},
		Identity{CardID: "card-1", ProjectID: projectID, Role: "Implementer"},
	)
	if err != nil {
		t.Fatalf("build the server: %v", err)
	}
	f.server = server
	return f
}

// rules is the card's permission rules as the session would give them: the mode the fixture is in
// and the profile Marshal ships with.
func (f *fixture) rules() (harness.Config, bool) {
	if f.noMode {
		return harness.Config{}, false
	}
	return harness.Config{Mode: f.mode, Profile: security.DefaultProfile()}, true
}

// client runs the whole handshake and answers a client session talking to the server. It is the path
// an agent takes, so a tool that cannot be described to a client fails here and not in production.
func (f *fixture) client(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx := t.Context()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	session, err := f.server.Connect(ctx, serverTransport)
	if err != nil {
		t.Fatalf("connect the server: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect the client: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// call runs one tool over the transport and answers the text a model would read and whether the call
// came back as an error. An error from CallTool itself fails the test: a refusal is a tool error and
// not a broken connection, and the difference matters (rule 11.4: a refused call comes back
// explained, not silently dropped).
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return textOf(res), res.IsError
}

// callOK runs a tool that is expected to succeed and answers its structured result, decoded into T.
func callOK[T any](t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) T {
	t.Helper()
	text, isError := call(t, cs, name, args)
	if isError {
		t.Fatalf("%s was refused: %s", name, text)
	}
	var out T
	if text != "" {
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("decode the answer of %s (%s): %v", name, text, err)
		}
	}
	return out
}

// callRefused runs a tool that is expected to be refused and answers the sentence it was refused
// with.
func callRefused(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	text, isError := call(t, cs, name, args)
	if !isError {
		t.Fatalf("%s was allowed, and this test is about it being refused: %s", name, text)
	}
	if strings.TrimSpace(text) == "" {
		t.Fatalf("%s was refused with nothing to read", name)
	}
	return text
}

// textOf is the text of a tool result: the JSON of its structured answer, or the refusal sentence.
func textOf(res *mcp.CallToolResult) string {
	var parts []string
	for _, content := range res.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// fakeCards is the projects service as these tools use it: cards in memory, numbered as they are
// created.
type fakeCards struct {
	cards   []protocol.Card
	created []protocol.CreateCardRequest
	updated []protocol.UpdateCardRequest
	next    int
	// deps is what ProjectDependencies answers, keyed by card ID. The real service reads the edge
	// rows it wrote as each card was created; here a test says what the board's edges are.
	deps map[string][]protocol.CardKey
	// depsErr, when set, is what ProjectDependencies fails with, so a test can check what a tool does
	// when the edges cannot be read.
	depsErr error
}

func newFakeCards() *fakeCards { return &fakeCards{next: 100} }

// addDep records an edge for a card, as creating that card with dependsOn would have.
func (f *fakeCards) addDep(cardID string, keys ...protocol.CardKey) {
	if f.deps == nil {
		f.deps = map[string][]protocol.CardKey{}
	}
	f.deps[cardID] = append(f.deps[cardID], keys...)
}

func (f *fakeCards) add(card protocol.Card) { f.cards = append(f.cards, card) }

func (f *fakeCards) Card(_ context.Context, id string) (protocol.Card, error) {
	for _, card := range f.cards {
		if card.ID == id {
			return card, nil
		}
	}
	return protocol.Card{}, protocol.NotFound("card")
}

func (f *fakeCards) CardByKey(_ context.Context, key protocol.CardKey) (protocol.Card, error) {
	for _, card := range f.cards {
		if card.ProjectID == key.ProjectID && card.Number == key.Number {
			return card, nil
		}
	}
	return protocol.Card{}, protocol.NotFound("card")
}

func (f *fakeCards) Cards(_ context.Context, projectID string) ([]protocol.Card, error) {
	var out []protocol.Card
	for _, card := range f.cards {
		if card.ProjectID == projectID {
			out = append(out, card)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

func (f *fakeCards) CreateCard(_ context.Context, projectID string, in protocol.CreateCardRequest, _ ...projects.CardOption) (protocol.Card, error) {
	f.created = append(f.created, in)
	number := f.next
	f.next++
	card := protocol.Card{
		ID: fmt.Sprintf("new-card-%d", number), ProjectID: projectID, Number: number,
		Key:   protocol.CardKey{ProjectID: projectID, Number: number}.String(),
		Title: in.Title, Body: in.Body, Role: in.Role, Package: in.Package,
		State: protocol.CardStateBacklog, Agent: protocol.AgentKindClaude,
	}
	f.add(card)
	return card, nil
}

// ProjectDependencies answers the edges of the cards it holds, filtered to one project, as the real
// service does in one query.
func (f *fakeCards) ProjectDependencies(_ context.Context, projectID string) (map[string][]protocol.CardKey, error) {
	if f.depsErr != nil {
		return nil, f.depsErr
	}
	out := map[string][]protocol.CardKey{}
	for _, card := range f.cards {
		if card.ProjectID != projectID {
			continue
		}
		keys, ok := f.deps[card.ID]
		if !ok {
			continue
		}
		// In number order, as the service answers and as boardCard's DependsOn is documented.
		sorted := append([]protocol.CardKey(nil), keys...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Number < sorted[j].Number })
		out[card.ID] = sorted
	}
	return out, nil
}

func (f *fakeCards) UpdateCard(_ context.Context, id string, in protocol.UpdateCardRequest) (protocol.Card, error) {
	f.updated = append(f.updated, in)
	for i := range f.cards {
		if f.cards[i].ID != id {
			continue
		}
		if in.DoingNow != nil {
			f.cards[i].DoingNow = *in.DoingNow
		}
		return f.cards[i], nil
	}
	return protocol.Card{}, protocol.NotFound("card")
}

// fakeNotes is the memory module's notes and lessons as these tools use them.
type fakeNotes struct {
	saved       map[string]string
	authors     map[string]protocol.NoteAuthor
	index       []protocol.Note
	lessonIndex []protocol.Lesson
	queries     []string
	lessonQuery []string
}

func (f *fakeNotes) Note(_ context.Context, cardID string) (protocol.Note, error) {
	if body, ok := f.saved[cardID]; ok {
		return protocol.NewNote(cardID, projectID, notePathOf(cardID), body,
			f.authors[cardID], fixtureNoteTime), nil
	}
	return protocol.NewNote(cardID, projectID, notePathOf(cardID),
		"A note Marshal would start for "+cardID, protocol.NoteAuthorPerson, time.Time{}), nil
}

func (f *fakeNotes) SaveNote(_ context.Context, cardID, body string, author protocol.NoteAuthor) (protocol.Note, error) {
	if f.saved == nil {
		f.saved, f.authors = map[string]string{}, map[string]protocol.NoteAuthor{}
	}
	f.saved[cardID], f.authors[cardID] = body, author
	return f.Note(context.Background(), cardID)
}

func (f *fakeNotes) SearchNotes(_ context.Context, projectID, query string, limit int) ([]protocol.Note, error) {
	f.queries = append(f.queries, query)
	var out []protocol.Note
	for _, note := range f.index {
		if !strings.Contains(note.Body, query) {
			continue
		}
		out = append(out, note)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeNotes) SearchLessons(_ context.Context, projectID, query string, limit int) ([]protocol.Lesson, error) {
	f.lessonQuery = append(f.lessonQuery, query)
	var out []protocol.Lesson
	for _, lesson := range f.lessonIndex {
		if !strings.Contains(lesson.Body, query) {
			continue
		}
		out = append(out, lesson)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// notePathOf is the vault path the fake answers with. It is the shape the real vault uses, so a test
// that reads a path is reading what a person would.
func notePathOf(cardID string) string {
	return projectID + "/cards/" + cardID + ".md"
}

// fakeClaims is the memory module's file claims as these tools use them.
type fakeClaims struct {
	held      map[string][]memory.Claim
	conflicts []memory.Conflict
	released  [][]string
	// err is what the memory module answers when it refuses a claim itself, such as a path that
	// climbs out of the repository. It is how a test checks that the refusal reaches the model.
	err error
}

func (f *fakeClaims) Claim(_ context.Context, cardID string, paths []string) (memory.ClaimResult, error) {
	if f.err != nil {
		return memory.ClaimResult{}, f.err
	}
	result := memory.ClaimResult{Conflicts: f.conflicts}
	for _, path := range paths {
		claim := memory.Claim{
			CardID: cardID, ProjectID: projectID, PathOrPackage: path, ClaimedAt: fixtureNoteTime,
		}
		known := false
		for _, existing := range f.held[cardID] {
			if existing.PathOrPackage == path {
				known = true
			}
		}
		if !known {
			f.held[cardID] = append(f.held[cardID], claim)
		}
		result.Claims = append(result.Claims, claim)
	}
	return result, nil
}

func (f *fakeClaims) Release(_ context.Context, cardID string, paths []string) error {
	f.released = append(f.released, paths)
	drop := make(map[string]bool, len(paths))
	for _, path := range paths {
		drop[path] = true
	}
	kept := make([]memory.Claim, 0, len(f.held[cardID]))
	for _, claim := range f.held[cardID] {
		if !drop[claim.PathOrPackage] {
			kept = append(kept, claim)
		}
	}
	f.held[cardID] = kept
	return nil
}

func (f *fakeClaims) Claims(_ context.Context, cardID string) ([]memory.Claim, error) {
	return f.held[cardID], nil
}

func (f *fakeClaims) ProjectClaims(_ context.Context, _ string) ([]memory.Claim, error) {
	var out []memory.Claim
	for _, claims := range f.held {
		out = append(out, claims...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PathOrPackage < out[j].PathOrPackage })
	return out, nil
}

// fakeAgents is the session manager's one method as ask_agent uses it.
type fakeAgents struct {
	sent []sentQuestion
	err  error
}

type sentQuestion struct {
	cardID string
	text   string
}

func (f *fakeAgents) Send(_ context.Context, cardID, text string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, sentQuestion{cardID: cardID, text: text})
	return nil
}

// fakeCodebase is the codebase map as search_codebase uses it.
type fakeCodebase struct {
	matches []CodeMatch
	err     error
	queried []string
	// notice stands in for a machine with no universal ctags, where the map answers from file names
	// alone and says so.
	notice string
}

// Notice answers what the map says about itself. A real map says nothing when it can read symbols.
func (f *fakeCodebase) Notice() string { return f.notice }

func (f *fakeCodebase) Search(_ context.Context, _, query string, limit int) ([]CodeMatch, error) {
	f.queried = append(f.queried, query)
	if f.err != nil {
		return nil, f.err
	}
	out := f.matches
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
