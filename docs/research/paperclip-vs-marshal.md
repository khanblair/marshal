# Paperclip vs Marshal: a side-by-side comparison

Date: 2026-09-29. Subject: https://github.com/paperclipai/paperclip, a shallow clone at commit `b3eb03f` (2026-09-29, "fix(sentry): preserve run failure stacks and diagnostic context"). The clone was temporary and is not kept in this repo.

## How this was made, and how far to trust it

Eight read-only research agents each studied one slice of the code: product and spec, server and CLI, data model, agents and adapters, plugins and MCP, web UI, governance and security, and Marshal's own inventory. Nothing was run, built or installed. Every claim below comes from a file the agents opened, and the ones that carry weight were spot-checked against the clone by hand:

- 871 route registrations in the server (368 GET, 356 POST, 58 PATCH, 57 DELETE, 32 PUT): confirmed.
- `server/src/services/heartbeat.ts` is 30,078 lines and `issues.ts` is 13,385: confirmed.
- 220 `pgTable` definitions and 287 migration files: confirmed.
- Paperclip's own feature plan lists "built-in diff viewer" and "merge queue" under "What we do not want" (`doc/plans/2026-03-13-features.md`, line 660): confirmed.
- The Claude adapter defaults `dangerouslySkipPermissions` to true (`packages/adapters/claude-local/src/index.ts`, line 67), and the local secrets provider stores an unsalted SHA-256 of each secret (`server/src/secrets/local-encrypted-provider.ts`, line 94): confirmed.

Limits:
- Paperclip's `doc/plans/` folder (72 files) was only sampled and its 108 KB `doc/SPEC-implementation.md` was read in part. Some plans may say things this document does not.
- "They don't have X" always means "not found in what the agents read". It is not proof of absence.
- Marshal's numbers come from the Marshal inventory agent, which read the code but did not run it. Route counts differ slightly by counting method (155 to 161 method-and-path pairs; 46 to 48 tables depending on whether the two search-index tables and internal ones are counted).
- The Paperclip doc set contradicts its own code in several places (listed in section 6). Where they disagree, this document follows the code.

## 1. What each product is

| | Paperclip | Marshal |
|---|---|---|
| One-line idea | "If OpenClaw is an employee, Paperclip is the company." A control plane for teams of AI agents that run a business. | A kanban board that runs coding agents on cards, in real git worktrees, and moves the cards by what the agents actually do. |
| Unit of work | An issue ("task") assigned to an agent that sits in an org chart under a company goal. | A card: one task, one lasting agent session, one worktree, one branch. |
| Who it is for | People running "autonomous AI organizations" (CEO, CTO, marketing agents, "$1M MRR" goals). | A solo developer or small team running several coding agents on real repositories. |
| What it says it is not | Not a chatbot, an agent framework, a workflow builder, a single-agent tool, or "a code review tool". | Not a hosted service. Local first. |
| Agents run by | The agent's own process ("agents run wherever they run and phone home"). Paperclip is the control plane. | The daemon starts and supervises the agent process itself. |
| Stack | TypeScript on Node 24, Express 5, React 19, Postgres (embedded by default), about 480,000 lines of server code. | Go daemon with pure-Go SQLite, SolidJS web app, Tauri desktop and Android shells. |
| Surfaces | Web UI only. A PWA manifest that opts out of standalone mode. No native app and no push notifications found. | Web UI served by the daemon, Tauri desktop, Tauri Android shell, Telegram, Discord and ntfy alerts with approval buttons. |
| License | MIT (with a "Paperclip EE" enterprise edition mentioned). | Private repository. |

The products overlap in one real way: both put a human in front of a queue of agent work and gate the agents with approvals and cost limits. They differ in direction. Paperclip goes wide, toward running a whole organisation. Marshal goes deep, toward getting a change from an idea to merged code with a person in control.

## 2. Scale and shape

| Measure | Paperclip | Marshal |
|---|---|---|
| Server code | About 480,000 production lines and 534,000 test lines in `server/src` | 50 Go packages under `daemon/internal`, 363 Go test files, 143 golden files |
| Database | 220 tables, 287 migrations in 7.5 months (2026-02-17 to 2026-09-29), Postgres only | About 46 to 48 tables, 23 migrations, SQLite |
| HTTP API | About 871 routes, company-scoped | About 155 to 161 routes, single owner |
| Largest files | `heartbeat.ts` 30,078 lines, `issues.ts` 13,385, `routes/openapi.ts` 11,494 | Not measured. The inventory found no single-file monsters. |
| Tests | 868 test files in the server, 644 in the UI, 59 to 74 Playwright files, 135 Storybook stories, pixel baselines | 363 Go, 219 web, 92 in `packages/`, 12 Playwright specs, coverage floors of 70% (85% on the safety packages) |
| Idle footprint budget | Node plus embedded Postgres (a heavy local story, per the data-model agent) | Daemon idle RAM budget under 50 MB, enforced in CI |
| Turnaround of change | 503 commits in one release (`releases/v2026.916.0.md`) | 371 commits in total |

Paperclip's surface is very large for one server: about 100 chat-channel files, plugins, a cloud mode, two overlapping case models (pipelines and cases), a "smoke lab", and 28 tables for tool access alone. Its agents also wrote most of it (`CONTRIBUTING.md` asks for a "Model Used" section in every PR). The cost shows in the file sizes and in a recovery document of about 18,000 words (`doc/execution-semantics.md`) that exists to explain the accumulated special cases.

## 3. Feature comparison

Legend: **Yes** built and used. **Part** partly built. **Flag** exists but off or experimental. **No** not found. **Daemon only** built in the Marshal daemon but not reachable from the UI yet.

### 3.1 Work and agents

| Capability | Paperclip | Marshal |
|---|---|---|
| Kanban board | Yes. Seven status columns, drag and drop, cold columns collapsed, "show 10 more" paging (`ui/src/components/KanbanBoard.tsx`). | Yes. Board, list, timeline and agents views. |
| List and timeline views | List/board toggle plus a zoomable work timeline. | Yes, both. |
| Calendar view | Not found in the UI. | Yes (schedules and due cards, Google Calendar events). |
| Sub-tasks and blockers | Yes. Parent/child and a `blocks` relation, first-class blockers. | Part. Dependencies are stored and drawn on the timeline. There is no editor, and sub-cards are schema only. |
| Templates | Skills and a teams catalog (4 teams). No card templates found. | Part. Table exists, no service or screen. |
| Org chart and reporting lines | Yes. `agents.reports_to`, an org chart page, live status. | No (not the product). |
| Goals and projects hierarchy | Yes. Company goal, projects, milestones, issues, sub-issues. | Projects only. |
| Multi-company / multi-user | Yes. Everything is company-scoped, with memberships and 21 permission keys. | No. Solo owner. Team mode is planned and undecided. |
| Recurring work | Yes. Routines with schedule, webhook and API triggers, catch-up and concurrency policies. | Yes. Schedules and briefs with cron and a missed-run policy. |
| Chat with the agent on the task | Yes. A rich thread: queued messages with interrupt, plan mode, paused takeover, question cards. | Yes. Chat tab plus a terminal view. Project-level chats can also create cards. |
| Comments, checklists, members | Comments and documents yes. Checklists not found. | Yes (this week: checks, checklists with evidence, comments with attachments, members). |
| Plan-first mode | Yes ("Deep Planning", revisioned plan documents). | Yes. Plan edit, approve, reject. |
| Documents and attachments | Yes. Revisions, diff, annotation threads, artifacts. | Notes per card in an Obsidian-compatible vault, comment attachments. |

### 3.2 Running agents

| Capability | Paperclip | Marshal |
|---|---|---|
| Agents supported | 16 adapter types: Claude, Codex, Gemini, Kimi, Cursor (local and cloud), Grok, OpenCode, Pi, Hermes (local and gateway), OpenClaw, a generic process and HTTP adapter, and a Rust runner. No Qwen, MiniMax, Antigravity or OpenClaude adapter. | Can start Claude Code, Gemini CLI and a built-in agent. Detects and tests Qwen, Kimi, MiniMax, Cursor Agent, Antigravity, OpenClaude and Pi but cannot start them. Codex is detection only. |
| How they are driven | Claude, Codex, Gemini and Kimi go through one shared ACP engine by default, with the command-line lane as an explicit fallback. Others use each tool's own JSON mode. | Claude through its stream-JSON mode, Gemini through ACP, a built-in agent on 7 provider families, and a PTY adapter. |
| Built-in agent on your own API keys | Four "built-in agents" (briefs, learning, reflection coach, summarizer) which are managed agent rows, not a general coding agent. | Yes. A full agent on 7 provider families with queueing, fallback and pricing. |
| Agent scan and per-agent test | An `testEnvironment` method per adapter, used in onboarding. No local scan of what is installed found in the UI. | Yes. Scans the machine, tests each program (an ACP greeting, a version check, mode checks), with a Scan again button. |
| Lasting session per task | Sessions stored per agent and task, with a rotation policy (runs, tokens, age) and a handoff summary. | Yes. One lasting session per card, resumed after restart, with an append-only typed event history and full-text search. |
| Git worktrees | Behind a flag, off by default (`enableIsolatedWorkspaces`). The docs that say "hard-gated off" are stale. Also cloud sandboxes (8 provider plugins). | Yes, always. One worktree per card with sparse checkout and containment checks. |
| Cloud sandboxes | Yes. Cloudflare, Daytona, e2b, Modal, Novita, Kubernetes and others as plugins. | No. Local only. |
| Restart recovery | Extensive. Reap orphaned runs, resume queued runs, reconcile stranded work, durable cancellation fence, hot restart with adoption of live runs. | Sessions restored on start. Depth not compared line by line. |
| Cost capture | From the provider's own output only. No price table, so cost is unknown for Codex and Cursor cloud. | Pricing table, usage tracking and cost limits per project and global. |

### 3.3 Reviewing and shipping the work

This is where the two products differ most.

| Capability | Paperclip | Marshal |
|---|---|---|
| Built-in diff viewer | Rejected by design. Its plan says it does not want one. A "Changes" tab exists as a plugin (`plugin-workspace-diff`) with only three capabilities. | Yes. Per-file diff, hunks, checkpoints with restore. |
| Merge queue | Rejected by design ("What we do not want"). | Yes, in the daemon: dry-run merge in a throwaway worktree, backup branch, tests, fast-forward, one at a time per project. Daemon only. |
| Pull request from a card | A GitHub review bot connector exists. Work products track PRs and previews. | Yes in the daemon. Daemon only. |
| Reviewer role and code-smell gate | Runtime-routed review and approval stages on an issue, with a required comment. Not code-aware. | Reviewer role and a code-smell check with a profile per project. Daemon only. |
| CI monitor and fix loop | Not found as a first-class feature. | Yes. Webhook plus poll, failures sent back to the agent, local CI too (the local part is daemon only). |
| Live preview and screenshots | Preview services per workspace with distinct origins. | Yes. Dev server per card and screenshots. |

Marshal's daemon has all of the ship path but the web UI still calls none of `/merge`, `/pull-request`, `/review`, `/findings*`, `/handoff`, `/local-ci` or `/v1/audit*` (about 17 routes). So today the "idea to merged code" loop cannot be driven from the screen. That is Marshal's largest self-inflicted gap, and it is the part Paperclip has explicitly chosen not to build.

### 3.4 Safety, cost and control

| Capability | Paperclip | Marshal |
|---|---|---|
| Permission gate per tool call | Only in the experimental Rust runner. The default lanes run in bypass: Claude `--dangerously-skip-permissions` (default true), Codex bypass, Gemini yolo, Cursor yolo. ACP has `approve-all` as its default. | Yes. A harness decides every tool call: five permission modes, blocklist, deploy rule, bypass locked per project with a banner and audit. |
| Approvals | A generic approvals table (hire agent, CEO strategy, budget override, board request) with `revision_requested` and resubmit. Approve and reject are board-only. | Approval blocks in chat, answered from the web or from Telegram and Discord buttons. Plan approvals. |
| Budgets | Company, agent and project scopes; soft warning at 80% and hard stop at 100%; a hard stop pauses the scope, cancels its runs and opens an override approval. Dollar-only, so subscription usage counts as zero. No card scope. | Cost and awake limits per project and global, with a Home cost tile. No card scope either. |
| Tool governance | A full MCP gateway (risk classes, quarantine on schema drift, argument-hash-bound approvals, trust rules, append-only call log). | An internal MCP server with 15 tools per card. No external MCP manager yet (Phase 11). |
| Secrets | Encrypted local provider, AWS provider, per-run manifest of granted bindings, an access-event log. GCP and Vault are stubs. | OS keychain only. A secret scanner runs on agent commits. |
| Audit log | Activity log plus append-only tool-call and secret-access logs. | Append-only audit log. Read routes exist. No UI. |
| Untrusted input | An opt-in `low_trust_review` preset that fails closed unless a sandbox and isolated workspace are in use. | Bypass confined to the worktree. No equivalent preset. |

### 3.5 Reach, notifications and remote control

| Capability | Paperclip | Marshal |
|---|---|---|
| Phone control | Responsive web app. A LAN/tailnet preview script. No native app. | Tailnet built into the daemon (tsnet, no Tailscale install), device pairing with a code and QR, a Tauri Android shell. Not yet checked on a real device. |
| Push alerts | Not found. | Telegram, Discord and ntfy, with approval buttons, alert routing per event, and sender checks. |
| Chat as an interface | Slack, Discord, Telegram, Teams, GitHub, email (AgentMail) and iMessage connectors, all experimental and off by default. One bot is bound to one agent and each conversation becomes one task. | Telegram and Discord can approve, reject and create cards. One owner chat only. |
| Desktop app | Listed as planned. | Tauri desktop app (macOS). |
| Deployment | `local_trusted` (no login), `authenticated` private or public, Docker, a managed Cloud variant. | Local daemon with a loopback listener plus an optional tailnet listener. |

### 3.6 Extension and sharing

| Capability | Paperclip | Marshal |
|---|---|---|
| Plugins | Yes. One forked Node worker per plugin speaking JSON-RPC 2.0 over stdio, a 79-entry capability list checked on the host, an `upgrade_pending` gate when a new version asks for more, hot install, a full SDK and scaffolder, 17 UI slot types. Their own docs say workers and plugin UI must be treated as trusted: there is no operating-system sandbox. | No. Planned in Phase 11 with the same process model (out-of-process, JSON-RPC over stdio). |
| Skills | `SKILL.md` directories, a catalog of 17 (none installed by default), trust tiers, per-file hashes, commit pinning, scripts blocked from external sources, a policy engine per role or agent. | Tables exist, no code (Phase 11). |
| Sharing a setup | The "Agent Companies" markdown spec with a secrets-free `.paperclip.yaml` sidecar, ZIP import/export, dry-run preview. | Not built (export/backup is Phase 12). |
| Public registry | ClipHub is a plan only (every checklist box unchecked). | No. |

### 3.7 Interface quality and process

| Capability | Paperclip | Marshal |
|---|---|---|
| Design system rules | A written policy (`DESIGN.md`): tokens only, systematic status colours, machine values in mono, motion tokens tunable at runtime, checked by scripts and Storybook pixel baselines. | Design tokens package and a rule that the prototype wins. Contrast is checked in CI. |
| Contextual feedback rules | Explicit: no toast for state already on screen, stale outcomes refresh silently, cancellation is neutral grey. | Not written down as rules. |
| Command palette | Yes (cmdk) plus a full search page with a query parser. | Yes, with projects, cards, chats, past sessions and notes. |
| Internationalisation | Scaffolding only. 39 locale files, but `en.json` is 9 lines with one string, and only 3 files use it. | None. |
| Onboarding | A 4-step wizard (name, agent, model connection, review) with an animated character. No machine scan or folder browser found. | A 5-step flow: real account, agent scan and test, folder browser for the first project, phone pairing and alert channels. |
| Release process | Four channels (canary on every merge, nightly, beta with a 3-day soak, stable), trusted publishing, Docker and Sigstore. | A signed macOS release and an Android job in one workflow. No channels. |

## 4. What Marshal has that Paperclip does not

Each item is "not found in what was read", with the reason it matters.

1. **A code-aware ship path.** Diff, checkpoints, dry-run merge queue, PR and review over code, and a code-smell gate. Paperclip lists a diff viewer and a merge queue as things it does not want. Marshal owns this ground, though the UI does not reach it yet.
2. **A permission gate on every tool call, by default.** Paperclip's default lanes are bypass. Marshal has five modes, a blocklist, a deploy rule and a locked, audited bypass.
3. **Native, local-first surfaces.** A desktop app, an Android app and an in-daemon tailnet node. Paperclip has a browser app, a Linux-only Tailscale broker (it needs root, systemd and `SO_PEERCRED`), and no push.
4. **Phone alerts you can act on.** Telegram, Discord and ntfy notices with approval buttons and sender checks.
5. **A very small footprint.** Pure-Go SQLite and a CI-enforced idle RAM budget against Node plus a Postgres bundle.
6. **Event replay.** The Marshal event stream replays missed events after a reconnect. Paperclip's live events are process-local with no replay (`server/src/services/live-events.ts`), so a reconnecting client falls back to polling.
7. **Worktrees always on**, with containment checks. Paperclip's are behind a default-off flag.
8. **An agent scan, test and folder browser in onboarding.** Paperclip's wizard tests an agent's environment but does not scan the machine.
9. **A calendar view**, plus briefs.
10. **A per-card MCP server for agent awareness.** Claims on files, notes, `ask_agent`, checklist ticking with evidence. Paperclip's MCP server is a REST wrapper for its own API.
11. **Pinned scope.** Marshal does one thing (a repository, a board, agents that change code) and its docs and code agree with each other more closely than Paperclip's do.

## 5. What Paperclip has that is better than ours

Sorted by how much it matters for Marshal, not by how large it is.

1. **Governance of external tools.** The MCP gateway is the most mature part of Paperclip: read/write/destructive risk classes from MCP annotations, quarantine of new tools on schema drift, approvals bound to the exact arguments (HTTP 409 with an argument hash, then a retry with the approved id), trust rules that lapse when the schema changes, and an append-only call log that does not tell the agent why it was denied. Marshal's Phase 11 has nothing like it yet.
2. **A real plugin runtime.** A supervised process per plugin, exponential backoff, a shutdown ladder (RPC, 10 seconds, SIGTERM, 5 seconds, SIGKILL), a static capability list, an approval gate when an update asks for more, and a test harness. Marshal has only the plan. Paperclip's weak point is the missing sandbox and the same-origin plugin UI holding the board session.
3. **Run recovery and honesty about failure.** Four recovery passes on start, a durable cancellation fence, write revocation the instant a run is stopped (inside the same transaction as the write), replay only when there is positive evidence the provider never started, provider error families with a "retry not before" time, and an informational silent-run watchdog that never cancels by itself. Marshal restores sessions but does not have this depth.
4. **Cost capture that separates "reported" from "unpriced".** Unknown is never shown as zero. Cumulative usage from ACP agents is turned into a per-run delta. Marshal has a price table but should also mark unknowns.
5. **Run outcome separate from run status.** A `liveness_state` says whether a "succeeded" run actually advanced, only planned, returned nothing, or is blocked. That is exactly what a person wants to see on a card.
6. **Adapter breadth and one ACP engine.** Ten agents plus generic adapters, with Claude, Codex, Gemini and Kimi on one engine, and Claude can run over ACP through a bridge. Marshal starts two.
7. **Session care.** It refuses to resume a Claude session if the folder, prompt bundle or tool set changed, retries with a fresh session on "unknown session", and rotates a long session with a handoff note.
8. **Budgets as incidents and approvals.** Soft and hard incidents deduplicated per policy, window and threshold; a hard stop opens an approval whose only remedies are "raise and resume" or "keep paused"; editing a budget re-evaluates at once. Their gaps (no card scope, no token metric, dollars only) are gaps for Marshal too.
9. **Sharing and portability.** A markdown-first, secrets-free company export with pinned external references, and a skill system with trust tiers, hashes and a policy engine.
10. **Recurring work triggers.** Webhook and API triggers on routines, not only cron.
11. **Interface craft.** The task thread details (paused takeover that keeps the draft, queued messages with Interrupt and Cancel, a menu with Stop run / Stop and cancel / Stop and done, pending-question cards above the composer), a decisions desk for "what needs me", a mobile bottom navigation that hides on scroll, swipe to archive, and the written feedback and design token rules with Storybook baselines.
12. **Test and release discipline.** Credential-free acceptance catalogs, recovery tests with failpoints, performance thresholds with before and after numbers, fail-closed CI routing, a script that forbids `git push` inside agent code, and release channels with a soak period.
13. **Small structural rules.** A `doctor` command with typed checks, and a module-boundary check script (only three of its 400 service files follow it).

## 6. Where Paperclip contradicts itself (and where we should not copy the docs)

The agents found these mismatches between docs and code:

- Approval states: the V1 spec says "pending, approved, rejected, cancelled". The code has `revision_requested` too.
- "Not a SaaS, single-tenant" and "not automatically self-healing": the roadmap and code describe managed Cloud multi-tenancy and bounded self-healing.
- "No repo management": the docs describe server-side clones, worktrees and a GitHub token.
- Worktree UI "hard-gated off": it is behind a flag that defaults to off.
- The Gemini adapter is "not in the type enum": it is (`gemini_local`).
- Two case models (pipelines and cases) exist side by side, and several features were built then dropped (cloud upstream, setup tokens).
- `doc/CHANNELS.md` is about release channels, not chat channels.
- Rate limiting for private mode: the doc says it is off, the code enables it in every authenticated mode.

The lesson for Marshal is procedural: Paperclip's docs run ahead of, and behind, its code at the same time. Marshal's own docs have the same problem (the progress tracker was last updated 2026-09-27, and the Phase 10 to 13 reports still say "Not started" although sections S12, S14, S15, S16 and S24b are on the daemon). Trust `sections.ts`, the route table and the code.

## 7. Paperclip's own weaknesses (so we do not copy them)

- Giant files: a 30,000-line run engine and a 13,000-line issues service. Its own newer `modules/` layout, checked by a script, covers only three of more than 400 services.
- Permissive defaults: bypass flags, `approve-all`, unsandboxed local runs, a 48-hour agent token signed with HS256 with a legacy raw-secret fallback on by default and no token id.
- The local secrets provider stores an unsalted SHA-256 of every value, and the master key sits on the same host as the database.
- Live events are process-local with no replay.
- Costs are only as good as the provider output, with no price table.
- Statuses are plain text with no transition table for issues. Any known status may follow any other.
- The company metaphor (CEO, CMO, "$1M MRR") fits a solo developer with a repository poorly. The coding features are layered on as experimental or plugins.
- Internationalisation is scaffolding: 39 locales with one string.
- Two parallel run stacks (legacy adapters and the native runner), where the runner's design record is still "Proposed" and it is off by default.
- Telemetry is on by default (opt out).

## 8. Where the two are equal or close

- A command palette and a full search.
- An out-of-process plugin plan using JSON-RPC over stdio (Marshal's Phase 11 recommendation matches what Paperclip already runs).
- Approvals answerable away from the desk.
- Per-project or per-scope cost limits.
- A ring of automated tests with fakes in place of real models (both run real CLIs only in opt-in nightly checks).

## 9. The three findings that matter most

1. **The clearest opening is the ship path.** Paperclip declines to build a diff viewer or a merge queue. Marshal already has both in the daemon, and the only thing between them and a user is web wiring for about 17 routes.
2. **The most valuable things to learn are mechanisms, not features.** Argument-hash-bound approvals, positive-evidence replay, a run outcome separate from run status, write revocation on stop, and soft/hard budget incidents can each be added to Marshal in a small change, and none needs Paperclip's scale.
3. **The biggest thing to avoid is scope.** Paperclip's size, file sizes and doc drift are the price of going wide. Marshal's advantage is that it is small and its safety defaults are strict. Keep both.

The adoption plan (what to build, in what order, and what to leave out) is in `docs/research/paperclip-adoption-plan.md`.
