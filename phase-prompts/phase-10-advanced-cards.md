# Prompt: Phase 10 — advanced cards

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything.

**Baseline.** Read `git status --short`. Keep a list of every file you create, modify, or delete, and put its counts in your report.

**Dates.** Use today's date from `date +%F`. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes.

---

## 0. The one-paragraph mission

Phase 10 makes real everything the prototype's card panel already shows on the mock: templates, dependencies, sub-cards, acceptance checks, card-from-anywhere, duplicate detection, fork-from-checkpoint, race mode, the agent scorecard, checklists, comments, and members. **For checklists, comments, and members, the UI is already fully built and working against the mock — this is the largest slice of the phase, and it is wiring, not design.** For templates/sub-cards/dependency-editing, race mode, and the scorecard, there is no existing UI at all; treat those as fresh screen work and check with the owner before drawing anything.

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
2. **An attachment is data, never code to run.** Any test involving a comment attachment proves it is never executed, only read.
3. Do not delete files you did not create, except mock code you retire as part of a cutover.
4. Do not create documents beyond `phase-reports/phase-10-advanced-cards.md` and what your slices need.
5. Do not edit generated files by hand.
6. **Never touch the owner's real daemons or data folders.**
7. Only macOS matters.
8. Do not weaken or delete a test to make it pass.

---

## 2. What's already built, and the real gaps

- **`template_id` and `parent_id` are already anticipated cards columns in the architecture doc**, but nothing has migrated them yet: no `TemplateID`, `ParentID`, or dependency field exists anywhere in the Go code today. The schema expects this work; it has not started.
- **Checklists, comments, and members already have full, real UI on the mock**: `ChecklistSection.tsx`/`ChecklistsTab.tsx`/`ChecksList.tsx` (items with who-did-it and when, hide/show done, add/remove, per-checklist delete with confirmation, and a distinct "acceptance checks" concept — "Marshal runs these by itself. The agent can't finish this card until every check passes."), `CommentComposer.tsx`/`CommentItem.tsx`/`CommentsTab.tsx` (text plus file/image/link attachments, an "Agent read this" badge, delete-own-comment-only), `MembersRow.tsx` (people chips, the agent shown as a member once started, add/remove). **The mock's exact behavior to match**, read fully before you design routes: `apps/web/src/mock/actions/checklists.ts` and `comments.ts`. Two things in the mock are wrong and must not be copied: its "@agent" detector matches any `@` character, not specifically `@agent` (fix this in the real implementation, per `docs/architecture.md` 19.2's actual rule: "mentions @agent, or ends with a question mark"), and it never wakes a sleeping agent to reply even though the architecture doc says a reply should wake the session if needed — the real implementation must add that wake path. The mock's acting user is hardcoded (`"ada"`); the real implementation takes it from the authenticated caller, the same way card creation already does.
- **The `Checklist` type in the mock has no `required` or `people_only` flags at all** — task 10.16 (the checklist-options screen) is genuinely new, not a wiring exercise, even though checklists themselves are well ported.
- **`card_checks`'s planned columns (`id, card_id, kind, spec_json, status`) have no run or commit reference**, but "a failing test unticks the item it proved" (B10.11) needs to know which run the evidence came from. Add whatever column that needs when you write the migration; do not discover this gap after building the tick-and-unwatch logic.
- **Duplicate detection needs `internal/search` (Phase 2 slice F), which does not exist yet.** If Phase 2 has not built it by the time you reach this slice, you build the minimal piece of it this phase needs rather than blocking on it, and note the dependency in your report. The mock's own heuristic (title-only, case-folded words of at least four characters, at least two shared words with an open card in the same project, capped at two results) is a floor, not a ceiling — `build-plan.md` task 10.15 asks for title **and** body, so make the real check broader than the mock's.
- **Templates today are a fixed five-item picklist that only sets a default permission mode and role** (`Blank`, `Bug fix`, `New endpoint`, `Refactor`, `Plan first`) — there is no reusable, editable, table-backed template system, and no dependency-editing UI on the card panel at all (dependencies exist only as a read-only list feeding the Timeline's Gantt lines). Both need real new screen work, per build-plan task 10.18.
- **Fork-from-checkpoint depends on two things that may not exist yet when you start**: Phase 2's basic fork (from a card's latest commit, no checkpoint) and Phase 5's checkpoints table. Read both phases' reports first; if either is not built, say so in your report and build what you can, or stop that item and describe exactly what is missing.
- **Race mode has no existing concept anywhere**, and gitx's `AddWorktree` refuses a second worktree on a branch that already exists — one branch, one worktree, already enforced. Race mode is naturally N independent cards (each its own branch, worktree, and session), not a new sub-card mechanic, but nothing in the schema ties them together for the side-by-side comparison screen. Decide how you group them (a race-group id on the card, most likely) and record it as a ruling.

---

## 3. Data and wire

New migration for `card_links` (dependencies), `checklists`, `checklist_items` (with `required`/`people_only` added, per section 2), `card_members`, `comments`, `attachments`, `card_checks` (with a run/commit reference added), `templates`, `file_claims`, and the `template_id`/`parent_id` columns on `cards` — all already columned in `docs/architecture.md` section 10 except the two additions this brief calls out. New wire types for each, with golden files.

---

## 4. Your work, in this order

### Using your own subagents

If your harness can spawn subagents, use them **to draft, not to build.** You are the only one who compiles, runs a test, or touches a shared file (a migration, the route table, the wire types) — a subagent hands you a file back; it does not run the full build or the full test suite itself. This is what keeps a shared, uncommitted working tree safe, and what keeps the machine you're running on from being asked to run several heavy test suites at once.

- **Work in small waves, one slice at a time.** A wave of helpers drafts, or does read-only research; you read what comes back, integrate it yourself, run the scoped test for just that, and reconcile any shared file. Only then does the next wave start. Never let two waves write at the same time.
- The first wave for every slice is read-only research: confirm the current state of whatever this brief assumes — a prior phase's report can be stale by the time you actually reach it. Research never conflicts with anything, so this is always safe to run wide.
- You fix the shared contract yourself first — the migration and the card-mutation service both checklists, comments, and members build on — before handing anything to a helper to draft against it.
- Slice 3 (checklists, comments, members) is the largest single slice here, and a wave can draft it three ways once that shared service exists.
- Once a slice is fully integrated and its own scoped test passes, spawn a reviewer to read the diff — reading is cheap, and safe to run alongside your next wave's drafting — and specifically to confirm no attachment is ever executed.
- **Never run `pnpm test:e2e` yourself.** It is slow, and this phase does not need it to prove its own work — write whatever end-to-end specs this phase's items ask for, but leave running the suite to the controller, who runs it once across everything you and earlier phases built. Run `node scripts/check.mjs` once, at the end of the phase, from one place only, never mid-slice.

### Slice 1 — templates, dependencies, sub-cards (B10.1, part of B10.3)
The `template_id`/`parent_id` columns, `card_links`, a real `templates` table (replacing the fixed five-item picklist), dependent cards starting after their dependency merges, and a parent card showing its sub-cards' combined progress. New screen work for template management and dependency editing (build-plan task 10.18) — check with the owner before drawing it.

### Slice 2 — acceptance checks and duplicate detection (B10.2, B10.3's other half, N9, N22)
Run and list calls for a card's checks (add the run/commit reference column from section 2). Duplicate detection on title and body, broader than the mock's heuristic, using or building the minimum of `internal/search` this needs.

### Slice 3 — checklists, comments, members (B10.5, B10.6) — the largest slice
Named checklists, items, hide-checked, the new `required`/`people_only` flags and their screen (10.16, new UI), agent ticks with evidence that auto-untick when the evidence stops being true, and required checklists gating the merge queue. Comments with files/images/links/mentions, the corrected `@agent` detector, the wake-if-needed reply path, attachments on disk under the size limit (never run, only read), and member notices. Wire the already-built UI to real calls in place of the mock; this needs Phase 7's `tick_checklist_item`/`read_comments`/`post_comment` MCP tools for the agent-facing half — if Phase 7 has not built them yet, build the person-facing half now and note the dependency.

### Slice 4 — fork from a checkpoint and race mode (B10.4)
Confirm Phase 2's basic fork and Phase 5's checkpoints both exist before starting (section 2); if not, stop this item and describe what's missing. Race mode: fork one card per contestant, run each independently, a side-by-side comparison screen (genuinely new — check with the owner), and a keep-the-winner action that archives the others.

### Slice 5 — the agent scorecard (B10.7)
Stats per agent, model, and role, including smells introduced, matching Phase 4's usage table, Phase 5's smell findings, and this phase's race-mode outcomes. Build this last — it depends on all three.

---

## 5. Cut over

S5b (Templates, dependencies, duplicates), S12 (Card checks), S15 (Checklists), S16 (Comments and members) move from mock to daemon, each only when its cutover gate passes.

---

## 6. Verification

The full gate (`node scripts/check.mjs`) is not yours to run (rule 1d): CI runs it on GitHub, and you prove your work with the fast checks of rule 1d, at the end of each slice (do not run `pnpm test:e2e` yourself — write the specs this phase's items ask for, and leave running the suite to the controller). A specific untrusted-content test: a comment attachment is read as data and never executed, in both a unit test and an end-to-end check.

---

## 7. Report

Fill in `phase-reports/phase-10-advanced-cards.md` in place. Section 6 must record the race-group design and how the duplicate-detection dependency on `internal/search` was resolved. Section 9 must list the checklist-options, race-mode/scorecard, and templates/dependency-editing screens for the owner's sign-off.

---

## 8. Definition of done and stop conditions

**Done** when: new cards start from real templates; dependent cards start after their dependency merges and a parent shows combined progress; a card cannot finish until its checks pass, and check results work as tick evidence; similar cards are flagged on title and body before creation; a fork runs on its own from a checkpoint and race mode's winner is kept, the others archived; required checklists block the merge queue and a failing test unticks the item it proved; `@agent` and a question mark both get a reply, waking the session if needed, and an attachment reaches the agent as data only; the scorecard's numbers match the usage and findings tables; S5b, S12, S15, and S16 are switched or listed as built-not-switched; and the report is complete.

**Stop and report immediately** if: fork-from-checkpoint's dependencies are not yet built; the new screens have no existing components to build from; or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**Then continue.** Once this phase's report is complete, do not stop and wait to be told: read `phase-prompts/phase-11-ecosystem.md` and start it the same way, treating this phase's own report as the "prior phase" it asks you to read. There is no controller checkpoint between phases — the owner wants Phases 3 through 13 worked in one continuous run, and the controller verifies afterward, not between each one. If this phase itself hit something in "Stop and report immediately" that you could not resolve, finish and save this phase's report exactly as it stands, note the block plainly, and still move on to the next phase for whatever in it does not depend on the blocked item.
