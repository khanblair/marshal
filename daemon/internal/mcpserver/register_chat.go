package mcpserver

// The tools a project chat's server serves. A chat has no card, so none of the card tools that name
// "this card" are here: no claims, notes, progress line, checklists, comments, or ask_agent. What a
// chat gets depends on its kind:
//
//   - every chat reads the board;
//   - the Orchestrator chat also plans: it creates cards and searches memory and the codebase;
//   - the Integrator chat answers merge tasks with merge_context, merge_report, and ask_owner.
//
// Which tools a kind has is the one list in registerChat, so a tool is never reachable by a chat by
// accident.

// registerChat installs the tools of the server's chat kind, in the order a model reads them.
func (s *Server) registerChat() {
	add(s, "board_status",
		"What the cards in this project are doing: their owner, goal, state, and the files they have "+
			"claimed. Use it to see the board before you plan.",
		s.boardStatus)
	switch s.identity.ChatKind {
	case ChatKindOrchestrator:
		s.registerPlanningTools()
	case ChatKindIntegrator:
		s.registerMergeTools()
	}
}

// registerPlanningTools installs what the Orchestrator chat plans with. Searching the notes needs the
// memory module, which a daemon always has but a test may leave out.
func (s *Server) registerPlanningTools() {
	add(s, "create_card",
		"Propose a new card in this project: it is added to the backlog and is not started, so the "+
			"owner starts it. Name the cards it waits for in dependsOn. Create cards after the owner "+
			"has agreed the plan.",
		s.createCard)
	if s.deps.Notes != nil {
		add(s, "search_memory",
			"Search what the cards of this project have left in their notes, and what the project has "+
				"learned in its lessons, best match first. The answer is an excerpt of each hit.",
			s.searchMemory)
	}
	add(s, "search_codebase",
		"Find where a name is written in this project's code - a function, a type, a method - without "+
			"reading files.",
		s.searchCodebase)
}

// registerMergeTools installs the Integrator chat's three tools, under the names, arguments, and
// descriptions the Integrator's own prompt and role instructions say (internal/integrator/merge_tools.go).
func (s *Server) registerMergeTools() {
	add(s, "merge_context",
		"The full task you were sent: the conflicted files and, for every card involved, its title, task, "+
			"plan, handoff note, branch, and changed files. Call it before you resolve anything.",
		s.mergeContext)
	add(s, "merge_report",
		"Report the verdict on a merge task. Call it once, when every conflicted file is resolved and "+
			"staged with git add, or when you cannot. If you are not sure, set confident to false and put "+
			"your questions in questions.",
		s.mergeReport)
	add(s, "ask_owner",
		"Ask the owner a question that blocks you on a merge task. It is posted in the Integrator chat "+
			"and on the card's Needs you. You must still finish with merge_report.",
		s.askOwner)
}

// chatInstructions is what a chat's server tells its client at the handshake.
func chatInstructions(kind ChatKind) string {
	switch kind {
	case ChatKindOrchestrator:
		return "Marshal's tools for this project's Orchestrator chat. Read the board, then plan: " +
			"propose cards for the backlog with create_card. You cannot change the project's files here."
	case ChatKindIntegrator:
		return "Marshal's tools for the Integrator. A merge task names itself in the message you are " +
			"sent: read it with merge_context, resolve it in your own workspace, and finish with merge_report."
	}
	return "Marshal's tools for this chat. They read the project's board."
}
