# Prompt: Phase 11 — ecosystem

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything.

**Baseline.** Read `git status --short`, and read `phase-reports/phase-07-orchestration-and-memory.md` in full — Phase 7 built the daemon's role as an MCP **server**; this phase adds its role as an MCP **client**, a distinct and easily confused concept. Keep a list of every file you create, modify, or delete, and put its counts in your report.

**Dates.** Use today's date from `date +%F`. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes.

---

## 0. The one-paragraph mission

Phase 11 adds skills, an MCP manager for per-card external servers, and a plugin API. **This phase has the least existing scaffolding of any remaining phase** — no schema was ever anticipated for skills or MCP servers (unlike, for example, Phase 10's `templates`/`file_claims`, which the data model already columns), and the plugin API has no process model, manifest format, or permission model specified anywhere beyond one sentence in `docs/marshal-product-scope.md`. You design both from a small amount of spec plus this brief's recommendation; you do not silently improvise where a real decision belongs to the owner or architect.

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
2. **An imported skill or plugin is untrusted input, always.** No test or code path executes an imported skill's or plugin's content as anything other than data until it has gone through whatever sandboxing this phase builds; do not skip this "to get the demo working."
3. Do not delete files you did not create, except mock code you retire as part of a cutover.
4. Do not create documents beyond `phase-reports/phase-11-ecosystem.md` and what your slices need.
5. Do not edit generated files by hand.
6. **Never touch the owner's real daemons or data folders.**
7. Only macOS matters.
8. Do not weaken or delete a test to make it pass.
9. **Resolve the plugin-architecture decision (section 4, slice 3) before writing plugin-loading code**, and record your reasoning.

---

## 2. What's already built, and the real gaps

- **No schema exists for skills or MCP servers at all.** This phase needs a schema-design step Phase 10 did not, since the data model in `docs/architecture.md` section 10 never anticipated these tables.
- **The role editor already renders skills and MCP servers, read-only.** `apps/web/src/views/settings/RoleEditorFields.tsx`'s `RoleTags` shows `role().skills` and `role().mcp` as plain tags today — the shell exists, nothing behind it is real yet. Build the daemon side, then make these tags interactive (add/remove), rather than replacing the display.
- **The daemon's MCP client role is three distinct things, not one uniform proxy — read this carefully before you design anything:**
  1. **A health-check client.** The daemon itself connects to each configured external MCP server just to test it (`docs/architecture.md` section 18's connection-test table: "Connects, lists tools, and answers within the time limit").
  2. **A config passthrough for CLI and ACP agents.** `docs/marshal-product-scope.md` describes this as "Marshal passes the right MCP servers to each agent in the format that agent expects" — meaning the daemon fills in each agent's own native MCP configuration so **that agent's own process connects directly**, not the daemon proxying every tool call. The seam is already stubbed: `daemon/internal/agents/acp/handshake.go`'s `noServers()` is exactly where the card's configured external servers get passed to the ACP agent, in the ACP protocol's own `McpServer` shape.
  3. **A true client-and-merge role, in-process, only for the built-in agent.** Since the built-in agent (Phase 4) has no separate process to hand a config to, the daemon's own agent loop must itself act as the MCP client, merging Phase 7's internal-server tools and this phase's per-card external-server tools into one list before it sends anything to a provider.
  Do not build one uniform "daemon proxies every call" mechanism assuming it covers all three; it does not fit case 2, where the whole point is that the agent's own process talks to the external server directly.
- **The plugin API is genuinely underspecified.** Beyond "a stable API for adding new agents, integrations, roles, and views without changing the core" and "a sample plugin adds a new integration" as the done-when, nothing names a process model, a manifest format, or a permission model. See slice 3.

---

## 3. Data and wire

New migration for `skills` and `mcp_servers` (design the columns yourself, since the architecture doc does not specify them; follow the existing conventions — `STRICT`, TEXT ids, integer-millisecond times — and add a row to `docs/architecture.md` section 10 once you have designed them, so the doc catches up). New wire types for a skill, an MCP server's config and health, and per-card MCP attachment.

---

## 4. Your work, in this order

### Using your own subagents

If your harness can spawn subagents, use them **to draft, not to build.** You are the only one who compiles, runs a test, or touches a shared file (a migration, the route table, the wire types) — a subagent hands you a file back; it does not run the full build or the full test suite itself. This is what keeps a shared, uncommitted working tree safe, and what keeps the machine you're running on from being asked to run several heavy test suites at once.

- **Work in small waves, one slice at a time.** A wave of helpers drafts, or does read-only research; you read what comes back, integrate it yourself, run the scoped test for just that, and reconcile any shared file. Only then does the next wave start. Never let two waves write at the same time.
- The first wave for every slice is read-only research: confirm the current state of whatever this brief assumes — a prior phase's report can be stale by the time you actually reach it. Research never conflicts with anything, so this is always safe to run wide.
- You fix the shared contract yourself first — the new `skills` and `mcp_servers` migration you design in section 3 — before handing anything to a helper to draft against it.
- A wave can draft skills (slice 1) and the MCP manager (slice 2) in parallel — they touch almost nothing in common.
- **Resolve slice 3's plugin-architecture decision yourself, alone, before handing any of it to a helper.**
- Once a slice is fully integrated and its own scoped test passes, spawn a reviewer to read the diff — reading is cheap, and safe to run alongside your next wave's drafting — and specifically to confirm an imported skill or plugin is treated as untrusted input.
- **Never run `pnpm test:e2e` yourself.** It is slow, and this phase does not need it to prove its own work — write whatever end-to-end specs this phase's items ask for, but leave running the suite to the controller, who runs it once across everything you and earlier phases built. Run `node scripts/check.mjs` once, at the end of the phase, from one place only, never mid-slice.

### Slice 1 — skills (B11.1, B11.2)
A skills folder and table, attaching skills to roles, templates, and cards, and import with a content preview from a GitHub link — the preview shows exactly what will be added before the person confirms, since an imported skill is untrusted input (rule 2).

### Slice 2 — the MCP manager (B11.3, B11.4)
Per-card external MCP server configuration, the health-check client (role 1 in section 2), the config passthrough for CLI/ACP agents replacing `acp.noServers()` (role 2), and, once Phase 4's built-in agent exists, the in-process client-and-merge role (role 3). Broken servers show as broken, not silently missing.

### Slice 3 — the plugin API (B11.5) — resolve the architecture decision first
Three process models are on the table, and nothing in the docs picks one: an in-process Go plugin (the standard library's `plugin` package), a WASM plugin run through a runtime, or an out-of-process plugin over a defined RPC (stdio or a socket). **Recommendation, to use unless you find a concrete reason against it while building: out-of-process plugins over JSON-RPC on stdio, reusing `daemon/internal/proc`** (the same process-lifecycle helper every agent adapter already uses for start, stop, and health). The standard library's `plugin` package needs the plugin and the host built with the identical Go toolchain, has no Windows support at all, and is effectively a cgo-adjacent, single-load mechanism — this conflicts directly with the project's no-cgo rule and its three-platform target, and it means every plugin breaks on every daemon rebuild. WASM is a reasonable second choice if plugins later need to run untrusted code fast inside the daemon's own process, but it adds a host-call interface and a capability model this phase does not need yet. Treat a plugin the same way the daemon already treats an agent or an MCP server: an external process, talked to over a wire protocol, health-checked and restartable — never linked code. Build a sample plugin that adds one new integration, extending the existing `Test(ctx) (TestResult, error)` connection-test shape as its first real extension point. **Record your final choice and reasoning as a ruling even if you follow the recommendation exactly** — the next reader should not have to re-derive why.

---

## 5. Cut over

This phase switches no new section (S27's skills and MCP fields become real, inside the existing Roles section, rather than a whole section moving). Update `docs/backend-checklist.md`'s register only if you find a genuine section-status change; otherwise your evidence goes in the report's item table, not the register.

---

## 6. Verification

The full gate (`node scripts/check.mjs`) is not yours to run (rule 1d): CI runs it on GitHub, and you prove your work with the fast checks of rule 1d, at the end of each slice (do not run `pnpm test:e2e` yourself — write the specs this phase's items ask for, and leave running the suite to the controller). A specific test that an imported skill and a running plugin are both treated as untrusted input — neither one's content is ever executed as Marshal's own code, only as data or through the sandboxed process boundary this phase builds.

---

## 7. Report

Fill in `phase-reports/phase-11-ecosystem.md` in place. Section 6 must record the plugin-architecture choice and reasoning. Section 9 must list the skills and MCP manager screens for the owner's sign-off, since neither has a design yet.

---

## 8. Definition of done and stop conditions

**Done** when: skills load for the right agents and an import from a GitHub link shows what it will add before it is added; each card gets only its own listed MCP servers, and a broken server shows as broken; a sample plugin adds a new integration through the connection-test extension point; an imported skill and a running plugin are both proven untrusted by a test; and the report is complete.

**Stop and report immediately** if: you find yourself building a uniform proxy that does not fit the config-passthrough case in section 2; the plugin architecture cannot be resolved with the recommendation or a clearly better reason against it; or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**Then continue.** Once this phase's report is complete, do not stop and wait to be told: read `phase-prompts/phase-12-scale.md` and start it the same way, treating this phase's own report as the "prior phase" it asks you to read. There is no controller checkpoint between phases — the owner wants Phases 3 through 13 worked in one continuous run, and the controller verifies afterward, not between each one. If this phase itself hit something in "Stop and report immediately" that you could not resolve, finish and save this phase's report exactly as it stands, note the block plainly, and still move on to the next phase for whatever in it does not depend on the blocked item.
