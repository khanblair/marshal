# Backend inventory

This document lists everything the prototype does and shows, and says for each thing where it lives once the daemon exists: which module owns it, which table stores it, which API call or event carries it, and which build plan task delivers it. It is the list that `backend-checklist.md` is checked against. If the prototype does something that is not listed here, this document is wrong and must be fixed first.

The source of truth for the prototype is the typed mock in `apps/web/src/mock/`, which was proven equal to the design's `store.js`. Behavior details (for example, which drags are refused and what the messages say) are read from `mock/actions/`.

---

## 1. How to read this document

Every prototype member gets one of three kinds:

| Kind | Meaning |
|---|---|
| **Daemon** | The daemon owns it. It needs a module, storage, an API call or event, and a build plan task. |
| **Client** | It is about the screen only (layout, menus, drafts, animation). It never goes to the daemon. |
| **Derived** | The client computes it from daemon data. Nothing new is stored. |

Columns used below:

- **Module** and **Storage** use the names in `architecture.md` sections 3 and 10.
- **API and events** uses the paths in section 11 of `architecture.md`. **New** means the path or table is not in that document yet. Every New row has a numbered entry in section 5 (N1 to N30), and building it includes updating `architecture.md` and adding a decisions log row in `progress-tracker.md`.
- **Task** is the build plan task ID. A dash means no build plan task covers it, so the checklist adds one.

Read the tables together with the cutover rules in section 2 of `backend-checklist.md`: a section of the prototype moves from mock data to the daemon only when its rows here are all built and verified.

---

## 2. Store members (`M.*`)

The prototype exposes 116 members on `M`. Each one appears in exactly one row.

### 2.1 Vocabulary, helpers, and plumbing

| Members | Kind | Notes |
|---|---|---|
| `STATUS`, `COLUMNS`, `CI`, `VIEWS` | Client | Labels, icons, and tones for card states, board columns, CI states, and views. The values of the states themselves come from the generated protocol types. |
| `THINK`, `PERMS` | Derived | The lists of thinking modes and permission modes come from protocol enums, so the harness and the UI cannot disagree. Labels stay in the UI. |
| `ROLE_NAMES` | Derived | Built from the roles the daemon returns (starter roles plus the user's own), never a fixed list. See N18. |
| `AGENTS`, `NO_THINK`, `thinkSupported` | Daemon | Agent detection and capabilities: which agents exist, their versions, their models, and whether a model supports thinking. Replaces the fixed list. See N19. |
| `T0`, `D`, `H`, `MIN`, `now`, `rel` | Client | Time helpers. `now` uses the daemon's clock offset. `rel` turns absolute timestamps into "4 min ago". See N25. |
| `money`, `full`, `tone`, `costTone`, `colOf` | Derived | Formatting of cost (from micro-units) and status colors. `costTone` compares cost with the limits the daemon returns. |
| `S`, `emit`, `set`, `nav`, `deco`, `decoMsgs`, `mobile`, `_framed`, `_scale` | Client | The store shell, change notification, keyboard navigation model, view-model helpers, and layout flags. The store keeps this shape. Its insides change. See backend-checklist section 2. |
| `setViewport`, `dragStart` | Client | Window size and drag pointer handling. |

### 2.2 Lookups and lists computed from mirrored data

| Members | Kind | Notes |
|---|---|---|
| `card`, `cardsOf`, `colCards`, `proj`, `person`, `chatsOf`, `chatById` | Derived | Lookups over the cards, projects, people, and chats the client has loaded. |
| `needs`, `awake`, `working`, `isAwake` | Derived | Counts and lists for badges and the Home dashboard. The Home tiles read the daemon's pre-computed numbers. Only lists on screen are computed here. |
| `costs` | Derived | Cost per project per day for the Home chart. Comes from `daily_stats`, never from summing cards. |
| `pendingApproval` | Derived | The first waiting approval, from `approval.requested` and `approval.resolved` events. |
| `filtered` | Derived | Applies the active filters and search text to the cards the client holds. Server-side filtering is added only when a list passes 100 items, as the standards require virtualization first. |
| `commands` | Derived | The command palette's action list, built from the current context. Project and card results come from the loaded data and from search. See N23. |
| `dupes` | Daemon | Duplicate detection while writing a card. New server call. See N22. Task 10.6. |

### 2.3 Navigation, dialogs, and screen state

| Members | Kind | Notes |
|---|---|---|
| `go`, `setView`, `openCard`, `closeCard`, `setTab`, `openChat`, `toggleTool`, `toast`, `confirm`, `closeDialog`, `startTour` | Client | Navigation, panels, menus, toasts, and confirmation dialogs. Two things around them are new for the daemon: the last view per project is saved as a preference (N30), and opening a card starts its output subscription. |
| `setTheme` | Client | The theme is applied on the screen and saved as a user preference (N30). |
| `setMode` | Daemon | Switching a card between chat view and terminal view stops the process and resumes the same session in the other mode. `POST /v1/cards/{id}/view`, event `session.state_changed`. Task 2.7. |

### 2.4 Onboarding and first launch

| Members | Kind | Notes |
|---|---|---|
| `finishOnboarding` | Daemon | Saves progress with `PATCH /v1/me/progress`. The sample project option creates a real project (N24). Task 2.16. |
| `resetFirstLaunch` | Daemon (dev only) | Resets onboarding and tutorial progress. Only exists in dev mode, next to `pnpm dev:reset`. New. See N24. |

### 2.5 Projects

| Members | Kind | Module and storage | API and events | Task |
|---|---|---|---|---|
| `addProject` | Daemon | `projects`; `projects`, `boards` | `POST /v1/projects` (folder, GitHub clone, or sample); `project.created` | 1.4, 2.14 |
| `renameProject` | Daemon | `projects`; `projects` | `PATCH /v1/projects/{id}`; `project.updated` | 1.4, 2.14 |
| `removeProject` | Daemon | `projects`; many tables | `DELETE /v1/projects/{id}` with keep-branches and keep-memory options (N16); `project.removed` | 1.4, 2.14 |

### 2.6 Cards: create, edit, move

| Members | Kind | Module and storage | API and events | Task |
|---|---|---|---|---|
| `newCard`, `createCard`, `quickAdd` | Daemon | `projects`; `cards` | `POST /v1/projects/{id}/cards`, with an optional start state (Backlog, Planning, Working) for "Add a card"; `card.created` | 2.4 |
| `rename`, `deleteCard` | Daemon | `projects`; `cards` and everything hanging off a card | New `PATCH` and `DELETE /v1/cards/{id}`; `card.updated`, `card.deleted` (N3) | 2.4 |
| `moveCard`, `refuse` | Daemon | `projects`; `cards` | New `POST /v1/cards/{id}/move` with the allowed-move rules and the refusal messages taken from `mock/actions/cards.ts` (N2). The client may pre-check with the same rules, but the daemon decides. `card.moved` | 2.4 |
| `fork` | Daemon | `projects`, `gitx`; `cards`, `checkpoints` | `POST /v1/cards/{id}/fork`; `card.created` | 10.7 |
| `setSetting` | Daemon | `session`, `harness`; `sessions`, `cards` | New `PATCH /v1/cards/{id}` for agent, role, model, thinking, and permission mode. Takes effect on the next turn and writes a system message and an activity row (N8). | 3.1, 3.8, 3.9 |
| `requestBypass`, `turnOffBypass` | Daemon | `harness`, `security`; `sessions`, `audit_log` | New `POST` and `DELETE /v1/cards/{id}/bypass`. The daemon checks the typed acknowledgement and the project's bypass lock, and audits both. (N8) | 3.2 |

### 2.7 Session control and notices

| Members | Kind | Module and storage | API and events | Task |
|---|---|---|---|---|
| `start` | Daemon | `session`; `sessions` | `POST /v1/cards/{id}/start`; `session.state_changed`, `card.moved` | 1.9 |
| `pause` | Daemon | `session`, `harness`; `sessions` | New `POST /v1/cards/{id}/pause` and `/resume`. Only Working cards pause. Working cards cannot sleep until paused. (N4) | none, added |
| `sleep`, `wake`, `pin` | Daemon | `session`; `sessions`, `cards` | `POST /v1/cards/{id}/sleep`, `/wake`, `/pin`, with the refusal rules from `mock/actions/sessions.ts` (N4) | 5.10 |
| `keepAwake`, `keepAllAwake`, `sleepAll`, `dismissNotice` | Daemon | `session`, `notify`; `notices` | New `POST /v1/notices/{id}/actions` for keep awake (with a length of time), sleep now, keep all, and sleep all, and a dismiss call; `notice.created`, `notice.updated` (N5) | 5.10 |

### 2.8 Chat view, plans, and approvals

| Members | Kind | Module and storage | API and events | Task |
|---|---|---|---|---|
| `send` | Daemon | `session`; `session_events` and log files | `POST /v1/cards/{id}/messages`; `session.output`, `session.tool_call` | 1.9, 2.6 |
| `approve`, `deny` | Daemon | `harness`; `approvals` | `POST /v1/approvals/{id}`; `approval.resolved`. An approval that a project chat asked for is the same approval as the card's, and both views update. (N7) | 3.5 |
| `approvePlan`, `rejectPlan`, `editPlan`, `savePlan` | Daemon | `session`, `projects`; `session_events` | New `POST /v1/cards/{id}/plan/approve` and `/reject`, and `PUT /v1/cards/{id}/plan` for edits; `plan.updated` (N6) | 5.2 |

### 2.9 Project chats

| Members | Kind | Module and storage | API and events | Task |
|---|---|---|---|---|
| `chatSend` | Daemon | `chats`, `session` | `POST /v1/chats/{id}/messages`; `session.output` | 2.11 |
| `newChat` | Daemon | `chats`; `chats`, `sessions` | `POST /v1/projects/{id}/chats`; `chat.created` | 2.11 |
| `renameChat`, `archiveChat`, `deleteChat` | Daemon | `chats` | `PATCH /v1/chats/{id}`, `POST /v1/chats/{id}/archive` and `/restore`, `DELETE /v1/chats/{id}`; `chat.updated`, `chat.archived`, `chat.deleted` | 2.11 |

### 2.10 Checklists, comments, and members

| Members | Kind | Module and storage | API and events | Task |
|---|---|---|---|---|
| `addChecklist`, `deleteChecklist`, `addItem`, `removeItem`, `toggleItem`, `toggleHideDone` | Daemon | `projects`; `checklists`, `checklist_items` | `POST /v1/cards/{id}/checklists`, `PATCH` and `DELETE /v1/checklists/{id}`, `POST /v1/checklists/{id}/items`, `PATCH` and `DELETE /v1/checklist-items/{id}`; `checklist.updated` | 10.10, 10.11 |
| `addComment`, `deleteComment` | Daemon | `comments`; `comments`, `attachments` | `GET` and `POST /v1/cards/{id}/comments`, `DELETE /v1/comments/{id}`, `POST /v1/comments/{id}/attachments`; `comment.created`, `comment.read_by_agent` | 10.12 |
| `toggleMember` | Daemon | `projects`; `card_members` | `PUT /v1/cards/{id}/members`; `card.members_changed` | 10.13 |

### 2.11 Filters and saved views

| Members | Kind | Notes |
|---|---|---|
| `addFilter`, `removeFilter`, `clearFilters` | Client | The live filters and search text of a board, list, or timeline. Kept on the screen and remembered per project as a preference (N30). |
| `applyView`, `saveView` | Daemon | Saved views (name, filters, swimlane) are stored: `saved_views`. New `GET`, `POST`, `PATCH`, `DELETE /v1/projects/{id}/saved-views` (N20). Task 2.5. |

### 2.12 Diff, files, checks, and CI

| Members | Kind | Module and storage | API and events | Task |
|---|---|---|---|---|
| `diffFor`, `filesFor` | Daemon | `gitx`; worktree | `GET /v1/cards/{id}/diff` for the file list and counts, and New per-file hunks loaded on demand for large files (N15); `card.updated` | 2.9 |
| `runChecks` | Daemon | `projects`, `quality`; `card_checks` | New `POST /v1/cards/{id}/checks/run` and `GET /v1/cards/{id}/checks`; `quality.checked` (N9) | 10.4 |
| `simulateCiFailure` | Daemon (dev options) | `ci`, `session` | New `POST /v1/cards/{id}/ci/simulate-failure`. Built as a real feature that runs the normal CI failure path with a synthetic run (N28). | 6.3 |

Check: the tables above list every one of the 116 members exactly once.

---

## 3. State fields (`M.S.*`)

The prototype's state has 78 fields. Each appears once.

### 3.1 Data the daemon owns

| Fields | Storage and source | Notes |
|---|---|---|
| `projects` | `projects`, `ci_runs` | Adds language, default branch, CI state, workflow runs, packages, dev command, and bypass lock (N16). `ciAgo` and `monthBase` in the mock are replaced by absolute times and real daily numbers (N25). |
| `cards` | `cards`, `sessions` | Adds the fields listed in N1. |
| `chat` | Session logs and `session_events` | One list of structured messages per card. Loaded by page, streamed by event (N13). |
| `act` | `session_events` | The card's activity feed (N14). |
| `checks` | `card_checks` | Acceptance checks and their results (N9). |
| `chats` | `chats`, `sessions` | Project chats and their messages (N13). |
| `people` | `users`, `memberships` | In solo use, one person. `person(id)` reads this list (N27). |
| `savedViews` | `saved_views` | Per project (N20). |
| `notices` | `notices` | Sleep reminders, CI on main, cost, plan, and CI notices (N5, N17). |
| `feed` | `activity` | The Home activity stream (N17). |
| `limits` | `limits` | Global and per-project daily cost, monthly cost, and awake limits. |
| `sleep` | `settings` | Idle time, warning time, reminder channel, and restore mode. |
| `roles` | `roles`, `role_overrides` | Roles, with starter flag and overridden flag (N18). |
| `providers` | `settings`, keychain | Provider status and a masked key. The key itself never reaches the client (N18). |
| `integrations` | `integrations` | Connected services, and the result of the last connection test (N18). |
| `schedules` | `schedules`, `schedule_runs` | Briefs and jobs (N18). |
| `calEvents` | `integrations` (Google Calendar) | Calendar events for the calendar view and the coming up list (N21). |
| `profile` | `users`, `devices` | Name, email, time zone, avatar, tailnet identity, and paired devices (N27). |
| `onboarding`, `obStep` | `user_progress` | Progress is saved per user. |
| `preview` | `preview` module | Preview state per card: stopped, starting, running (N10). |
| `notes` | Vault card note | The card's Notes tab (N11). |

### 3.2 Screen state that stays on the client

| Fields | Notes |
|---|---|
| `ready`, `vw`, `vh`, `reduced`, `resolvedTheme`, `announce`, `toasts`, `dialog`, `palette`, `menu`, `newCard`, `newProject`, `removeProject`, `newChatOpen`, `renaming`, `sideOpen`, `noticesOpen`, `settingsPid`, `schedEdit`, `roleSel`, `settingsSection`, `dragId`, `dropCol`, `quickAddAt`, `mobileCol`, `mobileTab`, `calExpand`, `allKind` | Menus, drafts, dialogs, drag state, and which screen part is open. Never sent to the daemon. `ready` also waits for the first daemon snapshot. |
| `route`, `openId`, `focusId`, `tab`, `mode`, `switching`, `chatOpen`, `chatQuery`, `archOpen` | Where the user is. Deep links to a card or chat use the same values. `mode` and `switching` follow the session's view state from the daemon while a switch is in progress. |
| `lastView`, `filters`, `query`, `swim`, `laneCollapsed`, `showAllDone`, `savedView` | Per-project screen preferences. Saved as user preferences so they follow the user between devices (N30). `savedView` names the saved view in use. |
| `theme`, `sidebarCollapsed`, `detailW`, `detailExpanded`, `split`, `listCols`, `sort`, `calMode`, `calCursor`, `dashRange`, `noticesSeen`, `tour` | Layout and view preferences. Theme, list columns, and sort are saved with the user. The rest are kept on the device (N30). `noticesSeen` becomes the read state of notices. `tour` step is screen state, and finishing or skipping it is saved in `user_progress`. |

Check: the tables above list every one of the 78 fields exactly once.

---

## 4. Data the prototype shows, by object

This part lists the fields of each object the screens draw, so none of them is forgotten when the wire types are designed. Items marked New are missing from the tables in `architecture.md`.

### 4.1 Card

| Field group | Where it lives | Notes |
|---|---|---|
| Identity and text: number, project, title | `cards` | The number is the small whole number shown as "#41". It stays a small integer, unique across projects. See N26. |
| State and reason: state, why it needs you | `cards` | The state list matches `architecture.md` section 6 plus "merging" as an indicator inside Ready to merge. The reason (plan ready, approval needed, stuck, limit, CI failed, conflict) is New (N1). |
| Agent settings: role, agent, model, thinking, permission mode, bypass | `cards`, `sessions` | Already modeled. |
| Session flags: asleep, waking, pinned, paused | `sessions`, `cards` | `paused` is New (N4). `waking` follows `session.state_changed`. |
| Progress: "doing now", context used, merge progress | `sessions`; event only for merge progress | "Doing now" comes from `report_progress`. Context used is stored on the session (N1). Merge progress travels only in `merge.progress` events. |
| Git and CI: branch, pull request, CI state, package | `cards`, `external_links`, `ci_runs` | Pull request and package are New on the card (N1). |
| Cost | `usage` | Summed per card. The list and board read a stored total. |
| Dates: started, ended, due | `cards` | New (N1). What the timeline draws must be confirmed first (open decision D1). |
| Labels | New tables | New (N1). |
| Dependencies, members, checklists, comments | `card_links`, `card_members`, `checklists`, `comments` | Already modeled. The prototype's checklists have "hide checked" and who ticked, and `architecture.md` also has "required" and "people only". The screens for the last two do not exist yet (section 6). |
| Last update | `cards` | Used for sorting. |

### 4.2 Project

| Field group | Where it lives | Notes |
|---|---|---|
| Name, folder, language, default branch | `projects` | Language is detected on create. |
| CI: overall state and recent runs | `ci_runs` | Each run has a workflow name, a state, a time, and a package in a monorepo. Times are absolute. |
| Packages | `projects` | Comes from monorepo tool detection (task 12.5). |
| Dev command for preview | `projects.settings_json` | New (N10). |
| Bypass lock | `projects.settings_json` | Already in the build plan (task 3.2). |
| Monthly cost history | `daily_stats` | Replaces the mock's made-up base number. |

### 4.3 Chat messages

The chat and terminal views draw these message kinds. Each becomes a typed event and a stored log entry (N13).

| Kind | Source of truth |
|---|---|
| User message | Written by the API when a message is sent |
| Agent message, streamed | `session.output`, stored in the session log |
| Tool call, with a result, a state (ok, running, failed), and a detail that opens | `session.tool_call`; detail loaded on demand |
| System note (session started, setting changed, resumed) | Written by the daemon |
| Diff summary (files, additions, deletions) | Written when a turn ends with changes |
| Plan (waiting, approved, rejected, edited; steps, files, risks, checks) | Written by the session in plan-first mode |
| Approval (waiting, approved, denied; command and reason) | `approval.requested` and `approval.resolved` |
| Card reference and card links | Written when an agent or the Orchestrator creates or mentions cards |

### 4.4 Activity, feed, and notices

| Object | Kinds the screens draw | Notes |
|---|---|---|
| Card activity | file, command, test, tool, approval; each with a result and a state (ok, running, failed, waiting) | `session_events`. The kinds are a fixed list in the protocol (N14). |
| Home feed | brief, merge, schedule, approval, plan, ci, tool | `activity`. Fixed list (N17). |
| Notices | sleep reminder (a list of cards and a deadline), CI failed on main, cost limit, plan ready, CI failed on a card | `notices`. Fixed list (N17). |

### 4.5 Settings objects

| Object | Fields the screens edit | Notes |
|---|---|---|
| Role | name, starter flag, overridden flag, description, agent, model, thinking, permission mode, strength, instructions, skills, MCP servers, backup model, limits for time, cost, and rounds | The role editor already shows the weak-model warning. |
| Provider | name, status (saved, empty, invalid), masked key, models, error, local flag | Keys go to the keychain only. |
| Integration | name, status (connected, none, error), detail line, last test result | Seven in the prototype: GitHub, Trello, Google Calendar, Gmail, Telegram, Discord, Obsidian. Tailscale is shown on the profile. |
| Schedule | name, kind (brief or job), trigger, time, days, action, project, enabled, missed-run policy | |
| Limits | daily and monthly cost and awake limit, globally and per project | |
| Sleep settings | idle time, warning time, reminder channel, restore mode | |
| Profile | name, email, time zone, avatar, paired devices | |

---

## 5. Work the prototype needs that the docs do not have yet

Each entry is built by the phase named in the last column, and includes updating `architecture.md` (data model, API, or events), `project-structure.md` if files change, and a row in the decisions log of `progress-tracker.md`. None of these changes a design decision already made. They fill in what the docs left as examples.

| ID | What is missing | What to build | Phase |
|---|---|---|---|
| N1 | Card fields with no home: labels, package, due date, start and end dates, pull request link, context used, needs-you reason, "doing now" | Add them to the data model, the API, and the generated types. Labels get their own table and a per-project list. | 2 |
| N2 | The rules for manual moves are not written down | Write the allowed-move matrix and the refusal messages from `mock/actions/cards.ts` into `architecture.md` section 6. The daemon enforces them and returns a stable reason code and a plain message. | 2 |
| N3 | No API to rename or delete a card | Add `PATCH` and `DELETE /v1/cards/{id}`. Delete asks for confirmation, stops the session, removes the worktree, comments, and attachments, and warns about unmerged work. | 2 |
| N4 | "Pause" has no meaning in the docs | Define pause as holding a Working card between turns. A working card cannot sleep until it is paused. Add pause and resume calls and a session flag. The sleep refusal rules (needs you stays awake, no awake session) come from `mock/actions/sessions.ts`. | 5 |
| N5 | Sleep reminder actions have no API | Add notice actions: keep awake for a set time (the prototype uses 15 minutes), sleep now, keep all, sleep all, and dismiss. The keep-awake time becomes a setting. | 5 |
| N6 | Plan approve, reject, and edit have no API | Add plan calls and a `plan.updated` event. Define the plan's parts: steps, files, risks, and checks. | 5 |
| N7 | Approvals asked in a project chat and on a card are one approval | Keep one row in `approvals`, and send its events to every chat and card that shows it. | 3 |
| N8 | Card setting changes and the bypass confirmation have no API | Add `PATCH /v1/cards/{id}` for settings and the bypass calls. The daemon writes the system message and the activity row and checks the acknowledgement. | 3 |
| N9 | "Run checks" has no API | Add run and list calls for a card's checks, with states pending, running, passed, and failed, and use their results as evidence for checklist ticks. | 10 |
| N10 | Preview start and stop, and the project's dev command | Add preview calls and state events, a dev command setting per project, and before and after screenshots for the Preview tab. | 6 |
| N11 | The Notes tab has no API | Read and write the card's vault note. Agents read it at the start of a turn. | 7 |
| N12 | The terminal's input, resize, and key bar have no channel | Add input and resize message types to the event stream. Add the key bar keys the phone layout shows. | 2 |
| N13 | Chat history and the message kinds are not defined | Define the typed message kinds in the protocol package (section 4.3). Add paged history for cards and chats, streaming text, and on-demand tool detail. | 2 |
| N14 | Activity list API | Add a paged activity list per card, with the fixed list of kinds. | 2 |
| N15 | Diff loading for large files | List files with counts first, and load hunks for a file when it is opened. Files over a size limit stay collapsed until asked. | 2 |
| N16 | Project fields and remove options | Add language, dev command, and bypass lock to project settings, and keep-branches and keep-memory to remove. | 1 |
| N17 | Vocabulary for the Home feed and notices, and the view-all pages | Fix the lists of feed kinds and notice kinds in the protocol. Add paged, filtered activity and CI lists. | 2 |
| N18 | Settings have no create, edit, or delete calls (only connection tests are listed) | Add calls for roles (create, duplicate, edit, delete, reset, per-project override, import, export), provider keys (save, remove, test), integrations (connect, disconnect, test), schedules (create, edit, delete, enable, run now), limits, and settings. | 3 to 8 |
| N19 | The agents and models are a fixed list | Add `GET /v1/agents` with kind, version, status (supported, untested, missing), models, and capabilities including thinking support. The UI lists come from it. | 1 |
| N20 | Saved views have a table but no API | Add the saved view calls. | 2 |
| N21 | The calendar has no single data call | Add a calendar call for a date range that returns events, scheduled jobs, briefs, and due cards. | 8 |
| N22 | Duplicate detection has no API | Add a call that returns similar cards for a draft title and body. | 10 |
| N23 | Search covers sessions and notes only | Extend search to projects, cards, and chats for the command palette. | 2 |
| N24 | The sample project and dev reset | Ship a small sample repository. Let `POST /v1/projects` create a project from it. Add a dev-only reset of first-launch progress. | 2 |
| N25 | Times in the mock are relative or made up | Send absolute timestamps and the daemon's current time. The client shows "4 min ago" and countdowns from them. Charts read `daily_stats`. | 1 |
| N26 | IDs | Cards use small whole numbers unique across projects, projects use short readable ids, and chats and other objects use opaque ids. Decide and write it in `architecture.md`. | 1 |
| N27 | People and the avatar | Add a users list call for member pickers and an avatar upload call. | 2 |
| N28 | "Simulate CI failure" | Build it as a real feature (see the checklist). | 6 |
| N29 | The screens the mock never needed | Loading, empty, error, offline, reconnect, and sign-in or pairing states. | 1 to 2 |
| N30 | Where screen preferences are saved | Confirm the split in open decision D2. | 2 |

---

## 6. What the docs need that the prototype has no screen for

These belong to the daemon and are built in their phases. Their screens do not exist yet, so they need design work first (a new view or section follows `ui-rules.md` and `ui-registry.md`, and goes through design review). They are listed here so the backend is not built without a way to see it.

- Checklist options "required to finish" and "people only", the evidence behind an agent tick, and the note when a tick is removed (tasks 10.10 and 10.11).
- Acceptance check editor and their results on the card (task 10.4).
- Code smell findings on the card, with ask to fix and dismiss (tasks 5.12 to 5.15), and the smell profile settings.
- Checkpoint list and restore, beyond the single "Restore a checkpoint" menu item (task 5.5).
- Race mode and the agent scorecard (tasks 10.8 and 10.9).
- Audit log with search and export (task 3.7).
- Resource panel (task 12.6).
- Skills, the MCP manager, and per-card MCP (tasks 11.1 to 11.4).
- Memory: the knowledge base, lessons, context budget meter, and pinned files (tasks 7.6 to 7.11).
- Card templates, sub-cards, and dependency editing beyond what the board shows (tasks 10.1 to 10.3).
- Provider fallback and usage screens (tasks 4.6 and 4.7).
- Remote machines and the export and import of a project (tasks 9.8 and 12.7).
- The Tailscale and device pairing screens (tasks 9.1 and 9.2), beyond the profile list.
