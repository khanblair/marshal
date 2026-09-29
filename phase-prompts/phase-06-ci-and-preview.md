# Prompt: Phase 6 — CI and preview

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything.

**Baseline.** Read `git status --short`, and read `phase-reports/phase-05-quality-loop.md` in full — this phase completes the GitHub client Phase 5 started behind an interface. Keep a list of every file you create, modify, or delete, and put its counts in your report.

**Dates.** Use today's date from `date +%F`. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes.

---

## 0. The one-paragraph mission

Phase 6 completes GitHub (the App, replacing Phase 5's personal-token client's authority for webhooks and events), builds the CI monitor and its fix loop, "Simulate CI failure" as a real two-mode feature, local CI from workflow files, and live preview with screenshots. **The daemon has zero webhook-handling code today, but the test harness for it is already scaffolded**: `tools/hooks-replay` already knows how to replay a recorded webhook with its original headers so a signature check still matches, and `daemon/testdata/hooks/github/ping.json` is already a recorded fixture — the route and its signature verification are the actual gap, not the tooling around it. The route registration pattern also has no category for a public, unauthenticated-but-signed webhook route yet; you add one.

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
2. **The real mode of "Simulate CI failure" pushes a deliberately failing commit to a card's own branch and uses real GitHub Actions minutes.** Never run it in an automated test. It is clearly labeled, audited, and shown only in dev mode or with a Developer options setting on (off by default). If you build it, prove it exists and is gated correctly; do not trigger it for real yourself, ever, without the owner watching.
3. **You do not create the real GitHub App.** The owner does (`docs/backend-checklist.md` section 3). Every test uses a recorded webhook through `tools/hooks-replay`, never a live installation.
4. Do not delete files you did not create, except mock code you retire as part of a cutover.
5. Do not create documents beyond `phase-reports/phase-06-ci-and-preview.md` and what your slices need.
6. Do not edit generated files by hand.
7. **Never touch the owner's real daemons or data folders.**
8. Only macOS matters.
9. Do not weaken or delete a test to make it pass.

---

## 2. What's already built, and the real gaps

- **No GitHub-specific code exists in the daemon at all** (confirmed by a full grep — every "github" hit is Go's own module-path convention, not integration code). `daemon/go.mod` has none of `go-github`, `ghinstallation`, `chromedp`, or `robfig/cron` yet — all four are net new for this phase (`docs/library-docs.md` already names them, "pin at setup").
- **The route registration pattern has exactly three categories today, and none fits a webhook.** `daemon/internal/api/router.go` has `public` (no token, used only by `GET /v1/health`), `protected` (bearer token, used by every domain route), and `stream` (the WebSocket, its own auth). A webhook route must skip the bearer-token check like `public` does, but still needs to read the raw body to verify an HMAC signature *before* the JSON-body/size-limit wrapping runs — none of the three existing methods do this. Add a fourth, for example `signedWebhook`, and use it for `POST /hooks/github`.
- **`tools/hooks-replay` and `daemon/testdata/hooks/github/ping.json` already exist and are ready to use** — they were built ahead of the route itself. Your job is the route and its signature check; the replay tool is how you test it without a live GitHub account.
- **`NeedsReasonKindCIFailed` already exists** in `protocol/enums.go` (added during Phase 2's slice A). Reuse this exact value for a card that fails CI; do not add a second "ci failed" reason.
- **The Preview tab's UI is already built and its state names are already exact**: `apps/web/src/views/card/PreviewTab.tsx` and `preview-model.ts` define `PreviewState = "stopped" | "starting" | "running"` (matching N10 precisely) and already assume a per-card dev-server URL, port, and a before/after screenshot pair. Keep these three state names and the before/after shape exactly; do not redesign the tab.
- **The GitHub integration row in Settings is mock-only today** (`apps/web/src/mock/seed/settings.ts`: `{ id: "github", st: "connected", detail: "GitHub App installed on 3 repositories" }`), and the mock's `Integration.st` type has only three states (`connected`, `none`, `error`) with **no in-progress/installing state** — a real App install is a multi-step flow. Decide whether a transient client-side state is enough or whether the type needs a fourth value, and record it as a ruling; do not silently assume one or the other.

---

## 3. Data and wire

No `ci_runs` migration exists yet (columns already specified in `docs/architecture.md` section 10). New wire types for CI runs, workflow state, and preview state, each with a golden file, matching `PreviewTab.tsx`'s exact three-state shape for preview.

---

## 4. Your work, in this order

### Using your own subagents

If your harness can spawn subagents, use them **to draft, not to build.** You are the only one who compiles, runs a test, or touches a shared file (a migration, the route table, the wire types) — a subagent hands you a file back; it does not run the full build or the full test suite itself. This is what keeps a shared, uncommitted working tree safe, and what keeps the machine you're running on from being asked to run several heavy test suites at once.

- **Work in small waves, one slice at a time.** A wave of helpers drafts, or does read-only research; you read what comes back, integrate it yourself, run the scoped test for just that, and reconcile any shared file. Only then does the next wave start. Never let two waves write at the same time.
- The first wave for every slice is read-only research: confirm the current state of whatever this brief assumes — a prior phase's report can be stale by the time you actually reach it. Research never conflicts with anything, so this is always safe to run wide.
- You fix the shared contract yourself first — the `signedWebhook` router category and the wire types — before handing anything to a helper to draft against it.
- Slices 1 through 5 here are five largely independent modules (GitHub auth, the CI monitor, Simulate CI failure, local CI, and preview) that mostly only share the route table — a wave can draft several at once once the shared contract exists.
- Once a slice is fully integrated and its own scoped test passes, spawn a reviewer to read the diff — reading is cheap, and safe to run alongside your next wave's drafting.
- **Never run `pnpm test:e2e` yourself.** It is slow, and this phase does not need it to prove its own work — write whatever end-to-end specs this phase's items ask for, but leave running the suite to the controller, who runs it once across everything you and earlier phases built. Run `node scripts/check.mjs` once, at the end of the phase, from one place only, never mid-slice.

### Slice 1 — GitHub App auth and the webhook route (B6.1, B6.7)
Pin `go-github` and `ghinstallation/v2`. New `daemon/internal/integrations/github` module for App authentication (installation tokens) — this becomes the second implementation behind the interface Phase 5 already defined; do not duplicate the interface. Add the `signedWebhook` router category and `POST /hooks/github`, verifying the signature before the body is read further. Prove it with `tools/hooks-replay` against `daemon/testdata/hooks/github/ping.json` and any further fixtures you record. Add the GitHub connection test (App installed, repos visible, permissions for issues/PRs/Actions, a ping webhook reaching the route).

### Slice 2 — CI monitor and its fix loop (B6.2, B6.3)
Webhooks with a conditional-polling backup. On a failed run on a card's branch: rerun the failed jobs once; on a second failure, fetch only the failed step's log, trim it, send it as a message to the card's session; count rounds against the card's loop limits (Phase 5); on the limit, move the card to Needs you using the already-existing `ci-failed` reason.

### Slice 3 — Simulate CI failure, both modes (B6.4)
Synthetic mode: inject a failed run through the CI monitor's own normal path (same rerun, same trimmed log, same loop limits, same notice) — it never touches GitHub, and it can be tested in CI. Real mode: after confirmation, push a deliberately failing change to the card's own branch. Both modes audited; both shown only in dev mode or with Developer options on. The synthetic mode's recorded webhook is shared with `tools/hooks-replay`. Never run the real mode yourself (rule 2).

### Slice 4 — local CI from workflow files (B6.5)
Parse workflow files enough to run the test and lint steps locally, marking unsupported steps rather than failing on them.

### Slice 5 — live preview and screenshots (B6.6, B6.7)
Pin `chromedp`. Per-card dev server on its own port with an isolated browser profile, driven by the project's dev command; before/after screenshots attach to the card and to the pull request. Two cards preview at once without shared state. Keep the exact `stopped`/`starting`/`running` state names the UI already expects. If neither Chrome nor Edge is present, the check is skipped with a clear notice, never silently passed (`docs/library-docs.md`'s own note). Owner rule: you never launch a real browser to prove this slice. Test the driver against a fake, and list the real-browser check under "Needs the controller" for the owner.

---

## 5. Cut over

S13 (Card preview), S21 (Home: CI health), S29a (Integration: GitHub) move from mock to daemon, each only when its cutover gate passes.

---

## 6. Verification

The full gate (`node scripts/check.mjs`) is not yours to run (rule 1d): CI runs it on GitHub, and you prove your work with the fast checks of rule 1d, at the end of each slice (do not run `pnpm test:e2e` yourself — write the specs this phase's items ask for, and leave running the suite to the controller). CI behavior tested with recorded webhooks only; no test needs a live GitHub account, and none of your tests ever push the real Simulate-CI-failure mode.

---

## 7. Report

Fill in `phase-reports/phase-06-ci-and-preview.md` in place. Section 6 must record the ruling on the Integration row's transient state. Section 8 must state plainly whether the real Simulate-CI-failure mode was ever run (it must not have been, without the owner watching).

---

## 8. Definition of done and stop conditions

**Done** when: the App installs (against a fixture, never real) and receives events via `tools/hooks-replay`; CI badges and Home's CI health update from a replayed webhook; a failing fixture test is fixed by the stub agent through the fix loop, and the limits stop the loop when they should; both Simulate-CI-failure modes produce the same events a real failure would, proven with the synthetic mode only; two fixture cards preview at once without shared state; S13, S21, and S29a are switched or listed as built-not-switched; and the report is complete.

**Stop and report immediately** if: you are about to run the real Simulate-CI-failure mode without the owner present; you would need a live GitHub account for any test; or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**Then continue.** Once this phase's report is complete, do not stop and wait to be told: read `phase-prompts/phase-07-orchestration-and-memory.md` and start it the same way, treating this phase's own report as the "prior phase" it asks you to read. There is no controller checkpoint between phases — the owner wants Phases 3 through 13 worked in one continuous run, and the controller verifies afterward, not between each one. If this phase itself hit something in "Stop and report immediately" that you could not resolve, finish and save this phase's report exactly as it stands, note the block plainly, and still move on to the next phase for whatever in it does not depend on the blocked item.
