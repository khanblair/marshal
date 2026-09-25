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
| **Current phase** | Phase 2: Core UI (next, not started) |
| **Next milestone** | Core UI: every remaining screen reads real data (end of Phase 2) |
| **Last updated** | 2026-09-26 |

---

## Phase status

| Phase | Status | Notes |
|---|---|---|
| 0. Foundation | Done locally | Done locally: Go tools, daemon module, protocol package, Go lint and smells, fixtures, stub agent, budgets, dev mode. macOS CI green. Windows and Linux are not a blocker (macOS first, decision 4). Deferred: keychain entries (library check first), desktop shell and release (after Phase 2). |
| 1. First card | Done (2026-09-26) | Built, tested, and pushed. Item by item in "Phase 1 delivered" below. Built and tested on this Mac: the SQLite store with embedded migrations, the event bus with replay, the single-instance lock and the per-user service installers, projects and boards with cards, all Git access through `gitx`, the agent interface with the ACP, Claude Code, Gemini CLI, PTY, and Codex-detection adapters, the session manager with resume and bounded logs, the `/v1` API and the `/v1/events` stream with token auth, the client data layer, the project-plus-number card key, the prototype fixture, and the cutover of sections S1, S3, and S4. The two checks that need the real machine were run on 2026-09-26 with the owner's approval: a real Claude Code session on a card (it answered), and the login item (installed, restarted after a crash, removed). The cards, chats, checklists, and everything else still on the mock are Phase 2 and later. |
| 2. Core UI | Not started | |
| 3. Control and safety | Not started | |
| 4. Models | Not started | |
| 5. Quality loop | Not started | |
| 6. CI and preview | Not started | |
| 7. Orchestration and memory | Not started | |
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
| 0.14 | Release pipeline with signing and updater | Not started | | Needs Apple and Windows signing certificates (Q9) |
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

## Decisions log

| Date | Decision | Reason |
|---|---|---|
| 2026-09-23 | The product is named Marshal | A marshal organizes and directs, which matches orchestrating agents on a board |
| 2026-09-23 | Daemon in Go | Many processes and connections, one binary, native tsnet, official MCP SDK, fast to build |
| 2026-09-23 | Desktop shell in Tauri v2 | Much lighter than Electron |
| 2026-09-23 | UI in SolidJS | Small, fast with streaming updates, familiar JSX |
| 2026-09-23 | SQLite in WAL mode for all state | Local, fast, no setup |
| 2026-09-23 | Tailscale built in through tsnet, Funnel for webhooks only | Private remote access without a separate install, and a public URL for webhooks |
| 2026-09-23 | One board per project, focus through filters and swimlanes | Conflict warnings and merging need to see every card |
| 2026-09-23 | One lasting session per card, with sleep, wake, and resume | No context loss and no new process per message |
| 2026-09-23 | Chat view by default, terminal view as an option, both on the same session | Chat is easier to read, terminal is there for full control |
| 2026-09-23 | Roles are editable templates with starter defaults | Every team works differently |
| 2026-09-25 | The app uses the Claude Design's exact colors, including the pairs that miss the contrast targets. `color-border-control` is removed, and muted text, status solids, and chart values are back to the design's values. The pairs that miss are listed in `packages/tokens/src/contrast-exceptions.ts`, and CI fails on a new miss or a stale exception. | The project owner decided that the design wins over `ui-tokens.md`. The contrast targets stay as goals for the next design revision. |
| 2026-09-25 | The parity check (a pixel and text comparison of the port against the prototype) is retired and its tooling removed | The design is exported and the app is now the source of truth. The tool stays in Git history. Views are checked by their tests, by the end-to-end responsive specs, and in the browser |
| 2026-09-25 | Buttons use larger corners (7 px up to 32 px tall, 10 px at 36 px and on touch screens), labels never wrap, and Split view and New card show only an icon in a header narrower than 720 px | Requested by the project owner after seeing wrapped labels and tight corners on small tablets. Set once, by size, in `Button` and `IconButton` |
| 2026-09-25 | The backend follows the prototype. What the prototype does that the docs do not describe is built and the docs are updated (`backend-inventory.md` section 5, entries N1 to N30) | The prototype is the design, and the docs only gave examples for the API |
| 2026-09-25 | Every integration is real, built in build plan order | No stand-ins. An integration whose account step is missing keeps its mock section until its connection test passes |
| 2026-09-25 | The mock is replaced section by section, and deleted once every section is verified on the daemon | Lets each part of the screens switch only when it works, and keeps the rest running. Rules and the register are in `backend-checklist.md` section 2 |
| 2026-09-25 | Go is installed with Homebrew (Go 1.27.1) | Needed for the daemon. Approved by the owner |
| 2026-09-25 | "Simulate CI failure" is built as a real feature with two modes: a synthetic failed run through the CI monitor, and a real failing run on the card's branch on GitHub. Shown only in dev mode or with a Developer options setting | The prototype had it as a hidden action that no doc covered. The owner asked for one integrated with the project, in both modes |
| 2026-09-25 | Build order follows the build plan as a guide, not strictly. A piece that needs something from a later phase is built early and skipped when its phase comes | Integration work depends on what exists. Rules are in `backend-checklist.md` section 2.7 |
| 2026-09-25 | Each project numbers its own cards. A card is known by its project and number, and lists that mix projects show the project name with the number | Chosen by the owner. The client's card key changes from the bare number (`backend-inventory.md` N26) |
| 2026-09-25 | The Tauri desktop shell and signed releases are built after Phase 2 | They are not needed to replace the mock, so the first desktop app ships with a working daemon |
| 2026-09-25 | The 30 pieces of work the prototype needs that the docs did not cover are numbered tasks in `build-plan.md` marked (prototype), each with its screen work, and the docs features that had no screen get screen tasks too | The owner wants the backend and its screens built together. Reference list: `backend-inventory.md` section 5 |
| 2026-09-25 | Timeline bars show the actual span for started cards and the planned span for cards not started | Shows slips and plans together |
| 2026-09-25 | Screen preferences (theme, list columns, sort, last view, filters, swimlane) are saved with the user. Layout stays on the device | Follows the user to a phone without forcing one layout on every screen |
| 2026-09-25 | Labels are a managed list per project, each with a name and a color from a fixed set of tokens | Chosen by the owner. Needs label color tokens that pass the contrast check |
| 2026-09-25 | Keep awake lasts 15 minutes and is a setting under General | Matches the prototype and lets users change it |
| 2026-09-25 | A service that is not set up shows "Not connected" with a Connect button and no sample data. Windows and Linux are covered by CI plus a manual pass before release. Solo use shows only the owner | Nothing looks real that is not. Owner's choices |
| 2026-09-25 | The stub agent uses `github.com/coder/acp-go-sdk` v0.13.5, so the ACP library choice for task 1.6 is made and the JSON-RPC layer is built early. Scenarios are JSON files, not YAML | The SDK is generated from the official schema, and JSON adds no dependency. Recorded under task 1.6 in the checklist |
| 2026-09-25 | Each Go program is its own module (the daemon and each program under `tools/`), with no `go.work`. The daemon keeps `internal/` private, and the tools share nothing with it | The tools need no daemon internals, so they stay small and build alone. The daemon builds and tests them by running them as programs |
| 2026-09-25 | The Go tools install without cgo (`CGO_ENABLED=0`), and the daemon uses no cgo | The tools then need no C toolchain, and this Mac's Command Line Tools cannot link cgo programs that use system frameworks. The race detector still works for tests |
| 2026-09-25 | `marshal dev reset` in Go replaces `scripts/dev-reset.mjs`, and only deletes a folder ending in `-dev` that sits well below the home folder | One place holds the folder rules, shared with how the dev daemon picks its data folder. Refuses while the dev daemon runs |
| 2026-09-25 | The prototype toolbar (device frames, Reset to first launch) is not part of the app | Requested by the project owner; it is a prototyping aid. The app fills the window and follows its size |
| 2026-09-23 | Strong models by default for Reviewer and Integrator | Weak models break code at review and merge time |
| 2026-09-23 | Plan first mode keeps plans in chat, no plan document unless asked | Plans are quick to review and should not create clutter |
| 2026-09-23 | Bypass permissions mode included, off by default, worktree only | Some users want no interruptions, with the risk made clear |
| 2026-09-23 | Memory as plain markdown, Obsidian compatible | Readable, portable, no lock-in |
| 2026-09-23 | Color system based on marshal signal flags, primary actions in ink | Color only means status, so the board is readable at a glance |
| 2026-09-23 | Atkinson Hyperlegible Next and Mono as typefaces | Built for legibility at small sizes, where most of Marshal's text lives |
| 2026-09-23 | Sentence case everywhere, no all-caps text | Calmer, more readable, and avoids a common generated-app look |
| 2026-09-23 | Cross-project card dependencies are not in scope | Not needed |
| 2026-09-23 | pnpm workspaces as package manager and the one script runner, including Go and Rust tasks | One way to run everything |
| 2026-09-23 | UI embedded into the daemon binary, daemon bundled into Tauri as a sidecar | The web UI works on phones without the desktop app, and one install gets everything |
| 2026-09-24 | Home is a dashboard with needs you, tiles, charts, recent activity, and more | One place to see what needs you, what is happening, and how it is going |
| 2026-09-24 | Many chats per project, still one board | Conversations stay organized, while every card stays on one board |
| 2026-09-24 | Projects get create, rename, edit, and remove. Chats get create, rename, archive, restore, and delete | Users need to manage both. Removing a project never deletes the repo. |
| 2026-09-24 | Profile avatar in the top right on every screen, opening the profile page | Standard, easy to find |
| 2026-09-24 | Four onboarding screens and a skippable tutorial tour on Home | Help new users start fast, without forcing it |
| 2026-09-24 | Full product on phone, tablet, and desktop, responsive from Phase 2 | Remote control needs everything on every size |
| 2026-09-24 | Code smell checks in Marshal on every card's diff, and on our own code in CI | Smells are cheapest to fix when written, and agents create them quickly |
| 2026-09-24 | Smell checks look only at new or worsened smells | Keeps findings short and fair, and avoids blaming old code on new cards |
| 2026-09-24 | Home dashboard order: summary tiles and charts first, then needs you, then details. Needs you stays the first tile. | Overview first, while the waiting count stays the first thing seen |
| 2026-09-24 | Test connection button on every integration, provider key, and MCP server | Users can check a connection works without waiting for a real event |
| 2026-09-24 | "Add a card" only in Backlog, Planning, and Working | Other columns are reached by real events, so adding there would make the board untrue |
| 2026-09-24 | Named checklists per card. "Required to finish" is on by default for Definition of done and Acceptance criteria, off for Sub-tasks. Required checklists block the merge queue. | Some parts of done need people, and they should gate merging the same way checks do |
| 2026-09-24 | The agent can tick checklist items only with evidence, and ticks undo themselves when the evidence stops being true. "Only people can tick" per checklist. | Keeps agent ticks honest |
| 2026-09-24 | Comments with files, images, and links. @agent or a question gets a reply. Marshal never fetches linked pages by itself. | Discussion stays with the card, while staying private and light |
| 2026-09-24 | Members get notices for mentions, replies, needs you, and merges. Non-members only for mentions. | The right people hear about their cards without noise |
| 2026-09-24 | Trello sync covers checklists, comments, attachments, and mapped members. The agent is a Trello label. Latest change wins on conflicts. | Trello users keep working in Trello |
| 2026-09-24 | People avatars are circles, the agent avatar is a rounded square with a bot icon | Told apart by shape, not color |
| 2026-09-24 | When the owner's Claude Design output conflicts with `ui-tokens.md`, `ui-rules.md`, or `ui-registry.md`, the design wins and those docs are updated to match | The owner is producing the visual design in Claude Design, so it is the newer source of truth |
| 2026-09-24 | **Superseded 2026-09-25, see the decision below.** Added `color-border-control` for input, select, checkbox, and switch borders. `color-border-strong` stays for dividers only. | Control borders must reach 3 to 1 to be seen, and dividers should stay soft. `color-border-strong` measured about 1.6 to 1. |
| 2026-09-24 | **Superseded 2026-09-25, see the decision below.** Changed light and dark values for muted text, the Planning, Needs you, Working, and Ready solids, and the chart series. Solid icons never sit on `subtle` backgrounds. | A check of every token pair found 23 pairs below the contrast rules in `ui-tokens.md` section 2.9. The pairs are now listed there and checked in CI. |
| 2026-09-24 | Split view allows up to two panes from 900 px, three from 1200 px, and four from 1600 px | Gives `bp-md` a defined role, and removes the conflict between "tablet: two panes" and "desktop: four panes". Provisional until the Claude Design output is reconciled. |
| 2026-09-25 | A card gets one session in Phase 1. A failed resume marks the session stopped and moves the card to Needs you; starting a fresh session for a card that already had one is not built yet. | One lasting session per card is already the rule (2026-09-23), and "resume or tell the person" is honest about what a lost context means. A fresh-session feature needs a decision about what happens to the old transcript, so it waits |
| 2026-09-25 | The Built-in agent entry is appended by the app, after the daemon's agents, until the daemon lists it in Phase 4. It is the only agent the app makes up, and it is the first entry removed when the daemon reports one. | Mock cards and roles still name it, and the design's pickers show it, so it must exist; but nothing should look like the daemon reported something it did not (`sync/agents.ts`, `BUILT_IN_AGENT`) |
| 2026-09-25 | The prototype fixture loads in dev mode only. `pnpm dev` passes `--fixture prototype`; a normal install ignores the setting. | The fixture makes real Git repositories in the data folder, which is a development convenience and must never appear in a person's install (`development.md` section 3.5) |
| 2026-09-25 | WebSocket auth uses the subprotocols `marshal.v1` and `bearer.<token>`, never a token in a URL. A gap in event `seq` is normal and is not a reason to resync; only a different `epoch` or an unreplayable position is. | A token in a URL lands in logs, history, and proxies. And a gap is expected because ordinary events may be dropped under backpressure, so treating a gap as a loss would resync constantly (`architecture.md` 11.5) |
| 2026-09-25 | The approved differences from the rendered prototype are only the rows in `design-port.md` "Known deviations from the rendered prototype". A visual change that is not on that list is stopped and reported instead of made. | The design is the source of truth and the port is finished, so an undocumented visual change is a regression against the owner's design, not a detail |
| 2026-09-25 | Verification happens once at the end of a phase, on this Mac, through the full check, the end-to-end suite, and a hands-on browser pass. | One expensive, complete run is cheaper and more trustworthy than running the whole suite after every edit, and the browser pass is the only way to see the screens |
| 2026-09-25 | The per-user service installers are our own code (`service_launchd.go`, `service_systemd.go`, `service_schtasks.go`). `kardianos/service` was considered and is not used: its zlib license is not on the allowed list in `library-docs.md` section 1. | Keeping all three platforms on one license-clean implementation beats a third-party library for two platforms plus custom code for the third |
| 2026-09-25 | The Claude Code adapter drives `claude -p` in stream-json mode directly (Claude Code has no ACP). Per-tool approvals arrive in Phase 3, so every session runs with `--permission-prompts=none` and `Respond` returns "unknown request". Codex is detected but not startable until it has an adapter. | Stream-json is Claude Code's own supported mode, so no protocol library is needed. A permission request nobody can answer must be refused at once rather than left hanging (`architecture.md` 4.2) |
| 2026-09-25 | macOS first. Windows and Linux jobs stay in CI, but their results are not a blocker. | The owner ships on macOS first, so a red Windows or Linux job must not stop Phase 1 work (decision 4, `backend-checklist.md` 1) |
| 2026-09-26 | The environment a child agent inherits from the daemon also includes `USER` and `LOGNAME` (they name the person and hold no secret). Still an allow-list, so tokens and keys do not pass | Found by running one real Claude Code session: on macOS it looks its login up in the Keychain by the user's name and answered "Not logged in" without it. A stub agent cannot show this. Test: `TestTheNameOfTheUserReachesTheChild` |

---

## Open questions

| ID | Question | Needed by | Status |
|---|---|---|---|
| Q1 | Which ACP SDK for Go, or do we write the JSON-RPC layer ourselves? | Task 1.6 | Answered 2026-09-25: `github.com/coder/acp-go-sdk` v0.13.5, built early for the stub agent |
| Q2 | Best way to run Claude Code as a lasting session: ACP, streaming JSON mode, or PTY? | Task 1.8 | Answered 2026-09-25: Claude Code's own streaming JSON mode (`claude -p`), driven directly, because Claude Code has no ACP (`architecture.md` 4.2) |
| Q3 | Which CLI versions do we pin as supported for launch? | Task 1.8 | Open. The catalog reports supported, untested, or missing per version, but the supported set is not pinned yet |
| Q4 | Codebase map approach: tree-sitter with cgo, ctags, or tree-sitter in WebAssembly? | Task 7.9 | Open |
| Q5 | Is `modernc.org/sqlite` fast enough under real load, or do we need a cgo driver? | Task 1.1 | Partly answered 2026-09-25: the store passes its tests with `-race` and the pragmas are set per connection, but there is no benchmark under a heavy write load yet, so the question stays open |
| Q6 | Windows user service: scheduled task or a different approach? | Task 1.3 | Answered 2026-09-25: a scheduled task, in our own code (`internal/platform/service_schtasks.go`); only its generated arguments and its status parsing are tested, never installed |
| Q7 | Are the first real projects monorepos? If yes, monorepo mode moves into Phase 1. | Task 1.5 | Answered 2026-09-25: detection finds packages and the board shows a swimlane per package, and a project with no packages works, so monorepo mode is in Phase 1 (`internal/projects` detection, the extra-package-lane deviation in `design-port.md`) |
| Q8 | How much does tsnet add to download size, and does it fit the budget? | Task 9.1 | Open. Nothing uses tsnet yet |
| Q9 | Who holds the Apple Developer and Windows code signing accounts? | Task 0.14 | Open. Blocks signing and the release pipeline |
| Q10 | Should the onboarding sample project be a real small repo we ship, or generated on first use? | Task 2.16 | Deferred by the owner on 2026-09-25: the sample-project option is removed from onboarding until Phase 2 ships a sample repository (build plan task 2.25), and this question decides what that repository is |
| Q11 | Which languages get built-in smell checks at launch? Suggested: Go, TypeScript and JavaScript, and Python. Depends on Q4. | Task 5.12 | Open |
| Q12 | Default size limit per attachment? Suggested: 25 MB, editable in settings. | Task 10.12 | Open |
| Q13 | Should people be able to review agent ticks, for example "Confirm agent ticks" on required checklists? | Task 10.11 | Open |
| Q14 | Prototype: posting a comment, attaching a file, adding an item, and the phone layouts for these have not been clicked through yet. | Design review | Open |
| Q15 | Who confirms the macOS CI run was green? The Phase 1 work session has no network access, so it cannot read the CI results; the tracker records "macOS CI green" on the strength of the controller's report. | Phase 1 review | Answered 2026-09-26: the controller read the CI results. At `75cff4a` macOS and Ubuntu are green and Windows is red (not a blocker) |
| Q16 | Which Claude Code version should be the tested one? The adapter drives `claude -p` stream-json without a real run in any automated test, so the "supported" version is untested in practice until the owner runs one card by hand. | Task 1.8, before Phase 4 | Answered 2026-09-26: one real card ran on Claude Code 2.1.282 and replied. The interrupt stage and per-tool approvals (Phase 3) are still not exercised against the real program |

---

## Session log

Newest entries first.

### 2026-09-26

- **Done:** Phase 1 is closed. The whole check and the end-to-end suite were re-run by the controller (nine of nine steps, 192 specs), a hands-on browser pass was made, and all remaining work was pushed as 45 commits. The item-by-item record is "Phase 1 delivered" above.
- **Real checks, with the owner's approval:** one real Claude Code session on a card, on a throwaway daemon and a scratch repository. It first failed with "Not logged in" because the child environment dropped `USER`, so the allow-list gained `USER` and `LOGNAME` (decisions log); the rerun replied. The daemon was installed as a login item in normal mode, ran under launchd, came back after a forced crash, and was uninstalled cleanly. The empty `~/Library/Application Support/Marshal` data folder it created is left in place.
- **Also fixed:** a request the browser cancelled was logged as an internal error (now debug); the New project dialog kept a folder error after switching to the clone source.
- **Found, not Phase 1:** the CI end-to-end job never runs while the Windows check is red, because it waits for all three. Decision for the owner: leave it (macOS first) or let it wait for macOS only.
- **Next:** Phase 2. The owner wants the backend and the screens wired so every remaining piece of mock data is replaced before the next browser pass, so the first step is a section-by-section plan for approval.

### 2026-09-25

- **Phase 1 (first card), backend:** the daemon now owns real state. `internal/store` (SQLite in WAL mode, goose migrations 0001 to 0003, sqlc queries), `internal/events` (bus with a replay ring; ordinary events drop oldest-first, critical events never drop and instead close the subscription for a resync), `internal/gitx` (all Git access: worktrees, branches, clone, sparse checkout, the Git 2.38 check), `internal/proc` (the one child-process helper, arguments as a list, filtered environment), `internal/projects` (projects, boards, cards, detection, removal in the `architecture.md` 16.1 order), `internal/session` (start, send with queueing, stop, resume in auto and manual mode, rotated logs with a bounded in-memory ring), and `internal/api` (the `/v1` routes, token auth, and the `/v1/events` stream with topics, batching, replay, and resync). Agent adapters: `acp`, `claude` (stream-json, no ACP), `gemini`, `pty`, and `codex` (detection only).
- **Also done:** the client data layer (`apps/web/src/data`: API client, event stream with reconnect and resync, connection machine, token store, daemon clock, optimistic helper, section switch, mappers from the golden files), the mirror in `apps/web/src/sync` with one `Syncer` per section, the card-key change to `<project>#<number>`, the prototype fixture in dev mode, and the cutover of S1 (connection and sign-in), S3 (projects), and S4 (agents and models). `docs/progress-tracker.md` says "In review" for Phase 1; the controller sets "Done" after verification.
- **Verified:** the full check (`node scripts/check.mjs`), the end-to-end suite against a built throwaway dev daemon on port 47811, the Go suite with `-race`, and a real-binary smoke test with two daemons on throwaway data folders. The end-to-end suite passed 192 specs, the Go suite passed in all 24 packages, and the smoke test showed a card remembering an earlier turn after a daemon restart.
- **Next:** the two manual checks that need the owner (one real Claude Code session on a card, and `marshal service install` / `status` / `uninstall`), then Phase 2. Both were run on 2026-09-26, see below.

### 2026-09-25

- **Phase 0 (backend foundation), local:** Go 1.27.1 and the pinned tools, the daemon module with a health endpoint, settings, dev mode and the guarded dev reset, the protocol package generated with tygo, Go lint and smell checks, fixture repos, the stub agent, the webhook replay tool, and the daemon budgets. `pnpm check` and the 81 end-to-end specs pass. Not verified: real CI on three platforms.

- **Done:** the frontend port of the prototype (SolidJS app, `@marshal/ui`, tokens, typed mock store) is finished and pushed to `khanblair/marshal`. The parity check was used to build it and is now retired. The prototype toolbar was removed, buttons got larger corners and no-wrap labels, and 81 end-to-end specs check phone, tablet, and desktop. `pnpm check` passes: 9 tokens, 260 UI, and 1,258 web tests.
- **Also done:** wrote `backend-checklist.md` and `backend-inventory.md`, which map all 116 store members and 78 state fields to daemon modules, tables, API calls, events, and build plan tasks, and list 30 pieces of work the docs did not cover. Installed Go 1.27.1.
- **Next:** agree the checklist, then start Phase 0 of it: the daemon module, the Go tools, the protocol package, Go in CI, the stub agent, and dev mode.

### 2026-09-24

- **Done:** added the home dashboard, many chats per project, project and chat management, profile, onboarding, tutorial tour, and full responsive rules to the docs. Updated the project structure.
- **Also done:** added code smell checks as a Marshal feature and as a standard for our own code, across the scope, architecture, standards, build plan, library, UI, and development docs.
- **Also done:** moved tiles and charts to the top of Home, and added connection tests for every integration, provider, and MCP server.
- **Also done:** added `project-structure.md` with every planned file, a docs index, and file and doc changes logs in this tracker.
- **Also done:** documented the prototype's new checklists, comments, members, and "Add a card", and decided how checklists block merging, how Trello syncs them, and who gets notices.
- **Also done:** read every doc end to end and fixed the problems found. Text: removed the stale "SolidJS or Svelte" wording in `marshal-product-scope.md`, corrected the "Checks tab" reference in `ui-rules.md`, separated `size-header` from `size-topbar`, and defined what `bp-md` does. Tokens: checked every color pair against the contrast rules and fixed 23 failing pairs, and listed the checked pairs in `ui-tokens.md` section 2.9.
- **Next:** receive the Claude Design output and reconcile `ui-tokens.md`, `ui-rules.md`, and `ui-registry.md` with it (the design wins). Then click through the new prototype parts and their phone layouts (Q14), and sign off the docs (task 0.1).

### 2026-09-23

- **Done:** agreed scope, name, stack, and design direction. Wrote `project-overview.md`, `architecture.md`, `build-plan.md`, `code-standards.md`, `library-docs.md`, `ui-tokens.md`, `ui-rules.md`, `ui-registry.md`, `development.md`, and this tracker.
- **Next:** review and sign off the docs (task 0.1), then set up the monorepo (task 0.2).

---

## File changes log

Every file added, moved, renamed, or deleted in the repo. Newest first. The planned structure is in `project-structure.md`.

| Date | File | Change | Task | Notes |
|---|---|---|---|---|
| 2026-09-26 | `daemon/internal/proc/env.go`, `daemon/internal/proc/proc_test.go` | Changed | 1.8 | The child environment allow-list gained `USER` and `LOGNAME`, found by the real Claude Code session; test added |
| 2026-09-26 | `daemon/internal/api/errors.go`, `daemon/internal/api/respond_test.go` | Changed | 1.12 | A request the client cancelled is logged at debug, not as an internal error |
| 2026-09-26 | `apps/web/src/app/dialogs/NewProjectDialog.tsx`, `NewProjectDialog.test.tsx` | Changed | 1.4 | The daemon's refusal is dropped when the source changes |
| 2026-09-26 | `daemon/internal/api/hijack.go`, `daemon/internal/agents/eventsink_test.go`, `apps/web/src/data/format.ts`, `apps/web/src/data/format.test.ts`, `apps/web/src/data/layering.test.ts` | Added | 1.12, 1.6, 1.14 | The raw-connection recorder that ends a hung WebSocket close, the event sink tests, `relTime` moved out of `mock/`, and a test that keeps the data layer from importing the mock |
| 2026-09-26 | `apps/web/src/mock/actions/projects.ts`, `apps/web/src/mock/seed/projects.ts` | Deleted | 1.17 (S3) | The mock projects code, replaced by the daemon's projects |
| 2026-09-25 | `daemon/internal/{store,events,proc,projects,session,api,fixture}`, `daemon/internal/store/{migrations,queries,db}`, `daemon/internal/agents/{acp,claude,gemini,pty,codex,catalog}`, `daemon/testdata/golden/*` | Added | 1.1 to 1.13 | The Phase 1 daemon: SQLite store with goose migrations and sqlc queries, event bus with a replay ring, the child-process helper, projects and boards with cards, the session manager with logs, the `/v1` API and `/v1/events`, the prototype fixture, the five agent adapters plus the catalog, and the golden wire files |
| 2026-09-25 | `daemon/cmd/marshald`, `daemon/cmd/marshal` | Changed | 1.3, 1.9, 1.10, 1.12 | `marshald` wires the modules, restores sessions on start, stops a removed project's sessions, and loads the fixture; `marshal` gained `service install|uninstall|status` beside `status`, `token`, and `dev reset` |
| 2026-09-25 | `apps/web/src/data/` and `apps/web/src/data/{mappers,testing}` | Added | 2.13 (N29) | The client data layer: API client with typed errors, event stream with reconnect and resync, connection machine, token store, daemon clock, optimistic helper, section switch, the mappers that read the golden files, and their test doubles |
| 2026-09-25 | `apps/web/src/sync/` | Added | 1.14, 1.17 (S3, S4) | The mirror outside `mock/`: the `Syncer` interface, `startSync`, the projects syncer and its actions, the agents syncer and the Built-in agent entry, the reservoir that keeps mock records out of `S`, and the connection actions |
| 2026-09-25 | `apps/web/src/testing/` | Added | 2.13, 1.17 | The test store, the fake daemon (API and event stream in memory), the golden-file builders for projects and agents, and the test-store installer |
| 2026-09-25 | `apps/web/e2e/` and `apps/web/e2e/support/` | Added | 1.14, 1.16, 1.17 | Playwright specs for connection, projects, agents, lifecycle, and responsive sizes, with `scripts/e2e-daemon.mjs`, the throwaway daemon on port 47811, and the global setup guard |
| 2026-09-25 | `apps/web/src/mock/`, `apps/web/src/app/`, `apps/web/src/views/`, `apps/web/src/onboarding/`, `apps/web/src/features/agents/` | Changed | 1.14, 1.15, 1.17 | The screens read the daemon: the pickers read the catalog, the card key became `<project>#<number>` everywhere, the connection, loading, empty, error, and offline states were added from existing components, and mock-only code for the cut-over sections was removed (`mock/actions/projects.ts` and `mock/seed/projects.ts` deleted, replaced by `sync/project-actions.ts` and the daemon) |
| 2026-09-25 | `packages/protocol/src/generated/index.ts`, `packages/protocol/test/*` | Changed | 1.12, 1.13 | Generated from the Go wire types with tygo, plus the hand-maintained enum list test and the golden-file tests |
| 2026-09-25 | `packages/ui/src/layout/` and `packages/ui/src/base/` | Changed | 1.12, 1.13, 2.13 | `ConnectionLost`, `OfflineBanner`, `SignIn`, and the skeletons for the connection screens, plus the button and icon-button work from the design deviations |
| 2026-09-25 | `scripts/{check,smells,budgets,gen-protocol,go-tool,setup-tools,e2e-daemon}.mjs`, `.golangci.yml`, `.golangci.warn.yml`, `daemon/sqlc.yaml`, `daemon/tygo.yaml`, `daemon/.air.toml` | Added, Changed | 0.15, 1.1, 1.12 | The check pipeline, the smell checks, the budgets, protocol generation, the pinned-tool runner, the e2e daemon, and the Go lint limits |
| 2026-09-25 | `daemon/` (`go.mod`, `package.json`, `.air.toml`, `tygo.yaml`, `cmd/marshald`, `cmd/marshal`, `internal/{api,buildinfo,config,gitx,platform,protocol,testutil}`, `testdata/{golden,hooks,repos}`) | Added | 0.2, 0.3, 0.8, 0.10 | Health endpoint on localhost, settings from flags and environment, dev token, guarded dev reset, gitx with the Git version check, fixture repos, golden files |
| 2026-09-25 | `tools/stub-agent/` | Added | 0.11, 1.6 (built early) | Scripted ACP agent with resume, six JSON scenarios, 105 tests |
| 2026-09-25 | `tools/hooks-replay/`, `tools/budgets/` | Added | 0.9, 0.13 | Webhook replay, and the daemon idle RAM and CPU budget |
| 2026-09-25 | `packages/protocol/` | Added | 0.7 | Types generated from Go with tygo, and the golden file test |
| 2026-09-25 | `scripts/setup-tools.mjs`, `scripts/go-tool.mjs`, `scripts/gen-protocol.mjs`, `.golangci.yml`, `.golangci.warn.yml` | Added | 0.1, 0.7, 0.15 | Pinned Go tools, a cross-platform tool runner, protocol generation, and the Go lint limits |
| 2026-09-25 | `package.json`, `pnpm-workspace.yaml`, `knip.json`, `.jscpd.json`, `biome.json`, `.gitignore`, `scripts/{check,smells,budgets}.mjs`, `apps/web/vite.config.ts`, `apps/web/src/app/keyboard.ts`, `.github/workflows/ci.yml` | Changed | 0.3, 0.4, 0.6, 0.9 | Go added to the root scripts, smells, and budgets, and the Vite proxy to the dev daemon. Windows script fix and a guarded focus timer. CI runs Go (not run yet) |
| 2026-09-25 | `apps/web/`, `packages/ui/`, `packages/tokens/`, `design/`, `.github/`, and the root config files | Added | 0.4, 0.6, 0.8, 0.12, 0.15 | The frontend workspace, the prototype port, and its checks, from earlier in the project |

---

## Doc changes log

Every change to a doc. Newest first.

| Date | Doc | Change |
|---|---|---|
| 2026-09-26 | `progress-tracker.md` | Phase 1 moved to Done, a "Phase 1 delivered" section with each checklist item and how it was verified, a session log entry, a decision for the `USER` and `LOGNAME` allow-list, Q15 and Q16 answered, and the file changes since |
| 2026-09-26 | `backend-checklist.md` | B1.0 to B1.17 ticked |
| 2026-09-26 | `project-structure.md` | The two work-session files (`prompt.md`, `deepseek-report.md`) are no longer listed, since they are not kept in the repo |
| 2026-09-25 | `progress-tracker.md` | Phase 1 moved to In review, current state and phase status updated, a Phase 1 session log entry, the Phase 1 decisions (card keys, Built-in agent, fixture, WebSocket auth, design deviations, verification, the service installers, the Claude Code adapter, macOS first), and the file and doc changes for all of Phase 1 |
| 2026-09-25 | `architecture.md` | Section 4.2 now points at the Claude adapter's own source instead of a package report that does not exist; section 11.1 says which routes exist today and which are planned; section 11.2 says which event types are published today, names the 25 ms flush interval, and points at `event_types.go`; section 11.6 documents the client data layer, the mirror, the syncers, and the agents syncer with the Built-in agent entry |
| 2026-09-25 | `development.md` | Every command and setting made true for today: the `pnpm dev` chain (the stub-agent build first), no `dev:desktop`, `dev:stub`, `test:agents`, `build:daemon*`, `build:desktop`, or `pnpm marshal` script yet, no `marshal keys` command yet, no `--tailnet` flag yet (so no real-device and no Funnel webhooks), the everyday-command and build tables brought in line with `package.json`, the workspace table gained `hooks-replay` and `budgets`, and the one real-agent check written out as a manual procedure |
| 2026-09-25 | `library-docs.md` | `coder/websocket` (v1.8.15) and `golang.org/x/sync/errgroup` (v0.23.0) entries written out with version, use, and the seven checks; `go-cmp` marked considered and not used (it is in no `go.mod`); the Go testing version pinned; `kardianos/service` recorded as considered and not used (zlib license) |
| 2026-09-25 | `design-port.md`, `backend-checklist.md`, `backend-inventory.md`, `project-structure.md` | S4 rows, the S4 register entries, and the Phase 1 file lists from the cutover of S1, S3, and S4 |
| 2026-09-25 | `build-plan.md`, `backend-checklist.md`, `backend-inventory.md`, `progress-tracker.md` | Added 39 tasks marked (prototype) to the build plan, linked each inventory item to its task, and recorded the ten decisions |
| 2026-09-25 | `backend-checklist.md`, `backend-inventory.md`, `README.md`, `project-structure.md`, `progress-tracker.md` | Added the backend checklist and the inventory of every prototype action and field, listed both in the index and the docs structure, and logged the backend decisions |
| 2026-09-25 | `ui-tokens.md`, `ui-rules.md`, `design-port.md`, `architecture.md`, `progress-tracker.md` | Colors back to the design's exact values, `color-border-control` removed, section 2.9 rewritten around the exceptions list, chart values fixed, prototype toolbar and `z-prototype` removed, web build size budget added. |
| 2026-09-24 | `ui-tokens.md`, `ui-rules.md`, `marshal-product-scope.md`, `build-plan.md`, `progress-tracker.md` | Consistency and contrast fixes: new `color-border-control` token, adjusted muted text, status solid, and chart values, listed the contrast pairs checked in CI, clarified `size-header`, `size-topbar`, and the breakpoints, fixed split view pane counts, and replaced stale UI framework and "Checks tab" wording |
| 2026-09-24 | `marshal-product-scope.md`, `architecture.md`, `ui-rules.md`, `ui-registry.md`, `ui-tokens.md`, `build-plan.md`, `project-structure.md`, `project-overview.md` | Added "Add a card", checklists, comments with attachments and links, members and their notices, and Trello sync for these |
| 2026-09-24 | `project-structure.md`, `README.md` | Added. Full planned file list and docs index. |
| 2026-09-24 | `code-standards.md`, `project-overview.md`, `ui-registry.md`, `build-plan.md` | Linked to `project-structure.md`. Screen components placed in `apps/web`. |
| 2026-09-24 | `ui-rules.md`, `marshal-product-scope.md`, `architecture.md`, `ui-registry.md`, `build-plan.md` | Home order changed to tiles and charts first. Connection tests added. |
| 2026-09-24 | All docs | Code smell checks added, in Marshal and for our own code |
| 2026-09-24 | All docs | Home dashboard, many chats, project and chat management, profile, onboarding, tutorial, and responsive rules |
| 2026-09-23 | `development.md` | Added. pnpm based dev and release workflow. |
| 2026-09-23 | Nine core docs | First versions written |

---

## Notes for later

Things found during work that are outside the current task. Each one should become a task or be closed.

| Date | Note | Found during |
|---|---|---|
| | | |
