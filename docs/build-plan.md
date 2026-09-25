# Build plan

This document is the order we build Marshal in. Each phase has a goal, a list of tasks, and clear done criteria. Tasks are sized so one person or one agent card can finish one task in a focused session.

Progress on every task is tracked in `progress-tracker.md`. Task IDs here (for example `1.3`) are the same IDs used there.

---

## How to use this plan

- **Work top to bottom, as a guide.** Phases 0 to 5 are the core. Later phases build on top of them and should not need the core to be rebuilt.
- **The order is not strict.** When a task needs something a later phase provides, build that part early and skip it when its phase comes. Tick it in its own phase and note which task needed it. `backend-checklist.md` section 2.7 has the rules.
- **Finish a phase before starting the next**, unless a task says it can run in parallel or it is built early for another task.
- **Tasks marked (prototype) were added on 2026-09-25** from `backend-inventory.md`. They cover what the prototype does or needs that this plan did not list. Each one includes its screen work, so the backend and its screens are built together.
- **A task is done only when its done criteria pass**, including tests and budget checks.
- **If a task turns out bigger than expected**, split it into smaller tasks here and in the tracker before continuing.
- **Every integration task includes its connection test**, as described in `architecture.md` section 18. An integration is not done until its test works.
- **If a decision changes**, update `architecture.md` and add an entry to the decisions log in `progress-tracker.md`.

### Task format

Each task lists:

- **What:** the work to do.
- **Done when:** the checks that prove it is finished.

### Milestones

| Milestone | Reached at the end of | Meaning |
|---|---|---|
| First card | Phase 1 | A card runs a real agent in its own worktree, and survives a restart |
| Usable board | Phase 2 | Daily use is possible from the UI |
| Safe to trust | Phase 3 | Permissions, secrets, and audit are in place |
| Idea to merge | Phase 5 | A card goes from plan to merged code with approvals only |
| Anywhere | Phase 9 | Everything works from a phone |
| v1 complete | Phase 12 | All in-scope features shipped |

---

## Phase 0: Foundation

**Goal:** a clean repo, working builds, CI, and budget checks before any feature code.

| ID | What | Done when |
|---|---|---|
| 0.1 | Write the project docs | All twelve docs and the docs index exist and are agreed |
| 0.2 | Set up the monorepo layout from `project-structure.md` | `daemon/`, `apps/desktop/`, `apps/web/`, `packages/ui/`, `packages/tokens/`, `packages/protocol/` exist with placeholder builds |
| 0.3 | Go module with `marshald` and `marshal` binaries | Both build on macOS, Linux, and Windows in CI |
| 0.4 | SolidJS app with Vite and Tailwind | App builds and shows an empty shell |
| 0.5 | Tauri v2 shell loading the SolidJS app | Desktop app opens on all three platforms |
| 0.6 | Tokens package generating CSS variables from `ui-tokens.md` values | Light and dark themes switch in the shell, and `check-contrast.ts` passes every pair listed in `ui-tokens.md` section 2.9 |
| 0.7 | Go to TypeScript type generation for `packages/protocol` | One sample type round-trips in a test |
| 0.8 | CI pipeline: lint, format, test, build for Go and TypeScript | Pipeline runs on every pull request |
| 0.9 | Budget harness: measure daemon RAM and CPU idle, UI RAM, download size | Budget report runs in CI and fails when over budget |
| 0.10 | Fixture repos for tests (small repo, monorepo) | Fixtures load in integration tests |
| 0.11 | Stub agent for tests: a fake CLI that speaks ACP and supports resume | Harness tests can run without real models |
| 0.12 | pnpm workspace and root scripts from `development.md` | `pnpm dev`, `pnpm dev:desktop`, `pnpm check`, and `pnpm build` work on all three platforms |
| 0.13 | Dev mode in the daemon: separate data folder, port, keychain entries, and dev token | Dev and a normal install run side by side without touching each other |
| 0.14 | Release pipeline with signing and the updater | A test release produces signed installers for all three platforms |
| 0.15 | Code smell tooling for our own repo: linter configs, `pnpm smells`, new-smells-only check in CI, empty baseline | CI fails on a test pull request that adds a blocking smell, and passes when it is fixed |

---

## Phase 1: First card

**Goal:** one project, one card, one real CLI agent, a lasting session, and a worktree.

| ID | What | Done when |
|---|---|---|
| 1.1 | Store module: SQLite in WAL mode, migrations, generated queries | Migrations run on start, tests pass with a temp database |
| 1.2 | Event bus | Publish and subscribe tests pass, including slow subscribers |
| 1.3 | Daemon as a user service on all three platforms, single instance lock | Daemon starts at login and survives closing the desktop app |
| 1.4 | Projects module: create, rename, edit, and remove projects, one board per project, cards, columns | All project actions work through the API, and removing a project never touches the repo folder |
| 1.5 | Git module: create and remove worktrees and branches | Worktree created on card start, removed on close |
| 1.6 | Agent interface and ACP adapter | Stub agent starts, sends events, and resumes |
| 1.7 | PTY adapter | A real CLI runs in a PTY and its output is captured |
| 1.8 | Claude Code support through the best available mode | A real Claude Code session runs on a card |
| 1.9 | Session manager: start, send, stop, persist session ID | Messages go into the same process, no new process per message |
| 1.10 | Resume after daemon restart and PC reboot, with auto and manual modes | A card continues with full context after a reboot |
| 1.11 | Session logs to disk with a small memory buffer | Logs rotate, memory stays flat under heavy output |
| 1.12 | HTTP API and WebSocket event stream with batching | Card events reach a test client in batches |
| 1.13 | Client auth tokens | Requests without a valid token are rejected |
| 1.14 | (prototype, N19) Agent catalog: `GET /v1/agents` with kind, version, status (supported, untested, missing), models, and capabilities including thinking support. Screen work: the agent, model, and thinking pickers read it, and a missing agent shows disabled with an install hint. | Pickers list the real agents, a missing agent such as Codex shows as missing, and thinking options follow the model's capability |
| 1.15 | (prototype, N25, N26) Protocol conventions: absolute UTC timestamps, the daemon's time in every snapshot, ID rules, one error shape, cursor paging, fixed lists of enumerations, and event sequence numbers for re-sync. Each project numbers its own cards, so a card's client key is its project plus its number. Screen work: every list that mixes projects (Home, notices, search, the palette, the Agents view, chats) shows the project name next to the number. | Contract tests cover each rule, and a fixture with card 12 in two projects shows both correctly everywhere |
| 1.16 | (prototype, N29) Client data layer and the states the mock never needed. Screen work: a full-screen "Can't reach the daemon" that recovers by itself, an offline banner, loading skeletons, a sign-in or pairing screen, and a "Not connected" empty state with a Connect button for services that are not set up, all built from existing components and tokens | Each state renders at three sizes and the app recovers when the daemon comes back |
| 1.17 | (prototype, N16) Project fields: language, default branch, dev command, bypass lock, and the remove options keep branches and keep memory. Screen work: wire the Project settings fields and the remove dialog | Values save and read back through the API, and removing a project never touches the repo folder |

**Milestone reached:** first card.

---

## Phase 2: Core UI

**Goal:** daily use from the UI.

| ID | What | Done when |
|---|---|---|
| 2.1 | App shell: sidebar, top bar with profile avatar, project switcher with badges, view switcher | Switch projects and views, last view remembered per project |
| 2.1a | Responsive layouts for phone, tablet, and desktop | The shell and every view built in this phase pass end-to-end checks at all three sizes |
| 2.2 | Base components from `ui-registry.md` (button, input, badge, menu, dialog, tooltip) | Components built with tokens, listed as built in the registry |
| 2.3 | Home dashboard: needs you, summary tiles, charts, recent activity, awake agents, CI health | Dashboard loads under one second from pre-computed stats and updates live |
| 2.4 | Kanban view with drag and drop, and "Add a card" in Backlog, Planning, and Working | Cards move live from daemon events, manual drags are checked, and each column's add action does what it says |
| 2.5 | Filters, swimlanes, and saved views | A saved view restores filters and swimlanes |
| 2.6 | Card detail view with chat UI | Messages, tool call blocks, and approvals render from structured events |
| 2.7 | Terminal view and view toggle | Toggle resumes the same session in the other mode |
| 2.8 | Activity feed per card | Files changed, commands, tests, and "doing now" show live |
| 2.9 | Diff view | Live diff per file, virtualized for large diffs |
| 2.10 | Agents view | Every session with role, model, modes, state, and cost |
| 2.11 | Chats view: many chats per project with create, rename, archive, restore, and delete | Each chat keeps its own session, and cards it creates land on the project's one board |
| 2.12 | Command palette | Actions and project switching from the keyboard |
| 2.13 | Empty, loading, and error states for every view | Each follows `ui-rules.md` |
| 2.14 | Project dialogs: create, rename inline, project settings, remove with confirm | All flows work at every size |
| 2.15 | Profile page and accounts module | Avatar opens the profile, edits save, devices listed |
| 2.16 | Onboarding: four screens with skip and resume | A new user reaches Home with an agent and a project, or skips safely |
| 2.17 | Tutorial tour on the Home dashboard with skip and replay | Tour runs on desktop and phone, and can be replayed from the profile menu |
| 2.18 | (prototype, N1) Card fields the model lacks: managed labels (a per-project list where each label has a name and a color chosen from a fixed set of label color tokens), package, due date, planned start and end, pull request link, context used, needs-you reason, and "doing now". Screen work: label management in Project settings, a label picker on the card, date fields, the reason on the card and on Home, and the pull request chip. Add the label colors to `ui-tokens.md` and the tokens package | Fields round-trip through the API, labels filter and group the board, and the label colors pass the contrast check |
| 2.19 | (prototype, N2, N3) Manual move rules and card edit: write the allowed-move matrix and the refusal messages from the prototype into `architecture.md`, and add `PATCH` and `DELETE /v1/cards/{id}`. Screen work: the drag snap-back and toast show the daemon's message, and the delete dialog warns about unmerged work | Every refused move returns a stable code and a plain message, and delete removes the session, worktree, comments, and attachments |
| 2.20 | (prototype, N12) Terminal channel: input, resize, and key bar keys on the event stream. Screen work: the phone key bar sends real keys | Typing and resizing reach the process, and the key bar works on a phone |
| 2.21 | (prototype, N13, N14, N15) Chat history, activity, and diff loading: typed message kinds in the protocol, paged history for cards and chats, a paged activity list with fixed kinds, and a diff file list with hunks loaded per file and large files collapsed. Screen work: the same screens load pages as the user scrolls, and "Load diff" fetches a large file's hunks | Long histories, long feeds, and a 1,240-line diff stay smooth |
| 2.22 | (prototype, N17) Fixed vocabularies for Home feed kinds, notice kinds, and activity kinds in the protocol, and paged, filterable view-all lists. Screen work: the view-all pages page and filter | Each kind appears with its icon and text, and the lists page without reloading |
| 2.23 | (prototype, N20, N30) Saved views API, and screen preferences saved with the user: theme, list columns, sort, last view per project, filters, and swimlane. Layout (side panel width, collapsed sidebar, split panes, calendar mode, dashboard range) stays on the device. Screen work: none new, the settings follow the user | Changing the theme or a saved view on one device shows on another |
| 2.24 | (prototype, N23) Search extended to projects, cards, and chats for the command palette and the top bar. Screen work: results grouped by kind, with the project name next to a card number | Typing a card title, a project name, or a chat title finds it from the keyboard |
| 2.25 | (prototype, N24) Sample project and Developer options: ship a small sample repository and let project creation use it, and add a Developer options section in Settings that appears only in dev mode or when a setting is on, starting with a first-launch reset. Screen work: "Use a sample project" creates a real project, and the section holds the reset button | A new user gets a working sample project, and the reset shows only in dev mode or with the setting on |
| 2.26 | (prototype, N27) Users list for member pickers and avatar upload. Screen work: upload and remove controls on the profile, and the member picker reads real users (only the owner in solo use) | An uploaded avatar shows in the top bar, and the picker lists the owner only |

**Milestone reached:** usable board.

---

## Phase 3: Control and safety

**Goal:** users can trust agents with their code.

| ID | What | Done when |
|---|---|---|
| 3.1 | Permission modes: ask, auto-accept edits, plan only, full auto | Each mode tested against file writes and commands |
| 3.2 | Bypass mode with confirm step, banner, worktree-only rule, project lock | Bypass cannot touch the main branch or run outside the worktree |
| 3.3 | Permission profiles | Denied actions are blocked in every mode except bypass |
| 3.4 | Command blocklist | Blocked commands are stopped or sent for approval |
| 3.5 | Approval flow in the UI | Approve and deny from card, chat view, and home view |
| 3.6 | Secret scanner on agent commits | A commit with a test key is blocked and the card moves to needs you |
| 3.7 | Audit log with search and export | Every action from a test run is recorded and findable |
| 3.8 | Thinking modes mapped per provider | Setting changes the provider request as expected |
| 3.9 | Model switching per card, role, and mid-session | Switch works for built-in and CLI agents |
| 3.10 | Deploy approval rule | Deploy workflows cannot be run by agents outside bypass |
| 3.11 | (prototype, N7) One approval, many views: an approval a project chat asks for is the same row as the card's, and events reach every place that shows it | Approving in the chat updates the card and Home, and the reverse |
| 3.12 | (prototype, N8) Card settings and bypass calls: agent, role, model, thinking, and permission mode through `PATCH /v1/cards/{id}`, and bypass on and off with a typed acknowledgement and the project's lock. The daemon writes the system message and the activity row. Screen work: the bypass dialog and banner use the calls | A change applies on the next turn with a system message, and bypass is refused when the project is locked |
| 3.13 | (prototype) Audit log screen: list, search, and export the audit log, read only. Screen work: a new view under Settings built from existing table components | Actions from a test run are listed, searchable, and exportable, and nothing can be edited |

**Milestone reached:** safe to trust.

---

## Phase 4: Models

**Goal:** the built-in agent and full provider support.

| ID | What | Done when |
|---|---|---|
| 4.1 | Provider interface with OpenAI-compatible adapter | OpenRouter and DeepSeek work through one adapter |
| 4.2 | Anthropic and Gemini native adapters | Thinking settings and tool use work on both |
| 4.3 | Local models through Ollama and LM Studio | Built-in agent works offline |
| 4.4 | Built-in agent loop: read, edit, run, tools, under the harness | Built-in agent completes a fixture task |
| 4.5 | Per-provider queue with retry | Ten parallel requests on one key do not fail on rate limits |
| 4.6 | Model fallback | Simulated outage switches to backup and notifies |
| 4.7 | Usage and cost tracking | Cost meter per card, role, and model matches provider usage |
| 4.8 | Cost limits per project and global | Hitting a limit pauses cards and notifies |
| 4.9 | Connection test framework and provider key tests | Test button shows passed, partly working, and failed results with fix hints |
| 4.10 | (prototype, N18) Provider and limits settings calls: save, remove, and test provider keys (only a masked value comes back), and read and write cost and awake limits. Screen work: the Providers and Limits sections read and write the daemon | A saved key is never shown again, and a limit change takes effect on the next check |
| 4.11 | (prototype) Fallback and usage screens: choose a backup model per role and see usage and cost per card, role, and model. Screen work: new sections built from existing components | The backup model is used in a simulated outage, and the numbers match the usage table |

---

## Phase 5: Quality loop

**Goal:** idea to merged code, with approvals only.

| ID | What | Done when |
|---|---|---|
| 5.1 | Role templates: starter set, edit, duplicate, delete, reset, per-project overrides, export and import | Roles fully editable, weak-model warning on Reviewer and Integrator |
| 5.2 | Plan first mode, plan in chat only | Card waits in planning until approved |
| 5.3 | Harness limits: time, cost, rounds | Limits stop work and move the card to needs you |
| 5.4 | Stuck detector | Repeated error and edit loops are caught in tests |
| 5.5 | Checkpoints and restore | Restore returns worktree and optionally conversation to a checkpoint |
| 5.6 | Pull request creation from a card | A card opens a PR on GitHub |
| 5.7 | Reviewer role on every PR | Review comments go back to the worker |
| 5.8 | Integrator queue: dry-run merge, backup branch, temp worktree merge, tests, abort | Clean and conflicting fixture merges behave as designed |
| 5.9 | Context-aware conflict resolution | Integrator resolves fixture conflicts using both cards' context, asks when unsure |
| 5.10 | Sleep and wake with grouped reminders, pin, and awake limits | Idle cards sleep and wake with context, working cards never sleep |
| 5.11 | Worktree and branch cleanup | Worktrees removed after merge, backups expire |
| 5.12 | Quality module: project linters and built-in smell checks on card diffs, new-or-worse filtering, caching per commit | Fixture diffs produce the expected findings, and old smells are not blamed on the card |
| 5.13 | Smell profiles per project with default thresholds and severity | Changing a threshold changes the findings |
| 5.14 | Blocking findings loop back to the agent, warnings go to the Reviewer, model-only smells checked by the Reviewer | A card with a blocking smell is fixed before review |
| 5.15 | Findings on the card: ask to fix, dismiss with reason, reasons feed lessons | Both actions work, and dismiss reasons appear in lessons |
| 5.16 | (prototype, N4) Pause and resume: hold a Working card between turns. A working card cannot sleep until it is paused, a needs-you card stays awake, and a card without an awake session cannot sleep. Screen work: Pause becomes Resume, and a paused indicator shows on the card and the board | The rules match the prototype's messages, and pause and resume work through the API |
| 5.17 | (prototype, N5) Sleep reminder actions: keep awake for a set time, sleep now, keep all, sleep all, and dismiss. The keep-awake time is 15 minutes by default and is a setting. Screen work: a "Keep awake time" field under General next to idle and warning time | Each action changes the cards it names and the notice updates for every device |
| 5.18 | (prototype, N6) Plan calls: approve, reject, and edit and save a plan with its steps, files, risks, and checks. Screen work: the plan block actions use the calls | A card waits in Planning until its plan is approved |
| 5.19 | (prototype, N18) Role calls: create, duplicate, edit, delete, reset, per-project override, import, and export. Screen work: Import and Export buttons in the role editor | Roles fully editable, and an exported role imports on another machine |
| 5.20 | (prototype) Smell findings screen: the findings on a card with ask to fix and dismiss with a reason, and a smell profile section in Project settings. Screen work: new components in the card panel and settings | Both actions work and the profile thresholds change the findings |
| 5.21 | (prototype) Checkpoints screen: a list of the card's checkpoints with labels and a restore action that can also restore the conversation. Screen work: a section in the card's Activity tab or menu | Restore returns the worktree, and optionally the conversation, to the chosen checkpoint |

**Milestone reached:** idea to merge.

---

## Phase 6: CI and preview

| ID | What | Done when |
|---|---|---|
| 6.1 | GitHub App connection | App installs and receives events |
| 6.2 | CI monitor with webhooks and conditional polling backup | Card CI badges update live |
| 6.3 | CI fix loop: rerun once, send trimmed failed log, loop within limits | A failing fixture test gets fixed by the stub agent |
| 6.4 | Project CI panel, per package in monorepos | Main branch health and workflows visible |
| 6.5 | Local CI from workflow files | Test and lint steps run locally, unsupported steps marked |
| 6.6 | Live preview per card on its own port with isolated browser profile | Two cards preview at once without shared state |
| 6.7 | Screenshot checks before and after | Screenshots attach to card and PR |
| 6.8 | GitHub connection test, including webhook ping | Test catches a missing permission and a blocked webhook |
| 6.9 | (prototype, N10) Preview calls: start, stop, and state (stopped, starting, running), the project's dev command, and before and after screenshots. Screen work: the Preview tab buttons and screenshot pair use the calls | Two cards preview at once, and screenshots show in the tab and the pull request |
| 6.10 | (prototype, N28) Simulate CI failure with two modes. The synthetic mode injects a failed run through the CI monitor's normal path (rerun, trimmed log, loop limits, notice) and never touches GitHub. The real mode asks for confirmation and pushes a deliberately failing change to the card's own branch so GitHub Actions really fails. Both are audited and shown only in dev mode or with Developer options on. Screen work: two items in the card menu, a confirmation dialog for the real mode, and a label that says which mode ran | Each mode produces the same events as a real failure, the fix loop can be tested from the screen, and the real mode leaves only the marked commit |
| 6.11 | (prototype, N18) GitHub row calls: connect, disconnect, and test. Screen work: the GitHub row shows connected, not connected, or error, with the last test result | The row reflects the real connection and a Connect button starts the flow |

---

## Phase 7: Orchestration and memory

| ID | What | Done when |
|---|---|---|
| 7.1 | Internal MCP server with all tools in `architecture.md` | Agents can call every tool |
| 7.2 | Board awareness summary per turn | Summary stays under its token budget |
| 7.3 | File claims and early conflict warnings | Overlapping claims warn both cards |
| 7.4 | Orchestrator role: plan a goal into cards with dependencies | Orchestrator creates approved cards from a goal |
| 7.5 | Handoff between agents | Card continues on a different agent with a clean summary |
| 7.6 | Memory module: knowledge base, card notes, lessons | Agents read memory at start and write lessons after fixes |
| 7.7 | Vault with Obsidian-friendly layout and links | Vault opens in Obsidian with working links and graph |
| 7.8 | Vault file watching | User edits in Obsidian are picked up |
| 7.9 | Codebase map | Agents answer "where is X" without reading many files |
| 7.10 | Session search | Search finds past work across cards |
| 7.11 | Context budget meter and pinned files | Meter warns before auto-compaction |
| 7.12 | (prototype, N11) Card notes calls: read and write the card's vault note. Screen work: the Notes tab uses the calls | Agents read the note at the start of a turn, and edits made in Obsidian show in the tab |
| 7.13 | (prototype) Memory screens: a viewer and editor for the knowledge base and lessons, the context budget meter on the card, and pinned files. Screen work: new sections built from existing components | The meter warns before compaction, and pinned files stay in context |

---

## Phase 8: Automation

| ID | What | Done when |
|---|---|---|
| 8.1 | Scheduler: cron, interval, one-time, missed run policy | Jobs run on time and after wake as configured |
| 8.2 | Event triggers | CI failure, PR comment, Trello card, and labeled email trigger jobs |
| 8.3 | Loops with hard limits | Loops stop on every limit type |
| 8.4 | Trello integration with two-way sync, one board per project, including checklists, comments, attachments, and member mapping | Moves, checklists, comments, and attachments sync both ways, and the agent shows as a label |
| 8.5 | Google Calendar integration | Events available to briefs and the calendar view |
| 8.6 | Gmail integration | Labeled emails become cards |
| 8.7 | Morning and evening briefs across all projects | Briefs deliver on time with only new changes |
| 8.8 | Brief times from Trello or Calendar | Changing the event changes the brief time |
| 8.9 | (prototype, N21) Calendar range call: one call for a date range that returns events, scheduled jobs, briefs, and due cards. Screen work: the calendar and the Home coming up list read it, and show "Not connected" until Google Calendar is set up | Both screens show the same items, and a disconnected calendar shows the not-connected state with no sample data |
| 8.10 | (prototype, N18) Schedule and integration calls: create, edit, delete, enable, and run now for schedules with the missed-run policy, and connect, disconnect, and test for Trello, Google Calendar, and Gmail. Screen work: the schedule editor and the integration rows use the calls, with a Run now button | Each schedule runs on time and after wake, and each integration row shows its real state |

---

## Phase 9: Remote

| ID | What | Done when |
|---|---|---|
| 9.1 | tsnet node inside the daemon | Daemon reachable on the tailnet with no separate Tailscale install |
| 9.2 | Device pairing | A phone pairs by scanning a code |
| 9.3 | Funnel for webhook routes only | Only `/hooks/*` is public, signatures verified |
| 9.4 | Serve the responsive UI over the tailnet, and test on real phones and tablets | Every view and action works on a real phone and tablet through Tailscale |
| 9.5 | Telegram bot: notices, approvals, actions, voice notes | Approve and create cards from Telegram |
| 9.6 | Discord bot with the same features | Same checks as Telegram |
| 9.7 | Notification routing and grouping | Channels per event type, grouped notices |
| 9.8 | Remote machines: a second daemon on the tailnet | A project runs on a remote machine from the laptop UI |
| 9.9 | (prototype) Pairing and Tailscale screens: show the pairing code, tailnet status, and Funnel status, and revoke a device. Screen work: new sections beside the paired devices list, and the pairing step in onboarding | A phone pairs from the code on screen, and a revoked device loses access at once |

**Milestone reached:** anywhere.

---

## Phase 10: Advanced cards

| ID | What | Done when |
|---|---|---|
| 10.1 | Card templates | New cards start from templates |
| 10.2 | Dependencies | Dependent cards start after merges |
| 10.3 | Sub-cards | Parent shows combined progress |
| 10.4 | Acceptance checks | Cards cannot finish until checks pass |
| 10.5 | Card from anywhere, including code comments | Each source creates a card |
| 10.6 | Duplicate detection | Similar cards are flagged before creation |
| 10.7 | Fork a card | Fork runs on its own from a checkpoint |
| 10.8 | Race mode | Side-by-side comparison, winner kept |
| 10.9 | Agent scorecard | Stats per agent, model, and role, including code smells introduced |
| 10.10 | Checklists: named checklists, items, hide checked, required to finish, people only | Required checklists block the merge queue |
| 10.11 | Agent ticks with evidence, and auto untick when evidence stops being true | A failing test unticks the item it proved |
| 10.12 | Comments with files, images, links, mentions, and "Agent read this" | @agent and questions get a reply, and attachments reach the agent |
| 10.13 | Card members and member notices | Members get notices for mentions, replies, needs you, and merges |
| 10.14 | (prototype, N9) Check calls: run and list a card's checks with states pending, running, passed, and failed, and use results as tick evidence. Screen work: the Checks tab runs checks and gets an editor to add, edit, and remove them | A card cannot finish until its checks pass |
| 10.15 | (prototype, N22) Duplicate detection call: return similar cards for a draft title and body. Screen work: the new card dialog shows the warning from the call | Similar cards are flagged before creation |
| 10.16 | (prototype) Checklist options screen: "required to finish" and "people only" per checklist, the evidence behind an agent tick, and the note when a tick is removed. Screen work: controls and notes in the checklist block | Required checklists block the merge queue and a failing test unticks the item it proved |
| 10.17 | (prototype) Race mode and scorecard screens: a side-by-side comparison with a keep-winner action, and stats per agent, model, and role. Screen work: new views built from existing components | The winner is kept and the numbers match the usage and findings tables |
| 10.18 | (prototype) Templates, sub-cards, and dependency editing screens: manage card templates, show a parent's combined progress, and edit dependencies on the card. Screen work: new sections in settings and the card panel | New cards start from templates and dependent cards start after merges |

---

## Phase 11: Ecosystem

| ID | What | Done when |
|---|---|---|
| 11.1 | Skills folder, attach to roles, templates, and cards | Skills load for the right agents |
| 11.2 | Skill import with content preview | Import from a GitHub link |
| 11.3 | MCP manager and per-card MCP | Each card gets only its listed servers |
| 11.4 | MCP health check | Broken servers show as broken |
| 11.5 | Plugin API for agents, integrations, roles, and views | A sample plugin adds a new integration |
| 11.6 | (prototype) Skills and MCP screens: the skills folder, import with a content preview, the MCP manager with health, and per-card MCP. Screen work: new Settings sections and card fields, and the role editor's skills and MCP fields become real | Skills load for the right agents and a broken server shows as broken |

---

## Phase 12: Scale

| ID | What | Done when |
|---|---|---|
| 12.1 | List view | Sort and filter cards as a table |
| 12.2 | Timeline view | Cards over time with dependency lines |
| 12.3 | Calendar view | Jobs, briefs, and due cards |
| 12.4 | Split view and focus mode | Up to four panes, focus shows needs you only |
| 12.5 | Monorepo mode: tool detection, package graph, sparse worktrees, affected tests | Fixture monorepo works end to end |
| 12.6 | Resource panel | RAM, CPU, and disk per card |
| 12.7 | Export and import | A project moves to another machine |
| 12.8 | Encrypted backup and sync over Tailscale | Settings and memory sync between two devices |
| 12.9 | Team: shared boards, human handoff, user roles, comments | Two users work on one board |
| 12.10 | (prototype) Export and import screens for a project, and remote machine choice per project. Screen work: buttons in Project settings and a machine picker | A project moves to another machine from the screen |

> If the first real projects are monorepos, move task 12.5 into Phase 1 right after task 1.5. Full worktrees of a large monorepo are too heavy to use day to day.

**Milestone reached:** v1 complete.

---

## Testing approach by phase

| Kind | Starts in | What it covers |
|---|---|---|
| Unit tests | Phase 0 | Every Go package and UI component |
| Integration tests with the stub agent | Phase 1 | Sessions, harness, merges, CI loop without real models |
| Real agent smoke tests | Phase 1 | Each supported CLI version, run nightly |
| UI end-to-end tests | Phase 2 | Main flows in the desktop shell, and every view at phone, tablet, and desktop sizes |
| Budget checks | Phase 0 | Every pull request |
| Cross-platform checks | Phase 0 | macOS, Linux, and Windows on every pull request |
