# What to take from Paperclip

Date: 2026-09-29. Companion to `paperclip-vs-marshal.md`, which holds the evidence and the caveats. Source: Paperclip at commit `b3eb03f`, read by eight research agents; nothing was run.

This list keeps only what fits Marshal: a local-first, single-owner board that runs coding agents in worktrees. It leaves out Paperclip's company model (org chart, goals, CEO agents, multi-company tenancy) and its size.

Each item names the idea, where it lives in Paperclip so it can be re-read, where it would land in Marshal, and the rough cost. Sizes are S (a day or less), M (a few days), L (a week or more). They are estimates, not measured.

## A. Do first: small, and they close real gaps

| # | Idea | Paperclip source | Marshal landing | Size |
|---|---|---|---|---|
| A1 | **Run outcome separate from run status.** A `liveness_state` (advanced, plan-only, empty response, blocked, failed) so "finished" does not read as "done". Shown on the card. | `packages/db/src/schema/heartbeat_runs.ts`, `server/src/services/run-liveness.ts` | Session store plus a card badge. Fits the existing typed event history. | M |
| A2 | **Never show unknown cost as zero.** Track a `usageBasis` (reported, unpriced, subscription) and store cumulative-to-delta for agents that report running totals. | `server/src/services/cost-usage.ts` (approx.), adapter usage parsers | Cost tile and the pricing table in the built-in agent. Mark Claude/Gemini subscription runs as "not billed" rather than 0. | S |
| A3 | **Card move table.** Paperclip has none and pays for it. Marshal already gates Ready with the checklist. Write the full allowed-move table in one place and test it. | Counter-example: `packages/shared/src/constants.ts` (statuses only) | Projects service. | S |
| A4 | **Conditional-UPDATE claim.** Take a card or a run slot with a single `UPDATE ... WHERE state = X` and treat zero rows as "someone else has it" (HTTP 409). | `server/src/services/issues.ts` checkout | Merge queue slot, agent start, plan approval, pull-request create. Audit for read-then-write races. | S |
| A5 | **Startup orphan reaper.** On boot, mark runs left "running" by a dead process, resume the queued, and list what needs a person. | `server/src/index.ts` recovery passes, `doc/execution-semantics.md` | Daemon start. Sessions are restored today; add an explicit pass with a written result. | M |
| A6 | **Cancel revokes writes.** When a run is stopped, the same transaction that records the stop invalidates the run's token, so a late write from the agent is refused. | run-token checks in `server/src/middleware/auth.ts` | Per-card MCP server and hooks: refuse tool writes for a stopped session. | S |
| A7 | **Positive evidence before replay.** Re-run an interrupted turn only when there is proof the provider never started it. Otherwise ask a person. | `doc/execution-semantics.md` (replay rules) | Session resume after a crash. Avoids double-applying edits or double-spending. | S |
| A8 | **Session identity check before resume.** Do not resume a session if the folder, prompt bundle or tool set changed; on "unknown session", retry fresh with a note. | `packages/adapters/claude-local/src/server/execute.ts` (resume guards) | Claude and Gemini adapters. | S |
| A9 | **Doc-vs-code drift guard.** One test that fails when `docs/backend-checklist.md` §2.2 and `data/sections.ts` disagree (Marshal already has half of this), and one that fails when `docs/phase-reports` claim "Not started" for a section that reports `daemon`. | Counter-example: the drift list in section 6 of the comparison | CI. | S |

## B. Worth a design pass: bigger, high value

### B1. MCP governance vocabulary (fits Phase 11)

Paperclip's gateway is the best-thought-out part of the read. Take the vocabulary, not the code:

- **Risk class per tool** (read, write, destructive), taken from MCP annotations, defaulting to the strict class when absent.
- **Quarantine on schema drift.** A tool whose schema changed is blocked until the owner re-approves. Trust rules lapse when the schema hash changes.
- **Argument-hash-bound approvals.** A blocked call returns 409 with a hash of the exact arguments; approving binds to that hash, so an approved call cannot be swapped for a different one.
- **Append-only call log** that records the decision but does not tell the agent why it was denied.

Marshal already has the delivery half: approval buttons on Telegram, Discord and the web. Mapping "approve this exact call" onto those buttons is a small step once the hash exists. Paperclip's own weak spot to avoid: its `packages/mcp-server` endpoint mode bypasses the governance layer, so make sure Marshal's manager has one path only.
Source: `server/src/services/mcp-gateway/` (approx.), `doc/plans/` MCP files. Size: L.

### B2. Plugin runtime rules (Phase 11)

Marshal's plan already matches Paperclip's process model (one out-of-process worker, JSON-RPC over stdio). Take these specifics:

- A **static capability list** checked on the host for every call, with each plugin declaring what it needs in its manifest.
- An **`upgrade_pending` gate**: a new version that asks for more capabilities does not run until the owner approves.
- A **shutdown ladder** (RPC ask, wait 10 s, SIGTERM, wait 5 s, SIGKILL) and exponential restart backoff.
- **Tell users plainly that plugins are trusted code** unless Marshal adds an OS sandbox. Paperclip's own docs say so. Do not claim a sandbox that is not there.

Do not copy: same-origin plugin UI (it holds the board session), or 79 capabilities on day one. Start with the dozen Marshal needs.
Source: `packages/plugins/sdk`, `server/src/services/plugin-*` (approx.). Size: L.

### B3. Budget incidents (extends the cost limits Marshal already has)

- Two thresholds per scope: **soft at 80%** raises a notice, **hard at 100%** pauses the scope and cancels its runs.
- Each crossing is one **incident, unique per (policy, window, threshold)**, so the same overrun never spams alerts.
- A hard stop opens an **approval** with two answers only: raise the limit and resume, or keep paused.
- **Re-evaluate at once** when the limit is edited.
- Add what Paperclip lacks: a **per-card scope** and a **token metric**, since dollars are zero for subscription users.

Lands on: cost limits, alert routing (Telegram/Discord/ntfy already exist), the approvals block.
Source: `server/src/services/budgets.ts` (approx.), `packages/db/src/schema/budget_*`. Size: M.

### B4. Error families and "retry not before"

Classify provider failures into families (rate limit, auth, overloaded, quota, transient network, permanent) and store a `retryNotBefore` on the session. The scheduler then waits instead of hammering, and the card says why. Marshal already has fallback and queueing in the built-in agent; this makes the outcome legible and shared across adapters.
Source: adapter error mappers under `packages/adapters/*`. Size: M.

### B5. Wake admission as a pure function

Paperclip funnels every reason to wake an agent (comment, schedule, blocker cleared, watchdog) through one deterministic function that answers "run now, queue, coalesce, or drop". Marshal already wakes sleepers on `@agent` or a trailing `?`. Pull that logic into one tested function before more triggers appear (webhooks, schedules, CI failures).
Source: wake logic inside `server/src/services/heartbeat.ts` (do not copy the file, only the idea). Size: M.

### B6. Silent-run watchdog that informs and never kills

Detect a run with no output for a long time, raise one **fingerprint-deduplicated** notice, and stop there. A person decides. Marshal has awake limits and idle detection; add the dedupe fingerprint and the "never cancel automatically" rule.
Size: S.

### B7. Structured blocked state

A blocked card carries `{ owner, action }`: who must act (you, an agent, an outside system) and what they must do. The board can then sort "needs me" from "waiting on CI". This is the seed of a decisions desk on Home.
Source: blocker fields in `packages/shared` issue types. Size: M.

### B8. Review stages with a required comment

Paperclip lets a status change require a review stage and a comment (a "comment-required backstop" so an agent cannot close a card silently). Marshal has the required-checklist gate; add a **required comment** to agent-driven moves to Done or Review. Size: S.

## C. Later: match Phase 12 and 13

| # | Idea | Note | Size |
|---|---|---|---|
| C1 | **Portable, secrets-free export/import.** A folder or ZIP of markdown plus a sidecar for machine-specific values, with a dry-run preview and pinned external references. | Paperclip's "Agent Companies" spec is markdown-first. Take the *secrets-free sidecar* and *dry-run preview*, skip the company schema. Fits the planned export/backup. | L |
| C2 | **SKILL.md trust tiers.** Directory per skill, per-file hashes, commit pinning, block scripts from external sources, a policy per project. | Lands on the skills tables that already exist. Take the hashes and "no external scripts" rule first. | M |
| C3 | **Webhook and API triggers for schedules.** Beyond cron. | Marshal already receives GitHub and CI webhooks; expose one generic trigger. | S |
| C4 | **`low_trust_review` preset.** A named mode for untrusted repos that fails closed unless the run is sandboxed and isolated. | Marshal already has the permission gate and worktrees; a preset is a bundle of settings. | S |
| C5 | **Per-run token with `jti`.** A short-lived token per session with an id, so one token can be revoked. | Paperclip's 48 h HS256 token without `jti` is what *not* to copy. Marshal's per-card MCP server can use this. | S |
| C6 | **Session rotation with handoff note.** Rotate a long session by runs, tokens or age and carry a summary. | Marshal has a handoff route; wire it to a policy. | M |
| C7 | **A `doctor` command with typed checks.** Each check has an id, status and fix hint. | Marshal has the agent scan and health tile; unify them behind one CLI and API. | S |
| C8 | **More agents through one ACP engine.** | Qwen, Kimi and MiniMax already speak ACP and Marshal detects them. Reuse the Gemini ACP adapter so they run, not only test. Do this before any new adapter type. | M |

## D. Interface ideas (no backend needed)

- **Contextual feedback rules**, written down: no toast for state already on screen; refresh silently after a stale outcome; cancellation is neutral, not red.
- **Task thread details:** paused takeover that keeps the composer draft; queued messages with Interrupt and Cancel; a Stop menu with *Stop run*, *Stop and cancel*, *Stop and done*; pending-question cards above the composer.
- **"Needs me" desk** on Home built from B7.
- **Mobile:** bottom navigation that hides on scroll, swipe to archive. Relevant to the Android shell.
- **Storybook stories and pixel baselines** for the shared UI package.

## E. Do not copy

- Bypass-by-default permission flags and `approve-all`. Marshal's permission gate is a selling point.
- A 30,000-line run engine or 13,000-line service. Hold a file-size lint.
- Local secret fingerprints as unsalted SHA-256, or a master key next to the database. Keep the OS keychain.
- Process-local live events with no replay. Marshal's replay is better.
- The company metaphor, org chart, multi-company tenancy.
- A default-on telemetry setting.
- Two parallel run stacks. Paperclip's legacy adapters and its Rust runner both exist; Marshal should keep one path.
- Docs that describe features the code does not have. See A9.

## F. Suggested order

1. **A1 to A9** as one hardening pass. Small, independent, and each ends in a test. Together they are the biggest reliability gain per line of code.
2. **The ship-path web wiring** for the roughly 17 daemon routes with no caller (`/merge`, `/pull-request`, `/review`, `/findings*`, `/handoff`, `/local-ci`, `/v1/audit*`). Paperclip does not build these, so no idea is needed from it; it is listed here because it is the largest gap the comparison exposed.
3. **B3 budget incidents and B4 error families**, since they reuse the alert channels already built.
4. **B1 and B2** inside Phase 11, with B1's approval hash designed before the first plugin exists.
5. **C1 to C8** in Phases 12 and 13.
6. **D** items opportunistically with the screen they touch.

## G. Open questions for the owner

1. Does Phase 11 ship plugins at all, or only an MCP manager plus skills? B1 and C2 are useful without B2.
2. Is the ship path meant to be Marshal's headline against tools like Paperclip? If yes, wire it before Phase 11.
3. Should unknown cost (A2) count against limits as zero, as a pessimistic estimate, or block? Paperclip counts subscription usage as zero, which makes its budgets meaningless for subscription users.
4. Is a per-card budget scope wanted? Neither product has it.

## H. What was not checked

- Paperclip's cloud sandbox plugins, the managed Cloud tenancy, ClipHub and the Rust runner were read only at the design-document level.
- No Paperclip test suite, build or server was run, so behaviour claims come from code reading.
- File and line references marked "(approx.)" name the area the agents read, not a verified line; open the directory before relying on it.
