package mcpattach

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/khanblair/marshal/daemon/internal/protocol"
)

// The board-awareness summary (docs/marshal-product-scope.md section 11.3, docs/backend-checklist.md
// B7.2, build-plan task 7.2): one short line per other card in the project saying what it is called,
// where it is, what its agent says it is doing, and the files it holds - and, ahead of those, a
// warning about any file this card holds together with another one.
//
// # The budget
//
// The summary is built to a byte budget rather than to the size of the board, because it is read at
// the start of *every* turn: an unbounded summary on a busy board would cost a token price on every
// turn of every card, which is exactly the cost docs/marshal-product-scope.md section 11.3 warns
// about ("short summaries, never full transcripts"). awarenessBudget is that cap. The conflict
// warnings are placed first and kept whole, because they are the part an agent must not lose; the
// board lines fill what is left, and whatever does not fit is counted rather than named. Every part
// of a line is clipped to its own length so the arithmetic has a floor.
const (
	// awarenessBudget is how long a summary may be, in bytes. 1400 bytes is roughly 350 tokens - a
	// small fraction of even a modest context window, and enough for the conflicts plus a dozen
	// cards and their holds.
	awarenessBudget = 1400
	// awarenessLimit caps how many other cards are named before the rest are counted. It is the same
	// choice the MCP tool board_status makes for the same reason: the summary says who else is
	// working and on what, it does not hand the agent the board.
	awarenessLimit = 20
	// awarenessTitleMax, awarenessDoingMax, and awarenessLabelMax clip the pieces of one card's line.
	awarenessTitleMax = 50
	awarenessDoingMax = 60
	awarenessLabelMax = 40
	// awarenessHoldsMax and awarenessPathMax clip the files a card is said to hold.
	awarenessHoldsMax = 4
	awarenessPathMax  = 48
	// awarenessConflictMax caps the conflict warnings, and awarenessConflictLineMax one of them, so a
	// card holding fifty files in common cannot fill the whole budget with warnings.
	awarenessConflictMax     = 3
	awarenessConflictLineMax = 300
	// awarenessConflictHoldersMax caps how many other cards one warning names.
	awarenessConflictHoldersMax = 3
	// awarenessTailReserve is the room held back while board lines are added, so there is always
	// somewhere to say how many cards were left out. It is wider than the longest tail
	// ("… and 199 more cards"), so the reserve is never itself the thing that does not fit.
	awarenessTailReserve = 40
	// awarenessConflictHead introduces the conflict warnings.
	awarenessConflictHead = "Files you and another card both hold:"
	// ellipsis marks something that was clipped. Read as one character, it is three bytes, which is
	// why clip reserves for it.
	ellipsis = "…"
)

// context builds the context a fresh session starts from, in the order docs/architecture.md section
// 7 gives it: the role's instructions, the project's memory, the card's task and its pinned files, a
// short board-awareness summary, and the list of tools the agent has.
//
// Every part is best-effort. A part that cannot be read is left out rather than failing the card: an
// agent that starts with four of the five sections is better off than one that does not start. The
// one exception is the card's own task, which is always there because it is what the agent was
// started for.
//
// The project-memory section is filled by the memory slice (B7.4): the `memory` and `lessons`
// folders the architecture doc names are written by that slice, and section 12's `<vault>/<project>`
// layout only has `cards/` in it today (internal/memory/vault.go). Until then there is no summary to
// read, and the section is left out rather than invented. Pinned files arrive with the context
// meter (build-plan task 7.11) for the same reason.
func (a *Attacher) context(ctx context.Context, card protocol.Card, tools []string) string {
	var b strings.Builder

	// 1. The role's instructions.
	if instructions := a.roleInstructions(ctx, card); instructions != "" {
		fmt.Fprintf(&b, "%s\n\n", instructions)
	}

	// 3. The card's task.
	fmt.Fprintf(&b, "Your card is %s: %s\n", card.Key, card.Title)
	if body := strings.TrimSpace(card.Body); body != "" {
		fmt.Fprintf(&b, "\n%s\n", body)
	}

	// 4. A short board-awareness summary, the same one the agent is given at the start of each
	// later turn (TurnAwareness).
	if awareness := a.awareness(ctx, card); awareness != "" {
		fmt.Fprintf(&b, "\n%s\n", awareness)
	}

	// 5. The tools the agent has.
	if len(tools) > 0 {
		fmt.Fprintf(&b, "\nYou have Marshal's own tools for this card: %s. "+
			"Every call is checked against the card's permission mode, and a call that is not allowed "+
			"comes back explaining why.\n", strings.Join(tools, ", "))
	}

	return strings.TrimSpace(b.String())
}

// TurnAwareness is the board-awareness summary a card's agent is given at the start of a turn. It is
// the session manager's Awareness (internal/session/aware.go), so it is read once as every turn
// begins, and it answers "" for a card that cannot be read - which is how the manager knows to send
// the turn's message on its own.
func (a *Attacher) TurnAwareness(ctx context.Context, cardID string) string {
	if a.deps.Cards == nil {
		return ""
	}
	card, err := a.deps.Cards.Card(ctx, cardID)
	if err != nil {
		return ""
	}
	return a.awareness(ctx, card)
}

// roleInstructions is the role's own system prompt, or empty when the card names no role or the role
// cannot be read.
func (a *Attacher) roleInstructions(ctx context.Context, card protocol.Card) string {
	if a.deps.Roles == nil || strings.TrimSpace(card.Role) == "" {
		return ""
	}
	role, err := a.deps.Roles.Role(ctx, card.Role, card.ProjectID)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(role.Spec.Instr)
}

// awareness assembles the summary: the conflict warnings, then the board. Nothing about the project
// being unreadable is an error - a board that cannot be read is a summary that is not shown.
func (a *Attacher) awareness(ctx context.Context, card protocol.Card) string {
	board, ok := a.readBoard(ctx, card.ProjectID)
	if !ok {
		return ""
	}
	budget := awarenessBudget
	var sections []string
	if lines := board.conflicts(card.ID); len(lines) > 0 {
		block := awarenessConflictHead + "\n" + strings.Join(lines, "\n")
		sections = append(sections, block)
		// The two bytes are the blank line that will separate this section from the next one, so
		// what the board is allowed is what will really be left.
		budget -= len(block) + 2
	}
	if section := board.section(card.ID, budget); section != "" {
		sections = append(sections, section)
	}
	return strings.Join(sections, "\n\n")
}

// board is one reading of a project for one summary: its cards, a short label per card for naming it
// in a sentence, and the files each card holds. It is read once and then walked three times, which
// keeps the two reads the summary needs off the per-turn path as much as it can.
type board struct {
	cards  []protocol.Card
	labels map[string]string
	holds  map[string][]string
}

// readBoard reads a project's cards and their claims. It reports false when the cards cannot be
// read, which is the one part a useful summary cannot be built without.
func (a *Attacher) readBoard(ctx context.Context, projectID string) (board, bool) {
	if a.deps.Cards == nil {
		return board{}, false
	}
	cards, err := a.deps.Cards.Cards(ctx, projectID)
	if err != nil {
		return board{}, false
	}
	b := board{cards: cards, labels: make(map[string]string, len(cards))}
	for _, card := range cards {
		b.labels[card.ID] = clip(card.Key+" "+card.Title, awarenessLabelMax)
	}
	b.holds = a.projectHolds(ctx, projectID)
	return b, true
}

// projectHolds reads what every card in a project holds, grouped by card, oldest claim first. A
// memory module that cannot be read is a board with no holds on it: the claims are the part of the
// summary that is most useful and the least essential, and dropping them must not drop the summary.
func (a *Attacher) projectHolds(ctx context.Context, projectID string) map[string][]string {
	out := make(map[string][]string)
	if a.deps.Claims == nil {
		return out
	}
	claims, err := a.deps.Claims.ProjectClaims(ctx, projectID)
	if err != nil {
		return out
	}
	for _, claim := range claims {
		out[claim.CardID] = append(out[claim.CardID], claim.PathOrPackage)
	}
	return out
}

// section is the "other cards" part of the summary, built to fit in room bytes including its own
// heading. Cards past awarenessLimit, and cards whose line does not fit, are counted in a closing
// line rather than named. It answers "" when there is no room for even the heading, or when the
// project has no other card.
//
// Room is held back for that closing line as the lines are added (awarenessTailReserve), because the
// count is only known once the last line has been tried: without the reserve, a board just large
// enough to fill the room would list its cards and then have nowhere to say how many it left out.
func (b board) section(cardID string, room int) string {
	head := "Other cards on this board:"
	if room < len(head)+1+awarenessTailReserve {
		return ""
	}
	used := len(head) + 1
	var lines []string
	unlisted := 0
	for _, other := range b.cards {
		if other.ID == cardID {
			continue
		}
		if len(lines) >= awarenessLimit {
			unlisted++
			continue
		}
		line := b.cardLine(other)
		if used+len(line)+1+awarenessTailReserve > room {
			unlisted++
			continue
		}
		lines = append(lines, line)
		used += len(line) + 1
	}
	if len(lines) == 0 {
		return ""
	}
	if unlisted > 0 {
		tail := fmt.Sprintf("%s and %d more %s", ellipsis, unlisted, plural(unlisted, "card", "cards"))
		if used+len(tail)+1 <= room {
			lines = append(lines, tail)
		}
	}
	return head + "\n" + strings.Join(lines, "\n")
}

// cardLine is one other card: its key and title, its state, what its agent says it is doing, and the
// files it holds. Every part is clipped, so one card with a very long title or very many claims
// cannot push the rest of the board out of the budget.
func (b board) cardLine(card protocol.Card) string {
	line := fmt.Sprintf("- %s %s (%s)", card.Key, clip(card.Title, awarenessTitleMax), card.State)
	if doing := strings.TrimSpace(card.DoingNow); doing != "" {
		line += ": " + clip(doing, awarenessDoingMax)
	}
	if held := b.holds[card.ID]; len(held) > 0 {
		line += " " + ellipsis + " holds " + summarisePaths(held)
	}
	return line
}

// conflicts are the warnings about files this card holds together with another one - the early
// conflict warning of docs/marshal-product-scope.md section 11.4 and build-plan task 7.3, from the
// side of the card that is about to edit the file. The card that claimed the file last is warned as
// it claims (memory.ClaimResult.Conflicts, and the claim_files tool's own answer); this is how the
// card that already held it is told, at the start of its next turn. Capped, so a card that shares
// many files gets the first few and a count rather than a wall of warnings.
func (b board) conflicts(cardID string) []string {
	mine := b.holds[cardID]
	if len(mine) == 0 {
		return nil
	}
	var lines []string
	hidden := 0
	for _, path := range mine {
		others := b.holders(path, cardID)
		if len(others) == 0 {
			continue
		}
		if len(lines) >= awarenessConflictMax {
			hidden++
			continue
		}
		lines = append(lines, clip(fmt.Sprintf("- %s is also held by %s; agree before either edits it.",
			clip(path, awarenessPathMax), strings.Join(others, ", ")), awarenessConflictLineMax))
	}
	if hidden > 0 {
		lines = append(lines, fmt.Sprintf("%s and %d more shared %s", ellipsis, hidden,
			plural(hidden, "file", "files")))
	}
	return lines
}

// holders names the other cards holding one path, newest first is not needed here: it answers at
// most three and counts the rest, so the sentence stays a sentence.
func (b board) holders(path, cardID string) []string {
	var out []string
	for other, held := range b.holds {
		if other == cardID {
			continue
		}
		if !slices.Contains(held, path) {
			continue
		}
		out = append(out, b.name(other))
	}
	sort.Strings(out)
	if len(out) > awarenessConflictHoldersMax {
		out = append(out[:awarenessConflictHoldersMax],
			fmt.Sprintf("and %d more", len(out)-awarenessConflictHoldersMax))
	}
	return out
}

// name is what one card is called in a sentence: its key and its title, or its id when the project's
// cards were not read (a claim can name a card the read did not return only if the two reads raced,
// and naming it by id is better than dropping the warning).
func (b board) name(cardID string) string {
	if label, ok := b.labels[cardID]; ok {
		return label
	}
	return cardID
}

// summarisePaths lists a card's holds for one line: the first few, then a count.
func summarisePaths(paths []string) string {
	shown := paths
	if len(shown) > awarenessHoldsMax {
		shown = shown[:awarenessHoldsMax]
	}
	out := make([]string, 0, len(shown))
	for _, path := range shown {
		out = append(out, clip(path, awarenessPathMax))
	}
	s := strings.Join(out, ", ")
	if rest := len(paths) - len(shown); rest > 0 {
		s += fmt.Sprintf(" and %d more", rest)
	}
	return s
}

// clip shortens s to at most max bytes, at a character boundary, marking it with an ellipsis. It
// never answers something longer than max, which is what makes the budget arithmetic above hold: a
// string that was not shortened is returned as it is, and one that was is cut short enough to leave
// room for the ellipsis itself.
func clip(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := max - len(ellipsis)
	if cut < 0 {
		cut = 0
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimRight(s[:cut], " ") + ellipsis
}

// plural picks the word for a count.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
