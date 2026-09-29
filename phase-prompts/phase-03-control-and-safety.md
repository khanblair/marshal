# Prompt: Phase 3 — control and safety

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything.

**Baseline.** Read `git status --short` now, and read `docs/progress-tracker.md`, `docs/backend-checklist.md` (the register in 2.2 and the Phase 2 block), and the "State today" paragraph of `docs/architecture.md` section 11.2: this phase builds directly on top of whatever Phase 2 actually finished, which may not match its own brief exactly. Do not revert, reword, or tidy anything you did not put there yourself; keep a list of every file you create, modify, or delete, and put its counts in your report.

**Dates.** Use today's date from `date +%F`. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes.

---

## 0. The one-paragraph mission

Phase 3's milestone is "safe to trust": permission modes, bypass mode, approvals, a secret scanner, an audit log, mid-session model/thinking switching, and the deploy-approval rule. **The single most important thing this brief tells you, found by reading the current code, is that the daemon is further along than the checklist text alone suggests, and unevenly so between agents.** The ACP adapter (which Gemini CLI uses) already has a complete, working, tested permission request-and-response round trip through the real ACP protocol. Claude Code and the raw PTY adapter have neither: both `Respond` methods unconditionally refuse. You are not building one approval mechanism from nothing; you are **wiring the session layer and the API to the mechanism ACP already has**, and separately **deciding what "safe to trust" means for the two agent kinds that have no channel to ask permission at all**. Read section 2 before you write a line of code.

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
2. **Never run a real coding agent with a prompt.** Every permission/approval test uses the stub agent or a fake ACP peer, never `claude`, `gemini`, or `codex` for real.
3. **The secret scanner is tested against synthetic test keys you invent for the test, never a real credential of any kind**, yours or the project's. If you need to prove it against a real-looking key, use an obviously fake pattern (`gitleaks` itself ships test fixtures for this — use those).
4. Do not delete files you did not create, except mock code you retire as part of a section cutover (name every one).
5. Do not create documents beyond `phase-reports/phase-03-control-and-safety.md` (fill in the existing skeleton) and what your slices need.
6. Do not edit generated files by hand. Change the source and run `pnpm gen`.
7. **Never touch the owner's real daemons or data folders.** Any daemon you start uses a throwaway `--data-dir` and a spare `--port`.
8. Only macOS matters.
9. Do not weaken or delete a test to make it pass.
10. Two reviews are required for changes to `security` and `harness` (`docs/code-standards.md` 9.3) — you cannot review your own code twice, so say plainly in your report that this phase's gate needs the controller to arrange a second reviewer, not just read your own report.

---

## 2. What's already built, and the one real gap in the specification

Read `daemon/internal/agents/agent.go`, `daemon/internal/agents/acp/permission.go`, `daemon/internal/agents/claude/adapter.go`, and `daemon/internal/agents/pty/adapter.go` before anything else. What you will find:

- **The `Agent` interface is already permission-shaped.** `Respond(ctx, handle, ApprovalResponse) error`, the `PermissionRequested` event (with `RequestID`, `ToolCallID`, a title, a kind, a path or command, and a list of `PermissionOption`s: allow-once, allow-always, reject-once, reject-always), and the error sentinels `ErrUnknownRequest`/`ErrUnknownOption` already exist. You add no new interface.
- **ACP (and therefore Gemini, which is `acp.Adapter` underneath) already asks for real.** `acp/permission.go`'s `askPermission` blocks the turn, emits `PermissionRequested`, and waits on a channel; `respond` validates the chosen option and delivers the answer through the real ACP `session/request_permission` method. This is complete and tested.
- **Claude Code and PTY cannot ask at all, by design, already documented in the code.** `claude/adapter.go`'s `Respond` always returns `agents.ErrUnknownRequest` wrapped with "Claude Code's streaming JSON mode has no way to ask the person about a tool yet." `pty/adapter.go`'s `Respond` does the same. `protocol.AgentCapabilities.Approvals` already exists on the wire, already `true` for Gemini's catalog spec and `false` (absent) for Claude's, with the doc comment: "When it is false, the 'Ask' permission mode makes the agent skip what needs approval instead of asking, and the screens should say so." **This is not a gap for you to close by inventing a new Claude Code integration mode** (no permission-prompt tool is wired into the Claude adapter, and adding one is a larger, separate piece of work outside this phase's scope unless the controller tells you otherwise) — it is an accepted, already-recorded limitation. Build the real approval flow against ACP/Gemini, and make the screens honestly show Claude's degraded "Ask silently becomes skip" behavior, per the capability flag that already exists. **Do not silently assume every agent gets real approvals**; if you find yourself building a fake approval channel for Claude, stop and put it in "Needs the controller" instead.
- **The session layer's hook point is already marked with a comment naming this exact phase.** `session/pump.go`'s `handleEvent` has, right where a `case agents.PermissionRequested:` branch belongs, a comment reading: "agents.PermissionRequested is logged and ringed above but not published: approvals are Phase 3 (B3.4), which does not exist yet." Add that branch. `protocol.SessionStateWaitingApproval` already exists in the enum and is never set anywhere today — this is where you set it.
- **`PATCH /v1/cards/{id}` already exists and is already wired**, including a `PermissionMode` field on `UpdateCardRequest` (`routes.go`, `routes_cards.go`, `projects.UpdateCard`). It does not yet have bypass's confirmation/lock/audit side effects, and nothing applies a change to a session that is already running (only at the next `Start`/`Resume`) — that live-apply gap is real and is yours to close (section 4, B3.6).
- **gitx has a path-containment check (`checkInsideRoot`) but no git-level containment.** Nothing today can answer "is this git operation restricted to branch X" or "can this process only push to the worktree's own branch". Bypass mode's "cannot touch the main branch or run outside the worktree" needs a new check; the filesystem-containment helper is a start, not the whole answer.
- **No `approvals`, `audit_log`, `harness`, or `security` package or table exists yet.** These are genuinely new for this phase, not partially built.
- **The audit log screen (build-plan task 3.13) has no prototype.** `design/` has a `.dc.html` file for every other built view and none for audit; `docs/backend-inventory.md` section 6 lists it explicitly as needing design review before it is built. Build the backend and the audit trail regardless; do not build the screen without checking `docs/backend-inventory.md` section 6 first, and list it in your report if you judge it needs the owner's sign-off before you draw it.

---

## 3. Data and wire

Add a migration for `approvals` (id, session_id, request_json, decision, decided_by) and `audit_log` (id, session_id, actor, action, target, detail_json, created_at) — the exact columns are already in `docs/architecture.md` section 10. Pin `github.com/zricethezav/gitleaks/v8` (already the approved choice in `docs/library-docs.md`) for the secret scanner. New wire types: `Approval`, `ApprovalDecision`, an audit-log entry and its list/search/export answer shapes, each with a golden file, following the existing pattern.

---

## 4. Your work, in this order

### Using your own subagents

If your harness can spawn subagents, use them **to draft, not to build.** You are the only one who compiles, runs a test, or touches a shared file (a migration, the route table, the wire types) — a subagent hands you a file back; it does not run the full build or the full test suite itself. This is what keeps a shared, uncommitted working tree safe, and what keeps the machine you're running on from being asked to run several heavy test suites at once.

- **Work in small waves, one slice at a time.** A wave of helpers drafts, or does read-only research; you read what comes back, integrate it yourself, run the scoped test for just that, and reconcile any shared file. Only then does the next wave start. Never let two waves write at the same time.
- The first wave for every slice is read-only research: confirm the current state of whatever this brief assumes — a prior phase's report can be stale by the time you actually reach it. Research never conflicts with anything, so this is always safe to run wide.
- You fix the shared contract yourself first — the migration, the wire types, the route-table entries — before handing anything to a helper to draft against it.
- Slice 3's new `harness` and `security` packages are separate enough for a wave to draft in parallel, one file each, once slices 1 and 2's plumbing exists. Slice 1's `session/pump.go` change touches a hot, shared file — make that edit yourself, not through a helper.
- Once a slice is fully integrated and its own scoped test passes, spawn a reviewer (or two, exactly where this brief already asks for two reviews — `security` and `harness`) to read the diff — reading is cheap, and safe to run alongside your next wave's drafting.
- **Never run `pnpm test:e2e` yourself.** It is slow, and this phase does not need it to prove its own work — write whatever end-to-end specs this phase's items ask for, but leave running the suite to the controller, who runs it once across everything you and earlier phases built. Run `node scripts/check.mjs` once, at the end of the phase, from one place only, never mid-slice.

### Slice 1 — approvals, through the path that already works (B3.4, N7)
Add the `case agents.PermissionRequested` branch in `session/pump.go`: set `SessionStateWaitingApproval`, write the `approvals` row, publish an event. Add `POST /v1/approvals/{id}` (approve or deny) and wire it to `Manager.Respond`. Build the "one row, many views" fan-out N7 asks for: the mock's `apps/web/src/mock/actions/approvals.ts` (`syncProjectApproval`) is the exact behavior to match — an approval a project chat asked for is the same row as the card's, and both update. Approve resumes the turn; deny tells the agent within the same session, no restart. Prove this against Gemini through a fake ACP peer, never a real one.

### Slice 2 — bypass mode (B3.2, N8)
`POST`/`DELETE /v1/cards/{id}/bypass` with a typed acknowledgement, a project-level bypass lock, and the containment check section 2 says gitx does not yet have (build it new — inspecting the git subcommand and the ref it touches, or diffing the branch before and after, whichever proves cleaner; record which you chose and why). Both turning bypass on and off are audited. Wire the already-ported `BypassBanner.tsx` and the card settings bypass control to these real calls in place of the mock.

### Slice 3 — permission modes, profiles, blocklist, deploy rule (B3.1, B3.3, B3.7)
New `daemon/internal/harness` and `daemon/internal/security` packages (neither exists). Enforce permission modes for the agents that can genuinely enforce them (ACP-based); for Claude, drive its own CLI flags as today and make the screens say what its capability flag already says, rather than pretending it is enforced Marshal-side too. Command blocklist checked in every mode except bypass; blocked commands are stopped or sent for approval. Deploy workflows refused outside bypass. **Before you scope this slice, resolve the one real ambiguity found in research**: whether "permission modes enforced in the harness" (`docs/architecture.md` section 13: "checked by the harness before every tool call... the agent's own claims are never trusted") means an independent Marshal-side gate on every tool call regardless of what the agent does, which is a materially bigger job than today's per-adapter flag-driving, or means correctly driving each agent's own enforcement. If you cannot resolve this from the docs, make the more conservative, safer choice (an independent gate, at least for the blocklist and deploy rule, which do not depend on an agent's cooperation) and record it as a ruling with its cost if wrong.

### Slice 4 — secret scanner and audit log (B3.5)
Scan every agent commit with gitleaks; a commit with a test key is blocked and the card moves to Needs you. Every action from slices 1-3 writes an audit row. Build the list, search, and export routes, read only, nothing editable from the API. Check `docs/backend-inventory.md` section 6 before building the audit log screen; if it needs a fresh design, say so in your report and build only the backend and the routes.

### Slice 5 — mid-session model and thinking switch (part of B3.6)
`UpdateCardRequest` already changes `card.Model`/`Thinking`/`PermissionMode` in the database; nothing today applies that to a session that is already running. Decide how a live session picks up the change (most likely: the next turn reads the card's current settings rather than what `Start` was given, since the `Agent` interface has no `SetModel`/live-update method and adding one would touch every adapter) and write the system message and activity row N8 asks for. Record your choice as a ruling.

---

## 5. Cut over

S7b (Bypass) and S8b (Approvals) move from mock to daemon, each only when its cutover gate passes in full. Update `apps/web/src/data/sections.ts` and the register together, remove the matching mock code, update the docs.

---

## 6. Verification

The full gate (`node scripts/check.mjs`) is not yours to run (rule 1d): CI runs it on GitHub, and you prove your work with the fast checks of rule 1d, at the end of each slice (do not run `pnpm test:e2e` yourself — write the specs this phase's items ask for, and leave running the suite to the controller). Coverage floors: `harness`, `security`, and `session` at 85 percent (`docs/backend-checklist.md`'s Phase 3 gate). A goroutine test (`goleak`) on any package that starts one.

---

## 7. Report

Fill in `phase-reports/phase-03-control-and-safety.md` in place. Section 6 must record the two rulings this brief calls for (the harness-enforcement scope, and how a live session picks up a mid-session setting change). Section 8 must say plainly that a second reviewer is needed for `security`/`harness` changes, and must list the audit log screen if you judged it needs design review before drawing it.

---

## 8. Definition of done and stop conditions

**Done** when: approvals work end to end against a fake ACP peer, from the card, the chat, and Home; bypass cannot leave the worktree or touch the main branch, proven by a test; the blocklist and deploy rule are enforced; a commit with a test secret is caught and blocks the card; the audit log is complete, searchable, and read-only; a mid-session setting change takes effect on the next turn with a system message; S7b and S8b are switched (or listed as built-not-switched with a reason); coverage floors hold; and the report is complete.

**Stop and report immediately** if: you find yourself building a fake permission channel for Claude Code rather than accepting its documented degraded behavior; the harness-enforcement scope cannot be resolved conservatively; the audit log screen has no existing components to build from; or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**Then continue.** Once this phase's report is complete, do not stop and wait to be told: read `phase-prompts/phase-04-models.md` and start it the same way, treating this phase's own report as the "prior phase" it asks you to read. There is no controller checkpoint between phases — the owner wants Phases 3 through 13 worked in one continuous run, and the controller verifies afterward, not between each one. If this phase itself hit something in "Stop and report immediately" that you could not resolve, finish and save this phase's report exactly as it stands, note the block plainly, and still move on to the next phase for whatever in it does not depend on the blocked item.
