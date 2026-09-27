package mcpserver

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/khanblair/marshal/daemon/internal/memory"
	"github.com/khanblair/marshal/daemon/internal/projects"
	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// board_status, report_progress, create_card, and ask_agent: the tools that let a card see the rest
// of the board and say what it is doing (docs/architecture.md section 11.4; docs/backend-checklist.md
// B7.1 and B7.2; build-plan tasks 7.1 and 7.2).
//
// The awareness a card gets at the start of a turn is a summary written into its instructions (the
// context order of section 7), and board_status is where the detail behind that summary is asked for.
// The two are built from the same reading of the board - boardCardOf and boardSummary below - so the
// summary and the answer cannot describe the board differently.

// goalRun of a card's description is how much of it board_status shows. A card's body is where the
// task is written and it can be long; the summary exists to say what a card is for, and an agent that
// needs the whole thing reads the card. It is measured in runes so a cut never splits a character.
const goalRun = 160

// boardStatusInput is board_status's arguments: there are none. The tool answers about the card's own
// project, which the server already knows, so an argument could only be a way to ask about someone
// else's board.
type boardStatusInput struct{}

// boardStatusOut is board_status's answer.
type boardStatusOut struct {
	// ProjectID is the project the summary is about.
	ProjectID string `json:"projectId"`
	// Summary is the one-line count: how many other cards there are and how they are spread across
	// the board. It is what an agent repeats to itself when it wants to know how busy the project is.
	Summary string `json:"summary"`
	// Cards are the other cards, in number order. Never null.
	Cards []boardCard `json:"cards"`
	// Truncated is true when there were more cards than the answer carries, and the list is the
	// first boardCardLimit of them.
	Truncated bool `json:"truncated,omitempty"`
}

// boardCard is one other card as the board summary describes it.
type boardCard struct {
	// Key is the card's key, such as "small-repo#12", which is how one card names another.
	Key string `json:"key"`
	// Title is the card's name.
	Title string `json:"title"`
	// State is where the card is on the board.
	State protocol.CardState `json:"state"`
	// Owner is whose work it is: the role's name when the card has one, else the agent program. Solo
	// use has one person, so naming the person every time would say nothing; this is what tells one
	// card's work from another's.
	Owner string `json:"owner"`
	// Goal is the first part of the card's description, capped at goalRun runes. Empty when the card
	// has no description.
	Goal string `json:"goal,omitempty"`
	// DoingNow is the card's own "doing now" line, when it has one.
	DoingNow string `json:"doingNow,omitempty"`
	// Claims are the files or packages the card has claimed, so an agent can see what is already
	// being touched before it starts on the same thing. Omitted when the card has claimed nothing.
	Claims []string `json:"claims,omitempty"`
	// DependsOn are the keys of the cards this one waits for, such as "small-repo#12", in number
	// order. It is the plan the Orchestrator wrote, read back so the next agent can see what has to
	// come first (migration 0020, build-plan task 7.4). Omitted when the card waits for nothing.
	DependsOn []string `json:"dependsOn,omitempty"`
}

// board_status answers what the other cards in this project are doing.
func (s *Server) boardStatus(ctx context.Context, _ *mcp.CallToolRequest, _ boardStatusInput) (*mcp.CallToolResult, boardStatusOut, error) {
	if err := s.allow("board_status", kindRead); err != nil {
		return nil, boardStatusOut{}, err
	}
	cards, err := s.deps.Cards.Cards(ctx, s.identity.ProjectID)
	if err != nil {
		return nil, boardStatusOut{}, fmt.Errorf("read the board: %w", err)
	}
	claims, err := s.deps.Claims.ProjectClaims(ctx, s.identity.ProjectID)
	if err != nil {
		return nil, boardStatusOut{}, fmt.Errorf("read the board's claims: %w", err)
	}
	byCard := claimsByCard(claims)
	// The plan the Orchestrator wrote: which cards wait for which (migration 0020, task 7.4). It is
	// one read for the whole board, and a board whose dependencies cannot be read is a board without
	// them rather than a failed call: what a card is doing matters more than the order it is in.
	deps, err := s.deps.Cards.ProjectDependencies(ctx, s.identity.ProjectID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("could not read the board's dependencies",
				"project_id", s.identity.ProjectID, "error", err)
		}
		deps = nil
	}
	out := boardStatusOut{ProjectID: s.identity.ProjectID, Cards: []boardCard{}}
	other := 0
	for _, card := range cards {
		if card.ID == s.identity.CardID {
			// The agent's own card is not news to it, and section 11.4 asks about "other cards".
			continue
		}
		other++
		if len(out.Cards) >= boardCardLimit {
			out.Truncated = true
			continue
		}
		out.Cards = append(out.Cards, boardCardOf(card, byCard[card.ID], deps[card.ID]))
	}
	out.Summary = boardSummary(other, out.Cards, out.Truncated)
	return nil, out, nil
}

// boardCardOf describes one card from the card, its claims, and the cards it waits for.
func boardCardOf(card protocol.Card, claims []memory.Claim, deps []protocol.CardKey) boardCard {
	out := boardCard{
		Key: card.Key, Title: card.Title, State: card.State,
		Owner: ownerOf(card), Goal: firstRun(card.Body, goalRun), DoingNow: card.DoingNow,
	}
	if len(claims) > 0 {
		out.Claims = make([]string, 0, len(claims))
		for _, claim := range claims {
			out.Claims = append(out.Claims, claim.PathOrPackage)
		}
	}
	if len(deps) > 0 {
		out.DependsOn = make([]string, 0, len(deps))
		for _, dep := range deps {
			out.DependsOn = append(out.DependsOn, dep.String())
		}
	}
	return out
}

// ownerOf names whose work a card is: the role it runs as, else the agent program it runs on.
func ownerOf(card protocol.Card) string {
	if role := strings.TrimSpace(card.Role); role != "" {
		return role
	}
	return string(card.Agent)
}

// boardSummary counts the other cards by state, in board order, and says when the list was cut. An
// empty board gets its own sentence rather than a line of zeroes.
func boardSummary(other int, shown []boardCard, truncated bool) string {
	if other == 0 {
		return "No other cards are on this project's board."
	}
	counts := make(map[protocol.CardState]int, len(protocol.CardStateValues()))
	for _, card := range shown {
		counts[card.State]++
	}
	parts := make([]string, 0, len(counts))
	for _, state := range protocol.CardStateValues() {
		if n := counts[state]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, state))
		}
	}
	line := fmt.Sprintf("%d other cards on this project's board: %s.", other, strings.Join(parts, ", "))
	if truncated {
		line += fmt.Sprintf(" The list below is the first %d.", len(shown))
	}
	return line
}

// claimsByCard groups a project's claims by the card holding them.
func claimsByCard(claims []memory.Claim) map[string][]memory.Claim {
	out := make(map[string][]memory.Claim, len(claims))
	for _, claim := range claims {
		out[claim.CardID] = append(out[claim.CardID], claim)
	}
	return out
}

// reportProgressInput is report_progress's arguments.
type reportProgressInput struct {
	// DoingNow is the line the board shows for this card, such as "writing the migration". An empty
	// line clears it, which is what a card that has stopped says.
	DoingNow string `json:"doingNow"`
}

// reportProgressOut is report_progress's answer.
type reportProgressOut struct {
	// Key is the card's key, so an agent can refer to itself the way another card would.
	Key string `json:"key"`
	// DoingNow is the line as it now stands, which is what was asked for.
	DoingNow string `json:"doingNow"`
}

// report_progress sets the card's one-line "doing now".
func (s *Server) reportProgress(ctx context.Context, _ *mcp.CallToolRequest, in reportProgressInput) (*mcp.CallToolResult, reportProgressOut, error) {
	if err := s.allow("report_progress", kindWrite); err != nil {
		return nil, reportProgressOut{}, err
	}
	card, err := s.deps.Cards.UpdateCard(ctx, s.identity.CardID, protocol.UpdateCardRequest{DoingNow: &in.DoingNow})
	if err != nil {
		return nil, reportProgressOut{}, fmt.Errorf("report progress: %w", err)
	}
	return nil, reportProgressOut{Key: card.Key, DoingNow: card.DoingNow}, nil
}

// createCardInput is create_card's arguments. It is deliberately small: a card an agent proposes is a
// name and a description, and the choices a person makes about a card - which agent, which model,
// which mode - are theirs. The card starts in the backlog and is not started, so proposing one costs
// nothing and starting it stays a person's decision.
type createCardInput struct {
	// Title is the card's name. It cannot be empty.
	Title string `json:"title"`
	// Body is the card's description: what the work is and what done means.
	Body string `json:"body,omitempty"`
	// Role is the role the card should run as, when the project has roles. Empty means none.
	Role string `json:"role,omitempty"`
	// Package is the monorepo package the card belongs to, when the project is one. Empty means none.
	Package string `json:"package,omitempty"`
	// DependsOn are the cards this one waits for, named by key ("small-repo#12") the way another card
	// is named everywhere else. It is how a plan becomes a set of linked cards: create the first,
	// read the key it answers with, and name it here on the next one (migration 0020, task 7.4). A
	// key that names no card in this project, or names this card's own project's cards wrongly,
	// refuses the whole call, so a plan is created whole or not at all.
	DependsOn []string `json:"dependsOn,omitempty"`
}

// createCardOut is create_card's answer.
type createCardOut struct {
	// ID is the new card's id.
	ID string `json:"id"`
	// Key is the new card's key, such as "small-repo#13".
	Key string `json:"key"`
	// Title is the card's name as it was made.
	Title string `json:"title"`
	// State is where the card is: always the backlog, because a proposed card is not started.
	State protocol.CardState `json:"state"`
	// DependsOn are the cards it was made to wait for, in number order, so the agent that is building
	// a plan can confirm the edge it just asked for.
	DependsOn []string `json:"dependsOn,omitempty"`
}

// create_card proposes a new card in this project.
func (s *Server) createCard(ctx context.Context, _ *mcp.CallToolRequest, in createCardInput) (*mcp.CallToolResult, createCardOut, error) {
	if err := s.allow("create_card", kindCreate); err != nil {
		return nil, createCardOut{}, err
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, createCardOut{}, fmt.Errorf("a new card needs a title")
	}
	keys, err := dependsOnKeys(in.DependsOn)
	if err != nil {
		return nil, createCardOut{}, err
	}
	card, err := s.deps.Cards.CreateCard(ctx, s.identity.ProjectID, protocol.CreateCardRequest{
		Title: in.Title, Body: in.Body, Role: in.Role, Package: in.Package,
	}, projects.WithDependsOn(keys))
	if err != nil {
		return nil, createCardOut{}, fmt.Errorf("create the card: %w", err)
	}
	out := createCardOut{ID: card.ID, Key: card.Key, Title: card.Title, State: card.State}
	for _, key := range keys {
		out.DependsOn = append(out.DependsOn, key.String())
	}
	return nil, out, nil
}

// dependsOnKeys reads the cards a proposed card is to wait for, in number order. It is strict about
// the shape of a key - the project id, the hash, and a number that starts at 1 - because a key the
// tool cannot read is a mistake in the plan and not a plan. Whether the key names a card that exists
// in this project is the projects service's answer to give, inside the transaction that writes the
// card, so a refusal there leaves nothing half-written; this only has to make sure the service is
// asked with keys rather than with text.
func dependsOnKeys(raw []string) ([]protocol.CardKey, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]protocol.CardKey, 0, len(raw))
	for _, text := range raw {
		key, err := protocol.ParseCardKey(strings.TrimSpace(text))
		if err != nil {
			return nil, fmt.Errorf("create_card: dependsOn: %w", err)
		}
		out = append(out, key)
	}
	// The answer names the plan in the order the work has to be done, and a key named twice is the one
	// edge it is, so what the tool echoes back is what reading the card would answer.
	slices.SortFunc(out, func(a, b protocol.CardKey) int { return a.Number - b.Number })
	return slices.Compact(out), nil
}

// askAgentInput is ask_agent's arguments.
type askAgentInput struct {
	// CardKey is the card to ask, in the "<project id>#<number>" form another card is named by.
	CardKey string `json:"cardKey"`
	// Question is what to ask. It arrives in the other card's session as a message, so it has to
	// read on its own.
	Question string `json:"question"`
}

// askAgentOut is ask_agent's answer.
type askAgentOut struct {
	// CardKey is the card that was asked.
	CardKey string `json:"cardKey"`
	// Title is that card's name, so the asking agent can name it afterwards.
	Title string `json:"title"`
	// Sent is true when the question reached the other card's session.
	Sent bool `json:"sent"`
}

// ask_agent puts a question to another card's agent.
//
// The answer stays where it is given: Marshal carries the question into the other card's session and
// nothing back, so the answer is read there (by that card's agent, and by the person watching it).
// The question says who is asking and where the answer will be, because the other agent cannot see
// this one's session and would otherwise read a bare question as its owner's.
func (s *Server) askAgent(ctx context.Context, _ *mcp.CallToolRequest, in askAgentInput) (*mcp.CallToolResult, askAgentOut, error) {
	if err := s.allow("ask_agent", kindWrite); err != nil {
		return nil, askAgentOut{}, err
	}
	if strings.TrimSpace(in.Question) == "" {
		return nil, askAgentOut{}, fmt.Errorf("ask_agent needs a question")
	}
	key, err := protocol.ParseCardKey(strings.TrimSpace(in.CardKey))
	if err != nil {
		return nil, askAgentOut{}, fmt.Errorf("ask_agent: %w", err)
	}
	other, err := s.deps.Cards.CardByKey(ctx, key)
	if err != nil {
		return nil, askAgentOut{}, fmt.Errorf("read the card to ask: %w", err)
	}
	if other.ID == s.identity.CardID {
		return nil, askAgentOut{}, fmt.Errorf("card %s is this card: ask the question in your own work instead", other.Key)
	}
	if other.ProjectID != s.identity.ProjectID {
		return nil, askAgentOut{}, fmt.Errorf("card %s is in another project, and one card may not reach across projects", other.Key)
	}
	me, err := s.deps.Cards.Card(ctx, s.identity.CardID)
	if err != nil {
		return nil, askAgentOut{}, fmt.Errorf("read your own card: %w", err)
	}
	if err := s.deps.Agents.Send(ctx, other.ID, questionFrom(me, in.Question)); err != nil {
		return nil, askAgentOut{}, fmt.Errorf("ask card %s: %w", other.Key, err)
	}
	return nil, askAgentOut{CardKey: other.Key, Title: other.Title, Sent: true}, nil
}

// questionFrom writes a question the way it should arrive in the other card's session: named, and
// clear about the fact that the answer is not carried back.
func questionFrom(me protocol.Card, question string) string {
	return fmt.Sprintf(
		"A question from card %s (%q), another card in the same project, asked through Marshal:\n\n%s\n\n"+
			"Answer as you normally would. Marshal does not carry your answer back to that card; it stays in this session.",
		me.Key, me.Title, strings.TrimSpace(question))
}
