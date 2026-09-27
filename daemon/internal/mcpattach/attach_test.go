package mcpattach

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/harness"
	"github.com/khanblair/marshal/daemon/internal/mcpserver"
	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
	"github.com/khanblair/marshal/daemon/internal/security"
)

// The attacher is the module the session manager names: it builds a card's server, hosts it, and
// answers the two things a session is given (docs/architecture.md sections 7 and 11.4). These tests
// drive it over fakes, so what a card's agent is told and what reaches the host are both checked.

// ---------------------------------------------------------------- fakes

type fakeCards struct {
	byID    map[string]protocol.Card
	inBoard []protocol.Card
}

func (f *fakeCards) Card(_ context.Context, id string) (protocol.Card, error) {
	card, ok := f.byID[id]
	if !ok {
		return protocol.Card{}, errors.New("no such card")
	}
	return card, nil
}

func (f *fakeCards) CardByKey(_ context.Context, key protocol.CardKey) (protocol.Card, error) {
	return protocol.Card{}, errors.New("not used")
}

func (f *fakeCards) Cards(_ context.Context, projectID string) ([]protocol.Card, error) {
	var out []protocol.Card
	for _, c := range f.inBoard {
		if c.ProjectID == projectID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeCards) CreateCard(_ context.Context, _ string, _ protocol.CreateCardRequest, _ ...projects.CardOption) (protocol.Card, error) {
	return protocol.Card{}, errors.New("not used")
}

func (f *fakeCards) ProjectDependencies(context.Context, string) (map[string][]protocol.CardKey, error) {
	return nil, errors.New("not used")
}

func (f *fakeCards) UpdateCard(_ context.Context, _ string, _ protocol.UpdateCardRequest) (protocol.Card, error) {
	return protocol.Card{}, errors.New("not used")
}

type fakeNotes struct{}

func (fakeNotes) Note(context.Context, string) (protocol.Note, error) { return protocol.Note{}, nil }
func (fakeNotes) SaveNote(context.Context, string, string, protocol.NoteAuthor) (protocol.Note, error) {
	return protocol.Note{}, nil
}
func (fakeNotes) SearchNotes(context.Context, string, string, int) ([]protocol.Note, error) {
	return nil, nil
}
func (fakeNotes) SearchLessons(context.Context, string, string, int) ([]protocol.Lesson, error) {
	return nil, nil
}

// fakeClaims is what the awareness summary reads a project's holds from. Its zero value holds
// nothing, which is what most tests want.
type fakeClaims struct {
	// project is what ProjectClaims answers, grouped by project, in the order given.
	project map[string][]memory.Claim
	// err, when set, is what ProjectClaims fails with, for the summary that must survive an
	// unreadable memory module.
	err error
}

func (fakeClaims) Claim(context.Context, string, []string) (memory.ClaimResult, error) {
	return memory.ClaimResult{}, nil
}
func (fakeClaims) Release(context.Context, string, []string) error        { return nil }
func (fakeClaims) Claims(context.Context, string) ([]memory.Claim, error) { return nil, nil }
func (f fakeClaims) ProjectClaims(_ context.Context, projectID string) ([]memory.Claim, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.project[projectID], nil
}

type fakeAgents struct{}

func (fakeAgents) Send(context.Context, string, string) error { return nil }

type fakeRoles struct{ instr string }

func (f fakeRoles) Role(context.Context, string, string) (protocol.Role, error) {
	if f.instr == "" {
		return protocol.Role{}, errors.New("no such role")
	}
	return protocol.Role{Name: "Implementer", Spec: protocol.RoleSpec{Instr: f.instr}}, nil
}

// ---------------------------------------------------------------- fixture

const (
	testCardID    = "card-1"
	testProjectID = "small-repo"
)

// fixtureCard is the card under test, and fixtureOther a second card the awareness summary should
// name.
func fixtureCard() protocol.Card {
	return protocol.Card{
		ID: testCardID, ProjectID: testProjectID, Number: 1, Key: testProjectID + "#1",
		Title: "Add a health check", Body: "Add a /healthz route.",
		State: protocol.CardStateWorking, Role: "Implementer",
	}
}

func fixtureOther() protocol.Card {
	return protocol.Card{
		ID: "card-2", ProjectID: testProjectID, Number: 2, Key: testProjectID + "#2",
		Title: "Ship the board", State: protocol.CardStateBacklog, DoingNow: "reading the diff",
	}
}

// newAttacher builds an attacher over fakes, with a resolvable command and address.
func newAttacher(t *testing.T, host *mcpserver.Host, roles Roles) *Attacher {
	t.Helper()
	card := fixtureCard()
	other := fixtureOther()
	a, err := New(Deps{
		Host: host,
		Cards: &fakeCards{
			byID:    map[string]protocol.Card{card.ID: card, other.ID: other},
			inBoard: []protocol.Card{card, other},
		},
		Notes: fakeNotes{}, Claims: fakeClaims{}, Agents: fakeAgents{}, Roles: roles,
		Harness: func(string) (harness.Config, bool) {
			return harness.Config{Mode: protocol.PermissionModeAutoEdits, Profile: security.DefaultProfile()}, true
		},
		Command: "/usr/local/bin/marshald", Address: "127.0.0.1:47800",
	})
	if err != nil {
		t.Fatalf("build the attacher: %v", err)
	}
	return a
}

// ---------------------------------------------------------------- tests

func TestAttachGivesTheSessionTheServerAndTheContextInOrder(t *testing.T) {
	host := mcpserver.NewHost()
	a := newAttacher(t, host, fakeRoles{instr: "You are the Implementer. Keep the diff small."})
	t.Cleanup(func() { a.Detach(context.Background(), testCardID) })

	got, err := a.Attach(context.Background(), fixtureCard())
	if err != nil {
		t.Fatalf("attach: %v", err)
	}

	// The server the agent is given: the daemon's own program in its `mcp` mode, told the card.
	if len(got.Servers) != 1 {
		t.Fatalf("the session was given %d servers, want one", len(got.Servers))
	}
	server := got.Servers[0]
	if server.Name != "marshal" || server.Command != "/usr/local/bin/marshald" {
		t.Errorf("the server is %+v", server)
	}
	wantArgs := []string{"mcp", "--address", "127.0.0.1:47800", "--card", testCardID}
	if strings.Join(server.Args, " ") != strings.Join(wantArgs, " ") {
		t.Errorf("the server's args are %q, want %q", server.Args, wantArgs)
	}
	// The secret is the one the host minted, in the environment and not in the command line.
	if len(server.Env) != 1 || !strings.HasPrefix(server.Env[0], TokenEnv+"=") {
		t.Fatalf("the server's env is %q, want one %s entry", server.Env, TokenEnv)
	}
	if !host.Has(testCardID) {
		t.Fatal("the card's server was not hosted, so the command the agent runs reaches nothing")
	}
	// The secret in the environment is the one the host accepts: a client using it gets through.
	if !reachesHost(t, host, server.Env[0]) {
		t.Error("the secret in the environment is not the one the host checks")
	}

	// The context, in the order section 7 gives it: role, then the card's task, then the board, then
	// the tools.
	text := got.Instructions
	for _, want := range []string{"You are the Implementer", "#1", "Add a health check", "Ship the board", "board_status"} {
		if !strings.Contains(text, want) {
			t.Errorf("the context does not mention %q:\n%s", want, text)
		}
	}
	if !inOrder(text, "You are the Implementer", "Your card is", "Other cards", "tools for this card") {
		t.Errorf("the context is not in the order section 7 gives:\n%s", text)
	}
}

// reachesHost says whether the secret the session was given is the one the host accepts, by
// connecting to the hosted card over a real in-process HTTP server the way `marshald mcp` does.
func reachesHost(t *testing.T, host *mcpserver.Host, env string) bool {
	t.Helper()
	secret, ok := strings.CutPrefix(env, TokenEnv+"=")
	if !ok {
		return false
	}
	srv := httptest.NewServer(host)
	t.Cleanup(srv.Close)
	transport := &mcp.StreamableClientTransport{
		Endpoint:             srv.URL + mcpserver.PathPrefix + testCardID,
		DisableStandaloneSSE: true,
		HTTPClient:           &http.Client{Transport: secretTransport{secret: secret}},
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(t.Context(), transport, nil)
	if err != nil {
		return false
	}
	_ = cs.Close()
	return true
}

// secretTransport adds the card's secret to every request.
type secretTransport struct{ secret string }

func (s secretTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+s.secret)
	return http.DefaultTransport.RoundTrip(clone)
}

func TestDetachTakesTheServerAway(t *testing.T) {
	host := mcpserver.NewHost()
	a := newAttacher(t, host, fakeRoles{})
	if _, err := a.Attach(context.Background(), fixtureCard()); err != nil {
		t.Fatalf("attach: %v", err)
	}
	a.Detach(context.Background(), testCardID)
	if host.Has(testCardID) {
		t.Error("the card is still hosted after Detach")
	}
	// A detach for a card that never attached is harmless.
	a.Detach(context.Background(), "never-attached")
}

func TestAttachWithoutACommandGivesTheContextAndNoServer(t *testing.T) {
	host := mcpserver.NewHost()
	a, err := New(Deps{
		Host: host, Cards: &fakeCards{byID: map[string]protocol.Card{}},
		Notes: fakeNotes{}, Claims: fakeClaims{}, Agents: fakeAgents{},
		Harness: func(string) (harness.Config, bool) { return harness.Config{}, false },
	})
	if err != nil {
		t.Fatalf("build the attacher: %v", err)
	}
	got, err := a.Attach(context.Background(), fixtureCard())
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if len(got.Servers) != 0 {
		t.Errorf("a session with nowhere to reach was given %+v", got.Servers)
	}
	if !strings.Contains(got.Instructions, "Add a health check") {
		t.Errorf("the context is missing the card's task: %q", got.Instructions)
	}
	if host.Len() != 0 {
		t.Error("a server was hosted with no command to reach it")
	}
}

func TestNewRequiresItsServices(t *testing.T) {
	cases := map[string]Deps{
		"no host":   {Cards: &fakeCards{}, Notes: fakeNotes{}, Claims: fakeClaims{}, Agents: fakeAgents{}, Harness: okRules},
		"no cards":  {Host: mcpserver.NewHost(), Notes: fakeNotes{}, Claims: fakeClaims{}, Agents: fakeAgents{}, Harness: okRules},
		"no notes":  {Host: mcpserver.NewHost(), Cards: &fakeCards{}, Claims: fakeClaims{}, Agents: fakeAgents{}, Harness: okRules},
		"no claims": {Host: mcpserver.NewHost(), Cards: &fakeCards{}, Notes: fakeNotes{}, Agents: fakeAgents{}, Harness: okRules},
		"no agents": {Host: mcpserver.NewHost(), Cards: &fakeCards{}, Notes: fakeNotes{}, Claims: fakeClaims{}, Harness: okRules},
		"no rules":  {Host: mcpserver.NewHost(), Cards: &fakeCards{}, Notes: fakeNotes{}, Claims: fakeClaims{}, Agents: fakeAgents{}},
	}
	for name, deps := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(deps); err == nil {
				t.Errorf("New(%s) built an attacher", name)
			}
		})
	}
}

func okRules(string) (harness.Config, bool) { return harness.Config{}, false }

// inOrder says whether every needle appears, and in the order given.
func inOrder(haystack string, needles ...string) bool {
	at := 0
	for _, needle := range needles {
		i := strings.Index(haystack[at:], needle)
		if i < 0 {
			return false
		}
		at += i + len(needle)
	}
	return true
}
