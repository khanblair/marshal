# Prompt: Phase 9 — remote

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything.

**Baseline.** Read `git status --short`, and read `phase-reports/phase-08-automation.md` in full: this phase turns on Funnel, which depends on Phase 8's webhook signature verification already being solid. Keep a list of every file you create, modify, or delete, and put its counts in your report.

**Dates.** Use today's date from `date +%F`. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes.

---

## 0. The one-paragraph mission

Phase 9's milestone is "anywhere": the daemon joins the tailnet, phones pair by scanning a code, Telegram and Discord notify and take actions, and notifications reach a phone in under five seconds. **This is a genuine, security-relevant change to how the daemon binds**, not a routine feature. `docs/architecture.md` section 13 already states the target plainly: "the daemon listens on localhost and on its tailnet address only. Never on all interfaces." Today it binds to loopback only, at exactly one call site — you are adding a second, public-reachable bind, and Funnel on top of that exposes `/hooks/*` to the whole internet. **Do not turn Funnel on until Phase 8's webhook signature verification is proven solid**; if it is not, stop and say so rather than exposing an unverified route.

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
2. **Never sign in to a real Tailscale account, and never turn Funnel on against the public internet in any automated test.** Every test uses a local-only tsnet instance or a fake.
3. **Never use a real Telegram or Discord bot token in an automated test.**
4. Do not delete files you did not create, except mock code you retire as part of a cutover.
5. Do not create documents beyond `phase-reports/phase-09-remote.md` and what your slices need.
6. Do not edit generated files by hand.
7. **Never touch the owner's real daemons or data folders.**
8. Only macOS matters.
9. Do not weaken or delete a test to make it pass.

---

## 2. What's already built, and the real gaps

- **The daemon has exactly one network bind today, and it is loopback-only.** `daemon/internal/api/server.go` defines `loopback = "127.0.0.1"` with its own comment: "the only address the daemon listens on until a tailnet address is added." tsnet is a genuinely new, additive second bind, not a change to the existing one.
- **The origin/CORS check has no case for a phone reaching the daemon over the tailnet.** `stream_origin.go`'s `originAllowed`/`originPatterns` today recognize same-host requests, dev-mode localhost, and the desktop app's `tauri://` origin only. Decide and implement the tailnet policy (extend the pattern list, rely on same-origin serving, or add a distinct rule) as part of slice 1; do not assume the existing check already covers it.
- **The schema already anticipated pairing.** The `devices` table already has `kind` (`web`, `desktop`, `mobile`, `cli`, `dev`), `paired_at`, `last_seen_at`, `revoked_at`, and `users` already has a `tailnet_identity` column. `daemon/internal/api/accounts.go` already has a `newDevice`/`createDevice()` helper, but today it is only ever called to create the `cli` or `dev` device at startup — there is no pairing-code route and no path that creates a `web` or `mobile` device yet. Build on the existing device-row shape; do not redesign it.
- **The paired-devices list UI already exists** (`apps/web/src/views/settings/DevicesList.tsx`, embedded in the profile section): each row is `{id, name, kind, last}`, with a destructive "Remove device" confirm and a "Pair a device" button that today reveals a hardcoded mock code. Replace the hardcoded code with a real one; keep the row shape. **No tailnet-status or Funnel-status UI exists anywhere** — this part of build-plan task 9.9 is genuinely new screen surface, not an extension.
- **Zero webhook signature verification exists anywhere in the daemon's Go code today** (only in docs and in the `tools/hooks-replay` test harness). Phase 8 is where the real route and its verification must already be proven solid, through the existing hooks-replay tool against recorded fixtures, before you turn Funnel on here. Read `phase-reports/phase-08-automation.md` and confirm this before slice 2.

---

## 3. Data and wire

No migration needed beyond what Phase 1 already built for `devices`/`users`. Add a pairing-code wire type and route payloads only.

---

## 4. Your work, in this order

### Using your own subagents

If your harness can spawn subagents, use them **to draft, not to build** — but this phase is mostly sequential by nature (the bind, then pairing, then Funnel, in that order, each depending on the last). The one real split is slice 4's Telegram and Discord bots, which share nothing. You are the only one who compiles, runs a test, or touches a shared file — a subagent hands you a file back; it does not run the full build or the full test suite itself.

- **Work in small waves, one slice at a time.** A wave of helpers drafts, or does read-only research; you read what comes back, integrate it yourself, run the scoped test for just that, and reconcile any shared file. Only then does the next wave start. Never let two waves write at the same time.
- The first wave for every slice is read-only research: confirm the current state of whatever this brief assumes, and re-confirm that Phase 8's webhook signing is genuinely solid before slice 3 turns Funnel on. Research never conflicts with anything, so this is always safe to run wide.
- You fix the shared contract yourself first — the tailnet listener and the origin policy — before handing anything to a helper to draft against it.
- A wave can draft slice 4's Telegram and Discord bots in parallel, one each, since they share nothing.
- Once a slice is fully integrated and its own scoped test passes, spawn a reviewer to read the diff — reading is cheap, and safe to run alongside your next wave's drafting.
- **Never run `pnpm test:e2e` yourself.** It is slow, and this phase does not need it to prove its own work — write whatever end-to-end specs this phase's items ask for, but leave running the suite to the controller, who runs it once across everything you and earlier phases built. Run `node scripts/check.mjs` once, at the end of the phase, from one place only, never mid-slice.

### Slice 1 — the tsnet node and the origin policy (B9.1)
Pin `tailscale.com/tsnet`. Add the tailnet listener alongside the existing loopback one (additive, never replacing it), per `docs/architecture.md` section 13. Measure the binary-size impact against the download budget (`docs/library-docs.md` flags this explicitly) and record the number. Resolve and implement the origin-check gap from section 2.

### Slice 2 — device pairing (B9.1, B9.2's device half)
A pairing-code route: generate a short-lived code, exchange it for a real device token, create the device row through the existing `createDevice()` helper with the right `kind`, and a revoke route (the schema's `revoked_at` already supports this). Replace `DevicesList.tsx`'s hardcoded mock code with the real flow; keep its row shape.

### Slice 3 — Funnel and serving (B9.2's Funnel half)
**Only after slice 2 and after confirming Phase 8's webhook signing is solid.** Expose only `/hooks/*` publicly through Funnel, with every request still signature-verified as Phase 8 built. Serve the responsive UI over the tailnet.

### Slice 4 — Telegram and Discord (B9.3)
Pin `github.com/go-telegram/bot` and `github.com/bwmarrin/discordgo`. Notices, approvals, actions, and voice notes for both, each with its own connection test against a fake bot server, never a real token.

### Slice 5 — notification routing, remote machines, and the status screens (B9.4, B9.5, B9.9)
Channels per event type, grouped notices, an event reaching a phone notice in under five seconds (measure it). A second daemon reachable on the tailnet, a project runnable on it from the laptop UI. The pairing-code display, tailnet status, and Funnel status screens (genuinely new UI per section 2) — check with the owner before drawing them if the existing profile/settings components are not enough.

---

## 5. Cut over

S2b (Devices and Tailscale identity), S29f (Telegram), S29g (Discord), S31b (Onboarding: connect from anywhere) move from mock to daemon, each only when its cutover gate passes.

---

## 6. Verification

The full gate (`node scripts/check.mjs`) is not yours to run (rule 1d): CI runs it on GitHub, and you prove your work with the fast checks of rule 1d, at the end of each slice (do not run `pnpm test:e2e` yourself — write the specs this phase's items ask for, and leave running the suite to the controller), including end-to-end tests that run against a daemon reached over the tailnet (not just loopback), and the security rules in `docs/architecture.md` section 13 specifically tested (the daemon never listens on all interfaces).

---

## 7. Report

Fill in `phase-reports/phase-09-remote.md` in place. Section 8 must state plainly whether Funnel was ever turned on for real (it must not have been, without the owner present) and whether a real Telegram or Discord token was ever used (it must not have been).

---

## 8. Definition of done and stop conditions

**Done** when: the daemon is reachable on the tailnet with no separate Tailscale install, binding never happens on all interfaces; a phone pairs by scanning a real code and a revoked device loses access at once; only `/hooks/*` is public through Funnel and every request is still signature-verified; every view and action works on a real phone and tablet through Tailscale (the controller confirms this manually — write the exact steps); Telegram and Discord approve and create cards through their connection tests; an event reaches a phone notice in under five seconds, measured; S2b, S29f, S29g, and S31b are switched or listed as built-not-switched; and the report is complete.

**Stop and report immediately** if: Phase 8's webhook verification is not yet solid when you reach slice 3; you would need a real Tailscale account, Telegram bot, or Discord bot for any test; or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**Then continue.** Once this phase's report is complete, do not stop and wait to be told: read `phase-prompts/phase-10-advanced-cards.md` and start it the same way, treating this phase's own report as the "prior phase" it asks you to read. There is no controller checkpoint between phases — the owner wants Phases 3 through 13 worked in one continuous run, and the controller verifies afterward, not between each one. If this phase itself hit something in "Stop and report immediately" that you could not resolve, finish and save this phase's report exactly as it stands, note the block plainly, and still move on to the next phase for whatever in it does not depend on the blocked item.
