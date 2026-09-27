package mcpserver

import (
	"fmt"
	"runtime/debug"
)

// register installs every tool of docs/architecture.md section 11.4 on the server, in the order that
// table lists them. The order is what a client sees in the tool list, and section 7's context order
// ends with "the list of the internal MCP tools" - so the table's own order is the one to keep, and
// every tool the table names is registered here, including the six that can only say their part of
// Marshal is not built yet.
//
// The descriptions are written for the model that reads them and not for a person browsing the
// source: each says what the tool does, and the one thing about it that is easy to get wrong (that a
// note replaces the whole note, that a claim is not a lock, that create_card is subject to the mode,
// that an answer to ask_agent does not come back). The argument and answer shapes carry the rest.
func (s *Server) register() (err error) {
	// mcp.AddTool panics when a tool's argument or answer type cannot be turned into a JSON schema.
	// That is a programming mistake in this package and not something a caller can fix, but it must
	// not take the daemon down: a session that cannot be given its tools should fail to start and say
	// why, which is what this turns the panic into.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("register an MCP tool: %v\n%s", r, debug.Stack())
		}
	}()

	add(s, "board_status",
		"What the other cards in this project are doing: their owner, goal, state, and the files they "+
			"have claimed. Call it before starting work on something another card may be on.",
		s.boardStatus)
	add(s, "claim_files",
		"Declare the files or packages this card is working on, so other cards are told before they "+
			"touch the same ones. Claiming is a declaration and not a lock: another card may hold the "+
			"same path and this answer will say so.",
		s.claimFiles)
	add(s, "release_files",
		"Give back claims this card no longer needs, so other cards are no longer warned about them.",
		s.releaseFiles)
	add(s, "post_note",
		"Write this card's note: the one markdown file Marshal keeps for the card in the project "+
			"vault, which the card's page and Obsidian both show. It replaces the whole note, so read "+
			"it first with read_notes when you mean to add to it.",
		s.postNote)
	add(s, "read_notes",
		"Read a card's note. With no card key, this card's own note.",
		s.readNotes)
	add(s, "ask_agent",
		"Put a question to another card's agent. The answer is given in that card's own session; "+
			"Marshal does not carry it back to you.",
		s.askAgent)
	add(s, "create_card",
		"Propose a new card in this project: it is added to the backlog and is not started. Some "+
			"permission modes leave a new card to the card's owner, and this answer will say so if "+
			"this one does.",
		s.createCard)
	add(s, "search_memory",
		"Search what the cards of this project have left in their notes, and what the project has "+
			"learned in its lessons, best match first. The answer is an excerpt of each hit; read the "+
			"whole note with read_notes.",
		s.searchMemory)
	add(s, "search_codebase",
		"Find where a name is written in this project's code - a function, a type, a method - without "+
			"reading files.",
		s.searchCodebase)
	add(s, "report_progress",
		"Set this card's one-line \"doing now\", which the board shows. An empty line clears it.",
		s.reportProgress)
	add(s, "list_checklists",
		"Read this card's checklists and their items.",
		s.listChecklists)
	add(s, "tick_checklist_item",
		"Tick or untick one item on this card's checklist, with the evidence that it is done. A "+
			"checklist only the person may tick refuses this.",
		s.tickChecklistItem)
	add(s, "read_comments",
		"Read this card's new comments, which marks them as read for you.",
		s.readComments)
	add(s, "read_attachment",
		"Read a file attached to this card.",
		s.readAttachment)
	add(s, "post_comment",
		"Post a comment on this card as its agent.",
		s.postComment)
	return nil
}
