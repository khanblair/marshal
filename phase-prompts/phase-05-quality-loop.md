# Prompt: Phase 5 — quality loop

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything. This is the largest remaining phase; pace yourself across its seven slices and do not rush the merge queue.

**Baseline.** Read `git status --short`, and read `phase-reports/phase-03-control-and-safety.md` and `phase-reports/phase-04-models.md` in full before you start: this phase needs the harness (Phase 3) and provider system (Phase 4) both in a real, evidenced state, not an assumed one. Keep a list of every file you create, modify, or delete, and put its counts in your report.

**Dates.** Use today's date from `date +%F`. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes.

---

## 0. The one-paragraph mission

Phase 5's milestone is "idea to merge": roles, plan-first mode, harness limits with checkpoints, pull requests with review, the Integrator's merge queue, automatic sleep (the idle-triggered part; manual pause/sleep/wake/pin already shipped in Phase 2), and the code-quality module. **Two things found by reading the code change how you should scope this**: first, the adapter-level plumbing for plan mode already exists for Claude and Gemini (both already map `PermissionMode.plan` to their own native plan modes) — what is missing is the harness workflow around it, not the adapter work. Second, two of the "(prototype)" tagged build-plan items for this phase, smell findings (5.20) and checkpoints (5.21), **have no actual prototype to extract from**, unlike their neighbors — treat them as fresh design, not extraction, and check with the owner before drawing new screens for either.

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
2. **Every merge, PR, and conflict test runs against a throwaway fixture repository, never a real one, and never pushes anywhere real.** If `gitx`'s merge functions ever touch a remote, it is a fixture remote you made in a temp folder.
3. **Do not sign in to GitHub for real, and do not open a real pull request against a real repository.** The owner does the personal GitHub sign-in (`docs/backend-checklist.md` section 3). Your tests use a fake GitHub server or recorded responses.
4. Do not delete files you did not create, except mock code you retire as part of a cutover.
5. Do not create documents beyond `phase-reports/phase-05-quality-loop.md` and what your slices need.
6. Do not edit generated files by hand.
7. **Never touch the owner's real daemons or data folders.**
8. Only macOS matters.
9. Do not weaken or delete a test to make it pass.

---

## 2. What's already built, and where the real gaps are

**gitx today** (`daemon/internal/gitx`): worktrees, branches, clone, sparse checkout, and the Git-version check are all built (Phase 1). **None of the merge-queue primitives exist**: no dry-run merge (`git merge-tree --write-tree`), no backup-branch helper, no "merge a branch into this worktree" step, no fast-forward-only ref update, no abort/rollback. All of this is additive to `gitx`, in its existing style (`(g *Git) Method(ctx, repo, ...) (..., error)`, wrapped errors) — nothing existing needs to change.

**Roles: nothing exists on the daemon side.** No `roles` or `role_overrides` table. `protocol.Card.Role` is already a plain text name, with a doc comment saying plainly "Phase 5 turns this into a role id; until then it is the text name of a starter role" — this is your job. The exact UI contract to match is `apps/web/src/mock/settings-types.ts`'s `Role` interface: `name, starter, overridden, skills, mcp, limits {time, cost, rounds}, backup, desc, agent, model, think, perm, strength, instr`. The weak-model warning is already client-built and exact ("Reviewer"/"Integrator" roles with a model matching `/haiku|mini|flash|deepseek|qwen/` get a non-blocking warning) — reproduce the same names and models, do not re-derive the pattern. **Import and export have no existing UI or mock code at all**, despite being listed alongside genuinely-prototyped role actions in `build-plan.md` task 5.19 — this needs fresh screen work, not extraction.

**Plan mode: the adapter side is already done.** `protocol.PermissionModePlan` exists, and both the Claude adapter and the Gemini adapter already map it to their own native plan modes (Gemini's adapter even has a documented failure path for when its own settings disable planning). `CardStatePlanning` and its board-guard rules already exist in `projects` (almost identical to the mock's move-refusal rules). **What's missing is the harness workflow**: producing and storing the plan object itself (steps, files, risks, checks), the approve/reject/edit API calls (N6), and gating the automatic move out of Planning on approval. The exact behavior and messages to match are in `apps/web/src/mock/actions/approvals.ts` (`approvePlan`, `rejectPlan`, `editPlan`, `savePlan`) — read it fully before you design the routes.

**Checkpoints and smells: no prototype.** `grep -i checkpoint` across the whole mock returns only two lines (inside `fork()`'s system message text) — no checkpoint list, no restore action anywhere. `grep -i smell` and `grep -i finding` across the mock return nothing at all. Both build-plan items 5.20 and 5.21 are tagged "(prototype)" but there is no prototype for either. `docs/architecture.md` section 17 (the smell-check pipeline) and the data model's `smell_profiles`/`smell_findings`/`checkpoints` columns are your only spec for these two — build the backend and the harness logic against them, but treat the screens as needing the owner's design sign-off, not extraction from a mock file that does not exist.

**Personal-token GitHub client, and its boundary with Phase 6's GitHub App.** Build one `daemon/internal/github` package (parallel to `gitx`) behind a small interface (`CreatePullRequest`, `CreateReviewComment`, `GetPullRequest`, `ListChecks`, and whatever else the Integrator needs). Ship one implementation now, an OAuth/personal-token client on `google/go-github` and `golang.org/x/oauth2`, for the owner's personal sign-in. Phase 6 later adds a second implementation on `ghinstallation/v2` for the real GitHub App, behind the same interface — **do not hard-code an OAuth-shaped client into the callers**; depend on the interface from day one, the same way every caller of `gitx` never touches `exec.Cmd` directly, so Phase 6 is additive and nothing you write here needs a rewrite.

---

## 3. Data and wire

New migration for: `roles`, `role_overrides`, `checkpoints`, `smell_profiles`, `smell_findings` (columns already in `docs/architecture.md` section 10). New wire types: `Role`, `Plan` (steps, files, risks, checks), `Checkpoint`, `SmellFinding`, each with a golden file. New enum: whatever the plan's own status needs (waiting, approved, rejected, edited) if it does not already exist — check `protocol/enums.go` first.

---

## 4. Your work, in this order

### Using your own subagents

If your harness can spawn subagents, use them **to draft, not to build.** You are the only one who compiles, runs a test, or touches a shared file (a migration, the route table, the wire types) — a subagent hands you a file back; it does not run the full build or the full test suite itself. This is what keeps a shared, uncommitted working tree safe, and what keeps the machine you're running on from being asked to run several heavy test suites at once.

- **Work in small waves, one slice at a time.** A wave of helpers drafts, or does read-only research; you read what comes back, integrate it yourself, run the scoped test for just that, and reconcile any shared file. Only then does the next wave start. Never let two waves write at the same time.
- The first wave for every slice is read-only research: confirm the current state of whatever this brief assumes — a prior phase's report can be stale by the time you actually reach it, and this phase in particular depends on Phase 3's and Phase 4's actual finished state, not their briefs' assumptions. Research never conflicts with anything, so this is always safe to run wide.
- You fix each slice's shared contract yourself first (a migration, a wire type, the `github` package's interface) before handing anything to a helper to draft against it.
- A wave can draft slice 3's limits, stuck detector, and checkpoints in parallel, one file each. Slice 6 (automatic sleep) is independent of slices 4 and 5 and can be drafted alongside them.
- **Build the Integrator merge queue (slice 5) yourself — do not hand it to a helper.** It is the riskiest, most load-bearing part of this phase, and a subtle bug there is expensive.
- Once a slice is fully integrated and its own scoped test passes, spawn a reviewer to read the diff — reading is cheap, and safe to run alongside your next wave's drafting.
- **Never run `pnpm test:e2e` yourself.** It is slow, and this phase does not need it to prove its own work — write whatever end-to-end specs this phase's items ask for, but leave running the suite to the controller, who runs it once across everything you and earlier phases built. Run `node scripts/check.mjs` once, at the end of the phase, from one place only, never mid-slice.

### Slice 1 — Roles (B5.1)
The `roles` and `role_overrides` tables, full CRUD (create, duplicate, edit, delete, reset — "reset only clears the overridden flag, it does not restore the starter text", exactly as the mock's own code comment says), per-project overrides, the weak-model warning (reproduce the mock's exact regex and wording), and import/export (new screen work — check with the owner if the existing dialog/button components are enough, or if this needs a fresh design). Wire the already-built `RoleEditor.tsx`/`RoleEditorFields.tsx` to real calls in place of the mock.

### Slice 2 — Plan-first mode (B5.2, N6)
The plan object and its approve/reject/edit/save calls, matching `mock/actions/approvals.ts`'s exact behavior and messages. Gate the card's move out of Planning on approval, using the board-guard rules that already exist in `projects`.

### Slice 3 — Limits, stuck detection, checkpoints (B5.3)
Time, cost, and round limits that move a card to Needs you with a reason when hit. A stuck detector that catches repeated error/edit loops. Checkpoints and restore (worktree, and optionally the conversation) — budget extra time here, since there is no mock to extract behavior from; design the restore semantics from the architecture doc's data model and record your choices as rulings.

### Slice 4 — Pull requests and review (B5.4)
The `github` package and its interface (section 2), a personal-token implementation, PR creation from a card, and the Reviewer role running on every PR with comments going back to the worker.

### Slice 5 — the Integrator merge queue (B5.5)
The full sequence in `docs/architecture.md` section 8: dry-run merge with `git merge-tree --write-tree`, a backup branch, a merge inside a temporary worktree, running tests (affected only, in a monorepo), fast-forward the target branch only after tests pass, or abort with the target branch untouched and the card moved to Needs you with a reason. One card at a time per project. Never force push. Conflict resolution uses the context of every card involved; when the Integrator is not confident, it says so instead of guessing.

### Slice 6 — automatic sleep (B5.6)
The idle-triggered part only — pause/sleep/wake/pin themselves are already built (Phase 2). Add the idle timer, grouped sleep-warning reminders, the awake limit (evicting the oldest idle awake card when a new one must wake and the limit is full), and the notice actions (keep awake for the setting's length, sleep now, keep all, sleep all, dismiss). A session in Working or WaitingApproval never gets a sleep warning; a pinned card never sleeps.

### Slice 7 — the quality module (B5.8)
Project linters and Marshal's own built-in smell checks run against a card's diff (never against Marshal's own repo's configs — `.golangci.yml`/`.jscpd.json` are Marshal's own thresholds for its own code, not a smell profile for arbitrary agent-written code in a card's project; build a separate, per-project, per-language profile mechanism, matching `smell_profiles.spec_json`). New-or-worse filtering against the target branch, cached per commit, blocking findings sent back to the agent, warnings surfaced to the Reviewer, findings shown on the card with ask-to-fix and dismiss-with-reason. Sequence this last: it needs cards actually reaching Review (slices 2-5 wired) to have a real diff to check, and it has the least existing scaffolding of any slice in this phase.

---

## 5. Cut over

S8c (Plans), S23 (Notices), S26a (Settings: sleep), S27 (Settings: Roles) move from mock to daemon, each only when its own cutover gate passes.

---

## 6. Verification

The full gate (`node scripts/check.mjs`) is not yours to run (rule 1d): CI runs it on GitHub, and you prove your work with the fast checks of rule 1d, at the end of each slice (do not run `pnpm test:e2e` yourself — write the specs this phase's items ask for, and leave running the suite to the controller). A clean fixture merge and a conflicting fixture merge both behave as designed, proven by a test, using the stub agent, and once more with a real agent if the owner has approved that check by the time you finish (otherwise leave it for the controller).

---

## 7. Report

Fill in `phase-reports/phase-05-quality-loop.md` in place. Section 6 must record: the boundary chosen between the Phase 5 GitHub client and Phase 6's App (confirm you built it behind one interface), and the checkpoint-restore semantics you designed. Section 9 must list the smell-findings and checkpoints screens for the owner's sign-off, since neither has a prototype to point to.

---

## 8. Definition of done and stop conditions

**Done** when: roles are fully editable through the API and the real editor; a card waits in Planning until its plan is approved; limits move a card to Needs you with a reason and the stuck detector catches a repeated loop in a test; a card opens a real PR against a fixture repository and review comments reach the worker; clean and conflicting fixture merges behave exactly as `architecture.md` section 8 describes; idle cards sleep and wake with full context and working cards never sleep; fixture diffs produce the expected smell findings with old smells never blamed on the card; S8c, S23, S26a, and S27 are switched or listed as built-not-switched; and the report is complete.

**Stop and report immediately** if: you are about to sign in to GitHub for real or push to a real repository; the checkpoint or smell-findings screens have no existing components to build from; or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**Then continue.** Once this phase's report is complete, do not stop and wait to be told: read `phase-prompts/phase-06-ci-and-preview.md` and start it the same way, treating this phase's own report as the "prior phase" it asks you to read. There is no controller checkpoint between phases — the owner wants Phases 3 through 13 worked in one continuous run, and the controller verifies afterward, not between each one. If this phase itself hit something in "Stop and report immediately" that you could not resolve, finish and save this phase's report exactly as it stands, note the block plainly, and still move on to the next phase for whatever in it does not depend on the blocked item.
