# Prompt: Phase 13 — retire the mock and accept

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything.

**Baseline.** Read `git status --short` now. Everything you find already modified or untracked belongs to work that came before you (later phases' scoping docs, other phase prompts and reports, or an in-progress build). Do not revert, reword, or tidy anything you did not put there yourself. Keep a list of every file you create, modify, or delete (a scratch file outside the repo), and put its counts in your report.

**Dates.** Use today's date from `date +%F` for every date you write. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes. Your job is a correct, evidenced closing pass.

---

## 0. The one-paragraph mission

Phase 13 is the last phase. By the time it starts, every section in `docs/backend-checklist.md` 2.2 should already say `Daemon`: Phases 1 through 12 have moved every part of the app off the mock. **Phase 13 has no new product feature.** It is five closing items (`docs/backend-checklist.md` B13.1 to B13.5): delete the mock, run the whole test pyramid once more, do a real-device pass, bring every doc and the tracker current, and commission a final whole-project review. You verify the premise (every section really is `Daemon`) before you delete anything, and you write an unusually careful report, because after this phase there is no earlier state to fall back on for what the mock used to prove.

---

## 1. Hard rules (read twice)

1. **Never run a mutating git command.** Allowed: `git status`, `git diff`, `git log`, `git show`, `git ls-files`, `git blame`. Forbidden: `add`, `commit`, `push`, `checkout`, `restore`, `reset`, `stash`, `clean`, `rebase`, `merge`, `mv`, `tag`, `branch -d`, and anything with `--force`.
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
2. **Do not delete anything until B13.1's precondition is proven** (section 5 below). If even one section is not `Daemon`, stop, do not delete the mock, and report exactly which sections and why.
3. To delete a file, use `node -e "require('fs').rmSync(path, {recursive: true})"` on a path you are sure of. Never delete `design/` (the design prototype stays forever, as the reference), `daemon/testdata/repos/`, anything under `.git/`, `node_modules/`, or any file this brief did not name.
4. Do not create documents beyond `phase-reports/phase-13-retire-the-mock-and-accept.md` (fill in the existing skeleton; do not replace it) and whatever this brief's items explicitly ask for. Do not create scratch files in the repo.
5. Do not edit generated files by hand (`daemon/internal/store/db/*`, `packages/protocol/src/generated/index.ts`, `packages/ui/src/index.ts`, `packages/tokens/dist/*`). Change the source and run `pnpm gen`.
6. **Never run a real coding agent with a prompt.** `claude --version` and `--help` are fine. Every test uses the stub agent.
7. **Never touch the owner's real daemons or data folders**: normal port `47800` / `~/Library/Application Support/Marshal`, dev port `47801` / `~/Library/Application Support/Marshal-dev`. Any daemon you start uses a throwaway `--data-dir` and a spare `--port`.
8. **The real-device pass (B13.3) is the owner's, not yours.** You cannot hold a phone. Write the exact steps and hand them to the controller; do not simulate it and call it done.
9. **The final review (B13.5) needs a reviewer that did not write the code** (`code-standards.md` 12.5, two reviews for `security`, `harness`, `integrator`). You may run the first pass yourself only if you did not write the code you are reviewing; otherwise, do the mechanical parts (gather the diff, run the checklist against it) and hand the judgment calls to the controller.
10. Only macOS matters. Ignore Windows and Linux CI results.
11. Do not weaken or delete a test to make it pass, except a mock-only test you are retiring as part of B13.1 (list every one you remove).

---

## 2. What "done" looks like before you start

Every phase from 0 to 12 has its own report in `phase-reports/` (or, for Phases 0 to 2, in `docs/progress-tracker.md`). Read `docs/progress-tracker.md`'s "Phase status" table and every `phase-reports/phase-*.md` file. Every phase must show a status the controller recorded as verified (not merely "the builder said so"). If a phase's report says "not switched" for any section, or is silent, that section is not ready for B13.1: stop and report it, and move to the parts of this brief that do not depend on it (the final review's mechanical prep, docs corrections) rather than deleting anything.

---

## 3. Your work

### Using your own subagents

If your harness can spawn subagents, use them **to gather evidence, not to decide.** You are the only one who deletes anything, runs the full test pass, or writes the closing judgment in the report — a helper researches and reports back; it never deletes or makes the final call itself. Never let two helpers write to the same file at the same time, and never delete anything until B13.1's precondition is proven.

- B13.2's test layers (unit, integration, contract, end-to-end, budgets) can be checked by a wave of read-only helpers in parallel, once you have confirmed B13.1's precondition and finished the deletion yourself.
- B13.4's doc sweep for stale "not built yet" language splits naturally, one helper per doc, reporting back what it found; you make the actual edits.
- B13.5's mechanical evidence-gathering (the diff since the Phase 1 baseline, package-by-package smell reports, every leftover `TODO`) can run alongside B13.2's checks; the judgment part of B13.5 stays with a single reviewer who did not write the code, as rule 9 already requires.
- **Never run the full gate (`node scripts/check.mjs`) or `pnpm test:e2e`** (rule 1d). They run on GitHub CI, and locally only when the owner asks.

### B13.1 Remove the mock

**Precondition (verify first, with evidence):** every row in `docs/backend-checklist.md` section 2.2's register says `Daemon`. Quote the table in your report. If any row is not `Daemon`, stop here; do not touch the mock; report exactly which rows and, from the phase reports, why.

**Size of the job, so you are not surprised:** as of Phase 2's start, `apps/web/src/mock/` held 81 TypeScript files and about 9,300 lines (root files: `agents.ts`, `engine.ts`, `index.ts`, `marshal.ts`, `selectors.ts`, `state.ts`, `storage.ts`, `types.ts`, `state-types.ts`, `settings-types.ts`, `constants.ts`, `ids.ts`, `card-key.ts`, `clock.ts`, `deco.ts`, `deco-msgs.ts`, `attach.ts`, `bind.ts`, `format.ts`, plus subfolders `actions/`, `dom/`, `seed/`, `sim/`, and others). By Phase 13 most of this should already be gone or migrated section by section (`docs/backend-checklist.md` 2.4 rule 6: a section's mock code is removed in the same change that cuts it over). Re-measure with `find apps/web/src/mock -name "*.ts" -o -name "*.tsx" | wc -l` before you start, and report the real number: it may be much smaller than 81 by now, or (if a phase left mock code behind "because it still passed `knip`") it may not be.

**Do, per `docs/backend-checklist.md` 2.5:**
1. Delete the mock folder, its seed data, its simulation, and the differential test harness that loads `design/store.js` (grep `apps/web/src` for anything importing `design/store.js` or comparing against it — the "twin" tests).
2. Keep `design/` itself untouched: it stays as the visual reference forever, only the code that ran the prototype's data goes.
3. The `M` store stays (the screens use it); confirm after deletion that it holds only mirrored daemon data and screen state, with no import left pointing at `~/mock`.
4. Tests that used mock data now use typed fixture builders made from the same golden files the daemon tests produce; end-to-end tests use the dev daemon with the fixture project (already the pattern every phase before you should have followed — you are only removing what a phase left behind, not inventing a new one).
5. Update `docs/project-structure.md`, `docs/design-port.md`, and `docs/ui-registry.md` to match: `apps/web/src/mock` is gone from the file listing, and `design-port.md`'s "Known deviations" table is checked against the CURRENT UI (a deviation approved for the mock's benefit that no longer applies is marked so, not silently dropped).

**Done when:** `pnpm smells` (which runs `knip`) reports nothing left over from the mock, every test that used to import mock data now uses a fixture builder or the dev daemon, and the design files remain exactly as they were.

### B13.2 Full test pass

These are the commands the controller runs, once, from the repo root, and only when the owner asks. You do not run them (rule 1d): you confirm each is wired and list them under "Needs the controller". `pnpm gen` twice (the second run changes nothing); `node scripts/check.mjs` (all eleven steps); `pnpm test:e2e` at three sizes and two themes is the controller's to run, not yours (owner rule: you never run a browser); confirm only that the suite's shape is still three sizes and two themes by reading its config, do not narrow it, and list the command under "Needs the controller"; the nightly real-agent smoke test path (`docs/build-plan.md`'s testing-approach table: "Real agent smoke tests | Phase 1 | Each supported CLI version, run nightly" — you cannot run this for real per rule 6, so read whatever nightly workflow exists and report whether it is wired, not run it yourself); budgets (`node scripts/budgets.mjs` after a `pnpm build`); and confirm the CI matrix covers macOS, Linux, and Windows (you only need the macOS result to be green, per the project's macOS-first rule, but confirm the other two jobs still run and are not accidentally deleted).

**Done when:** every layer in `docs/backend-checklist.md` 2.6's table is wired and passes in CI on GitHub (or, if the owner asked for a local run, is green on this machine with the numbers in your report), and the commands for the controller are listed.

### B13.3 Real-device pass

You cannot do this yourself. Write the exact steps for the controller: which phone and tablet, which Tailscale setup (from Phase 9), which views and actions to try, and what "passes" means for each. Put it in your report's "Needs the controller" section, not as something you attempted.

### B13.4 Docs and tracker

1. Every doc matches what was built. In particular, `docs/architecture.md` accumulated forward references across every phase ("nothing writes those states yet", "Phase 1 does not build this", "the routes built today are..."). Grep the whole file for "not yet", "Phase 1", "Phase 2", "Phase 3", "does not build", "not built yet", "planned" and read each hit: if the feature described is now built (it should be, by Phase 13), rewrite the sentence to say so plainly, in the same voice as the rest of the doc. If anything is still not built, that contradicts B13.1's precondition — stop and report it rather than silently leaving the doc wrong.
2. Every open decision in `docs/backend-checklist.md` section 8 and every open question in `docs/progress-tracker.md`'s "Open questions" table is closed (answered and dated) or explicitly logged as still open with why it's acceptable to ship without it.
3. The tracker's phase-status table, file-changes log, and doc-changes log are complete for every phase.
4. Delete this repo's `phase-prompts/` and `phase-reports/` folders' `INDEX.md` entries for phases 0 through 13 by marking every row "Done" (do not delete the folders themselves without asking — the controller decides whether to keep them as a build history or remove them once merged).

### B13.5 Final review

A whole-project review against `docs/code-standards.md` section 12.5's eight questions (names say what things do; comments still true; one change spread across many modules without an abstraction; a function using another module's data more than its own; data passed through layers that never use it; anything added "for later" that nothing uses; clever code where a standard approach exists; related code kept close together), with two separate reviews for the `security`, `harness`, and `integrator` modules specifically (per `code-standards.md` 9.3). You may do the mechanical gathering (diff since Phase 1's `fd2937c`/`a11b2e4` baseline, package-by-package smell reports, a list of every `TODO`/`FIXME` left in the daemon and web source) yourself; the judgment calls on each of the eight questions belong to a reviewer who did not write the code, so hand that part to the controller with your gathered evidence attached, not a self-assessment.

---

## 4. Report

Fill in `phase-reports/phase-13-retire-the-mock-and-accept.md` (it already exists with ten section headings; do not replace the file, edit it in place). Update it after B13.1, again after B13.2, and finish it after B13.4 and B13.5's mechanical parts. Section 3 is "Retirement evidence", not "Cutover evidence" — one row per file or folder you removed, with a reason, plus the final `knip` result. Section 8 ("Needs the controller") is where B13.3's real-device steps and B13.5's judgment review go.

---

## 5. Definition of done and stop conditions

**Done** when: B13.1's precondition was checked and is true (or you stopped and reported which sections are not ready); the mock is gone and `knip` is clean; the full test pass in B13.2 is green with numbers recorded; B13.3's steps are written for the controller; B13.4's doc sweep is complete with zero remaining "Phase N: not built yet" sentences describing something that is in fact built; B13.5's evidence is gathered; and the report is complete.

**Stop and report immediately** if: any section is not `Daemon` yet (do not delete the mock); a test you would need to remove is not clearly mock-only (ask, in the report, rather than guess); or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**This is the last phase.** There is no next file to continue to. Once this report is complete, wait for the controller — this is the one point in the whole run where that is the right thing to do.
