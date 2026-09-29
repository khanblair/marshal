# Prompt: Phase 4 — models

You are an autonomous senior engineer working in the repository at `/Users/kolaborateplatforms/BLAIR/marshal` on macOS. This file is your complete brief. Read all of it before you touch anything.

**Baseline.** Read `git status --short` now, and read `phase-reports/phase-03-control-and-safety.md` (Phase 3's report) in full: it tells you the real state of the daemon you are extending, since Phase 3 runs before you and may have changed things this brief cannot predict exactly. Do not revert, reword, or tidy anything you did not put there yourself; keep a list of every file you create, modify, or delete (a scratch file outside the repo), and put its counts in your report.

**Dates.** Use today's date from `date +%F` for every date you write. Do not guess.

A second engineer (the "controller") verifies your work and makes final fixes.

---

## 0. The one-paragraph mission

Phase 4 gives Marshal a **Built-in agent**: one more agent kind, alongside the CLI agents (Claude Code, Gemini CLI, Codex) Phases 0-1 already built, that talks directly to model provider APIs the owner pays for with their own keys, instead of shelling out to a CLI. You build: the `providers` package (one interface, an Anthropic adapter as the reference implementation, then an OpenAI-compatible adapter covering OpenRouter/DeepSeek/Ollama/LM Studio, then a native Gemini adapter), the built-in agent's own read/edit/run/tools loop on top of `agents.Agent` (the same interface the CLI adapters already implement — you do not need a new one), the OS keychain for provider keys, usage/cost tracking, per-project and global limits, and a **generic connection-test framework** that Phases 6, 8, and 9 will also use for their own integrations. You must never call a real provider API with a real key in any test.

---

## 1. Hard rules

1. **Never run a mutating git command** (`add`, `commit`, `push`, `checkout`, `restore`, `reset`, `stash`, `clean`, `rebase`, `merge`, `mv`, `tag`, `branch -d`, anything with `--force`). Allowed: `status`, `diff`, `log`, `show`, `ls-files`, `blame`.
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
2. **Never call a real provider API with a real key.** Every provider adapter test uses a recorded response or a fake HTTP server. If you believe you need a live key to verify something, stop and put it in "Needs the controller" instead.
3. Do not delete files you did not create, except mock code you retire as part of a section cutover (name every one in your report).
4. Do not create documents beyond `phase-reports/phase-04-models.md` (fill in the existing skeleton; do not replace it) and what your slices need in code. No scratch files in the repo.
5. Do not edit generated files by hand (`daemon/internal/store/db/*`, `packages/protocol/src/generated/index.ts`, `packages/ui/src/index.ts`, `packages/tokens/dist/*`). Change the source and run `pnpm gen`.
6. **Never run a real coding agent with a prompt**, CLI or built-in. Tests use fakes only.
7. **Never touch the owner's real daemons or data folders** (normal `47800` / `~/Library/Application Support/Marshal`; dev `47801` / `~/Library/Application Support/Marshal-dev`). Any daemon you start uses a throwaway `--data-dir` and a spare `--port`.
8. Do not add a dependency beyond the ones named in `docs/library-docs.md` for this phase (`anthropic-sdk-go`, `openai-go`, `google.golang.org/genai`, `zalando/go-keyring`) without stopping and describing why in your report; each still needs the seven-point check in `library-docs.md` section 1 recorded when you pin it.
9. Only macOS matters. Ignore Windows and Linux CI results.
10. Do not weaken or delete a test to make it pass.

---

## 2. What's already built that you extend

Read these before you write anything:
- `daemon/internal/agents/agent.go` — the `Agent` interface (`Start`, `Resume`, `Send`, `Interrupt`, `Events`, `Respond`, `Stop`, `Capabilities`). **It already fits a provider-backed agent**: nothing in it names a process or a PTY, `StartSpec.Env`'s own doc comment already says "for example a provider key", and `Capabilities.StructuredEvents` exists precisely so a non-CLI adapter can report itself the way `claude`/`gemini` do rather than the way `pty` does. Build the built-in agent as one more package, `daemon/internal/agents/builtin`, implementing this same interface — do not invent a parallel one.
- `daemon/internal/agents/eventsink.go` — the shared `EventSink` every session type already uses to turn provider/tool output into `agents.AgentEvent`s (`MessageChunk`, `ThoughtChunk`, `ToolCall`, `ToolCallUpdate`, `PlanUpdate`, `PermissionRequested`, `TurnEnded`, `Failed`, `Exited`). Reuse it; do not write a second one.
- `daemon/internal/agents/claude/turn.go` and `convert.go` — the shape to copy: one turn is one request, the provider's streamed content is converted into `ToolCall`/`ToolCallUpdate` events, and the turn ends with one `TurnEnded`. Your Anthropic/OpenAI-compatible/Gemini adapters convert their own SDK's stream the same way.
- `daemon/internal/agents/registry.go` — `Registry` is `map[protocol.AgentKind]Factory`, already kind-agnostic; registering `protocol.AgentKindBuiltin` needs no change to the registry itself.
- `daemon/internal/agents/catalog/specs.go` and `models.go` — **`Kinds()` returns only `{Claude, Gemini, Codex}` today, and `daemon/internal/protocol/agents.go`'s doc comment on `AgentCatalog.Agents` says outright "The built-in agent is not listed here."** This directly contradicts what the frontend already assumes: `apps/web/src/sync/agents.ts`'s `withBuiltIn` has its own comment saying the daemon's catalog does not list it *"until the built-in agent exists (build plan Phase 4)"*, and that the client-side fixture is removed *"when the daemon lists it."* **You must resolve this by making the daemon's catalog actually include a builtin row** (with its models computed dynamically from providers that have a valid key, not a fixed table the way the three CLI kinds work in `catalog/models.go`), and you must fix `protocol/agents.go`'s doc comment to say so. This is not optional cleanup: it is the literal "done when" of B4.1/B4.4, and the frontend cutover in slice 5 depends on it.
- `apps/web/src/mock/settings-types.ts`'s `Provider` interface — the exact contract to match: `{ id, name, st: "saved" | "empty" | "invalid", masked, models, error?, local? }`. `apps/web/src/mock/seed/settings.ts` seeds six providers (`anthropic`, `openai`, `gemini` saved; `deepseek` empty; `openrouter` invalid with an error string; `ollama` saved with `local: true` and a URL instead of a key in `masked`). **LM Studio has no seed row yet** — decide its exact shape (base URL, local flag) as part of slice 4, and record the decision as a ruling.
- `apps/web/src/views/settings/ProviderKeyForm.tsx` — the mutation UI today: one field (key, or a server-URL text field when `local`), a client-side minimum-length check, "Save key" and "Cancel". **There is no Test button and no Remove action in the UI yet**, and the masked value is computed client-side. Your routes return a server-computed masked value; the real key never reaches the client (`backend-inventory.md` N18).
- `daemon/internal/platform` token files (`tokens.go`) — **not reusable for provider keys.** They write a plaintext token to a 0600 file; a provider key needs the real OS keychain (`go-keyring`), a genuinely new dependency and a new module, not a variant of the existing file-based pattern.
- `docs/architecture.md` section 18 "Connection tests" — the `Test(ctx) (TestResult, error)` interface every adapter of every kind (agent, provider, integration, MCP server) implements. **This is shared infrastructure, not Phase-4-only**: `backend-inventory.md` N18 cross-references build-plan tasks 4.10, 5.19, 6.11, and 8.10, all needing the same "connect, disconnect, test, save the result, cooldown" mechanics. Build the generic mechanism once, here, in a place later phases can call into (for example a small `internal/connectiontest` package, or a shared helper on whatever module owns `integrations`), so Phase 6/8/9 only ever write the one `Test` method for their own adapter.

---

## 3. Data and wire

No migration exists yet for anything provider-related. Add one (`0005_providers.sql` or the next free number — check what Phase 3 added first) for whatever the provider/usage/limits tables need, following the existing `STRICT`, `TEXT` id, integer-millisecond convention (`daemon/internal/store/migrations/0001` to `0004`). `usage` and `limits` are already named and columned in `docs/architecture.md` section 10 — use those exact columns. No `Provider` wire type exists yet in `daemon/internal/protocol`; add one matching the UI contract above, with a golden file and a TypeScript test, the same pattern every prior phase's wire type followed.

---

## 4. Your work, in this order

### Using your own subagents

If your harness can spawn subagents, use them **to draft, not to build.** You are the only one who compiles, runs a test, or touches a shared file (a migration, the route table, the wire types) — a subagent hands you a file back; it does not run the full build or the full test suite itself. This is what keeps a shared, uncommitted working tree safe, and what keeps the machine you're running on from being asked to run several heavy test suites at once.

- **Work in small waves, one slice at a time.** A wave of helpers drafts, or does read-only research; you read what comes back, integrate it yourself, run the scoped test for just that, and reconcile any shared file. Only then does the next wave start. Never let two waves write at the same time.
- The first wave for every slice is read-only research: confirm the current state of whatever this brief assumes — a prior phase's report can be stale by the time you actually reach it. Research never conflicts with anything, so this is always safe to run wide.
- You fix the shared contract yourself first — the `providers` interface — before handing anything to a helper to draft against it.
- A wave can draft slice 2's three adapters (OpenAI-compatible, native Gemini) in parallel, one file each, once you've fixed the interface. Slice 4's keychain-and-CLI-command, provider routes, and queue/retry/fallback/usage pieces are similarly disjoint files a wave can draft at once.
- Once a slice is fully integrated and its own scoped test passes, spawn a reviewer to read the diff — reading is cheap, and safe to run alongside your next wave's drafting.
- **Never run `pnpm test:e2e` yourself.** It is slow, and this phase does not need it to prove its own work — write whatever end-to-end specs this phase's items ask for, but leave running the suite to the controller, who runs it once across everything you and earlier phases built. Run `node scripts/check.mjs` once, at the end of the phase, from one place only, never mid-slice.

### Slice 1 — provider interface + Anthropic adapter (reference implementation)
Define the `providers` package: one interface covering a non-streamed and a streamed call, tool use, and a per-provider thinking-mode map (`docs/library-docs.md`: "Thinking modes are mapped per provider in one place in `providers`. Do not spread provider-specific settings through other modules" — follow the same one-place convention `agents/acp/settings.go`'s `mapped()` already uses for ACP agents' thinking modes). Pin `github.com/anthropics/anthropic-sdk-go`, implement Anthropic as the only concrete adapter for now (it is also the built-in agent's default model). Tests: recorded HTTP responses only, no real key.

### Slice 2 — OpenAI-compatible fan-out + Gemini native
Pin `github.com/openai/openai-go` for one adapter that serves OpenRouter, DeepSeek, Ollama, and LM Studio by swapping the base URL (`docs/library-docs.md` section 2.4), and `google.golang.org/genai` for a native Gemini adapter. Decide LM Studio's exact provider row (base URL field, local flag, no key required) and record it as a ruling.

### Slice 3 — the built-in agent loop, and the catalog fix
Build `daemon/internal/agents/builtin` implementing `agents.Agent` (section 2): a read/edit/run/tools loop over the `providers` interface, reusing `EventSink` and emitting the same event vocabulary the CLI adapters do. Register it under `protocol.AgentKindBuiltin` in the `Registry`. **Fix `catalog.Kinds()`/`specFor`/`modelsFor` to actually include a builtin row** in `GET /v1/agents`, with dynamic models from configured providers, and correct `protocol/agents.go`'s stale doc comment. This retires the frontend's client-side `withBuiltIn`/`BUILT_IN_AGENT` shim in `apps/web/src/sync/agents.ts` — remove it in the same change, and update its test.
**Ruling to make and record:** `Agent.Resume` and `Capabilities.Resume` are written for an external program that kept its own session id to hand back (the CLI adapters' pattern). A built-in agent has no external process to resume. Decide how it satisfies `Resume` — most likely by replaying the persisted message history (`session_events`, built in Phase 2) rather than passing a session id anywhere — and write the ruling down; do not leave it implicit.

### Slice 4 — keychain, `marshal keys`, provider routes, queue, fallback, usage, limits
Pin `github.com/zalando/go-keyring`. Add `daemon/cmd/marshal/cmd_keys.go` (`set`, `list`, `remove` — it does not exist yet; `docs/development.md` currently says this is deferred, fix that sentence once it is not). Add the routes N18 asks for (save, remove, test a provider key; the real key never returns, only a server-masked value) using the `Provider` wire type from section 3. Add a per-provider request queue with retry (ten parallel requests on one key must not fail on rate limits), model fallback with a notice, usage and cost tracking feeding `daily_stats`, and cost/awake limits per project and globally through the settings calls.

### Slice 5 — the generic connection-test framework, screens, and chat titles
Build the shared `Test(ctx) (TestResult, error)` mechanics once (section 2's last bullet): running a test, saving `integrations.last_test_result_json`, a cooldown, and running automatically right after a connection is added. Implement it for providers first (a tiny request to the cheapest model, rate-limit headers read). Wire the Providers section's missing Test button, the Limits section, and the fallback/usage screens (`build-plan.md` tasks 4.10, 4.11 — check `docs/backend-inventory.md` section 6 for whether these need a fresh design or can be assembled from existing components before building a new screen; if genuinely new visual language is needed, stop and list it in your report rather than inventing it). Add chat titles: a new chat gets a short title from its first message, written by the cheapest configured model.

---

## 5. Cut over

Sections S19b (Home: cost), S26b (Settings: limits), S28 (Settings: providers) move from mock to daemon in this phase, each only when its own cutover gate (`docs/backend-checklist.md` 2.4) passes in full. Update `apps/web/src/data/sections.ts` and the register in `docs/backend-checklist.md` 2.2 together, remove the matching mock code, and update `docs/architecture.md`, `docs/project-structure.md`, and `docs/progress-tracker.md`.

---

## 6. Verification

The full gate (`node scripts/check.mjs`) is not yours to run (rule 1d): CI runs it on GitHub, and you prove your work with the fast checks of rule 1d, at the end of each slice (do not run `pnpm test:e2e` yourself — write the specs this phase's items ask for, and leave running the suite to the controller) (not after every edit). Coverage floors hold (70 percent daemon packages, 85 for `session`). No real provider API call anywhere in any automated test — grep your own new tests for a real base URL before you finish, and say so plainly in your report's rule confirmation.

---

## 7. Report

Fill in `phase-reports/phase-04-models.md` in place (it already has the ten section headings). Section 6 ("Rulings") must include the Resume-semantics decision and the LM Studio row shape. Section 8 ("Needs the controller") must include: the owner adding real provider keys with `marshal keys` and running a real connection test once you are done (`docs/backend-checklist.md` section 3), and any screen you judged needs fresh design rather than existing components.

---

## 8. Definition of done and stop conditions

**Done** when: the built-in agent completes a fixture task through a recorded-response provider; the catalog genuinely lists it; the scoped checks pass (the full gate and the end-to-end specs are run by CI and the controller, not by you); coverage floors hold; S19b, S26b, and S28 are switched (or explicitly listed as built-not-switched with a reason); no real provider call happened anywhere; and the report is complete.

**Stop and report immediately** if: you would need a real API key to prove something; a screen needs a visual decision the existing components cannot make (label it, do not invent it); or you are about to run a git command that changes the working tree.

When finished, print the path of the report and a five-line summary.

**Then continue.** Once this phase's report is complete, do not stop and wait to be told: read `phase-prompts/phase-05-quality-loop.md` and start it the same way, treating this phase's own report as the "prior phase" it asks you to read. There is no controller checkpoint between phases — the owner wants Phases 3 through 13 worked in one continuous run, and the controller verifies afterward, not between each one. If this phase itself hit something in "Stop and report immediately" that you could not resolve, finish and save this phase's report exactly as it stands, note the block plainly, and still move on to the next phase for whatever in it does not depend on the blocked item.
