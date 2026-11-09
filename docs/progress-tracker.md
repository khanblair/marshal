# Progress tracker

This document is the live status of the Marshal build. It is the first thing to check at the start of a work session and the last thing to update at the end.

Task IDs match `build-plan.md`.

---

## How to update this document

At the end of every work session, whether by a person or an agent:

1. Update the status of every task you touched.
2. Add a short entry to the session log: what was done, what is next.
3. Add any decision to the decisions log, with the reason.
4. Add any new question or blocker to the open questions list.
5. If a task was split, add the new tasks to `build-plan.md` and here.
6. If you added, moved, renamed, or deleted a file, update `project-structure.md` and add a row to the file changes log.
7. If you changed a doc, add a row to the doc changes log.
8. Keep each log to its latest 10 entries (5 sessions). Move older entries to `archive/progress-tracker-archive-2026-10.md`; read the archive only on demand.

### Status values

| Status | Meaning |
|---|---|
| Not started | No work yet |
| In progress | Someone is working on it |
| In review | Work is done and waiting for review |
| Done | Merged, and its done criteria pass |
| Blocked | Cannot continue. The reason goes in open questions. |

---

## Current state

| | |
|---|---|
| **Current phase** | Phase 4: Models (S19b, S26b, and S28 switched; built and in review for the controller) |
| **Next milestone** | Phase 4's gate: no real model API in CI, and Home's cost numbers match the usage table (then Phase 5) |
| **Last updated** | 2026-11-10 |
| **Current focused change** | Fix the remaining PATH test and order GitHub CI, dual-platform builds, and publication automatically for requested v0.1.3. |

---

## Phase status

| Phase | Status | Notes |
|---|---|---|
| 0. Foundation | Done locally | Done locally: Go tools, daemon module, protocol package, Go lint and smells, fixtures, stub agent, budgets, dev mode. macOS CI green. Windows and Linux are not a blocker (macOS first, decision 4). Deferred: keychain entries (library check first), desktop shell and release (after Phase 2). |
| 1. First card | Done (2026-09-26) | Built, tested, and pushed. Item by item in "Phase 1 delivered" below. Built and tested on this Mac: the SQLite store with embedded migrations, the event bus with replay, the single-instance lock and the per-user service installers, projects and boards with cards, all Git access through `gitx`, the agent interface with the ACP, Claude Code, Gemini CLI, PTY, and Codex-detection adapters, the session manager with resume and bounded logs, the `/v1` API and the `/v1/events` stream with token auth, the client data layer, the project-plus-number card key, the prototype fixture, and the cutover of sections S1, S3, and S4. The two checks that need the real machine were run on 2026-09-26 with the owner's approval: a real Claude Code session on a card (it answered), and the login item (installed, restarted after a crash, removed). The cards, chats, checklists, and everything else still on the mock are Phase 2 and later. |
| 2. Core UI | All 16 sections switched; not yet verified | Scoped 2026-09-26 into eight slices (A cards, B inside a card, C project chats, D Home numbers, E you and your preferences, F finding and starting, G session hold, H prove it). **All sixteen sections are on the daemon** (S2a, S5a, S6a, S7a, S7c, S8a, S9, S10, S11, S17, S18, S19a, S20, S24a, S31a, S32), and `data/sections.ts` matches the register in `backend-checklist.md` 2.2. **Not yet verified:** the full gate (`node scripts/check.mjs`, last run when five of eleven steps failed, since fixed one by one but not re-run together), the browser suite (`pnpm test:e2e`: eleven specs, none run since the last changes), the size and memory budgets after the terminal work, and the owner's own hands-on check. No B2.x box is ticked. **Known limits:** the terminal view runs Claude Code only in real mode (Gemini CLI and Codex answer that they have no terminal view) and draws escape-stripped text, not a full terminal emulator; cards an agent creates from a project chat wait for Phase 7 (Q17); a project chat's agent works in the repository folder itself, with the permission mode the chat was made with, which the owner may want to make safer until Phase 3's approvals exist. Open questions Q17, Q20, and Q21 are still waiting for the owner, each with a proposed answer; Q19 was built as proposed |
| 3. Control and safety | In review (updated 2026-11-03) | S7b (bypass) and S8b (approvals) are on the daemon, matching the section register. Approval IDs and resolved history are covered by HTTP tests. Native Claude/Codex approvals passed real daemon-driven lifecycle checks. Two independent Luna review passes covered the current approval/harness changes and their fixes. No B3.x UI gate is claimed; browser verification remains with the owner. |
| 4. Models | In review (2026-09-27) | Built and green; its report is `phase-reports/phase-04-models.md` (since deleted). The `providers` package (one neutral call interface; Anthropic, one OpenAI-compatible, and a native Gemini adapter), the built-in agent (`internal/agents/builtin`, now genuinely listed by `GET /v1/agents` with models from the providers that are set up), the OS keychain and `marshal keys`, the `Provider` wire type and its routes, usage/cost tracking into `daily_stats`, cost and awake limits, the shared connection-test framework (`internal/connectiontest`), and chat titles written by the cheapest configured model. **All three sections switched**: S19b (Home cost), S26b (Settings limits), and S28 (Settings providers), each with its §2.2 register row moved in the same change. No B4.x box is ticked — that is the controller's, after verification. Known: the full gate, the end-to-end specs, and the budget numbers are the controller's (CI); `pricing.go`'s table is written from memory and unverified against published list prices, so `Cheapest`'s ordering is only as good as that table; `openai-go v1.12.0` deviates from `library-docs.md` section 1's "a release in the last 12 months" (`v2.7.1` is the maintained alternative); and B4.3's fallback and usage **screens** (task 4.11) are not built, because they need a fresh design. |
| 5. Quality loop | In progress (2026-09-27) | Slices 1–3 (roles, plan-first mode, limits/stuck/checkpoints) are built in the working tree from an earlier run; `S8c` and `S27` read Daemon. This run added the first half of slice 4 (`internal/github` — the `Client` interface + a personal-token HTTP client — and `internal/pullrequest`, which opens a card's branch as a real PR, records the link, and moves it to In review, wired as `POST /v1/cards/{id}/pull-request`) and **completed slice 5**: `gitx/merge.go` (dry-run merge, backup branch, detached merge worktree, `--no-ff` merge, forward-only `FastForwardRef`, abort) and `internal/integrator` (the merge queue, one card at a time per project, `POST /v1/cards/{id}/merge`), both tested. **Not built: the Reviewer on every PR, slice 6 (automatic sleep + notices), slice 7 (quality module), the `S23`/`S26a` cutovers, and the end-of-phase gate.** Details in `phase-reports/phase-05-quality-loop.md` (since deleted). |
| 6. CI and preview | Not started | |
| 7. Orchestration and memory | In progress (2026-09-28) | Slices 1–5 are now built in the working tree; the hand-off between the two most recent sessions had understated this, and the report was corrected to match what is actually on disk. **Slice 1 (memory store core):** migration `0019_memory.sql` (`notes` — now holding both card notes and lessons, told apart by `kind` — `file_claims`, `mcp_servers`, `skills`), the `note` and `lesson` wire types with goldens, and `internal/memory` (vault, notes, lessons, claims, search, the vault watcher, the project-memory remover). **Slice 2 (the internal MCP server):** `internal/mcpserver` registers all fifteen tools of `architecture.md` 11.4 in that table's order (`post_note`/`read_notes` is one table row but two tools); every call is permission-checked and a refused call is explained, never silently dropped. Five of them (`list_checklists`, `tick_checklist_item`, `read_comments`, `read_attachment`, `post_comment`) are registered and callable but answer "not built yet" until Phase 10 gives Marshal checklists, comments, and attachments to read; `search_codebase` is wired for real (`internal/codemap`, below) and is not one of the five. `search_memory` now also searches a project's lessons alongside its cards' notes. **Slice 3 (StartSpec plumbing):** `agents.StartSpec.MCPServers`, the ACP handshake sends the real list instead of `noServers`, `internal/session/attach.go` attaches after Start and detaches on every ending path. **Slice 4 (the codebase map):** `internal/codemap` reads symbols with universal ctags as an external binary, one index per project, updated by file, wired into `search_codebase` through `cmd/marshald/main.go`'s real `codeMap` instance. **Slice 5:** board awareness (`session/aware.go`) and file claims (`memory/claims.go`, `mcpserver/tools_claims.go`) are built - a claim's own answer names any card already holding the same path at once, and the incumbent card sees the new claim the next time its own board_status or awareness summary reads the project's claims, which is a real but delayed second half rather than a push to the other card. An "Orchestrator" role is seeded (`internal/roles/seed.go`) and `create_card`'s `dependsOn` field records dependency links at creation time (migration 0020's `card_links`, read back by `board_status`) - the mechanism a goal-planning agent needs is there, but no automated "goal in, dependent cards out" pipeline was built or tested end to end; the planning is left to the role's own instructions plus this tool, the way an LLM-driven role is meant to work, not to a bespoke planner. Card handoff (`session/handoff.go`, `protocol/handoff.go`) and session search (wired into `internal/search`) are built. The memory-viewer and lessons screens (build-plan task 7.13) are **not** - the lessons backend is ready for them but no web screen exists yet. The context-budget meter and pinned files remain out of scope (blocked on missing adapter and data-model support - see the report §7). **Cutovers: S29b (Obsidian) is switched this run; S14 (card notes) and S24b (session/note search) have complete daemon sides but no web side yet.** No B7.x box is ticked - that is the controller's, after verification, and nothing after the coordinator disabled mid-session checks has been run at all. Details in `phase-reports/phase-07-orchestration-and-memory.md` (since deleted). |
| 8. Automation | Not started | |
| 9. Remote | Not started | |
| 10. Advanced cards | Not started | |
| 11. Ecosystem | Not started | |
| 12. Scale | Not started | |

---

## Phase 0 tasks

| ID | Task | Status | Owner | Notes |
|---|---|---|---|---|
| 0.1 | Write the project docs | In review | | Twelve docs and the docs index drafted, waiting for sign off |
| 0.2 | Monorepo layout | In review | | `pnpm-workspace.yaml` lists `apps/*`, `daemon`, `packages/*`, and `tools/*` |
| 0.3 | Go module with `marshald` and `marshal` | In review | | `daemon/` builds with `CGO_ENABLED=0`; `marshal` has `status`, `token`, `service`, `dev reset`, and `version` |
| 0.4 | SolidJS app with Vite and Tailwind | In review | | `apps/web/`, ported from the prototype and cut over for S1, S3, and S4 |
| 0.5 | Tauri v2 shell | Not started | | Deferred until after Phase 2 (decision 2026-09-25); `apps/desktop/` does not exist yet |
| 0.6 | Tokens package | In review | | `packages/tokens`, generated and checked for contrast in CI |
| 0.7 | Go to TypeScript type generation | In review | | tygo writes `packages/protocol/src/generated/index.ts`; `pnpm gen` is reproducible |
| 0.8 | CI pipeline | In review | | `.github/workflows/ci.yml` runs the check; the Windows and Linux results are not a blocker, and the macOS run is reported green |
| 0.9 | Budget harness | In review | | `tools/budgets` measures the daemon's idle RAM and CPU; `scripts/budgets.mjs` measures the web build |
| 0.10 | Fixture repos | In review | | `daemon/testdata/repos/` has `small-repo` and `monorepo`, loaded as the prototype fixture in dev mode |
| 0.11 | Stub agent | In review | | `tools/stub-agent`, a scripted ACP agent with resume and JSON scenarios |
| 0.12 | pnpm workspace and root scripts | In review | | See `development.md` section 4 |
| 0.13 | Daemon dev mode | In review | | `--dev` gives its own data folder, port 47801, dev token, and the fixture |
| 0.14 | Release pipeline with signing and updater | In review | | v0.1.2 published for macOS with a signed update feed and verified startup. Apple notarization and Windows signing remain open (Q9). |
| 0.15 | Code smell tooling for our own repo | In review | | `scripts/smells.mjs` (knip, jscpd, Go smell linters), see `code-standards.md` section 12 |

Tasks for later phases are added here when their phase starts.

---

## Phase 1 delivered

Phase 1 (first card) is done as of 2026-09-26. Each item below is the checklist item in `backend-checklist.md` section 3, with what shipped, where it lives, and how it was verified. "Real" means it was run on this Mac against the real program, not a test double.

| Item | What shipped | Where | Verified by | Status |
|---|---|---|---|---|
| B1.0 Protocol conventions | ULID ids, UTC millisecond times, the daemon's time in every snapshot, one error shape, cursor paging, fixed enums, the event envelope with topic and sequence | `daemon/internal/protocol`, `architecture.md` 11, generated `packages/protocol` | Contract tests and golden files shared by Go and TypeScript; `pnpm gen` is reproducible | Done |
| B1.1 Store | SQLite in WAL mode, one writer and a reader pool, forward-only embedded migrations 0001 to 0003, generated queries | `daemon/internal/store` | Migration, failed-write rollback, and concurrency tests on a temp database | Done |
| B1.2 Event bus | Publish and subscribe with per-subscriber buffers, a replay ring, and critical events that are never dropped (the subscription closes and the client re-syncs) | `daemon/internal/events` | Slow subscriber, overflow, and replay tests | Done |
| B1.3 Service and lock | Single-instance lock and per-user service installers for launchd, systemd, and Task Scheduler; `marshal service install`, `uninstall`, and `status`, with `--dev` for the dev daemon's own service | `daemon/internal/platform`, `daemon/cmd/marshal` | Unit tests on generated files and parsed output. **Real, 2026-09-26:** installed in normal mode, ran under launchd, came back after a forced crash, and uninstalled cleanly. Logging out and back in was not exercised; the agent is set to run at load | Done |
| B1.4 Projects module | Create from a folder or a clone, rename, edit, remove in the order of `architecture.md` 16.1, one board per project, language and monorepo detection | `daemon/internal/projects` | Tests including one that proves the repository folder is untouched; end-to-end specs and a hands-on pass (create, error sentences, save, remove) | Done |
| B1.5 Git module | All Git access, worktrees, branches, clone, sparse checkout, the Git 2.38 check | `daemon/internal/gitx` | Tests against real repositories; a worktree exists on start and is gone on close | Done |
| B1.6 Agent interface and ACP adapter | The `Agent` interface, the shared event sink, the one child-process helper, the ACP adapter | `daemon/internal/agents`, `agents/acp`, `internal/proc` | The stub agent starts, streams events, and resumes, driven for real by the session tests | Done |
| B1.7 PTY adapter | A real program in a pseudo-terminal with captured output and a bounded ring | `daemon/internal/agents/pty` | Tests with a helper program, including a 50 MB burst that keeps memory flat | Done |
| B1.8 Claude Code, Gemini CLI, Codex | The agent catalog (`GET /v1/agents`), the Claude Code adapter (stream-json), the Gemini CLI adapter (ACP), and Codex detection only | `daemon/internal/agents/{catalog,claude,gemini,codex}` | Fake-program tests; Codex reports `missing` in real mode. **Real, 2026-09-26:** one Claude Code session on a card replied, with events, worktree, and log. It first failed with "Not logged in" because the child environment dropped `USER`; fixed and tested | Done |
| B1.9 Session manager | One lasting process per card, sends into the same process, queueing while busy, stop | `daemon/internal/session` | Tests including the real stub agent; 85 percent coverage | Done |
| B1.10 Resume | Sessions restored on start (auto or manual), one card resumed by hand, a failed resume moves the card to Needs you | `daemon/internal/session`, `POST /v1/cards/{id}/resume` | Tests, and a real-binary smoke test that restarts the daemon and gets "I remember 1 earlier turns" | Done |
| B1.11 Session logs | Rotated JSON-lines logs and a bounded in-memory ring | `daemon/internal/session` | Rotation, reopen, and heavy-output tests | Done |
| B1.12 API and event stream | The `/v1` routes for projects, cards, sessions, and agents, and the `/v1/events` WebSocket with topics, 25 ms batches, replay, and resync | `daemon/internal/api` | Wire, stream, and route tests; end-to-end suite against a built daemon | Done |
| B1.13 Auth and binding | A token on every route, loopback only, the token never logged, the stream refuses before upgrading | `daemon/internal/api`, `daemon/internal/platform` | A test that fails when a route joins without a token, and a test that the daemon listens on loopback only | Done |
| B1.14 Client data layer | API client with typed errors, event stream with reconnect and resync, connection machine, token store, daemon clock, optimistic helper, mappers from the golden files, and the "Can't reach the daemon" screens | `apps/web/src/data`, `apps/web/src/sync`, `packages/ui` | Unit tests; a hands-on check that stopping the daemon shows the screen and restarting it recovers without a reload | Done |
| B1.15 Card keys | A card is `<project>#<number>`, numbered per project, in the store, views, links, and tests | `daemon/internal/projects`, `apps/web/src/mock`, `apps/web/src/two-projects.test.tsx` | A fixture with card 12 in two projects shows both correctly on every screen; view keys are unique | Done |
| B1.16 Prototype fixture | The prototype's three projects as real repositories, loaded in dev mode only | `daemon/internal/fixture` | Tests, and the project end-to-end specs run against it | Done |
| B1.17 Cutover of S1, S3, S4 | Connection and sign-in, projects, and agents and models now come from the daemon; the mock code for them is removed | `apps/web/src/sync`, `apps/web/src/data/sections.ts` | The register agrees in code and docs; the agent pickers list the daemon's agents; end-to-end specs for each | Done |

**Whole-phase verification (2026-09-26):** `node scripts/check.mjs` passes all nine steps; `pnpm test:e2e` passes 192 specs; the Go suite passes with `-race` in all 24 packages, and the three busiest packages passed three runs in a row; a real-binary smoke test and a real-mode catalog check ran on throwaway daemons; the CI checks at `75cff4a` are green on macOS and Ubuntu and red on Windows (not a blocker, decision 4). The CI end-to-end job waits for all three checks, so it was skipped there; the suite was run locally instead. The hands-on browser pass covered onboarding, the agent pickers, project create errors, settings save, remove, the command palette, and connection loss and recovery, at phone, tablet, and desktop widths in both themes.

**Found by running it for real, and fixed:** the child environment dropped `USER`, so Claude Code reported "Not logged in"; a request the browser cancelled was logged as an internal error; the New project dialog kept a folder error after switching to the clone source. The hardening pass also fixed the `Card.thinking` type, view keys that collided across projects, a 5 second shutdown with a hung WebSocket client, the Vite config warning, a data-to-mock import, and a budgets test that had gone stale.

**Not part of Phase 1 (still on the mock, on purpose):** cards, chats, checklists, comments, approvals, CI, notices, the feed, and Home figures. Because of that, the counts in the remove-project dialog (awake agents, cards) still come from mock cards, and a card that finishes a turn stays in Working (the rule that moves it to Needs you belongs to a later phase). The Claude Code adapter has no approvals yet, so it runs without permission prompts. `cmd/marshald` is at 23 percent coverage because it is the composition root; the end-to-end suite and the smoke test start the real binary.

---

## Phase 2 tasks

Phase 2 (core UI) is scoped as eight slices, A to H, in `backend-checklist.md` section 5 ("Phase 2 scope and order"). "In progress" here means the screens exist and run on the mock, and the daemon side has not started. Nothing below is on the daemon yet except what the Phase 1 cutover already moved (connection, projects, agents).

| ID | Task | Status | Slice | Notes |
|---|---|---|---|---|
| 2.1 | App shell: sidebar, top bar, project switcher, view switcher | In progress | E | Built and cut over for the project list. The last view per project is a saved preference (S32) |
| 2.1a | Responsive layouts for phone, tablet, and desktop | In progress | H | The responsive spec runs every view at five sizes and both themes. It runs again as each section cuts over |
| 2.2 | Base components | In review | | In `packages/ui`, listed in `ui-registry.md` |
| 2.3 | Home dashboard | In progress | A, D | Needs you and awake lists move with cards (A). Charts, stats, and activity follow (D). The cost tile stays on the mock until Phase 4 |
| 2.4 | Kanban view with drag and drop | In progress | A | Moves with List, Timeline, and the Agents view (one section, S5a) |
| 2.5 | Filters, swimlanes, and saved views | In progress | E | S6a and S32 |
| 2.6 | Card detail view with chat UI | In progress | B | S8a |
| 2.7 | Terminal view and view toggle | In progress | B | S9. The hardest piece: the toggle resumes the same session in the other mode |
| 2.8 | Activity feed per card | In progress | B | S10 |
| 2.9 | Diff view | In progress | B | S11 |
| 2.10 | Agents view | In progress | A | Part of S5a |
| 2.11 | Chats view | In progress | C | S17 |
| 2.12 | Command palette | In progress | F | S24a |
| 2.13 | Empty, loading, and error states for every view | In progress | A to H | Done for connection, projects, and agents. Each other view gets its states when its section cuts over |
| 2.14 | Project dialogs | In review | F | Create, rename, settings, and remove are on the daemon (S3). The sample project option waits for 2.25 |
| 2.15 | Profile page and accounts module | In progress | E | S2a. Devices stay on the mock until Phase 9 |
| 2.16 | Onboarding | In progress | F | S31a. The project step has no sample option until 2.25 |
| 2.17 | Tutorial tour | In progress | F | Progress is saved per user |
| 2.18 | Card fields the model lacks (N1) | Not started | A | Labels, package, dates, pull request link, context used, needs-you reason, doing now. The daemon side only for labels: the label screens and colors need a design first (Q20) |
| 2.19 | Manual moves, card edit, and delete (N2, N3) | Not started | A | The move rules and messages are written in `architecture.md` 6.1 |
| 2.20 | Terminal channel (N12) | Not started | B | Input, resize, and the phone key bar |
| 2.21 | Chat history, activity, and diff loading (N13 to N15) | Not started | B | Typed message kinds, paged history, paged activity, per-file hunks |
| 2.22 | Feed, notice, and activity vocabularies (N17) | Not started | D | Fixed lists in the protocol, and paged, filterable view-all lists |
| 2.23 | Saved views API and screen preferences (N20, N30) | Not started | E | Decisions D2 |
| 2.24 | Search over projects, cards, and chats (N23) | Not started | F | |
| 2.25 | Sample project and Developer options (N24) | Not started | F | Restores the onboarding sample option the owner removed on 2026-09-25. The dev-only reset is a route only; the Developer options screen needs a design first (Q21) |
| 2.26 | Users list and avatar upload (N27) | Not started | E | Solo use lists the owner only (D10) |
| 2.27 | Manual session controls: pause, sleep, wake, pin, and Start resuming a stopped card (N4) | Not started | G | Moved here from Phase 5 (5.10 in part, 5.16) on 2026-09-26. The idle timer, sleep warning, awake limit, and notice actions stay in Phase 5 |

---

## Decisions log

| Date | Decision | Reason |
|---|---|---|
| 2026-11-10 | The release gate is the fast `ci` workflow only: light checks, focused regressions, and client tests. The macOS daemon tests and the browser tests run in `ci-full` and do not block a release. | Owner choice 2026-11-10. The full gate took about 47 minutes before the browser job started, and one flaky test cost another hour. A red `ci-full` can now ship, so it is checked by hand before relying on a build |
| 2026-11-05 | Offer a Terminal installer for fresh macOS installs while retaining the native signed updater for installed copies. | A Terminal download avoids the browser quarantine flag. Using the same public DMG avoids a separate build, and existing installs keep Tauri signature verification. |
| 2026-11-03 | Use the original project folder with one coordinating main coding session and its native editing subagents; stop automatic worktree creation and merging | The owner selected coordinated editing workers in the same folder. This avoids extra checkouts/dependency copies while reserving the folder against independent competing card/chat editors. Existing worktree files remain available for deliberate recovery. |
| 2026-09-23 | The product is named Marshal | A marshal organizes and directs, which matches orchestrating agents on a board |
| 2026-09-23 | Daemon in Go | Many processes and connections, one binary, native tsnet, official MCP SDK, fast to build |
| 2026-09-23 | Desktop shell in Tauri v2 | Much lighter than Electron |
| 2026-09-23 | UI in SolidJS | Small, fast with streaming updates, familiar JSX |
| 2026-09-23 | SQLite in WAL mode for all state | Local, fast, no setup |
| 2026-09-23 | Tailscale built in through tsnet, Funnel for webhooks only | Private remote access without a separate install, and a public URL for webhooks |
| 2026-09-23 | One board per project, focus through filters and swimlanes | Conflict warnings and merging need to see every card |
| 2026-09-23 | One lasting session per card, with sleep, wake, and resume | No context loss and no new process per message |

---

## Open questions

| ID | Question | Needed by | Status |
|---|---|---|---|
| Q1 | Which ACP SDK for Go, or do we write the JSON-RPC layer ourselves? | Task 1.6 | Answered 2026-09-25: `github.com/coder/acp-go-sdk` v0.13.5, built early for the stub agent |
| Q2 | Best way to run Claude Code as a lasting session: ACP, streaming JSON mode, or PTY? | Task 1.8 | Answered 2026-09-25: Claude Code's own streaming JSON mode (`claude -p`), driven directly, because Claude Code has no ACP (`architecture.md` 4.2) |
| Q3 | Which CLI versions do we pin as supported for launch? | Task 1.8 | Open. The catalog reports supported, untested, or missing per version, but the supported set is not pinned yet |
| Q4 | Codebase map approach: tree-sitter with cgo, ctags, or tree-sitter in WebAssembly? | Task 7.9 | Answered 2026-09-27: universal ctags as an external binary, with a filename-only fallback and a clear "ctags is not installed" notice (decisions log; `library-docs.md` 2.10; built as `internal/codemap`) |
| Q5 | Is `modernc.org/sqlite` fast enough under real load, or do we need a cgo driver? | Task 1.1 | Partly answered 2026-09-25: the store passes its tests with `-race` and the pragmas are set per connection, but there is no benchmark under a heavy write load yet, so the question stays open |
| Q6 | Windows user service: scheduled task or a different approach? | Task 1.3 | Answered 2026-09-25: a scheduled task, in our own code (`internal/platform/service_schtasks.go`); only its generated arguments and its status parsing are tested, never installed |
| Q7 | Are the first real projects monorepos? If yes, monorepo mode moves into Phase 1. | Task 1.5 | Answered 2026-09-25: detection finds packages and the board shows a swimlane per package, and a project with no packages works, so monorepo mode is in Phase 1 (`internal/projects` detection, the extra-package-lane deviation in `design-port.md`) |
| Q8 | How much does tsnet add to download size, and does it fit the budget? | Task 9.1 | Open. Nothing uses tsnet yet |
| Q9 | Who holds the Apple Developer and Windows code signing accounts? | Task 0.14 | Open. Required for Apple notarization and Windows signing. The macOS updater uses its own key. |
| Q10 | Should the onboarding sample project be a real small repo we ship, or generated on first use? | Task 2.16 | Deferred by the owner on 2026-09-25: the sample-project option is removed from onboarding until Phase 2 ships a sample repository (build plan task 2.25), and this question decides what that repository is |
| Q11 | Which languages get built-in smell checks at launch? Suggested: Go, TypeScript and JavaScript, and Python. Depends on Q4. | Task 5.12 | Open |
| Q12 | Default size limit per attachment? Suggested: 25 MB, editable in settings. | Task 10.12 | Open |
| Q13 | Should people be able to review agent ticks, for example "Confirm agent ticks" on required checklists? | Task 10.11 | Open |
| Q14 | Prototype: posting a comment, attaching a file, adding an item, and the phone layouts for these have not been clicked through yet. | Design review | Open |
| Q15 | Who confirms the macOS CI run was green? The Phase 1 work session has no network access, so it cannot read the CI results; the tracker records "macOS CI green" on the strength of the controller's report. | Phase 1 review | Answered 2026-09-26: the controller read the CI results. At `75cff4a` macOS and Ubuntu are green and Windows is red (not a blocker) |
| Q16 | Which Claude Code version should be the tested one? The adapter drives `claude -p` stream-json without a real run in any automated test, so the "supported" version is untested in practice until the owner runs one card by hand. | Task 1.8, before Phase 4 | Answered 2026-09-26: one real card ran on Claude Code 2.1.282 and replied. The interrupt stage and per-tool approvals (Phase 3) are still not exercised against the real program |
| Q17 | Do cards that a project chat creates have to work in Phase 2? Real agents can only create cards through the internal MCP server, which is Phase 7. | Slice C (B2.10) | Open. Proposed: no. Build the chat, its session, and its history now. Check "cards it creates land on the board" with a stub-agent scenario that goes through the same card-creation service the MCP tool will use later |
| Q18 | What do Pause, Sleep, Wake, and Pin do on a real card before Phase 5? The controls are on the screens (Card panel, Home's awake list). | Slice A (B2.4) | Answered 2026-09-26 by the owner: they ship in Phase 2 (slice G, B2.15). Until slice G lands, the four controls do nothing for a real card |
| Q19 | How does a card in terminal view run? Chat view drives the agent's structured mode, and terminal view needs the agent's own interactive command. | Slice B (B2.7) | Open. Proposed: the toggle stops the chat-mode process and starts the PTY adapter with the agent's interactive command and its saved session id, and back again, within one to three seconds |
| Q20 | Who designs label management (Project settings), the label picker on a card, and the label colors? The prototype has none of them, and task 2.18 lists all three. | Slice A (B2.4) | Open. Proposed: Phase 2 builds the labels in the daemon and the screens keep showing names as today. The three new screens and the color tokens wait for a design from the owner |
| Q21 | Who designs the Developer options section of Settings (task 2.25)? The prototype has no such section; only the mock's `resetFirstLaunch` exists. | Slice F (B2.12) | Open. Proposed: Phase 2 builds the dev-mode-only reset route and no screen. The section waits for a design from the owner |

---

## Session log

### 2026-11-10 — First automatic release run failed

- The first automatic release run (`42706000182`, after `ci` passed on `b3c1516`) failed in `Select verified release commit` with "No commit found for SHA: v0.1.3". For a tag that does not exist yet, GitHub answers a commit lookup with 422, not 404, and `release-plan.mjs` treated only 404 as "missing". A new version could never start its first automatic release. A 422 whose message starts with "No commit found" now counts as missing; any other 422 still stops the release. The test fixture now answers like GitHub does, and a new test covers both cases.
- `ci-full` browser run on the same commit: the built-in agent test assumed the fixture daemon lists no models for it, but the daemon listed five. The test now follows what the daemon reports: a warning when there are no models, none otherwise. Only five failures ran before the browser limit stopped the run, so more may show.
- With that fixed, the next release run (`42708528137`) built both platforms and uploaded all six downloads to a draft `v0.1.3`, then failed in the last step. The by-tag release lookup never returns a draft, and a draft has no tag yet, so the publish check could not find it. The step now lists the releases, matches the tag name, and compares a draft's recorded target commit with the verified commit (a published release is still checked through its tag). The owner chose to rebuild through the normal pipeline instead of publishing that draft by hand, and wants every release published automatically, never left as a draft.
- Validation is GitHub-only apart from the 7 release-plan unit tests, which ran locally and pass.

### 2026-11-10 — Fast release gate

- Owner asked to cut CI time before a release. `ci.yml` is now only the release gate: the light checks, focused regressions, and every client test with its coverage floors (about eight minutes). The macOS daemon and tool tests, coverage, build and budgets, and the Linux browser tests moved unchanged into `ci-full.yml`. It runs on the same triggers beside the gate and does not block a release.
- Why: the last full run took about 47 minutes before the browser job started, the macOS test job alone was about 40 minutes with the race detector, one flaky test cost another hour, and none of the last 40 runs finished green.
- Release still waits only for a successful `ci` run on the latest `main` commit, so a red `ci-full` can ship. Check `ci-full` before relying on a build.
- Not changed: the browser test limits, `release-plan.mjs` (a superseded commit is still skipped), and the universal macOS build time (about 60 minutes).
- Validation is GitHub-only. No local tests, lint, builds, or browser checks ran.

### 2026-11-10 — Browser picker contracts

- CI `42699668345` passed early and client gates. Browser boot now succeeded with the isolated fixture keychain. The browser job failed in two minutes and uploaded diagnostics, exposing stale native-select locators, a duplicated built-in catalog entry, and retired hard-coded prototype model assumptions. The attached watcher detected the completed failing job while macOS work was still running; that remaining run was cancelled to avoid validating a known failed candidate.
- Added custom dropdown and card-disclosure browser helpers. Updated agent and hold specs to use real menu interactions, daemon catalog model/thinking flags, explicit unconfigured-provider messaging, and disclosed onboarding notes. Kept disabled-agent and missing-install assertions. No production UI behavior or tests were removed; no paid model calls are used.
- No local tests, lint, builds, or browser checks ran. GitHub verification and v0.1.3 publication remain pending.


### 2026-11-10 — Headless browser startup

- CI `42685870479` passed all light checks, every client test and coverage floor, all daemon/tool tests and daemon coverage, the build, and memory/size budgets. Browser testing then hit the thirty-minute job limit. Its logs showed repeated real Linux keychain reads failing because Secret Service was absent; the stub fixture daemon had incorrectly used the OS keychain.
- Added a shared, empty in-memory keychain only for explicit dev stub fixtures, with tests for normal/dev/real/stub/fixture combinations and process isolation. Production modes retain the OS keychain; no missing-service error is silently treated as a real saved key.
- Added authenticated boot endpoint checks before the browser matrix. Set five-failure and twenty-minute browser limits so failures return before the job deadline and preserve traces, screenshots, and HTML diagnostics. Artifacts exclude the daemon data directory. The complete suite is still required to pass. The early Go gate includes the fixture keychain tests. Browser and macOS validation now run independently after client tests; both remain required release gates, so browser startup failures can surface without waiting for the macOS suite.
- Validation remains GitHub-only. No local tests, lint, builds, or browser checks ran. Requested release v0.1.3 remains unpublished until required checks and both release builds succeed.


### 2026-11-09 — Full client regression failures

- CI `42677607801` passed the early gate and every daemon/Go tool test. Full web testing passed 3,362 tests and found two failures: the API contract list omitted the three preview methods, and the settings summary used a phone sizing ternary contrary to the existing responsive guard. Updated both explicit API lists and replaced the ternary with phone CSS variants and the control-height token. Kept all guards and assertions enabled.
- Moved the complete client package test suites and coverage floors into a Linux job before the slower macOS daemon/tool job. Retained all existing workspace test packages and race instrumentation. Added API/responsive guard files to the early selection. CI and release remain pending; no local tests, lint, builds, or browser checks were run.

### 2026-11-09 — Early card regression failure

- CI `42676743566` passed formatting, lint, types, and release-plan fixtures. Fast card regressions found two fixtures that expected a merge note without supplying the merge phase required by the existing decorator. Supplied the testing phase and added a regression for hiding stale notes without a phase. No production UI behavior or checks were weakened.
- The detached watcher recorded the failure but could not resume this chat. It is a notification/status helper, not autonomous failure repair. Current validation remains on GitHub; no local tests, lint, builds, or browser checks were run.

### 2026-11-09 — GitHub release sequencing

- CI `42668973961` passed light checks but failed the catalog PATH priority test. It assumed the service PATH must immediately follow the executable directory, while runtime discovery now recovers the login PATH first. Isolated that test from machine login configuration; existing runtime fixtures continue to cover recovered PATH behavior.
- Added early release/card/mobile/runtime regression steps before the slower macOS job, cancellation of superseded CI runs, and a CI summary identifying the release blocker.
- Recorded version `0.1.3` in `.github/release.json`. Successful CI on the latest main commit now triggers release directly on GitHub. Build both platforms from that exact commit, upload artifacts, then verify and publish all downloads together. New releases remain drafts until uploaded assets pass verification. Existing versions and stale commits are skipped; manual retries require a verified source tag.
- Made the duplicate macOS browser workflow manual. Required browser coverage remains inside CI. No active AI watcher is needed to advance release after CI passes. Failure repair still requires a code change; workflow chaining does not claim to repair code automatically.
- Verification remains on GitHub only. No local tests, lint, type checks, builds, or browser checks were run. Release is not yet published.


## File changes log

_Older entries: [archive/progress-tracker-archive-2026-10.md](archive/progress-tracker-archive-2026-10.md)._

Every file added, moved, renamed, or deleted in the repo. Newest first. The planned structure is in `project-structure.md`.

| Date | File | Change | Task | Notes |
|---|---|---|---|---|
| 2026-11-10 | `apps/web/e2e/support/{dropdown,card-details}.ts`, agent/hold specs | Added, Changed | Browser CI | Match custom menus and mobile disclosures; derive catalog expectations from the daemon. |
| 2026-11-10 | `daemon/cmd/marshald/keychain{,_test}.go`, module wiring, browser setup/config, CI | Added, Changed | Browser CI | Isolate explicit stub fixtures from desktop credentials and fail early with diagnostics. |
| 2026-11-09 | `.github/release.json`, `apps/desktop/scripts/release-plan{,.test}.mjs`, workflows, desktop package scripts, catalog PATH test | Added, Changed | Release order | Event driven CI to dual-platform build to verified publication; focused early regressions. |
| 2026-11-09 | Runtime PATH/local-CI, workspace-lock, preview relay/exposure, native system-bars, and chat/disclosure files listed in project-structure.md | Added, Changed, Deleted | Runtime/mobile | Portable tool lookup, isolated previews, shared project ownership, mobile controls; removed bypass banner. |
| 2026-11-09 | `daemon/internal/testutil/store{,_test}.go`, `daemon/internal/testutil/approvalcli.go`, `daemon/internal/api/testdata/approvalcli/main.go` | Added | CI reliability | Isolated schema fixtures and a standalone approval CLI fixture. |
| 2026-11-09 | `daemon/internal/chatcmd/conversations{,_test}.go`, `daemon/internal/chatbot/conversation_test.go`, `daemon/cmd/marshald/chat_watch.go` | Added | Remote coding | Save conversation links and delivery revisions; test transport metadata, routing, and automatic replies. |
| 2026-11-09 | `daemon/internal/chatcmd/control{,_test}.go`, `daemon/cmd/marshald/chat_control{,_test}.go` | Added | Remote coding | Route bot coding commands through the real session, history, and completion services. |
| 2026-11-05 | `README.md` | Added | Docs | Add a GitHub entry point for users and developers. |
| 2026-11-05 | `install.sh` | Added | 0.14 | Install the public universal DMG from Terminal and preserve existing app copies. |
| 2026-11-05 | `apps/desktop/src-tauri/entitlements.plist`, `apps/desktop/scripts/check-bundle.mjs`; desktop config, package scripts, release workflow | Added, Changed | 0.14 | Allow the secret scanner’s generated code under hardened runtime and gate release publication on a signed-archive CLI/server check. |
| 2026-11-05 | `apps/web/src/features/updates/*`, `views/settings/UpdatesSection{,.test}.tsx`, `apps/desktop/scripts/prepare-release{,.test}.mjs`, `apps/desktop/src-tauri/permissions/updates.toml`, `docs/app-updates.md`; existing desktop/update/release/navigation files | Added, Changed | Updates | Add visible signed app-update controls and release plumbing. |
| 2026-11-05 | `daemon/internal/session/oneshot_internal_test.go`, `oneshot_failure_test.go`, `oneshot.go` | Added, Changed | Verification | Preserve the actual review failure instead of returning partial text as an answer. |

---

## Doc changes log

_Older entries: [archive/progress-tracker-archive-2026-10.md](archive/progress-tracker-archive-2026-10.md)._

Every change to a doc. Newest first.

| Date | Doc | Change |
|---|---|---|
| 2026-11-10 | `project-structure.md`, `progress-tracker.md` | Recorded browser fixture recovery and current picker contracts, with CI/release still pending. |
| 2026-11-10 | `development.md`, `project-structure.md`, `progress-tracker.md` | Documented headless fixture credentials, startup assertions, browser failure bounds, and actual successful CI gates. |
| 2026-11-09 | `app-updates.md`, `development.md`, `project-structure.md`, `progress-tracker.md` | Replaced manual tag-triggered release instructions with the CI completion chain, blocking rules, and pending validation status. |
| 2026-11-09 | `agent-sessions.md`, `mobile.md`, `ui-rules.md`, `direct-workspace.md`, `project-structure.md`, `progress-tracker.md` | Documented runtime discovery, preview access and limits, shared locks, and mobile behavior; validation remains pending GitHub. |
| 2026-11-09 | `development.md`, `project-structure.md`, `progress-tracker.md` | Documented CI fixture isolation, setup deadlines, and approval protocol startup. |
| 2026-11-09 | `agent-sessions.md`, `project-structure.md`, `progress-tracker.md` | Documented automatic answers, linked replies and threads, delivery bounds, and existing approval routing. |
| 2026-11-09 | `agent-sessions.md`, `mobile.md`, `marshal-product-scope.md`, `project-structure.md`, `progress-tracker.md` | Documented bot coding commands and the current limits of project previews on phones. |
| 2026-11-05 | `agent-sessions.md`, `ui-rules.md` | Documented the Codex startup check, the verified CLI version, and where to find version numbers. |
| 2026-11-05 | `README.md`, `direct-workspace.md`, `marshal-product-scope.md` | Documented adding a local folder and how Marshal sets up local Git for card diffs. |
| 2026-11-05 | `README.md`, `development.md`, `project-structure.md`, `progress-tracker.md` | Added the root README and corrected desktop development prerequisites. |
| 2026-11-05 | `app-updates.md`, `project-structure.md`, `progress-tracker.md` | Documented fresh installation from Terminal and its verification. |
| 2026-11-05 | `app-updates.md`, `README.md`, `project-structure.md`, `development.md`, `progress-tracker.md` | Documented the update flow, signed release setup, browser/mobile limits, and acceptance checks. |

---

## Notes for later

Things found during work that are outside the current task. Each one should become a task or be closed.

| Date | Note | Found during |
|---|---|---|
| | | |
