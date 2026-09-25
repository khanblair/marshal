# Project overview

This is the first document anyone should read, whether a person or an AI agent working on Marshal. It explains what we are building, why, and how the rest of the docs fit together.

---

## What Marshal is

Marshal is a lightweight desktop app and background service for running AI coding agents on kanban boards.

- Each **project** (a repository) has one **board**.
- Each **card** on the board is one task.
- Each card has **one agent session** that lives as long as the card, through app restarts and PC reboots.
- Agents work in isolated **Git worktrees**, so many tasks can run at once without breaking each other.
- A background **daemon** runs the agents, watches their work, and moves cards by itself based on what is really happening: agent activity, pull requests, CI, and reviews.

Marshal works with the coding CLIs people already use (Claude Code, Codex, Gemini CLI, and others) and with a built-in agent powered by the user's own API keys (Anthropic, OpenAI, Gemini, DeepSeek, OpenRouter, and local models). It connects to GitHub, Trello, Google Calendar, Gmail, Telegram, Discord, and Obsidian.

## What we are trying to achieve

One developer should be able to run many coding agents at once, across several projects, and always know:

- what each agent is doing right now,
- what needs their attention,
- what is ready to merge,
- what it all costs,

without juggling terminals, losing context, or fighting merge conflicts, and without a heavy app slowing their machine down.

### The problems we solve

| Problem today | What Marshal does |
|---|---|
| Many terminals and no single view | One board per project, a home view across all projects |
| Context lost when a CLI restarts | One lasting session per card, resumed after restarts |
| Work hidden inside background worktrees | Live activity feed and live diffs per card |
| Painful merges between parallel agents | Early conflict warnings and a context-aware Integrator agent |
| Agents unaware of each other | Shared board state, file claims, and notes through an internal MCP server |
| Manual follow-up on CI, reviews, and trackers | Cards move by themselves, CI failures go back to the card's agent |
| Heavy tools | Go daemon, Tauri shell, strict performance budgets enforced in CI |

### Success criteria for v1

We will know v1 works when:

1. A user can run at least five cards in parallel on a 16 GB laptop, with the daemon idle under 50 MB and the UI under 150 MB.
2. A card's session survives closing the app and restarting the PC, with no lost context.
3. A card can go from idea to merged code (plan, code, review, CI, merge) with the user only approving at key points.
4. Two cards that touch the same files are flagged before merge, not at merge.
5. The user can see and act on everything from their phone over Tailscale.

## Who it is for

- **Solo developers** who want several agents working in parallel.
- **Small teams** who want a shared board where people and agents work side by side.
- **Power users** who want to automate recurring work with schedules, loops, and briefs.

## Design principles

When two options conflict, these decide.

1. **Lightweight first.** Every feature must fit the performance budgets in `architecture.md`.
2. **One card, one lasting session.** Sessions are resumed, never silently replaced.
3. **Visible, not hidden.** Everything an agent does is shown: activity, diffs, commands, and cost.
4. **Safe by default, powerful when asked.** Safe permission modes are the default. Powerful modes such as bypass exist, but must be turned on clearly.
5. **Strong models where mistakes are costly.** Review and merge use strong models by default.
6. **Editable, not hard coded.** Roles, templates, schedules, and views are editable. We ship good defaults.
7. **Local first.** Data lives on the user's machine in SQLite and plain files. No cloud is required.
8. **Bring your own tools.** Marshal works with the agents, models, and services people already use.
9. **Color is a signal.** The interface is quiet and neutral. Color is used to show status, not to decorate.

## Core concepts

| Concept | Meaning |
|---|---|
| **Project** | A repository or monorepo that Marshal manages. |
| **Board** | The kanban view of all cards in a project. Each project has exactly one board. |
| **Chat** | A conversation in a project with the Orchestrator, a role, or a card's agent. A project can have many chats. Cards they create go onto the project's one board. |
| **Card** | One task, with its role, agent, session, worktree, branch, checks, and status. |
| **Session** | The single, lasting conversation between a card and its agent. |
| **Role** | An editable template for how an agent behaves: instructions, model, thinking mode, permissions, skills, MCP servers, and limits. |
| **Agent** | The engine doing the work: a CLI agent or Marshal's built-in agent. |
| **Worktree** | An isolated working folder on its own branch, for one card. |
| **Daemon** | `marshald`, the background service that owns all state and work. |
| **Harness** | The control loop the daemon wraps around every agent. |
| **Brief** | A scheduled morning or evening summary across all projects. |
| **Memory** | Project knowledge stored as markdown files, compatible with Obsidian. |

## Feature areas

The full scope, with how and why for each feature, is in `marshal-product-scope.md`. In short:

- **Boards and cards:** one board per project with filters and swimlanes, cards that move by themselves, "Add a card" in Backlog, Planning, and Working, templates, dependencies, sub-cards, checklists, acceptance checks, comments with files and links, members, checkpoints, forks, race mode, and cost per card.
- **Sessions:** one lasting session per card, sleep and wake with reminders, restore after reboot.
- **Agents and models:** CLI agents through ACP or PTY, a built-in agent, many providers, model switching, thinking modes, and automatic fallback.
- **Roles and orchestration:** editable role templates (Orchestrator, Worker, Reviewer, Integrator, and more), plan first mode, handoffs, a stuck detector, and a scorecard.
- **Harness and awareness:** limits, retries, an internal MCP server, file claims, and notes between agents.
- **Memory:** project knowledge, auto lessons, a codebase map, session search, and an Obsidian-compatible vault.
- **Worktrees and merging:** live activity and diffs, early conflict warnings, sparse worktrees for monorepos, and the Integrator merge queue.
- **Safety:** permission modes (ask, auto-accept edits, plan only, full auto, bypass), permission profiles, a command blocklist, a secret scanner, and an audit log.
- **Testing and CI:** live preview, screenshot checks, local CI, and GitHub Actions monitoring with a fix loop.
- **Code quality:** code smell checks on every card's changes, with blocking findings fixed by the agent before review. We hold Marshal's own code to the same checks (see `code-standards.md`).
- **Automation:** scheduler, crons, loops, and morning and evening briefs.
- **Remote:** Tailscale built in, a mobile web UI, and Telegram and Discord control.
- **Views:** a home dashboard with analytics and recent activity, chats, agents, board, list, timeline, and calendar.
- **Projects and chats:** create, rename, edit, and remove projects. Many chats per project, with create, rename, archive, restore, and delete, while each project keeps one board.
- **Getting started:** four onboarding screens and a skippable tutorial tour on the Home dashboard.
- **Profile:** a profile avatar in the top right corner of every screen, opening the profile page in Settings.
- **Every screen size:** the full product works on desktop, tablet, and phone, so everything can be controlled remotely.
- **Ecosystem:** skills, skill import, MCP manager and health, and a plugin API.
- **Team:** shared boards, human handoff, and user roles.

## Out of scope for v1

Model router, protected files, diff risk score, dependency guard, test impact analysis, custom event hooks, calendar-aware quiet hours, Gmail reply drafting, and integrations with Linear, Jira, GitLab, Notion, Slack, and Sentry.

## Locked technical decisions

| Area | Decision |
|---|---|
| Daemon | Go, one binary named `marshald` |
| Command line tool | `marshal`, a thin client that talks to the daemon |
| Desktop shell | Tauri v2 |
| UI | SolidJS, TypeScript, Vite, Tailwind CSS |
| Database | SQLite in WAL mode |
| Remote | Tailscale through tsnet, with Funnel for webhooks only |
| Secrets | OS keychain |
| Memory | Plain markdown, Obsidian compatible |
| Package manager | pnpm workspaces, also used as the one script runner |

The reasons for each are in `architecture.md` and `library-docs.md`.

## The docs and how they fit together

| Doc | Answers | Read it when |
|---|---|---|
| `project-overview.md` | What are we building and why? | Always, first |
| `architecture.md` | How is the system built? | Before touching any module |
| `project-structure.md` | Which folders and files exist, and what each is for? | Before creating, moving, or deleting a file |
| `build-plan.md` | What do we build, in what order? | Before picking up a task |
| `code-standards.md` | How do we write code here? | Before writing any code |
| `development.md` | How do we run, test, and build Marshal? | When setting up, and before running anything |
| `library-docs.md` | Which libraries do we use, and how? | Before adding or using a library |
| `ui-tokens.md` | What are the design values? | Before styling anything |
| `ui-rules.md` | How should the UI look, read, and behave? | Before building any UI |
| `ui-registry.md` | Which components exist? | Before creating a new component |
| `progress-tracker.md` | Where are we right now? | At the start and end of every work session |

### Reading order for AI agents

1. `project-overview.md`
2. `progress-tracker.md`, to find the current phase and task
3. `build-plan.md`, for the task's details and done criteria
4. `architecture.md` and `project-structure.md`, for the modules and files the task touches
5. `code-standards.md` and `development.md`
6. For UI work: `ui-tokens.md`, `ui-rules.md`, then `ui-registry.md`
7. `library-docs.md`, when using or adding a library

At the end of every work session, update `progress-tracker.md`.

## Writing style for all docs and UI copy

- Use sentence case for headings, labels, buttons, and messages.
- Do not write words in all capital letters for emphasis. Use bold text instead. Standard acronyms such as API, CI, and MCP are fine.
- Use plain, simple words. Name things by what users understand, not by how the system is built.
