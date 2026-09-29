# Prompt: Phase 7 — orchestration and memory

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything.

**Baseline.** Read `git status --short`. Keep a list of every file you create, modify, or delete, and put its counts in your report.

**Dates.** Use today's date from `date +%F`. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes.

---

## 0. The one-paragraph mission

Phase 7 gives agents awareness of the board and each other, and gives Marshal memory. You build the internal MCP server (the daemon **serving** tools to agents — every tool in `docs/architecture.md` section 11.4), board awareness and file claims, the Orchestrator role and handoff between agents, the memory module and an Obsidian-friendly vault, and a codebase map with session search and a context budget meter. **Nothing here exists in code yet** — no MCP server package, no memory/vault/codemap packages, no tables. The one piece of existing plumbing you extend is `session/start.go`'s `startAgent`, whose own comment already says plainly that `Instructions` is empty in Phase 1 "because the roles module (Phase 5) and the memory module (Phase 7) that would fill them do not exist yet" — that sentence is your task list.

---

## 1. Hard rules

1. **Never run a mutating git command.** Allowed: `status`, `diff`, `log`, `show`, `ls-files`, `blame`.
1a. **No browser, ever.** Never open a browser or a preview for any reason: no Browser-pane tools, no `preview_start`, no Claude in Chrome, no simulator, no screenshots of a running app, and no `pnpm test:e2e`. Verify only as rule 1d allows. Anything that needs a browser goes under "Needs the controller" in your report for the owner's hands-on check.
1b. **Use the code review graph, for searching, for changing, and for checking.** This repo's code (Go, TypeScript, SQL) is mapped in a local knowledge graph (`.code-review-graph/`, git-ignored, never committed), and its MCP server is set up in `.mcp.json` (the `code-review-graph` server; tools such as `semantic_search_nodes_tool`, `query_graph_tool`, `get_impact_radius_tool`, `detect_changes_tool`).
   - **Searching.** Before you grep, glob, or read files to find something, ask the graph: `semantic_search_nodes_tool` (a function, type, or test by name or keyword), `query_graph_tool` (`callers_of`, `callees_of`, `imports_of`, `importers_of`, `tests_for`, `file_summary`), and `get_architecture_overview_tool` or `list_communities_tool` for structure. Use grep, glob, and plain reads only for what the graph does not hold: docs, SQL text, config, and string literals.
   - **Before changing shared code** (a wire type, the route table, a store method, `marshal.ts`, `api-client.ts`, the session manager, anything many files import): run `get_impact_radius_tool` to see what depends on it, and `query_graph_tool` with `tests_for` to find the tests that cover it. Run those tests after the change.
   - **After changing code:** run `detect_changes_tool` and `get_review_context_tool` for the risk-scored blast radius and the changed functions that have no test, and close the gaps before you report. `get_affected_flows_tool` shows the execution paths you touched.
   - **Keep it fresh.** The graph updates on file edits when your harness runs hooks. Otherwise, after any large batch of edits and before you rely on it in a new area, run `code-review-graph update` (incremental, a few seconds).
   - **No MCP in your harness?** Use the same graph from the shell: `code-review-graph search "<name>"`, `code-review-graph query callers_of <name>` (also `callees_of`, `tests_for`, `imports_of`, `file_summary`), `code-review-graph impact --files <files>` (list the files `git status --short` shows when the tree is uncommitted), `code-review-graph detect-changes --brief`, `architecture`, `communities`, `flows`. If `code-review-graph` is not on the PATH, run it as `uvx code-review-graph <command>`.
   - **The graph is a map, not proof.** It never replaces running the tests, and a claim in your report about what depends on what must come from a query you ran.
1c. **When the phase is done, run a code review update.** As the last step before you finalize your report, refresh the graph with `code-review-graph update`, then run `code-review-graph detect-changes --brief` (add `--base <commit>` to compare against where the phase started; the default is `HEAD~1`), or `detect_changes_tool` with `get_review_context_tool`. Put the numbers (nodes, edges, files, and the risk summary, including changed functions with no test coverage) in your report's verification results, and fix or list every high-risk or untested change.
1d. **Verify only with fast checks. The full gate, and anything that starts a process to check your work by hand, are not run on the owner's machine unless the owner asks.**
   - **Allowed, one at a time:** `pnpm gen` when a wire type or SQL changes (never edit generated files by hand); the typecheck of the workspace you touched (`pnpm --filter <workspace> typecheck`; in `daemon/`, `go build ./... && go vet ./...`); `biome check` on the files you touched and `golangci-lint` on the packages you touched; and the fast unit tests of the files you touched: vitest with file filters at `--maxWorkers=4`, and Go `-run` with the touched tests (or a small package).
   - **Not allowed:** the full gate (`node scripts/check.mjs`) and every heavy run inside it: whole-repo suites (`pnpm test`, `go test -race ./...`, the whole web suite, and whole slow Go packages such as `internal/api`, `internal/projects`, and `internal/session`, which take minutes), `pnpm build`, `pnpm budgets`, `pnpm smells`, `knip`, `jscpd`, and the browser suite. **And nothing that starts a process to check your work by hand:** no daemon (not even a throwaway one), no stub-agent or `curl` smoke, no dev server, and none of `scripts/agent-smoke.mjs`, `scripts/check-generated.mjs`, or `scripts/coverage.mjs`. A unit test that starts its own in-process server or fake process is a unit test and is fine. GitHub CI runs everything else on every pull request.
   - Where this brief tells you to run something that is not allowed, do not run it. Write "not run (the owner has not asked)" next to it in your report and list it under "Needs the controller", so it runs in CI or when the owner asks.
2. **Never run a real coding agent with a prompt.** MCP tool calls are tested against the stub agent.
3. Do not delete files you did not create, except mock code you retire as part of a cutover.
4. Do not create documents beyond `phase-reports/phase-07-orchestration-and-memory.md` and what your slices need.
5. Do not edit generated files by hand.
6. **Never touch the owner's real daemons or data folders**, and never write outside a throwaway `<data-dir>/vault` in a test.
7. Only macOS matters.
8. Do not weaken or delete a test to make it pass.
9. **Resolve the codebase-map library decision (section 4, slice 4) before you build it**, and record your reasoning; do not silently default to whichever is easiest to wire up.

---

## 2. What's already built, and the real gaps

- **No MCP server code exists anywhere** (a full grep of the daemon and cmd trees for "mcp" turns up only unrelated capability flags reporting whether an agent *accepts* MCP servers — nothing that *serves* one). Pin `github.com/modelcontextprotocol/go-sdk` (already the approved choice). Build `daemon/internal/mcpserver` (the exact package name `docs/architecture.md` section 3's module table already uses — match it) implementing every tool in section 11.4: `board_status`, `claim_files`, `release_files`, `post_note`/`read_notes`, `ask_agent`, `create_card`, `search_memory`, `search_codebase`, `report_progress`, `list_checklists`, `tick_checklist_item`, `read_comments`, `read_attachment`, `post_comment`. Build the cheap, read-only tools first; the two that depend on the codebase map and memory search come after those modules exist.
- **`agents.StartSpec` has no field for MCP servers to attach, and no agent session is given any today.** `acp/handshake.go`'s `noServers` is literally named and commented "the list of MCP servers that a session is given: none yet." Add a field to `StartSpec`, thread it through `session/start.go`'s `startAgent`, and replace `noServers` with the real list — every card's session gets the internal MCP server over stdio (CLI agents) or in-process (the built-in agent, once Phase 4 exists).
- **`Instructions` in `StartSpec` is always empty today, by design, for exactly the reason your module fills.** Once roles (Phase 5) exist, build the context order `docs/architecture.md` section 7 already specifies: role instructions, then project memory summary, then the card's task and pinned files, then a short board-awareness summary, then the list of available internal MCP tools.
- **Two package names are already fixed in the architecture doc's module table** — use them exactly: `memory` (knowledge base, lessons, card notes, vault sync, session search) and `codemap` (the codebase map). Do not invent different names.
- **The Notes tab is a single markdown note per card, not a list**, and its mock is exact: `apps/web/src/views/card/NotesTab.tsx`/`card-note.ts` — `noteText(card)` reads one string, `saveNote` overwrites the whole note. **The mock's note path omits the project segment** the architecture doc's vault layout specifies (`vault/<project>/cards/`, not `vault/cards/`) — follow the architecture doc's path when you build the real vault; the mock's simplification is the thing that's slightly wrong here, not the doc.
- **No tables exist for notes, usage, mcp_servers, or skills.** All greenfield.

---

## 3. Data and wire

New migration for `notes`, `mcp_servers`, and whatever registry Phase 7 needs for its own tools (columns in `docs/architecture.md` section 10). New wire types for a card note, an MCP server's status, and the context-budget meter's reading, each with a golden file.

---

## 4. Your work, in this order

### Using your own subagents

If your harness can spawn subagents, use them **to draft, not to build** — but this phase is more sequential than most: the MCP server's tools build on each other, and slice 3's plumbing gates slice 2 ever reaching a real agent. Look for independence inside slice 5's separate pieces (awareness and claims, the Orchestrator and handoff, memory screens) rather than across the whole phase. You are the only one who compiles, runs a test, or touches a shared file (a migration, the route table, the wire types) — a subagent hands you a file back; it does not run the full build or the full test suite itself.

- **Work in small waves, one slice at a time.** A wave of helpers drafts, or does read-only research; you read what comes back, integrate it yourself, run the scoped test for just that, and reconcile any shared file. Only then does the next wave start. Never let two waves write at the same time.
- The first wave for every slice is read-only research: confirm the current state of whatever this brief assumes — a prior phase's report can be stale by the time you actually reach it. Research never conflicts with anything, so this is always safe to run wide.
- You fix the shared contract yourself first — the `memory`/`mcpserver`/`codemap` package boundaries and the new migration — before handing anything to a helper to draft against it.
- Inside slice 5, a wave can draft its separate pieces (awareness and claims, the Orchestrator and handoff, memory screens) in parallel.
- Once a slice is fully integrated and its own scoped test passes, spawn a reviewer to read the diff — reading is cheap, and safe to run alongside your next wave's drafting.
- **Never run `pnpm test:e2e` yourself.** It is slow, and this phase does not need it to prove its own work — write whatever end-to-end specs this phase's items ask for, but leave running the suite to the controller, who runs it once across everything you and earlier phases built. Run `node scripts/check.mjs` once, at the end of the phase, from one place only, never mid-slice.

### Slice 1 — memory store core (B7.4 groundwork)
New `daemon/internal/memory` package, plus the `notes` table (matching the mock's one-note-per-card shape, but at the project-scoped vault path the architecture doc specifies) and whatever registry tables the rest of this phase needs. Everything else reads or writes through this.

### Slice 2 — the internal MCP server (B7.1)
`daemon/internal/mcpserver`, serving every tool in section 11.4 over stdio for CLI agents and in-process for the built-in agent (once it exists). Build the cheap tools first (`board_status`, `read_notes`, `list_checklists`, `read_comments`, `read_attachment`), then the mutating ones (`post_note`, `claim_files`, `release_files`, `tick_checklist_item`, `post_comment`, `report_progress`, `create_card`, `ask_agent`), then the two that need slices 4 and 1 (`search_memory`, `search_codebase`) last. Every call is permission-checked; a refused call comes back explained, not silently dropped.

### Slice 3 — StartSpec plumbing (small, but blocks slice 2 reaching a real agent)
Add the MCP-servers field to `StartSpec`, thread it through `startAgent`, replace `acp.noServers` with the real list, and fill `Instructions` per the order in section 2.

### Slice 4 — the codebase map (B7.5, resolve the open decision first)
`docs/library-docs.md` section 2.10 leaves this genuinely undecided among three options: Tree-sitter through cgo bindings, universal ctags as an external binary, or Tree-sitter compiled to WASM. **Read that section, then decide and record your reasoning before writing code.** Given the daemon is pure Go everywhere today (no cgo appears anywhere in the codebase) and already prefers shelling out to external processes over linking C libraries (`gitx` shells out to the real `git`, agents run as external CLI processes, `internal/proc` and `internal/platform` already manage long- and short-lived external processes as the project's established pattern), **ctags as an external binary is the most consistent fit** and is the recommended default unless you find a concrete reason against it during implementation; Tree-sitter-to-WASM is the second choice (still no cgo, but adds a WASM runtime and a grammar-build step); cgo bindings are the weakest fit given the explicit no-cgo rule. Build a light, incremental index (updates on a changed file, never a full re-scan) so agents answer "where is X" without reading many files.

### Slice 5 — awareness, claims, orchestrator, handoff, session search, memory screens (B7.2, B7.3, remaining B7.4/B7.5)
A board-awareness summary per turn that stays inside its token budget; file claims with early conflict warnings on overlap; the Orchestrator role planning a goal into cards with dependencies; a clean-summary handoff between agents; session search across cards; the context-budget meter and pinned files. Memory-viewer and lessons screens (build-plan task 7.13) have no prototype — check with the owner before building anything beyond what existing components already support.

---

## 5. Cut over

S14 (Card notes), S24b (Session and note search), S29b (Integration: Obsidian) move from mock to daemon, each only when its cutover gate passes.

---

## 6. Verification

The full gate (`node scripts/check.mjs`) is not yours to run (rule 1d): CI runs it on GitHub, and you prove your work with the fast checks of rule 1d, at the end of each slice (do not run `pnpm test:e2e` yourself — write the specs this phase's items ask for, and leave running the suite to the controller), including a check that idle RAM and CPU budgets still hold with the vault's file watcher running.

---

## 7. Report

Fill in `phase-reports/phase-07-orchestration-and-memory.md` in place. Section 6 must record which codebase-map option you chose and why.

---

## 8. Definition of done and stop conditions

**Done** when: agents can call every MCP tool and a refused call explains why; overlapping file claims warn both cards; the Orchestrator creates approved cards from a goal; the vault opens in Obsidian with working links, and an edit made in Obsidian is picked up; the Notes tab reads and writes the card's real note at the correct project-scoped path; agents answer "where is X" without reading many files; session search finds past work across cards; the context meter warns before compaction; idle RAM/CPU budgets hold with the vault watcher running; S14, S24b, and S29b are switched or listed as built-not-switched; and the report is complete.

**Stop and report immediately** if: the codebase-map decision cannot be made confidently from the library-docs options; the memory screens have no existing components to build from; or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**Then continue.** Once this phase's report is complete, do not stop and wait to be told: read `phase-prompts/phase-08-automation.md` and start it the same way, treating this phase's own report as the "prior phase" it asks you to read. There is no controller checkpoint between phases — the owner wants Phases 3 through 13 worked in one continuous run, and the controller verifies afterward, not between each one. If this phase itself hit something in "Stop and report immediately" that you could not resolve, finish and save this phase's report exactly as it stands, note the block plainly, and still move on to the next phase for whatever in it does not depend on the blocked item.
