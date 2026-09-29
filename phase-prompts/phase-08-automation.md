# Prompt: Phase 8 — automation

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything.

**Baseline.** Read `git status --short`. Keep a list of every file you create, modify, or delete, and put its counts in your report.

**Dates.** Use today's date from `date +%F`. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes.

---

## 0. The one-paragraph mission

Phase 8 gives Marshal a scheduler, Trello two-way sync, Google Calendar and Gmail, and morning/evening briefs. **The webhook route this phase needs is a real prerequisite for Phase 6's GitHub App and Phase 9's Funnel, not just for Trello** — if you build `/hooks/{provider}` with real signature verification here, later phases only add a provider case, not the plumbing. The test harness for this already exists ahead of the route: `tools/hooks-replay` can replay a recorded webhook with its original headers so a signature check still matches, and a GitHub fixture is already recorded.

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
2. **Never use a real Trello token, Google OAuth client, or send a real email/calendar write in any automated test.** Every integration test uses a fake server or a recorded response.
3. Do not delete files you did not create, except mock code you retire as part of a cutover.
4. Do not create documents beyond `phase-reports/phase-08-automation.md` and what your slices need.
5. Do not edit generated files by hand.
6. **Never touch the owner's real daemons or data folders.**
7. Only macOS matters.
8. Do not weaken or delete a test to make it pass.

---

## 2. What's already built, and the real gaps

- **Nothing exists for schedules or integrations at the code level**: no `schedules`, `schedule_runs`, or `integrations` tables, no `daemon/internal/schedules` or `daemon/internal/integrations` package. All greenfield.
- **The Schedules and Integrations screens are already built on the mock, with an exact contract to match.** `apps/web/src/mock/settings-types.ts`'s `Schedule` type: `id, name, kind: "brief"|"job", icon, trigger, when, time, days, action, project, enabled, missed`. `trigger` is one of "Cron"/"Interval"/"One-time"/"Event"; `when` is a **natural-language string** ("Every weekday at 9:00"), not a real cron expression, and the edit form only lets a person change `when` after creation — `time`/`days` are set once at creation and never resurfaced. **This means the scheduler needs a natural-language-to-cron translation layer, not a raw `robfig/cron` string** — decide how before you build the engine, and record it as a ruling. `Integration`'s `detail` field is a single free-text string carrying different information per provider (a Trello board name, a Gmail state sentence, a Discord channel) — the real backend needs a proper per-provider connection shape; do not try to cram everything back into one string.
- **The route-registration pattern is unchanged** (`routeNeeds` bitmask, `has()`, `domainRoutes()`) — add a bit for schedules/integrations the same way every prior phase did.
- **No webhook route exists yet anywhere in the daemon** (confirmed by grep), but `tools/hooks-replay` and a recorded GitHub ping fixture already exist, built ahead of the route. Build `POST /hooks/{provider}` with real per-provider signature verification here; Phase 6 and Phase 9 depend on this being solid.

---

## 3. Data and wire

New migration for `schedules`, `schedule_runs`, `integrations` (columns in `docs/architecture.md` section 10). New wire types matching the mock's `Schedule` and a real, structured `Integration` connection shape (not the mock's single free-text `detail` string) — decide the per-provider fields you need and record the decision as a ruling, since three different provider shapes (Trello board, Calendar/Gmail account, generic status) are being unified into one type.

---

## 4. Your work, in this order

### Using your own subagents

If your harness can spawn subagents, use them **to draft, not to build.** You are the only one who compiles, runs a test, or touches a shared file (a migration, the route table, the wire types) — a subagent hands you a file back; it does not run the full build or the full test suite itself. This is what keeps a shared, uncommitted working tree safe, and what keeps the machine you're running on from being asked to run several heavy test suites at once.

- **Work in small waves, one slice at a time.** A wave of helpers drafts, or does read-only research; you read what comes back, integrate it yourself, run the scoped test for just that, and reconcile any shared file. Only then does the next wave start. Never let two waves write at the same time.
- The first wave for every slice is read-only research: confirm the current state of whatever this brief assumes — a prior phase's report can be stale by the time you actually reach it. Research never conflicts with anything, so this is always safe to run wide.
- You fix the shared contract yourself first — the webhook route and its signature verification — before handing anything to a helper to draft against it.
- A wave can draft Trello, Google Calendar, and Gmail (slices 2 and 3) in parallel, one client each, once the webhook route exists.
- Once a slice is fully integrated and its own scoped test passes, spawn a reviewer to read the diff — reading is cheap, and safe to run alongside your next wave's drafting.
- **Never run `pnpm test:e2e` yourself.** It is slow, and this phase does not need it to prove its own work — write whatever end-to-end specs this phase's items ask for, but leave running the suite to the controller, who runs it once across everything you and earlier phases built. Run `node scripts/check.mjs` once, at the end of the phase, from one place only, never mid-slice.

### Slice 1 — the scheduler core (B8.1)
Pin `github.com/robfig/cron/v3`. New `daemon/internal/schedules` module: cron, interval, and one-time triggers, the missed-run policy, event triggers, and loops with hard limits. Resolve the natural-language-`when` question from section 2 before building the trigger engine.

### Slice 2 — the webhook route and Trello (B8.2)
Build `POST /hooks/{provider}` with real signature verification (prove it against `tools/hooks-replay` and the existing GitHub fixture, plus a Trello fixture you record). Then `daemon/internal/integrations/trello`, a small typed client (no maintained official Go client exists, per `docs/library-docs.md` — write your own), two-way sync per `docs/architecture.md` section 19.4: checklists, items, comments, attachments, and member mapping, the agent shown as a Trello label, latest-change-wins on conflict.

### Slice 3 — Google Calendar and Gmail (B8.3)
Pin `google.golang.org/api` (`calendar/v3`, `gmail/v1`) and `golang.org/x/oauth2`. Events feed the briefs and the calendar; labeled email becomes a card. Each has its own connection test.

### Slice 4 — the calendar range call (B8.4, N21)
One call for a date range returning events, scheduled jobs, briefs, and due cards. The calendar view and Home's "coming up" list both read it; a disconnected Google Calendar shows "Not connected" with no sample data.

### Slice 5 — briefs (B8.5)
Morning and evening briefs across all projects, only new changes since the last brief, with brief times taken from Trello or Calendar when configured.

---

## 5. Cut over

S22 (Home: coming up), S25 (Calendar), S29c (Trello), S29d (Google Calendar), S29e (Gmail), S30 (Settings: Schedules) move from mock to daemon, each only when its cutover gate passes.

---

## 6. Verification

The full gate (`node scripts/check.mjs`) is not yours to run (rule 1d): CI runs it on GitHub, and you prove your work with the fast checks of rule 1d, at the end of each slice (do not run `pnpm test:e2e` yourself — write the specs this phase's items ask for, and leave running the suite to the controller). Every integration's connection test proven with recorded responses, never real user data, in any automated test.

---

## 7. Report

Fill in `phase-reports/phase-08-automation.md` in place. Section 6 must record: how `when`'s natural language maps to the real trigger engine, and the structured `Integration` connection shape you designed to replace the mock's single free-text field.

---

## 8. Definition of done and stop conditions

**Done** when: jobs run on time and after wake as configured, and every loop stops on every limit type in a test; Trello moves, checklists, comments, and attachments sync both ways against a fixture board, and a conflict resolves to latest-change-wins; Google Calendar events and labeled Gmail both work against recorded responses; the calendar and coming-up list read one call and show "Not connected" honestly when not set up; briefs deliver on time with only new changes; S22, S25, S29c, S29d, S29e, and S30 are switched or listed as built-not-switched; and the report is complete.

**Stop and report immediately** if: you would need a real Trello token or Google OAuth client for any test; or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**Then continue.** Once this phase's report is complete, do not stop and wait to be told: read `phase-prompts/phase-09-remote.md` and start it the same way, treating this phase's own report as the "prior phase" it asks you to read. There is no controller checkpoint between phases — the owner wants Phases 3 through 13 worked in one continuous run, and the controller verifies afterward, not between each one. If this phase itself hit something in "Stop and report immediately" that you could not resolve, finish and save this phase's report exactly as it stands, note the block plainly, and still move on to the next phase for whatever in it does not depend on the blocked item.
