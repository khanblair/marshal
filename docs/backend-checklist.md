# Backend checklist

This is the checklist for building the daemon and connecting it to the app we already have, so the prototype's dummy data is replaced by real work. It says what to build, in which order, how each piece is finished, how it is tested, and which rules it is checked against. It contains no code.

It works together with three other documents:

- `build-plan.md` is the order of the work. This checklist follows it phase by phase as a guide, and uses its task IDs (shown as "Task 1.4"). The order is not strict (see 2.7).
- `backend-inventory.md` lists everything the prototype does and shows, and where each piece lives in the backend. Everything in it that the docs did not cover yet is built here (entries N1 to N30).
- `architecture.md`, `code-standards.md`, `development.md`, `library-docs.md`, `ui-rules.md`, and `design-port.md` are the rules. Section 7 maps each of them to the items below.

Item IDs look like B2.4 (phase 2, item 4). Tick an item only when its "Done when" is true and the definition of done in section 4 is met.

---

## 1. Decisions from the owner

Made on 2026-09-25. They are logged in `progress-tracker.md`.

- The backend follows the prototype. Anything the prototype does that the docs do not describe is built, and the docs are updated to match. The full list is in `backend-inventory.md` section 5.
- Everything is real, and built roughly in build plan order. The order is not strict: when a piece needs something a later phase provides, that piece is built early and skipped when its turn comes (see 2.7). No integration is left as a stand-in. Trello, Gmail, Google Calendar, Telegram, Discord, Tailscale, GitHub, and the model providers are each connected for real when their phase comes.
- The mock is replaced section by section. A section stops using mock data only after it has been verified working with the daemon. A section that is not integrated yet keeps its mock data. When every section is integrated, the mock is deleted.
- Go is installed (version 1.27.1, on 2026-09-25). The pinned Go tools are set up in item B0.1.
- "Simulate CI failure" is built as a real feature with two modes: a synthetic failed run that goes through the daemon's CI monitor, and a real failing run on GitHub (N28).
- Screen preferences follow the user between devices (theme, list columns, sort, last view, filters, swimlane), and layout stays on the device. Labels are a managed list per project with a name and a color. Keep awake lasts 15 minutes and is a setting. Timeline bars show the actual span, and the planned span for cards not started. Solo use shows only the owner.
- A service that is not set up shows a "Not connected" state with a Connect button, and no sample data.
- Windows and Linux are covered by CI, plus a manual pass on real machines before the first release.
- The 30 items from the inventory are numbered tasks in `build-plan.md`, each with its screen work, so the backend and its screens are built together.
- Each project numbers its own cards. A card is identified by its project and its number, and any place that mixes projects shows the project name next to the number (N26).
- The desktop shell and signed releases (Tasks 0.5 and 0.14) are built after Phase 2, once the board works against the daemon.
- Parity checking against the prototype stays retired. Approved deviations stay in `design-port.md`.
- Commits touch at most 10 files, and pushes to GitHub are batched, because every push to `main` runs the three-platform CI.

---

## 2. How the daemon replaces the mock

### 2.1 The seam

The screens keep calling the same store they call today, `M`, with the same actions and the same state shape. What changes is what is behind it:

- **An API client.** One client for every call, as `code-standards.md` section 4.2 requires. No screen calls `fetch` on its own.
- **An event stream.** One WebSocket connection that applies daemon events to the state, reconnects when it drops, and re-syncs from a snapshot when it has missed events.
- **A mapper.** A small tested layer that turns the generated protocol types into the shapes the screens already use, including turning absolute timestamps into "4 min ago" and countdowns.
- **Optimistic updates.** An action changes the screen at once, then the daemon's answer confirms it or rolls it back with a plain message.

The daemon owns all state. The client mirrors it. This keeps the daemon the single source of truth, as `architecture.md` section 1 says.

### 2.2 Sections and the cutover register

A section is one part of the screens that can switch from mock data to the daemon on its own. Its status is Mock until its cutover gate (2.4) passes, then Daemon. Update the status in the same commit that switches it.

| Section | Screens | Phase and build plan tasks | Status |
|---|---|---|---|
| S1 Connection and sign-in | Boot, token or pairing, "Can't reach the daemon", reconnect | 1: 1.12, 1.13 | Daemon |
| S2a Profile | Avatar menu, name, email, time zone, avatar | 2: 2.15 | Daemon |
| S2b Devices and Tailscale identity | Paired devices, tailnet name and node on the profile | 9: 9.1, 9.2 | Mock |
| S2c Team and people | Member pickers and people beyond the owner | 12: 12.9 | Mock |
| S3 Projects | Sidebar list and badges, create, rename, remove, project settings | 1: 1.4, 2.14 | Daemon |
| S4 Agents and models | Agent, model, and thinking pickers everywhere | 1: 1.8 | Daemon |
| S5a Cards | Board, List, Timeline, Agents view, new card, quick add, move, rename, delete, basic fork | 2: 2.4, 2.10 | Daemon |
| S5b Templates, dependencies, duplicates | Template choice, dependency links, duplicate warning | 10: 10.1, 10.2, 10.6 | Mock |
| S5c Package swimlane and filter | Monorepo packages on cards | 12: 12.5 | Mock |
| S6a Saved views | Saved views menu | 2: 2.5 | Daemon |
| S7a Card header and control | Header, settings selects, start and stop | 2: 1.9, 2.4 | Daemon |
| S7b Bypass | Bypass confirmation, banner, project lock | 3: 3.2 | Mock |
| S7c Pause, sleep, wake, pin | Session hold and sleep controls | 2: 5.10, 5.16 (the controls a person presses; the idle timer and reminders stay in Phase 5) | Daemon |
| S8a Card chat | Messages, tool calls, diff summaries, system notes, composer | 2: 2.6 | Daemon |
| S8b Approvals | Approval blocks in cards, chats, and Home | 3: 3.5 | Mock |
| S8c Plans | Plan blocks and plan actions | 5: 5.2 | Mock |
| S9 Terminal | Terminal view and the chat and terminal switch | 2: 2.7 | Daemon |
| S10 Card activity | Activity tab | 2: 2.8 | Daemon |
| S11 Card diff | Diff tab | 2: 2.9 | Daemon |
| S12 Card checks | Checks tab | 10: 10.4 | Mock |
| S13 Card preview | Preview tab | 6: 6.6, 6.7 | Mock |
| S14 Card notes | Notes tab | 7: 7.6, 7.7 | Mock |
| S15 Checklists | Checklists on a card | 10: 10.10, 10.11 | Mock |
| S16 Comments and members | Comments tab, members row | 10: 10.12, 10.13 | Mock |
| S17 Project chats | Chats view | 2: 2.11 | Daemon |
| S18 Home: needs you and awake | Needs you list, working and merged tiles, awake agents | 2: 2.3 | Daemon |
| S19a Home: cards finished | Cards finished per day chart | 2: 2.3 | Daemon |
| S19b Home: cost | Cost tile and cost per project chart | 4: 4.7, 4.8 | Mock |
| S20 Home: activity | Recent activity and its view-all page | 2: 2.3 | Daemon |
| S21 Home: CI health | CI health list and its view-all page | 6: 6.2, 6.4 | Mock |
| S22 Home: coming up | Events, jobs, briefs, due cards | 8: 8.1, 8.5 | Mock |
| S23 Notices | Bell and every notice kind, including sleep reminders | 5: 5.10 | Mock |
| S24a Palette and search | Command palette and top bar search over projects, cards, and chats | 2: 2.12 | Daemon |
| S24b Session and note search | Search over past sessions and notes | 7: 7.10 | Mock |
| S25 Calendar | Calendar view | 8: 8.5, 12.3 | Mock |
| S26a Settings: sleep | Idle time, warning time, reminder channel, restore mode | 5: 5.10 | Mock |
| S26b Settings: limits | Cost and awake limits | 4: 4.8 | Mock |
| S27 Settings: Roles | Role list and editor | 5: 5.1 | Mock |
| S28 Settings: Providers | Provider keys and tests | 4: 4.1, 4.9 | Mock |
| S29a Integration: GitHub | GitHub row in Settings | 6: 6.1 | Mock |
| S29b Integration: Obsidian | Obsidian row | 7: 7.7 | Mock |
| S29c Integration: Trello | Trello row | 8: 8.4 | Mock |
| S29d Integration: Google Calendar | Google Calendar row | 8: 8.5 | Mock |
| S29e Integration: Gmail | Gmail row | 8: 8.6 | Mock |
| S29f Integration: Telegram | Telegram row | 9: 9.5 | Mock |
| S29g Integration: Discord | Discord row | 9: 9.6 | Mock |
| S30 Settings: Schedules | Briefs and jobs | 8: 8.1 | Mock |
| S31a Onboarding and tour | Welcome, agents, project, and the Home tour | 2: 2.16, 2.17 | Daemon |
| S31b Onboarding: connect from anywhere | The pairing and chat apps step | 9: 9.2 | Mock |
| S32 Screen preferences | Theme, list columns, sort, last view per project, filters, swimlane | 2: 2.5 | Daemon |

Shortcuts and Help in Settings are static and have no backend. They are not sections.

### 2.3 Rules while some sections are on the daemon and some are not

1. **Sections that show the same objects move together.** If the Board shows real cards, the List, the Timeline, the Agents view, the card panel header, and the Home needs you list must show real cards too, or a card could appear in one place and not another. That is why S5a is one section, and why a card-based section never cuts over before S5a has.
2. **A mock-backed section must tolerate real IDs.** When a real card, project, or chat opens a section that is still on the mock (for example Comments), that section shows its empty state for it. It never fails and never invents content for it. This is tested in both directions.
3. **One switch per section, never half a section.** A part of a screen that arrives in a later phase is its own section (for example S8a, S8b, and S8c for chat, approvals, and plans), so each section switches once. The register in 2.2 is the only place that says which sections are on the daemon.
4. **A section that needs a service that is not set up shows "Not connected"**, with a Connect button and no sample data, until the connection test for that service passes. Sections whose backend is not built yet stay on the mock. A screen is never mixed, for example real cards with mock calendar events, unless it is one of the Home blocks, which are separate sections on purpose.
5. **The mock's removal is part of the cutover.** When a section switches, its mock seed data, its simulation, and its mock-only tests are deleted or moved in the same change.

### 2.4 Cutover gate

A section may switch to the daemon only when all of this is true. Use it as the "Done when" of every cutover item.

- [ ] Every row of the section in `backend-inventory.md` is built: storage, API, events, and generated types.
- [ ] Daemon tests pass: unit, integration with the stub agent and fixture repos, and the contract test for each API call and event.
- [ ] The mapper has tests built from golden files the daemon tests produce, so the two sides cannot drift.
- [ ] The screens work against the dev daemon at phone, tablet, and desktop sizes, in both themes, with no console errors, no sideways scrolling, and no wrapped button labels.
- [ ] The section has its loading, empty, error, and offline states, built from existing components and tokens (`ui-rules.md`, task 2.13).
- [ ] The section's flows have end-to-end tests that run against the dev daemon with a fixture project.
- [ ] Its mock data, simulation, and mock-only tests are removed or migrated, and `knip` reports no dead code.
- [ ] The register in 2.2, `architecture.md`, `project-structure.md`, `progress-tracker.md`, and the file changes log are updated.
- [ ] `pnpm check` passes, and the web budgets in `architecture.md` section 14 still hold.

### 2.5 Retiring the mock

When every section is on the daemon (item B13.1):

- The mock folder, its seed data, its simulation, and the differential test harness that loads the design's `store.js` are deleted. The design files stay in `design/` as the reference.
- The `M` store stays, because the screens use it, but it holds only mirrored daemon data and screen state.
- Tests that used mock data now use typed fixture builders, made from the same golden files as the mapper tests, and the end-to-end tests use the dev daemon with a fixture project.

### 2.6 Testing once the mock is gone

| Layer | How |
|---|---|
| Daemon units | Table-driven Go tests with the race detector and `goleak` where goroutines start |
| Daemon integration | The stub agent, fixture repos, and recorded webhooks. Never a real model API in CI. |
| Contract | Golden files for every API response and event, checked on the Go side and used to test the client mapper |
| Client units and components | Vitest with fixture builders. No network. |
| End to end | Playwright against the dev daemon started in dev mode with a fixture project, at phone (390 by 844), tablet (820 by 1180), and desktop (1440 by 900), in both themes |
| Real agents | Nightly smoke tests against pinned CLI versions. Claude Code and Gemini CLI are installed on the dev machine. Codex is not, which exercises the "missing" state. |
| Budgets | RAM, CPU, size, and timing checks on every pull request |
| Platforms | macOS, Linux, and Windows in CI |

### 2.7 Building out of order

The phases are a guide, not a fence. Integration often needs something a later phase provides (for example, moving a card to In review needs a pull request, which Phase 5 builds).

1. **Build it early when something needs it.** Do the item at the moment it is needed, with its full "Done when" and the definition of done in section 4.
2. **Record it once.** Tick the item in its own phase, add "built early in" and the item that needed it, and log it in `progress-tracker.md`.
3. **Skip it when its turn comes.** When the phase arrives, only re-check that its "Done when" still holds.
4. **Stay inside the sections.** Building early never switches a section early. A section still cuts over only when its own gate passes.
5. **Phase gates still apply** at the end of each phase, and cover everything built early inside it.

---

## 3. What the owner does

Some steps need an account, a key, or a decision that only the owner can make. These are never typed into chat or stored in files. Keys go into the keychain with the `marshal keys` command, and account setup is done by the owner in the provider's own screens.

| When | What | Task |
|---|---|---|
| Now | Approve installing Go (done) | 0.3 |
| Phase 3 or 4 | Add model provider keys with the keychain command | 4.1, 4.2 |
| Phase 5 | Sign in to GitHub for pull requests | 5.6 |
| Phase 6 | Create and install the GitHub App | 6.1 |
| Phase 8 | Create the Trello key and token, and the Google Cloud OAuth client for Calendar and Gmail | 8.4 to 8.6 |
| Phase 9 | Sign in to Tailscale, allow Funnel for `/hooks/*` in the tailnet policy, and create the Telegram and Discord bot tokens | 9.1 to 9.6 |
| Release | Provide the macOS Developer ID and notarization access and the Windows signing certificate as CI secrets | 0.14 |
| Optional | Install the Codex CLI to test an installed Codex agent | 1.8 |

An integration whose account step is not done yet shows "Not connected" (rule 4 in 2.3).

---

## 4. Definition of done for every item

An item is done only when all of these are true. This is the code standards, the architecture rules, and the development guide as one list.

- [ ] The "Done when" of the item is true, and its tests pass.
- [ ] New code has tests. A bug fix has a test that failed before the fix.
- [ ] Coverage stays at or above 70 percent for daemon modules and 60 percent for the UI, and 85 percent for `harness`, `security`, `integrator`, and `session`.
- [ ] `pnpm check` passes: format, lint, code smells, tests, and budgets. No new smell. Blocking limits (function 100 lines, file 800 lines, 6 parameters, complexity 25) are not passed, and warnings are fixed or explained.
- [ ] Go code is formatted with `gofmt` and `goimports`, passes `golangci-lint`, and its tests pass with `-race`. Packages that start goroutines pass `goleak`.
- [ ] Every blocking call takes a context, every goroutine has an owner and a way to stop, and there is no global mutable state.
- [ ] Errors are wrapped with context and never swallowed. Errors shown to users are plain sentences at the API layer, following the copy rules in `ui-rules.md`.
- [ ] Logs use `log/slog` with `project_id`, `card_id`, or `session_id` where relevant, and never contain secrets, tokens, full prompts, or full file contents.
- [ ] Schema changes are forward-only migrations. Queries are SQL turned into Go by sqlc. No transaction wraps a model or outside call.
- [ ] Git is called only through `gitx`. Child processes are started only through the one helper. User or agent text is never put into a shell.
- [ ] Modules talk through their interfaces and the event bus, never through each other's tables.
- [ ] No polling loop unless `architecture.md` calls for one. High-rate events are batched every 16 to 50 milliseconds. Every buffer, queue, and cache has a documented bound. Large data goes to disk.
- [ ] Untrusted input (agent output, webhook bodies, integration data, imported skills, vault content, attachments) is treated as data, webhook signatures are checked before the body is read, and permission checks happen in the harness, not only in the UI.
- [ ] No secret is stored in code, config, tests, or logs. Credentials go through the keychain module.
- [ ] A new library passes the seven checks in `library-docs.md` section 1 before it is added, is pure Go for the daemon, and gets an entry there.
- [ ] Wire types are defined once in Go and generated for the UI. Generated files are never edited by hand.
- [ ] UI code uses tokens and components from `@marshal/ui`, has no raw hex colors, pixel sizes, or font names, no all-caps text, and no inline style except a truly dynamic value.
- [ ] Screens work at phone, tablet, and desktop sizes, with touch targets of at least 44 px on touch screens.
- [ ] A budget-affecting change includes a before and after measurement in the commit message or pull request.
- [ ] `architecture.md`, `project-structure.md`, `library-docs.md`, `development.md`, and `ui-registry.md` are updated where the change touches them, with a row in the file changes log and the doc changes log in `progress-tracker.md`.
- [ ] Commits use Conventional Commits, mention the task ID in the body, touch at most 10 files, and are pushed in batches.

---

## 5. Phases

Each phase lists its items, the sections it cuts over (from 2.2), the work from the inventory it delivers (N1 to N30), and a gate. Items marked "already done" were finished for the frontend and are listed so nothing is done twice.

### Phase 0: Foundation (build plan Phase 0)

**Goal:** the repo can build, test, and check Go, TypeScript, and the generated types before any feature code.

Already done for the frontend:

- [x] The pnpm workspace, root scripts, and Node and pnpm pins (Task 0.12, frontend part)
- [x] The SolidJS app with Vite and Tailwind (Task 0.4)
- [x] The tokens package with the contrast check (Task 0.6)
- [x] Biome, `knip`, `jscpd`, and the smell script for TypeScript, and the CI for TypeScript (Tasks 0.8 and 0.15, frontend part)
- [x] The web size budget (Task 0.9, frontend part)

To do:

- [x] **B0.1 Go and tools.** Go 1.27.1 was installed with Homebrew on 2026-09-25 (approved by the owner). Then `pnpm setup:tools` installs `air`, `sqlc`, `tygo`, and `golangci-lint` into `.tools/`. Done when: the pinned versions are in `daemon/go.mod` and `library-docs.md`, CI uses the same versions, and `go version` and each tool run on this machine. (Task 0.3) Status: done on 2026-09-25.
- [ ] **B0.2 Daemon module.** Create `daemon/` with `cmd/marshald`, `cmd/marshal`, a scripts-only `package.json`, and `internal/` packages as they are needed (no empty packages). Done when: both binaries build on macOS, Linux, and Windows in CI. (Tasks 0.2, 0.3) Status: not ticked. Both binaries build and pass tests here, and cross-compile for macOS, Linux, and Windows, but three-platform CI has not run.
- [x] **B0.3 Root scripts.** Add `pnpm dev`, `dev:daemon`, `dev:web`, `gen`, `test:daemon`, and Go in `lint`, `smells`, `check`, and `build`, as in `development.md` section 4. The Vite dev server forwards API and WebSocket calls to the dev daemon so the browser talks to one address. Done when: each command works on all three platforms. (Task 0.12)
- [x] **B0.4 Go smell tooling.** Add the `golangci-lint` config that matches `code-standards.md` section 12.2, jscpd over Go, and the new-smells-only check with an empty baseline. Done when: CI fails on a test change that adds a blocking smell and passes when it is fixed. (Task 0.15)
- [x] **B0.5 Protocol package.** Create `packages/protocol` and generate TypeScript from Go with `tygo` inside `pnpm gen`. Done when: one sample type round-trips in a test, and the generated folder is marked as generated, ignored by hand edits, and covered by `knip`. (Task 0.7)
- [ ] **B0.6 Go in CI.** Add format, lint, race tests, and builds for Go on the three platforms to the existing workflow. Done when: the pipeline runs on every pull request. (Task 0.8) Status: not ticked. The workflow is written, but CI has not run because nothing was pushed.
- [x] **B0.7 Daemon budgets.** Add the budget harness (`tools/budgets`) that measures daemon idle RAM and CPU, and connect it to `pnpm budgets` next to the web budgets. Done when: the report runs in CI and fails when over budget. (Task 0.9)
- [x] **B0.8 Fixture repos.** Create a small repo and a monorepo under `daemon/testdata/repos`. Tests copy them to a temp folder. Done when: both load in an integration test. (Task 0.10)
- [x] **B0.9 Stub agent.** Create `tools/stub-agent`, a fake agent that speaks ACP, supports resume, and is scripted: ask for approval, fail a test, get stuck in a loop, survive a restart, write code with known smells. Start its scripts from the mock's simulation (`mock/sim/scripts.ts` and `ci-failure.ts`), which already act these cases out. Done when: harness tests run without real models. (Task 0.11) Status: done. It uses `github.com/coder/acp-go-sdk` v0.13.5, so the ACP decision of task 1.6 is made and its JSON-RPC layer was built early. Scenarios are JSON files.
- [x] **B0.10 Dev mode.** Add the `--dev` flag with a separate data folder, port 47801, keychain entries, a dev token accepted on localhost only, the stub agent by default, the `MARSHAL_*` environment settings, `pnpm dev:reset` with confirmation, and `pnpm hooks:replay` with recorded webhooks in `daemon/testdata/hooks`. Done when: a dev daemon and a normal install run side by side without touching each other. (Task 0.13, `development.md` sections 3.4 to 3.9) Status: done except the keychain entries, which wait for the `go-keyring` library check in `library-docs.md` section 1.
- [ ] **B0.11 Desktop shell and release (deferred).** The Tauri shell and the signed release pipeline do not block replacing the mock. The owner decided to build them after the Phase 2 milestone. Done when: the Tauri window loads the app and the daemon starts as its sidecar on all three platforms, and a test release produces signed installers. (Tasks 0.5, 0.14)

**Gate:** `pnpm check` runs Go and TypeScript checks on three platforms, generated types are reproducible, and the stub agent and fixture repos are usable from tests.

### Phase 1: First card (build plan Phase 1)

**Goal:** one project, one card, one real agent, a lasting session, and a worktree, reachable from the screens.

**Sections cut over:** S1, S3, S4. **Inventory delivered:** N16, N19, N25, N26, N29 (connection part).

- [x] **B1.0 Protocol conventions.** Write down and generate the rules every later item uses: absolute timestamps in UTC and the daemon's current time in each snapshot (N25), IDs (N26), one error shape with a stable code and a plain message, cursor paging, the fixed lists of enumerations (card states, permission modes, thinking modes, feed kinds, notice kinds, activity kinds), and the event envelope with a topic and a sequence number for re-sync. Done when: `architecture.md` section 11 describes them and a contract test covers each. (Task 1.12, N25, N26)
- [x] **B1.1 Store.** SQLite in WAL mode with one writer connection and a small reader pool, embedded migrations, generated queries, and the tables the phase needs. Done when: migrations run on start, are forward only, and tests pass with a temp database. (Task 1.1)
- [x] **B1.2 Event bus.** In-process publish and subscribe with per-subscriber buffers, and critical events never dropped. Done when: tests cover slow subscribers and a dropped buffer that re-syncs. (Task 1.2)
- [x] **B1.3 Service and lock.** Install the daemon as a per-user service on all three platforms with a single-instance lock. Done when: it starts at login and survives closing the app. (Task 1.3)
- [x] **B1.4 Projects module.** Create from a folder or a clone, rename, edit, remove, one board per project, columns, and the project fields the screens show: language, default branch, dev command, and bypass lock (N16). Remove follows the order in `architecture.md` section 16.1 with the keep-branches and keep-memory options and never touches the repo folder. Done when: every project action works through the API, and a test proves the repo folder is untouched. (Task 1.4, N16)
- [x] **B1.5 Git module.** Create and remove worktrees and branches through `gitx` only, and require Git 2.38 or newer. Done when: a worktree exists on card start and is gone on close. (Task 1.5)
- [x] **B1.6 Agent interface and ACP adapter.** Done when: the stub agent starts, sends events, and resumes. (Task 1.6)
- [x] **B1.7 PTY adapter.** Done when: a real CLI runs in a pseudo-terminal and its output is captured on all three platforms. (Task 1.7)
- [x] **B1.8 Claude Code, Gemini CLI, and Codex.** Support each through its best available mode, and detect which are installed and which version, marking each supported, untested, or missing. `GET /v1/agents` returns kind, version, status, models, and capabilities including thinking support, and the screens' fixed agent and model lists are replaced by it (N19). Done when: a real Claude Code session runs on a card, Gemini CLI is detected, and Codex shows as missing on this machine. (Task 1.8, N19)
- [x] **B1.9 Session manager.** Start, send, stop, and persist the session ID. Done when: messages go into the same process, with no new process per message. (Task 1.9)
- [x] **B1.10 Resume.** After a daemon restart or a reboot, in automatic and manual modes. Done when: a card continues with full context. (Task 1.10)
- [x] **B1.11 Session logs.** Full output to disk under `<data>/logs/sessions/`, rotated, with a small in-memory ring buffer. Done when: memory stays flat under heavy output. (Task 1.11)
- [x] **B1.12 API and event stream.** The `/v1` HTTP API and the `/v1/events` WebSocket with topics and batching every 16 to 50 milliseconds. Done when: card events reach a test client in batches and a client that missed events can re-sync. (Task 1.12)
- [x] **B1.13 Auth and binding.** Client tokens on every request, listening on localhost and the tailnet address only, and a dev token on localhost only in dev mode. Done when: a request without a valid token is rejected and a test proves the daemon does not listen on all interfaces. (Task 1.13)
- [x] **B1.14 Client data layer.** Build the API client, the event stream client with reconnect and re-sync, the mapper, the optimistic update helper, the connection state, and the per-section switch that the register in 2.2 controls. Add the screens the mock never needed: loading, empty, error, offline, reconnecting, and sign-in or pairing, from existing components (N29). Done when: each part has tests, the app shows "Can't reach the daemon" and recovers on its own when the daemon returns, and no screen calls `fetch` directly. (Task 2.13, N29)
- [x] **B1.15 Card keys.** Each project numbers its own cards, so a card is known by its project and number (N26). The client's card key becomes the project plus the number, the visible label stays "#41", and every place that mixes projects (Home, notices, search, the palette, the Agents view, chats) shows the project name next to the number. Done when: the store, views, deep links, and tests use the new key, and a fixture with card 12 in two projects shows both correctly everywhere. (N26)
- [x] **B1.16 Prototype seed fixture, part 1.** Add `MARSHAL_FIXTURE=prototype`, which loads the three projects the prototype shows (api-gateway, web-dashboard, and mobile-app) on fixture repos. Part 2 adds their cards and chats in Phase 2. Done when: the project screens and the end-to-end specs for projects run against the dev daemon. (Task 0.10, `development.md` section 3.5)
- [x] **B1.17 Cut over S1, S3, S4.** Done when: the cutover gate in 2.4 passes for each, the projects list, badges, and create, rename, and remove dialogs work against the dev daemon, and the agent pickers list the real agents. (Tasks 1.4, 1.8, 2.14)

**Gate (milestone: first card):** a card runs a real agent in its own worktree and survives a restart, and the end-to-end tests for the cut-over sections pass against the dev daemon at three sizes. Budgets: daemon idle RAM under 50 MB and idle CPU about 0 percent on the fixture project.

### Phase 2: Core UI (build plan Phase 2)

**Goal:** daily use from the screens we already have.

**Sections cut over:** S2a, S5a, S6a, S7a, S7c, S8a, S9, S10, S11, S17, S18, S19a, S20, S24a, S31a, S32. **Inventory delivered:** N1, N2, N3, N4, N12 to N15, N17, N20, N23, N24, N27, N29.

#### Phase 2 scope and order

Scoped on 2026-09-26, after Phase 1 was closed. On the same day the owner moved the manual session controls (pause, sleep, wake, and pin, S7c) into this phase. The screens for every Phase 2 section already exist and run on the mock, so Phase 2 is mostly daemon work plus the wiring that lets each section switch. What is not in Phase 2 stays on the mock on purpose: S2b, S2c, S5b, S5c, S7b, S8b, S8c, S12 to S16, S19b, S21 to S23, S24b, S25, S26a, S26b, S27 to S30, and S31b.

**How it is built.** In vertical slices. A slice is finished from the daemon to the screen: migration, module, wire types and golden files, routes and events, the client mapper, syncer and actions, the screens' loading, empty, error, and offline states, end-to-end specs, the cutover of its sections with their mock code removed, and the docs. A slice never leaves a section half switched (rule 3 in 2.3). The slices go in this order because each one needs the one before it, and a stopped build still leaves a working, verified app.

| Slice | Items | Sections cut over | Needs | Done when |
|---|---|---|---|---|
| A. Cards, the trunk | B2.1 (cards), B2.4, the live parts of B2.3 | S5a, S7a, S18 | Phase 1 | Board, List, Timeline, the Agents view, the card header, and Home's needs-you and awake lists all show the fixture's 29 cards from the daemon. Create, quick add, rename, delete, drag, a refused drag, settings changes, start, stop, and fork work, and the card end-to-end specs pass against the daemon |
| B. Inside a card | B2.1 (history), B2.6, B2.7, B2.8, B2.9 | S8a, S10, S11, S9 | A | A real card opens with its chat history, activity, and diff from the daemon, a message streams back, and the terminal toggle resumes the same session |
| C. Project chats | B2.10 | S17 | A, B | Chats are created, renamed, archived, restored, deleted, and each keeps its own session and history |
| D. Home numbers | rest of B2.3 | S19a, S20 | A | Cards finished per day and recent activity come from stored numbers, update live, and page in the view-all list |
| E. You and your preferences | B2.2, B2.5 | S2a, S6a, S32 | Phase 1 | The profile saves, the avatar uploads, and theme, list columns, sort, last view, filters, swimlane, and saved views follow the user between two browsers |
| F. Finding and starting | B2.11, B2.12, B2.13 | S24a, S31a | A to C | The palette and top bar search projects, cards, and chats from the daemon, "Use a sample project" creates a real project, and onboarding and the tour save their progress |
| G. Session hold | B2.15 | S7c | A, C | Pause, Sleep, Wake, and Pin work on a real card under the rules of `architecture.md` 5.1, Start resumes a stopped, asleep, or paused card, and the Home awake list's Sleep buttons work |
| H. Prove it | B2.14 | none | A to G | Every cut-over view passes at three sizes and two themes against the dev daemon, the budgets hold, and the gate below is met |

Slices A to D and F touch the same card objects, so the order matters more than the count. Slice E has no dependency on cards and may be built at any point after A if that is easier, but it is listed late so the trunk lands first.

**Modules.** New daemon packages `internal/accounts` (profile, progress, avatar, users list, preferences), `internal/dashboard` (Home numbers and activity), `internal/chats`, and `internal/search`. The existing `internal/projects` grows the card fields, labels, moves, edit, delete, fork, and saved views. `internal/session` grows typed history, the activity index, and the terminal channel. `internal/gitx` grows the diff. Modules talk through small interfaces and the event bus, never through each other's tables (section 4).

**What Phase 1 left that Phase 2 must close first** (found by reading the daemon on 2026-09-26). Each is fixed inside the slice that needs it, with a test:

- **History is not stored.** A user's message goes to the agent and is never written down. The session log lines carry no message id or sequence, nothing reads the log segments back, and the adapters cut tool content at 16 KiB before it is logged. Slice B makes the session manager write each typed message and activity item, with an id and a sequence, into the `session_events` index as it is emitted, so history pages by cursor and survives a restart. Tool detail on demand returns what the adapter kept, with its truncated flag.
- **A session belongs to a card only.** `sessions.card_id` is `NOT NULL UNIQUE`, and the manager is keyed by card id. Slice C rebuilds the table so a session belongs to a card or a chat (SQLite needs a rebuild for this), and generalizes the manager. It is a change to a module with an 85 percent coverage floor.
- **The terminal has no path.** The client can send only `hello` on the stream, terminal output events are ringed but never published, and the PTY adapter's `WriteRaw`, `Resize`, and `Snapshot` are on the concrete type, not on `agents.Agent`, and the adapter is not registered in `marshald`. The replay ring is shared (2,000 events across every topic), so terminal bytes must not go through it as ordinary events, or card events fall out of it and clients resync. Slice B decides and documents how (for example, terminal output is not replayed, and a reconnecting client asks for the PTY snapshot).
- **There is no diff.** `gitx` has only the generic `Run`, which buffers all output. Slice B adds a bounded per-file diff to `gitx`, with a size cap.
- **Moves have no rules.** `SetState` allows any move, and the session manager uses it. The rules of `architecture.md` 6.1 belong in the manual-move entry point only, not in `SetState`.
- **The card type will stop being comparable.** Several Go tests compare `protocol.Card` with `==`, and a labels list breaks that. They move to `reflect.DeepEqual` in slice A.
- **The fixture cannot set a card's number.** `CardOption` has only `WithCreatedBy`, and numbers come from a counter, so "card 41" needs new options (number, state, dates, and the N1 fields).
- **Delete needs a per-card stop.** `SessionStopper` has only `StopProjectSessions`. A card delete also removes the worktree and the branch under the repository lock, and the session log folder, which nothing deletes today.
- **The router has no route without JSON rules.** An avatar upload needs its own registration that skips the JSON body rules, and `TestEveryRouteRequiresAToken` must cover it.

**What the client must extend first** (found by reading the web app on 2026-09-26). The S3 cutover (`apps/web/src/sync`) is the template, but it does not fit cards as it stands:

- **The syncer interface is too static.** `Syncer.topics` is a fixed list and `load(api)` gets no context, but card events go to `project:<id>` and session events to `card:<ULID>`, and there is no call that lists cards across projects (only `GET /v1/projects/{id}/board`). Slice A lets a syncer's topics depend on the projects that exist, load per project, and subscribe and unsubscribe as projects come and go.
- **The reservoir would bring the mock cards back.** `createContext` always seeds the 29 mock cards, and `reconcileMock` shows them for any project that exists on the daemon, whatever the section status. The fixture projects use the same ids as the mock seed, so flipping S5a alone changes nothing. Seeding and reconciling of cards are gated on the section status, while chats, notices, and the feed stay in the reservoir. The fixture reproduces the same 29 cards with the same keys, so the mock notices and feed items that name a card (such as `api#43`) still point at a real one.
- **The simulation would change real cards.** `mock/sim` scripts move `api#41` to review, merge `api#35`, and fail CI on `api#40`, and the tick adds cost to working cards. With real cards mirrored into the store they would mutate them locally. The card scripts are off for a section on the daemon. The tick stays, because it drives every "N min ago" through `ctx.clock.pulse()`.
- **The card needs a mapper both ways.** The mock card keeps everything under its key `"<project>#<n>"`, and the daemon uses a ULID for routes and topics and `Card.key` for the text. The mock says "Claude Code", "Auto-accept edits", and "Medium", and the wire says `claude`, `auto-edits`, and `medium`, with thinking possibly `null`. The mapper carries both ids and maps names in both directions. Mock-only fields (dependencies, members, checklists, comments, checks, cost, sleep and pin flags) are empty for a real card, and the sections that own them show their empty states.
- **Numbers that are computed on the client.** The sidebar badges, the Home tiles, and the needs-you list count `S.cards`. Once cards are real those counts are right, but the daemon's `badges.needs` and `badges.awake`, which `mirrored()` in `sync/projects.ts` drops today, must agree with them, and a test proves it. A real card's cost is 0, so the cost tile and chart (S19b) show only the mock's constants until Phase 4.
- **Tests.** The fake daemon (`apps/web/src/testing/fake-daemon.ts`) serves only health, whoami, projects, and agents, so it learns the board, card, and event routes of each slice, and the test store loads cards through the real mapper. About 52 unit test files use the seeded card keys, and three assert 29 cards (`sync/index.test.ts`, `sync/projects.test.ts`, `mock/seed/seed.test.ts`). The eight twin tests that compare the mock with the prototype for card actions are retired or migrated to fixture builders with the section. In the end-to-end suite, `daemon-agents.spec.ts` creates cards through the dialog and is the most affected, `responsive.spec.ts` opens `api#41`, so the fixture must create it, and `support/daemon-api.ts` gets card helpers.

**Data.** Forward-only migrations from 0004, one numbered file per module, all tables `STRICT` with UTC millisecond integers for times (`architecture.md` section 10). They add: the N1 columns on `cards` and the `labels` and `card_labels` tables (D3); `saved_views` (which the inventory says exists but no migration creates) and the user's preferences (D2); `chats` and the chat side of `sessions`; the `session_events` index, and the `activity` and `daily_stats` tables. Full output stays in the session log files on disk and the database holds an index of it.

**Wire.** Every type is defined once in Go, gets a golden file, and is generated into `packages/protocol`. These enums already exist: `FeedKind`, `NoticeKind`, `ActivityKind`, `CIState`, and `CardViewMode`, so N17's lists are mostly there and only need checking against the screens. New enums, each with a row in `allEnums()` in Go and a hand-maintained row in `packages/protocol/test/enums.test.ts`: the needs-you reason kinds (plan ready, approval needed, stuck, limit, CI failed, conflict, question), the label colors, the chat message kinds (user, agent, tool call, system note, diff summary, card reference), the activity states (ok, running, failed, waiting) if they are not already there, and the refusal reasons of `architecture.md` 6.1. The Card gains its role, labels, package, dates (planned start, planned end, due, actual start, actual end), pull request, CI state, context used, needs-you reason, and "doing now" (N1). New wire types: the profile and progress, the user, the preferences, a label, a saved view, a chat, a chat message, an activity item, the dashboard, a diff and a file's hunks, and the terminal messages.

**Routes** (all need the token; `architecture.md` 11.1 holds the table and is updated with them):

| Group | Routes |
|---|---|
| Cards | `PATCH` and `DELETE /v1/cards/{id}`; `POST /v1/cards/{id}/move` (rules in `architecture.md` 6.1); `POST /v1/cards/{id}/fork`; `POST /v1/projects/{id}/cards` gains an optional start state; the board and card answers carry the N1 fields |
| Session hold | `POST /v1/cards/{id}/pause`, `/sleep`, `/wake`, and `/pin` (and the calls that undo a pause and a pin), with the refusals of `architecture.md` 5.1; `POST /v1/cards/{id}/start` resumes a stopped, asleep, or paused card; the existing `/resume` stays the restart-recovery route |
| Labels | `GET` and `POST /v1/projects/{id}/labels`; `PATCH` and `DELETE /v1/labels/{id}` |
| History | `GET /v1/cards/{id}/messages` (paged, typed); `GET` of one tool call's detail; `GET /v1/cards/{id}/activity` (paged) |
| Diff | `GET /v1/cards/{id}/diff` (files with counts, no hunks); `GET` of one file's hunks |
| Terminal | `POST /v1/cards/{id}/view` (chat or terminal); on the event stream, the client messages `terminal.input` and `terminal.resize`, and the event `session.terminal_output` |
| Chats | `GET` and `POST /v1/projects/{id}/chats`; `PATCH` and `DELETE /v1/chats/{id}`; `POST /v1/chats/{id}/archive` and `/restore`; `POST /v1/chats/{id}/messages`; `GET /v1/chats/{id}/messages` (paged) |
| Home | `GET /v1/home/dashboard?range=` (7, 30, or 90 days); `GET /v1/home/activity` (paged, filterable by kind and project) |
| You | `GET` and `PATCH /v1/me`; `POST` and `DELETE /v1/me/avatar` and a `GET` for the image; `GET /v1/users`; `PATCH /v1/me/progress`; `GET` and `PATCH /v1/me/preferences`; `GET`, `POST /v1/projects/{id}/saved-views`, `PATCH` and `DELETE /v1/saved-views/{id}` |
| Search | `GET /v1/search?q=` over projects, cards, and chats, grouped by kind |
| Projects and dev | `POST /v1/projects` accepts the source `sample`; in dev mode only, a reset of first-launch progress |

**Events and topics.** Topics: `home`, `project:<id>`, `card:<id>`, and the new `chat:<id>` and `me` (so preferences and the profile follow the user between devices). New event types beyond the nine already published: `card.deleted`, `label.updated`, `chat.created`, `chat.updated`, `chat.archived`, `chat.deleted`, `activity.created`, `session.terminal_output`, `me.updated`, and `saved_view.updated`. Each is added to `daemon/internal/protocol/event_types.go`, documented in `architecture.md` 11.2, and covered by a golden file.

**Controls whose backend comes later.** These stay on the screen (the design wins) and never invent behavior. Approval and plan blocks (S8b, S8c) do not exist for real cards, because the chat's message kinds for them arrive in Phases 3 and 5. The Checks, Checklists, Comments, Preview, and Notes parts of a real card show their empty states (rule 2 in 2.3, tested in both directions). The cost figures stay on the mock until Phase 4. **Labels need design first.** The prototype shows a card's labels as plain names, and it has no label management in Project settings, no label picker on a card, and no label colors (task 2.18 asks for all three). Those are new screens and new tokens, so under the design rule they wait for design (inventory section 6). Phase 2 builds the daemon side (a project's labels, each with a name and a color from the fixed set, and the labels on a card) and the screens keep showing label names exactly as they do today. Nothing new is drawn, and no color token is added, until the owner provides the design (Q20). The same holds for the Developer options section of Settings (task 2.25): the first-launch reset is built as a route that exists in dev mode only, and the section that would hold its button waits for design (Q21). The onboarding sample option is different. It is in the prototype, the owner removed it only until this phase, so slice F puts it back and deletes its row from `design-port.md`.

**Decided by the owner on 2026-09-26** (logged in `progress-tracker.md`): (1) Start on a card whose session is stopped, asleep, or paused **resumes the same session** through its saved id, so nothing strands a card; if the agent cannot resume, the card moves to Needs you with a plain sentence. (2) A manual move to In review or Ready to merge is **refused with the prototype's own sentence and a stable reason** until a real pull request and a passing CI state exist (`architecture.md` 6.1, rules 4 to 6). This is a change from the prototype, where the mock invented a pull request, and it is on the list of approved differences in `design-port.md`. (3) **Pause, Sleep, Wake, and Pin ship in this phase** (slice G, S7c), instead of waiting for Phase 5. Only the controls a person presses move. The idle timer, the sleep warning and its notice, the awake limit, keep awake, and sleep all stay in Phase 5. (4) **The work is handed over uncommitted.** The docs edited on 2026-09-26 for this scope are in the working tree and are not committed, on the owner's instruction, so the builder's changes and the controller's doc edits show together in `git status`. The builder does not revert or reword them and lists every file it touches in its report, and the controller separates the two with the baseline recorded at hand-over.

**Open questions for the owner, with the proposed answer** (also in `progress-tracker.md`): Q17, do cards a chat creates have to work in Phase 2? Proposed: no. The chat, its session, and its history are built. Cards created by an agent need the internal MCP server (Phase 7), so Phase 2 checks the "cards it creates land on the board" clause with a stub-agent scenario that goes through the same card-creation service the MCP tool will use. Q19, how does a card in terminal view run? Proposed: the toggle stops the chat-mode process and starts the PTY adapter with the agent's own interactive command and its saved session id, and back again, within the one to three seconds the item asks for.

- [ ] **B2.1 Prototype seed fixture, part 2.** Extend the fixture from B1.16 with the prototype's 29 cards (11 in the api project, 9 in web, 9 in mobile), their chats, and scripted stub sessions, so the screens look the same as the prototype and the existing end-to-end specs, which use the api project and card 41, run against the daemon. Done when: the responsive and card end-to-end specs pass against the dev daemon with every card coming from the daemon, unchanged. (Task 0.10, `development.md` section 3.5)
- [ ] **B2.2 Accounts module.** `GET` and `PATCH /v1/me`, devices, onboarding and tutorial progress, avatar upload, and a users list for member pickers (N27). Done when: edits save and the profile screen reads the daemon. Cut over S2a (devices stay on the mock until Phase 9). (Tasks 2.1, 2.15) — **Built and switched (2026-09-26).** The daemon side was already done and tested. The client vertical slice: the 15 `api-client.ts` methods (the client's request helper gained a raw-body upload with the image's own `Content-Type` and a bytes answer for the avatar), `sync/profile.ts` (S2a: `GET /v1/me` and `GET /v1/users` on connect, `me.updated` on the new `me` topic; the profile fills `S.profile` with the daemon's initials and the users fill `S.people`, which the member pickers read; the tailnet, the node, and the paired devices stay the mock's until Phase 9), `sync/avatar.ts` (the avatar is fetched with the token and drawn from an object URL, which is revoked when the picture is replaced and when the store stops), `sync/profile-actions.ts` (`M.saveProfile` sends only the fields that changed and shows the daemon's own sentence for a refusal, keeping the form as typed; `M.chooseAvatar` opens the file chooser and uploads the image raw), the Profile screen and the avatar menu (`Avatar` in `@marshal/ui` gained an optional `src`; the time zone select lists a zone the person has that is not one of its six, and "Not set" when there is none), and the fake daemon (`testing/fake-me.ts`, with the daemon's refusal sentences). The prototype has an Upload image button and no Remove control, so no remove control was drawn; the client method and the fake route exist for the day design adds one. `sync/progress.ts` maps the onboarding and tour progress to `S.onboarding`, `S.obStep`, and `S.tour` and is tested, but it belongs to S31a and runs only when that section is switched, so the onboarding screens behave as they did. In the mock the person the comments and checklists name as the signed-in user is now `M.meId()` (the daemon's id on the daemon, `ada` on the mock). Nothing else changed on screen.
- [ ] **B2.3 Dashboard module.** Pre-computed `daily_stats` updated from events, the activity stream written from events and trimmed after 90 days, the fixed feed and notice kinds (N17), and the Home calls with ranges of 7, 30, and 90 days and a paged, filterable view-all list. Done when: Home loads in under one second from stats, never scans all cards, and updates live. Cut over S18, S19a, and S20. (Task 2.3)
- [ ] **B2.4 Cards.** The card fields the screens show that the model lacks (N1), the allowed-move rules and refusal messages (N2), rename and delete (N3), create and quick add with a start state, the board call, card settings changes (stored now, enforced by the harness in Phase 3), start and stop of a card's session (Task 1.9), a basic fork from the card's latest commit (checkpoints arrive in Phase 5), and the events. Done when: cards move live from daemon events, a refused drag snaps back with the daemon's message, each column's add action does what it says, and starting a card runs its session. Cut over S5a, including List, Timeline, and Agents view, and S7a. (Tasks 1.9, 2.4, 2.10)
- [ ] **B2.5 Filters and saved views.** Saved view calls (N20). Live filters and search stay on the screen and are remembered as preferences (N30). Screen preferences are saved as decided in D2. Done when: a saved view restores filters and swimlane, and the theme, list columns, sort, and last view follow the user. Cut over S6a and S32. (Task 2.5) — **Built and switched (2026-09-26).** The daemon side was already done and tested. The client: `sync/saved-views.ts` (S6a: each project's saved views on connect, `saved_view.updated` on the project's topic, `M.saveView` through `POST /v1/projects/{id}/saved-views`, so a used name replaces that view; the menu lists the client's own "All cards" first unless the daemon has a view of that name; a view deleted while in use ends its use, and a rename keeps it, both without a save) and `sync/preferences.ts` (S32). The screens write the theme, the List's columns, both sorts, and each project's last view, filters, search, swimlane, folded lanes, and view in use straight into `M.S`, so saving does not wait for an action: an effect compares them with what the daemon last agreed to (`sync/prefs-model.ts`) and, after 150 ms (700 ms for a search, so typing is never a request per key), sends only what differs to `PATCH /v1/me/preferences`, one request at a time. `me.updated` is applied key by key: a value the person changed and the daemon has not seen yet is kept, so the echo of an earlier save cannot undo newer typing, and two browsers converge (tested with two stores on one fake daemon). A refusal shows the daemon's sentence and puts the store back to what the daemon has; a daemon that cannot be reached keeps the change and sends it after the next load. `project.removed` drops the project's preferences and its saved views. The prototype's seeded saved views are not the store's on the daemon (the controller adds them to the daemon fixture). For a project the daemon has no row for, what the client compares against is the daemon's own defaults, whose swimlane is none, while the client starts a monorepo project on the package swimlane: so that swimlane is sent once, at load or with the first other change of the project, and the row the daemon then makes keeps it (a monorepo set back to none is then remembered too). "All cards" with no filters and no grouping, whether the client's own or a daemon view of that name, is told to the daemon as no view in use, so a seeded "All cards" does not make every project send a view at load.
- [ ] **B2.6 Card chat.** The typed message kinds, paged history for cards and chats, streaming text, and tool detail loaded on demand (N13). Plan and approval kinds are added in Phases 3 and 5. Done when: messages, tool call blocks, diff summaries, and system notes render from structured events. Cut over S8a. (Task 2.6)
- [ ] **B2.7 Terminal.** Input, resize, and the phone key bar keys on the event stream (N12), and the view switch that resumes the same session in the other mode. Done when: toggling resumes the same session in one to three seconds with no lost context. Cut over S9. (Task 2.7) — **Built and switched (2026-09-26).** The daemon side (`POST /v1/cards/{id}/view`, the three terminal client messages, the `terminal.screen`/`terminal.refused` frames, and the live-only `session.terminal_output` event) was already done and tested. The client vertical slice: `data/stream-frames.ts` (the two new frame readers and the three client builders), `data/event-stream.ts` and `data/index.ts` (`sendTerminal`, `onTerminalFrame`), `data/mappers/card.ts` (`Card.viewMode` mapped the same way `session` is), `sync/card-view.ts` (`switchView`, the daemon-backed `setMode`, keyed and toasted through `ctx.optimistic` exactly like `card-hold.ts`; `followOpenCardTerminal`, which asks for a fresh screen when the open card shows the terminal view and again after a resync or a reconnection; and the buffer that decodes and escape-strips a card's output), `TerminalView.tsx` (the daemon path for a real card, alongside the unchanged mock path), and the fake daemon (`testing/fake-terminal.ts`, plus the socket-level `onSend` hook `data/testing/fake-web-socket.ts` gained for it). **Decided here, not by design:** the terminal is rendered as decoded, escape-stripped plain text in the same monospace block the mock always used, not a real terminal emulator — no `@xterm/xterm`, no cursor positioning, no alternate screens. Adding a terminal-emulator dependency was a bundle-budget question that cannot be measured without a build, which this pass could not run, so the owner decided against it up front; a real emulator is a future enhancement, not a defect of this pass. The view sends a fixed 120x32 size (`daemon/testdata/golden/terminal-screen.json`'s own default) rather than measuring its own box. Not ticked here: that is the controller's, once verified.
- [ ] **B2.8 Card activity.** A paged activity list with the fixed kinds (N14). Done when: files changed, commands, tests, and "doing now" show live. Cut over S10. (Task 2.8)
- [ ] **B2.9 Card diff.** The file list with counts, hunks loaded when a file opens, and large files collapsed until asked (N15). Done when: large diffs stay smooth, and the list is virtualized. Cut over S11. (Task 2.9) — **Built and switched (2026-09-26).** The daemon side (`GET /v1/cards/{id}/diff`, `GET /v1/cards/{id}/diff/{path}`) was already done and tested. The client vertical slice is `apps/web/src/sync/diff.ts` (`loadCardDiff`/`loadFileHunks`, daemon for a real card, the mock's own `diffFor` for a mock-only one), the two `api-client.ts` methods, the fake-daemon route (`fake-cards.ts`'s `diffRoute`/`cardDiff`/`fileHunks`), and the card views: `views/card/diff-list.ts` (the card panel fetches the changed-file list once per open card, keyed by card key, so the Diff tab and the tab bar's count read the same answer and a card that changes does not refetch it; the tab asks again when it opens), `DiffTab.tsx` and `DiffSummary.tsx` (loading skeleton, error with Try again, empty state, and the summary line), and `diff-hunks.ts` (per-file hunks on demand, one request per file, none for a file that is not drawn, the daemon's own sentence as a toast when one fails). The file list is virtualized by the new `VirtualList` in `packages/ui/src/base` (with its windowing math in `virtual-window.ts`): only the files near the screen plus 400 px of overscan are in the DOM, rows are measured because an open file is taller than a closed one, the row at the top of the screen is kept still when rows above it change, and the row that has focus stays drawn. A test renders a 2,000-file diff and counts fewer than 60 rows. S11 is switched in `sections.ts` and the register in 2.2. Not in this pass: the daemon has no event for a card's diff changing, so the list is read when the card opens and again each time the Diff tab opens, not live.
- [ ] **B2.10 Project chats.** The chats module and its calls. A new chat's title comes from its first words until a model provider exists (Phase 4). Done when: each chat keeps its own session, and cards it creates land on the project's one board. Cut over S17. (Task 2.11)
- [ ] **B2.11 Search and palette.** Extend search to projects, cards, and chats (N23). Done when: actions and project switching work from the keyboard, and results come from the daemon. Cut over S24a (session and note search comes in Phase 7). (Task 2.12) — **Built and switched (2026-09-26).** The daemon side (`GET /v1/search?q=`, the `internal/search` package) was already done and tested. The client vertical slice is `apps/web/src/sync/search.ts` (`createPaletteSearch`, bound on the store as `M.startSearch`), the `search` method in `api-client.ts`, the fake daemon's `/v1/search` route (`testing/fake-search.ts`), and the palette (`app/palette`). The palette makes one search session each time it opens: typing is debounced (150 ms), an earlier request is cancelled when a newer query is typed, and an answer is used only when its `query` (the daemon's trimmed, space-collapsed form) is the newest typed. With a query, the rows are the actions and settings that match (still built and filtered on the client), then the daemon's projects, cards, and chats in place of the store's own project and card rows, in the palette's own row shape (a card row reads `api-gateway #41 Title` with the state icon, a chat row names its project in the hint). While the answer is on its way, when the daemon is away or the request fails, and for a query over 200 characters, the palette searches what the store holds, exactly as it did on the mock, and says nothing more (the shell's offline banner already does). Choosing a row opens what the mock's rows open; a card or chat the store does not hold yet is read first (`getCard`, the project's chat list), a project it does not hold reads everything again, and a result that has been removed since shows the daemon's own sentence. Chat rows are left out until the chats are the daemon's (S17), since a daemon chat id is not a mock chat's. The top bar's search box is the button that opens this palette, so it is covered by the same change. S24a is switched in `sections.ts` and the register in 2.2. Not in this pass: sessions and notes (Phase 7).
- [ ] **B2.12 Project dialogs and the sample project.** Ship a small sample repository, let project creation use it, and add the dev-only first-launch reset (N24). Done when: create, rename inline, settings, and remove with confirmation work at every size. (Task 2.14)
- [ ] **B2.13 Onboarding and tour.** Four screens with skip and resume, agent detection on the agents screen, the sample project on the project screen, and the tour with skip and replay, with progress saved per user. Done when: a new user reaches Home with an agent and a project, or skips safely. Cut over S31a (the connect-from-anywhere step comes in Phase 9). (Tasks 2.16, 2.17)
- [ ] **B2.14 Responsive and end-to-end.** Run every cut-over view at three sizes and two themes against the dev daemon, and add the main flows: add a project, create and start a card, chat, move, and switch views. Done when: all pass in CI. (Task 2.1a)
- [ ] **B2.15 Session hold: pause, sleep, wake, and pin.** Moved here from Phase 5 on 2026-09-26. The controls a person presses, under the rules of `architecture.md` 5.1: Sleep stops the agent process, keeps the session id, and writes Asleep; Wake resumes it and writes Waking, then Awake; Pause holds a Working card between turns; Pin keeps a card from sleeping; Start resumes a stopped, asleep, or paused card. Done when: a card can be paused, resumed, put to sleep, woken with its full context, and pinned from the card panel and from Home's awake list, each refusal returns the plain sentence of the rule, and the session states survive a daemon restart. Cut over S7c. The idle timer, the sleep warning and its notice, the awake limit, keep awake, and sleep all stay in B5.6. (Tasks 5.10 in part, 5.16, N4)

**Gate (milestone: usable board):** daily use is possible from the screens against the daemon. `pnpm check` and the end-to-end suite pass on all three platforms. Budgets: click to response under 100 ms, card start to agent ready under 3 seconds with the stub agent, UI memory under 150 MB with a 50-card board open, and the web size budgets.

### Phase 3: Control and safety (build plan Phase 3)

**Goal:** users can trust agents with their code.

**Sections cut over:** S7b, S8b. **Inventory delivered:** N7, N8.

- [ ] **B3.1 Permission modes.** Ask, auto-accept edits, plan only, and full auto, enforced in the harness. Done when: each mode is tested against file writes and commands. (Task 3.1)
- [ ] **B3.2 Bypass mode.** The confirmation with a typed acknowledgement, the banner, the worktree-only rule, and the project lock, through the bypass calls (N8). Done when: bypass cannot touch the main branch or run outside the worktree, and both turning it on and off are audited. Cut over S7b. (Task 3.2)
- [ ] **B3.3 Permission profiles and command blocklist.** Done when: denied actions are blocked in every mode except bypass, and blocked commands are stopped or sent for approval. (Tasks 3.3, 3.4)
- [ ] **B3.4 Approvals.** One approval row shared by the card and any project chat that asked, with events to every place that shows it (N7), and the approve and deny calls. Done when: approve and deny work from the card, the chat view, and the Home needs you list, and every view updates together. Cut over S8b. (Task 3.5)
- [ ] **B3.5 Secret scanner and audit log.** Scan every agent commit, block a commit with a test key and move the card to Needs you, and record every action in a log that the screens cannot edit, with search and export. Done when: a test run's actions are all recorded and findable. (Tasks 3.6, 3.7)
- [ ] **B3.6 Thinking modes and model switching.** Map thinking modes per provider and switch the model per card, per role, and mid-session, through card settings (N8). Done when: a setting change changes the provider request, writes the system message and the activity row, and applies on the next turn. (Tasks 3.8, 3.9)
- [ ] **B3.7 Deploy approval rule.** Done when: agents cannot run deploy workflows outside bypass. (Task 3.10)

**Gate (milestone: safe to trust):** permission, secret, and audit tests pass, and the harness, security, and session modules are at 85 percent coverage. Two reviews are required for changes to `security` and `harness`.

### Phase 4: Models (build plan Phase 4)

**Goal:** the built-in agent and full provider support.

**Sections cut over:** S19b, S26b, S28. **Inventory delivered:** N18 (providers, limits).

- [ ] **B4.1 Providers.** The provider interface, an OpenAI-compatible adapter (OpenRouter, DeepSeek), native Anthropic and Gemini adapters, and Ollama and LM Studio. Provider keys are saved to the keychain and the client only ever sees a masked value. Done when: each adapter passes its tests with thinking settings and tool use, and the built-in agent works offline with a local model. (Tasks 4.1 to 4.3)
- [ ] **B4.2 Built-in agent.** The read, edit, run, and tools loop under the harness. Done when: it completes a fixture task. (Task 4.4)
- [ ] **B4.3 Queue and fallback.** A per-provider queue with retry, and model fallback with a notice. Done when: ten parallel requests on one key do not fail on rate limits, and a simulated outage switches to the backup. (Tasks 4.5, 4.6)
- [ ] **B4.4 Usage and cost.** Track tokens and cost per card, role, and model, and feed `daily_stats`. Done when: the cost meter matches provider usage. Cut over S19b. (Task 4.7)
- [ ] **B4.5 Limits.** Cost and awake limits per project and globally through the settings calls. Done when: hitting a limit pauses cards and creates a cost notice, which the bell shows once S23 is cut over in Phase 5. Cut over S26b. (Task 4.8)
- [ ] **B4.6 Connection test framework and provider tests.** Done when: the Test button shows passed, partly working, or failed with fix hints, and the result is saved. Cut over S28. (Task 4.9)
- [ ] **B4.7 Chat titles.** New chats get a short title from their first message, written by a cheap model. Done when: a chat is titled after its first message and can still be renamed. (`architecture.md` section 16.2)

**Gate:** no real model API is called in CI, provider tests use recorded responses, and the cost numbers on Home match the usage table.

### Phase 5: Quality loop (build plan Phase 5)

**Goal:** idea to merged code, with approvals only.

**Sections cut over:** S8c, S23, S26a, S27 (S7c moved to Phase 2). **Inventory delivered:** N5, N6, N18 (roles).

- [ ] **B5.1 Roles.** Starter roles, edit, duplicate, delete, reset, per-project overrides, export and import, and the weak-model warning on Reviewer and Integrator (N18). Done when: roles are fully editable through the API and the role editor. Cut over S27. (Task 5.1)
- [ ] **B5.2 Plan first.** The plan calls: approve, reject, and edit (N6), with the plan's steps, files, risks, and checks. Done when: a card waits in Planning until the plan is approved. Cut over S8c. (Task 5.2)
- [ ] **B5.3 Limits, stuck detection, and checkpoints.** Time, cost, and round limits, the stuck detector, and checkpoints with restore. Done when: limits move the card to Needs you with a reason, repeated error and edit loops are caught, and restore returns the worktree and optionally the conversation. (Tasks 5.3 to 5.5)
- [ ] **B5.4 Pull requests and review.** Create a pull request from a card, and run the Reviewer role on every one. Done when: a card opens a pull request on GitHub and review comments go back to the worker. The GitHub adapter starts here and is completed in Phase 6. (Tasks 5.6, 5.7)
- [ ] **B5.5 Integrator.** The merge queue: dry run, backup branch, temporary worktree merge, tests, and abort, plus conflict resolution with the context of every card involved. Done when: clean and conflicting fixture merges behave as designed and the target branch only moves forward after tests pass. (Tasks 5.8, 5.9)
- [ ] **B5.6 Sleep and wake.** Grouped reminders, awake limits (the manual pause, sleep, wake, and pin calls moved to B2.15), and the notice actions for keep awake, sleep now, keep all, sleep all, and dismiss (N5), with the keep-awake time as a setting. Done when: idle cards sleep and wake with context, waking takes 1 to 3 seconds, working cards never sleep, a working card sleeps only after it is paused, and a pinned card never sleeps. Cut over S23, S7c, and S26a. (Task 5.10)
- [ ] **B5.7 Cleanup.** Remove worktrees and expire backup branches after merge. Done when: nothing is left behind by a fixture card. (Task 5.11)
- [ ] **B5.8 Quality module.** Project linters and built-in smell checks on card diffs, new-or-worse filtering, caching per commit, smell profiles, blocking findings sent back to the agent, warnings to the Reviewer, and findings with ask to fix and dismiss. Done when: fixture diffs produce the expected findings and old smells are not blamed on the card. The screens for findings need design first (`backend-inventory.md` section 6). (Tasks 5.12 to 5.15)

**Gate (milestone: idea to merge):** a fixture card goes from plan to merged code with approvals only, using the stub agent, and once with a real agent. Integration tests cover the merge safety rules.

### Phase 6: CI and preview (build plan Phase 6)

**Goal:** the screens show real CI, and cards can be previewed.

**Sections cut over:** S13, S21, S29a. **Inventory delivered:** N10, N28.

- [ ] **B6.1 GitHub App.** Complete the connection: install, receive events, and verify every webhook signature before reading the body. Done when: the app installs and receives events. The owner creates the App (section 3). Cut over S29a. (Task 6.1)
- [ ] **B6.2 CI monitor.** Webhooks with a conditional polling backup. Done when: card CI badges and the Home CI health update live. Cut over S21. (Tasks 6.2, 6.4)
- [ ] **B6.3 CI fix loop.** Rerun failed jobs once, then send the trimmed log of the failed step to the card's session, counted against the loop limits. Done when: a failing fixture test is fixed by the stub agent and the limits stop the loop. (Task 6.3)
- [ ] **B6.4 Simulate CI failure.** Build the action as a real feature with two modes (N28). The synthetic mode makes the daemon inject a failed run for a card and lets the CI monitor handle it through the normal path: the rerun, the trimmed log, the loop limits, and the notice. It never touches GitHub. The real mode asks for confirmation and then pushes a deliberately failing change to the card's own branch, so GitHub Actions really fails and the webhook comes back. It needs the GitHub App connected, uses Actions minutes, and is clearly labeled. Both modes are audited, and both are shown only in dev mode or when a Developer options setting is on (off by default in a normal install). They replace the mock's hidden menu item, and the synthetic mode shares its recorded webhook with `pnpm hooks:replay`. Done when: each mode produces the same events as a real failure, the fix loop is testable from the screens, and the real mode leaves only the marked commit on the branch. (Task 6.3, N28)
- [ ] **B6.5 Local CI.** Run test and lint steps from workflow files locally, and mark unsupported steps. Done when: a fixture workflow's supported steps run and the rest are marked. (Task 6.5)
- [ ] **B6.6 Preview and screenshots.** Preview per card on its own port with an isolated browser profile, the project's dev command, before and after screenshots, and state events for stopped, starting, and running (N10). Done when: two cards preview at once without shared state, and screenshots attach to the card and pull request. Cut over S13. (Tasks 6.6, 6.7)
- [ ] **B6.7 GitHub connection test.** Check the app, repos, permissions, and a ping webhook. Done when: the test catches a missing permission and a blocked webhook, and a test runs automatically after the connection is added. (Task 6.8)

**Gate:** CI behavior is covered by recorded webhooks, and no test needs a live GitHub account.

### Phase 7: Orchestration and memory (build plan Phase 7)

**Goal:** agents see the board and each other, and remember.

**Sections cut over:** S14, S24b, S29b. **Inventory delivered:** N11, and the context-used field of N1.

- [ ] **B7.1 Internal MCP server.** All tools listed in `architecture.md` section 11.4, with permission checks. Done when: agents can call every tool and a refused call is explained. (Task 7.1)
- [ ] **B7.2 Awareness and claims.** A board awareness summary per turn within its token budget, file claims, and early conflict warnings. Done when: overlapping claims warn both cards. (Tasks 7.2, 7.3)
- [ ] **B7.3 Orchestrator and handoff.** Plan a goal into cards with dependencies, and continue a card on another agent from a clean summary. Done when: the Orchestrator creates approved cards from a goal. (Tasks 7.4, 7.5)
- [ ] **B7.4 Memory and vault.** The knowledge base, card notes, lessons, an Obsidian-friendly vault with file watching, and the card Notes tab calls (N11). Done when: the vault opens in Obsidian with working links, edits made in Obsidian are picked up, and the Notes tab reads and writes the card's note. Cut over S14 and S29b. (Tasks 7.6 to 7.8)
- [ ] **B7.5 Codebase map, search, and context.** A light incremental codebase map, session search, the context meter, and pinned files. Done when: agents answer "where is X" without reading many files, search finds past work across cards, and the meter warns before compaction. Cut over S24b. (Tasks 7.9 to 7.11)

**Gate:** memory and index updates are incremental, and idle RAM and CPU budgets still hold with the vault watcher running.

### Phase 8: Automation (build plan Phase 8)

**Goal:** scheduled work, and Trello, Calendar, and Gmail connected for real.

**Sections cut over:** S22, S25, S29c, S29d, S29e, S30. **Inventory delivered:** N18 (schedules, integrations), N21.

- [ ] **B8.1 Scheduler.** Cron, interval, one-time jobs, and the missed-run policy, with event triggers and loops with hard limits, through the schedule calls (N18). Done when: jobs run on time and after wake as configured, and loops stop on every limit type. Cut over S30. (Tasks 8.1 to 8.3)
- [ ] **B8.2 Trello.** Two-way sync with one board per project, including checklists, comments, attachments, and member mapping, and the agent shown as a label. Done when: moves, checklists, comments, and attachments sync both ways, conflicts follow the latest change, and the connection test passes. Cut over S29c. The owner creates the key and token (section 3). (Task 8.4)
- [ ] **B8.3 Google Calendar and Gmail.** Events feed the briefs and the calendar, and labeled emails become cards. Done when: events appear, labeled email creates a card, and each connection test passes. Cut over S29d and S29e. (Tasks 8.5, 8.6)
- [ ] **B8.4 Calendar data.** One call for a date range returning events, jobs, briefs, and due cards (N21). Done when: the calendar and the Home coming up list read it. Cut over S22 and S25. (Tasks 12.3, N21)
- [ ] **B8.5 Briefs.** Morning and evening briefs across all projects with only new changes, and brief times taken from Trello or Calendar. Done when: briefs deliver on time and changing the event changes the time. (Tasks 8.7, 8.8)

**Gate:** every integration has a working connection test that passes without real user data in CI, using recorded responses.

### Phase 9: Remote (build plan Phase 9)

**Goal:** everything works from a phone.

**Sections cut over:** S2b, S29f, S29g, S31b.

- [ ] **B9.1 Tailscale node and pairing.** The tsnet node inside the daemon, device pairing by scanning a code, and revoking a device. Done when: the daemon is reachable on the tailnet with no separate install, and a phone pairs. The owner signs in (section 3). Cut over S2b and S31b. (Tasks 9.1, 9.2)
- [ ] **B9.2 Funnel and serving.** Expose only `/hooks/*` publicly with verified signatures, and serve the responsive UI over the tailnet. Done when: only the hook routes are public, and every view and action works on a real phone and tablet through Tailscale. (Tasks 9.3, 9.4)
- [ ] **B9.3 Telegram and Discord.** Notices, approvals, actions, and voice notes, with their connection tests. Done when: approving and creating a card from each works. The owner creates the bot tokens (section 3). Cut over S29f and S29g. (Tasks 9.5, 9.6)
- [ ] **B9.4 Notification routing.** Channels per event type and grouped notices. Done when: an event reaches a phone notice in under 5 seconds. (Task 9.7)
- [ ] **B9.5 Remote machines.** A second daemon on the tailnet. Done when: a project runs on a remote machine from the laptop UI. (Task 9.8)

**Gate (milestone: anywhere):** end-to-end tests run against a daemon reached over the tailnet, and the security rules in `architecture.md` section 13 are tested.

### Phase 10: Advanced cards (build plan Phase 10)

**Goal:** the card features the prototype already shows are real.

**Sections cut over:** S5b, S12, S15, S16. **Inventory delivered:** N9, N22.

- [ ] **B10.1 Templates, dependencies, and sub-cards.** Done when: new cards start from templates, dependent cards start after merges, and a parent shows combined progress. (Tasks 10.1 to 10.3)
- [ ] **B10.2 Acceptance checks.** The run and list calls (N9). Done when: a card cannot finish until its checks pass, and check results are usable as tick evidence. Cut over S12. (Task 10.4)
- [ ] **B10.3 Card from anywhere and duplicates.** Cards from code comments and other sources, and duplicate detection while writing a card (N22). Done when: each source creates a card and similar cards are flagged before creation. Cut over S5b together with B10.1. (Tasks 10.5, 10.6)
- [ ] **B10.4 Fork and race mode.** Fork from a checkpoint, and race mode with a side-by-side comparison. Done when: a fork runs on its own, and the winner is kept. (Tasks 10.7, 10.8)
- [ ] **B10.5 Checklists.** Named checklists, items, hide checked, required to finish, and people only, and agent ticks with evidence that auto-untick when the evidence stops being true. Done when: required checklists block the merge queue and a failing test unticks the item it proved. Cut over S15. (Tasks 10.10, 10.11)
- [ ] **B10.6 Comments and members.** Comments with files, images, links, and mentions, the "Agent read this" marker, immediate replies to @agent and questions, attachments stored on disk under the size limit, and member notices. Done when: @agent and questions get a reply and attachments reach the agent as data. Cut over S16. (Tasks 10.12, 10.13)
- [ ] **B10.7 Scorecard.** Stats per agent, model, and role, including smells introduced. Done when: the numbers match the usage and findings tables. (Task 10.9)

**Gate:** attachments are never run, and untrusted-content tests cover comments and attachments.

### Phase 11: Ecosystem (build plan Phase 11)

**Goal:** skills, MCP servers, and plugins.

**Sections touched:** S27 (the skills and MCP fields of the role editor become real).

- [ ] **B11.1 Skills.** The skills folder and attaching skills to roles, templates, and cards, and import with a content preview. Done when: skills load for the right agents and an import from a GitHub link shows what it will add. (Tasks 11.1, 11.2)
- [ ] **B11.2 MCP manager.** Per-card MCP servers, a health check, and the connection test. Done when: each card gets only its listed servers and broken servers show as broken. (Tasks 11.3, 11.4)
- [ ] **B11.3 Plugin API.** Done when: a sample plugin adds a new integration. (Task 11.5)

**Gate:** imported skills and plugins are treated as untrusted input, and each new library passed the checks in `library-docs.md`.

### Phase 12: Scale (build plan Phase 12)

**Goal:** the remaining views and modes are backed by the daemon.

**Sections cut over:** S2c, S5c.

- [ ] **B12.1 Monorepo mode.** Tool detection, the package graph, sparse worktrees, affected tests, and the package field on cards. Done when: the fixture monorepo works end to end and the board's package swimlane and filter use real packages. Cut over S5c. (Task 12.5)
- [ ] **B12.2 Split view, focus mode, and resources.** Done when: up to four panes work with real data, focus shows Needs you only, and the resource panel shows RAM, CPU, and disk per card. (Tasks 12.4, 12.6)
- [ ] **B12.3 Export, import, and backup.** Done when: a project moves to another machine and settings and memory sync between two devices with encryption. (Tasks 12.7, 12.8)
- [ ] **B12.4 Team.** Shared boards, human handoff, user roles, and comments, with the people list and member pickers reading real users. Done when: two users work on one board. Cut over S2c. (Task 12.9)

**Gate (milestone: v1 complete):** all in-scope features are shipped and tested on three platforms.

### Phase 13: Retire the mock and accept

**Goal:** the app runs only on the daemon.

- [ ] **B13.1 Remove the mock.** Every section in 2.2 says Daemon. Delete the mock as described in 2.5, and update `project-structure.md`, `design-port.md`, and `ui-registry.md`. Done when: `knip` reports nothing left over, all tests use fixtures or the dev daemon, and the design files remain.
- [ ] **B13.2 Full test pass.** Unit, integration, contract, end-to-end at three sizes and two themes, nightly real-agent smoke tests, budgets, and the three platforms, all green.
- [ ] **B13.3 Real-device pass.** Every view and action on a real phone and tablet over Tailscale.
- [ ] **B13.4 Docs and tracker.** Every doc matches what was built, every open decision below is closed or logged, and the tracker's task statuses, file changes log, and doc changes log are complete.
- [ ] **B13.5 Final review.** A whole-project review against `code-standards.md` section 12.5, done by a model or reviewer that did not write the code, with two reviews for `security`, `harness`, and `integrator`.

---

## 6. Where each screen ends up

When the last item of a phase is ticked, each section in 2.2 is switched in the same commit, so the register always shows the truth. The order in which the screens lose their mock data is the order of the phases:

1. Phase 1: connection, projects, and the agent pickers (S1, S3, S4).
2. Phase 2: profile, cards and every card-based view, card control (including pause, sleep, wake, and pin), chat, terminal, activity, diff, project chats, filters and preferences, Home (needs you, cards finished, activity), search, and onboarding (S2a, S5a, S6a, S7a, S7c, S8a, S9, S10, S11, S17, S18, S19a, S20, S24a, S31a, S32).
3. Phase 3: bypass and approvals (S7b, S8b).
4. Phase 4: providers, cost, and limits (S19b, S26b, S28).
5. Phase 5: roles, plans, notices, and sleep settings (S8c, S23, S26a, S27). Pause, sleep, wake, and pin (S7c) moved to Phase 2.
6. Phase 6: CI health, preview, and GitHub (S13, S21, S29a).
7. Phase 7: notes, session search, and Obsidian (S14, S24b, S29b).
8. Phase 8: coming up, calendar, schedules, Trello, Google Calendar, and Gmail (S22, S25, S29c, S29d, S29e, S30).
9. Phase 9: devices, Telegram, Discord, and the connect-from-anywhere step (S2b, S29f, S29g, S31b).
10. Phase 10: templates, dependencies, duplicates, checks, checklists, comments, and members (S5b, S12, S15, S16).
11. Phase 12: monorepo packages and team (S5c, S2c).

---

## 7. Checked against our rules and guides

Every rule document maps to at least one item or gate here. If a rule has no row, the checklist is missing something.

| Document and section | Where it is checked |
|---|---|
| `code-standards.md` 1 Repository layout | B0.2, B0.5, and the module rules in section 4 |
| `code-standards.md` 2 General rules | Section 4 |
| `code-standards.md` 3 Go standards (tooling, style, errors, concurrency, logging, database, processes and Git) | B0.1, B0.4, B0.6, B1.1, B1.5, and section 4 |
| `code-standards.md` 4 TypeScript and SolidJS (single API client, tokens, responsive, performance) | B1.14, section 2.4, section 4, and the Phase 2 gate |
| `code-standards.md` 5 Rust | B0.11 (deferred until the shell is scheduled) |
| `code-standards.md` 6 Testing | Section 2.6 and every phase gate |
| `code-standards.md` 7 Security | Phase 3, B1.13, and section 4 |
| `code-standards.md` 8 Performance | B0.7 and section 4 |
| `code-standards.md` 9 Git workflow | Section 4 (Conventional Commits, task IDs, small commits) and section 1 |
| `code-standards.md` 10 Writing and copy | Section 4 (plain errors, sentence case) |
| `code-standards.md` 11 Rules for AI agents | Section 4 (tracker, structure, `pnpm check`) |
| `code-standards.md` 12 Code smells | B0.4 and section 4 |
| `architecture.md` 1 to 3 (layers, processes, modules) | B0.2, B1.3, and the module rule in section 4 |
| `architecture.md` 4 Agents | B1.6 to B1.8, B4.2 |
| `architecture.md` 5 Sessions | B1.9, B1.10, B5.6 |
| `architecture.md` 6 to 9 (card life cycle, starting, merging, CI loop) | B2.4 (move rules), B5.5, B6.3 |
| `architecture.md` 10 Data model | Migrations in every phase, inventory sections 3 to 5 |
| `architecture.md` 11 API and events | B1.0, B1.12, the contract tests, and N1 to N30 |
| `architecture.md` 12 Files on disk | B1.1, B1.11, B7.4 |
| `architecture.md` 13 Security | B1.13, Phase 3, Phase 9 gate |
| `architecture.md` 14 Budgets | B0.7 and every phase gate |
| `architecture.md` 16 to 19 (projects, chats, quality, connection tests, checklists) | B1.4, B2.10, B5.8, each integration item, B10.5, B10.6 |
| `development.md` (setup, dev mode, environment, real agents, webhooks, phones, reset, commands, builds) | B0.1, B0.3, B0.10, B6.4, B9.2, B0.11 |
| `library-docs.md` 1 Adding a library | Section 4 and B0.1 |
| `ui-rules.md` (states, copy, responsive, empty and error states) | B1.14, section 2.4, section 4 |
| `ui-tokens.md` and `ui-registry.md` | Section 4 (tokens and components only) and B13.1 |
| `design-port.md` (approved deviations, checking a view) | Section 1 and section 2.4 |
| `project-structure.md` and `progress-tracker.md` | Section 4 and section 8 |
| `build-plan.md` (phase order, done criteria, integration tests) | Section 5 follows it, each item cites its task |
| `marshal-product-scope.md` | The item text, and `backend-inventory.md` for what the prototype adds |

---

## 8. Decisions

All ten are decided. If one changes, update this table, `architecture.md`, and the decisions log in `progress-tracker.md`.

| ID | Decision | Answer or recommendation | Status | Needed by |
|---|---|---|---|---|
| D1 | What the Timeline draws with the start, end, and due dates | Bars show the actual working span for started cards, and the planned span (start and due) for cards not started. | Decided 2026-09-25 | B2.4 |
| D2 | Where screen preferences are saved | Theme, list columns, sort, per-project last view, filters, and swimlane are saved with the user so they follow them between devices. Layout (side panel width, collapsed sidebar, split panes, calendar mode, dashboard range) stays on the device. | Decided 2026-09-25 | B2.5 |
| D3 | The label model | A managed list of labels per project, each with a name and a color from a fixed set of color tokens. Cards pick from the list, and labels map to Trello labels later. | Decided 2026-09-25 | B2.4 |
| D4 | Keep awake time | 15 minutes, as a setting under General. | Decided 2026-09-25 | B5.6 |
| D5 | What "Simulate CI failure" does | Two modes: a synthetic run through the CI monitor, and a real failing run on GitHub. Shown in dev mode or with Developer options on. | Decided 2026-09-25 | B6.4 |
| D6 | Card numbers | Each project has its own counter. A card is known by project and number, and mixed lists show the project name with the number. | Decided 2026-09-25 | B1.15 |
| D7 | When to build the Tauri shell and the signed release | After the Phase 2 milestone. | Decided 2026-09-25 | B0.11 |
| D8 | Windows and Linux testing | CI runs the full checks on both for every pull request, and a manual pass on real machines happens before the first release. | Decided 2026-09-25 | B13.2 |
| D9 | An integration whose owner account step is missing | It shows a "Not connected" state with a Connect button and no sample data, until its connection test passes (rule 4 in 2.3). | Decided 2026-09-25 | Phases 6 to 9 |
| D10 | People in the prototype seed (Ada and Blair) | They exist only in the sample fixture. Solo use shows only the owner until team mode in Phase 12. | Decided 2026-09-25 | B2.2 |

---

## 9. Tracking

- Tick the item here and update its row in `progress-tracker.md` in the same change. Use the item ID (B2.4) and the build plan task IDs together.
- Switch the section's status in 2.2 in the same commit that cuts it over.
- Every doc this checklist touches gets a row in the doc changes log, and every file that is added, moved, or deleted gets a row in the file changes log.
- At the end of each phase, record the gate results (test counts, coverage, budget numbers) in the session log.
