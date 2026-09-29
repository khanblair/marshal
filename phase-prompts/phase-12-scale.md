# Prompt: Phase 12 — scale

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything.

**Baseline.** Read `git status --short`. Keep a list of every file you create, modify, or delete, and put its counts in your report.

**Dates.** Use today's date from `date +%F`. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes.

---

## 0. The one-paragraph mission

Phase 12's milestone is "v1 complete." **Its own build plan is partly stale, and you must not rebuild what is already done.** Three of its listed tasks — List view, Timeline view, and Calendar view — were already finished by earlier phases: `build-plan.md` numbered them as Phase 12 tasks before `docs/backend-checklist.md` later consolidated List and Timeline into Phase 2's `S5a` cutover (`B2.4`'s own text says "Cut over S5a, including List, Timeline, and Agents view") and Calendar into Phase 8's `B8.4` (whose own parenthetical cites "Tasks 12.3" as already folded in). The real, genuinely new work here is monorepo mode's package graph and sparse worktrees (detection already exists from Phase 1), split view and focus mode, the resource panel, export/import/backup, and team mode — the last of which has the single largest open architectural question left in the whole project.

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
2. Do not delete files you did not create, except mock code you retire as part of a cutover.
3. Do not create documents beyond `phase-reports/phase-12-scale.md` and what your slices need.
4. Do not edit generated files by hand.
5. **Never touch the owner's real daemons or data folders.**
6. Only macOS matters (this phase's own gate wants three platforms tested before release — do that part only once the owner is ready for a release pass; your own verification stays macOS-only).
7. Do not weaken or delete a test to make it pass.
8. **Do not start team mode (slice 5) until the open question in section 4 is resolved**, either by the owner or by an explicit, conservative ruling you record.

---

## 2. What's already built, and the real gaps

- **List and Timeline are real, working, daemon-backed UI already, not placeholders.** `apps/web/src/views/list/` (7 files) and `apps/web/src/views/timeline/` (15 files, including dependency lines and drag geometry) are already built and already part of `S5a`, cut over in Phase 2. **Calendar is the same story**: `apps/web/src/views/calendar/` (13 files) already exists and already reads the one date-range call Phase 8's `B8.4` built, cutting over `S22`/`S25`. **Your first slice is to verify these three, end to end against the real daemon, not to rebuild them.** If verification finds a real gap, fix it and say so plainly — do not assume it is automatically fine just because this brief says it should be.
- **Monorepo detection already exists from Phase 1**, decided by an already-answered open question (`docs/progress-tracker.md`'s Q7: monorepo detection moved into Phase 1). `daemon/internal/projects/monorepo.go` already detects npm/pnpm/Go/Cargo workspaces by finding real folders that hold a marker file (`package.json`, `go.mod`, `Cargo.toml`, and others) through each ecosystem's own workspace-pattern file. **What is missing**: a package **dependency graph** (nothing builds one today), **sparse worktrees scoped to one package** (the git worktree code is generic, not package-aware), and **affected-tests-only in CI** (the only hits for this phrase anywhere are mock fixture text, not real logic). The package swimlane and filter UI already exists generically in the mock (`SwimKey`/`FilterKey` already include `"package"`), so this section is about giving it real package data, not building new UI.
- **Split view already has a real button and state machine, on the mock.** `apps/web/src/app/ViewHeader.tsx`'s "Split view" button and `M.S.split` array already exist and are tested. Your job is making each pane carry real per-pane daemon data, not building the button.
- **Focus mode and the resource panel do not exist anywhere, in any form.** Both are genuinely new, and the resource panel additionally has no design yet (`docs/backend-inventory.md` section 6 lists it explicitly) — check with the owner before drawing it.
- **Export/import and encrypted backup have no existing code or screen.** Genuinely new; the screens need a design check per `docs/backend-inventory.md` section 6.
- **Team mode has zero existing concept in the auth code.** `daemon/internal/protocol/auth.go`'s `Caller` struct today is exactly `{DeviceID, DeviceKind, UserID}` — no role field, nothing anywhere checks owner-versus-member. `docs/architecture.md` section 16.4 confirms solo use is everything built through Phase 11.

---

## 3. Data and wire

New migration only for whatever monorepo package-graph storage, resource-panel readings, and (if the section 4 decision proceeds) team/membership tables this phase needs — check `docs/architecture.md` section 10 for `memberships`, already columned there, before adding a new table for it.

---

## 4. The open architectural question — team mode (B12.4)

This is not yours to silently decide. `Caller` has no role concept today, and the docs specify only the goal ("two users work on one board"), not the mechanism. Before starting slice 5, resolve, with the owner or through an explicit architecture decision you record as a ruling:

1. Does a second person's browser reach the **same** daemon (over the tailnet Phase 9 built) and the same database, or does team mode imply a second daemon or host machine model entirely?
2. How does a second user authenticate — a second device token under the same `UserID`, or a genuinely separate `UserID` with its own devices?
3. What does "role" (owner versus member) actually gate — which routes, which actions (removing a project, approving bypass mode, and so on)?
4. Does the audit log and bypass-mode confirmation (Phase 3) need a permission check added once a second user exists?

**A defensible starting point, if the owner has no strong preference**: the lowest-lift path is a second user's browser authenticating with its own device token against the same daemon and database (one shared project and board state, fitting the single-host model everywhere else in the docs), with a `role` column added to wherever `memberships` already lives, checked only at the handful of points that matter (remove project, bypass mode, delete). Do not build this without recording it as a ruling either way.

---

## 5. Your work, in this order

### Using your own subagents

If your harness can spawn subagents, use them **to draft, not to build.** You are the only one who compiles, runs a test, or touches a shared file (a migration, the route table, the wire types) — a subagent hands you a file back; it does not run the full build or the full test suite itself. This is what keeps a shared, uncommitted working tree safe, and what keeps the machine you're running on from being asked to run several heavy test suites at once.

- **Work in small waves, one slice at a time.** A wave of helpers drafts, or does read-only research; you read what comes back, integrate it yourself, run the scoped test for just that, and reconcile any shared file. Only then does the next wave start. Never let two waves write at the same time.
- The first wave for every slice is read-only research: confirm the current state of whatever this brief assumes — a prior phase's report can be stale by the time you actually reach it. Slice 1 of this phase is itself a verification pass a wave can split by section (List, Timeline, Calendar, monorepo detection) and run at once — research never conflicts with anything, so this is always safe to run wide.
- A wave can draft slice 3's split view, focus mode, and resource panel in parallel, one screen each, once slice 1's verification pass is done.
- **Resolve slice 5's team-mode question yourself, alone, before handing any of it to a helper.**
- Once a slice is fully integrated and its own scoped test passes, spawn a reviewer to read the diff — reading is cheap, and safe to run alongside your next wave's drafting — watching the UI-RAM budget specifically once real panes carry live data.
- **Never run `pnpm test:e2e` yourself.** It is slow, and this phase does not need it to prove its own work — write whatever end-to-end specs this phase's items ask for, but leave running the suite to the controller, who runs it once across everything you and earlier phases built. Run `node scripts/check.mjs` once, at the end of the phase, from one place only, never mid-slice.

### Slice 1 — verify, do not rebuild (folds build-plan tasks 12.1, 12.2, 12.3)
Confirm List, Timeline, and Calendar genuinely work end to end against the real daemon at the sizes and states their cutover gates require. Confirm monorepo detection's Phase 1 fixture still passes. This is a regression pass, sized small, not a build slice — if it turns anything up, fix it and note the fix; otherwise record the evidence and move on.

### Slice 2 — monorepo mode, completed (B12.1)
The package dependency graph, sparse worktrees scoped to one package, affected-tests-only in CI, and wiring the existing generic package swimlane/filter UI to real package data (cuts over `S5c`). The fixture monorepo works end to end.

### Slice 3 — split view, focus mode, resource panel (B12.2)
Wire Split view's existing pane mechanism to real per-pane daemon data. Build focus mode from scratch (Needs-you cards only). Build the resource panel (RAM, CPU, disk per card) from scratch — check with the owner on its design first. Watch the UI-RAM budget closely here: up to four real panes of live daemon data is the tightest test of the 150 MB-at-50-cards budget in the whole project; measure it, do not assume it holds.

### Slice 4 — export, import, and encrypted backup (B12.3)
New screens (a project-move flow in Project settings, a machine picker per `docs/backend-inventory.md` section 6) and encrypted settings/memory sync between two devices over the tailnet Phase 9 built.

### Slice 5 — team mode (B12.4) — only after section 4 is resolved
Shared boards, human handoff, a `role` concept, and comments/pickers reading real users, cutting over `S2c`. Two users work on one board.

---

## 6. Cut over

S2c (Team and people) and S5c (Package swimlane and filter) move from mock to daemon, each only when its cutover gate passes.

---

## 7. Verification

The full gate (`node scripts/check.mjs`) is not yours to run (rule 1d): CI runs it on GitHub, and you prove your work with the fast checks of rule 1d, at the end of each slice (do not run `pnpm test:e2e` yourself — write the specs this phase's items ask for, and leave running the suite to the controller). The UI-RAM budget measured with four real split-view panes open, not just the single-view baseline. The fixture monorepo verified end to end. This phase's own gate ("v1 complete") wants three-platform testing before a release — leave that to the controller and the owner; your own pass stays macOS-only.

---

## 8. Report

Fill in `phase-reports/phase-12-scale.md` in place. Section 2 ("Slice results") records the verify-only slice 1 evidence, not a rebuild. Section 6 must record the team-mode ruling in full, with the four questions from section 4 each answered or explicitly deferred to the owner.

---

## 9. Definition of done and stop conditions

**Done** when: slice 1's verification is clean (or its fixes are recorded); the fixture monorepo works end to end with a real package graph, sparse worktrees, and affected-tests-only; split view carries real data within the RAM budget, focus mode and the resource panel work; a project moves to another machine and two devices sync encrypted; team mode's open question is resolved one way or the other before any of its code is written, and (if built) two users work on one board; S2c and S5c are switched or listed as built-not-switched; and the report is complete.

**Stop and report immediately** if: the team-mode question has no answer from the owner and you are not confident in the conservative default; the resource panel or export/import screens have no existing components to build from; or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**Then continue.** Once this phase's report is complete, do not stop and wait to be told: read `phase-prompts/phase-13-retire-the-mock-and-accept.md` and start it the same way, treating this phase's own report as the "prior phase" it asks you to read. There is no controller checkpoint between phases — the owner wants Phases 3 through 13 worked in one continuous run, and the controller verifies afterward, not between each one. If this phase itself hit something in "Stop and report immediately" that you could not resolve, finish and save this phase's report exactly as it stands, note the block plainly, and still move on to the next phase for whatever in it does not depend on the blocked item.
