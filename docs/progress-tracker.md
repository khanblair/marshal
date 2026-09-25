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
| **Current phase** | Phase 0: Foundation |
| **Next milestone** | First card (end of Phase 1) |
| **Last updated** | 2026-09-24 |

---

## Phase status

| Phase | Status | Notes |
|---|---|---|
| 0. Foundation | In progress | Docs written, repo setup next |
| 1. First card | Not started | |
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
| 0.2 | Monorepo layout | Not started | | |
| 0.3 | Go module with `marshald` and `marshal` | Not started | | |
| 0.4 | SolidJS app with Vite and Tailwind | Not started | | |
| 0.5 | Tauri v2 shell | Not started | | |
| 0.6 | Tokens package | Not started | | Values in `ui-tokens.md` |
| 0.7 | Go to TypeScript type generation | Not started | | |
| 0.8 | CI pipeline | Not started | | |
| 0.9 | Budget harness | Not started | | |
| 0.10 | Fixture repos | Not started | | Needs a small repo and a monorepo |
| 0.11 | Stub agent | Not started | | |
| 0.12 | pnpm workspace and root scripts | Not started | | See `development.md` |
| 0.13 | Daemon dev mode | Not started | | |
| 0.14 | Release pipeline with signing and updater | Not started | | Needs Apple and Windows signing certificates |
| 0.15 | Code smell tooling for our own repo | Not started | | See `code-standards.md` section 12 |

Tasks for later phases are added here when their phase starts.

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

---

## Open questions

| ID | Question | Needed by | Status |
|---|---|---|---|
| Q1 | Which ACP SDK for Go, or do we write the JSON-RPC layer ourselves? | Task 1.6 | Open |
| Q2 | Best way to run Claude Code as a lasting session: ACP, streaming JSON mode, or PTY? | Task 1.8 | Open |
| Q3 | Which CLI versions do we pin as supported for launch? | Task 1.8 | Open |
| Q4 | Codebase map approach: tree-sitter with cgo, ctags, or tree-sitter in WebAssembly? | Task 7.9 | Open |
| Q5 | Is `modernc.org/sqlite` fast enough under real load, or do we need a cgo driver? | Task 1.1 | Open |
| Q6 | Windows user service: scheduled task or a different approach? | Task 1.3 | Open |
| Q7 | Are the first real projects monorepos? If yes, monorepo mode moves into Phase 1. | Task 1.5 | Open |
| Q8 | How much does tsnet add to download size, and does it fit the budget? | Task 9.1 | Open |
| Q9 | Who holds the Apple Developer and Windows code signing accounts? | Task 0.14 | Open |
| Q10 | Should the onboarding sample project be a real small repo we ship, or generated on first use? | Task 2.16 | Open |
| Q11 | Which languages get built-in smell checks at launch? Suggested: Go, TypeScript and JavaScript, and Python. Depends on Q4. | Task 5.12 | Open |
| Q12 | Default size limit per attachment? Suggested: 25 MB, editable in settings. | Task 10.12 | Open |
| Q13 | Should people be able to review agent ticks, for example "Confirm agent ticks" on required checklists? | Task 10.11 | Open |
| Q14 | Prototype: posting a comment, attaching a file, adding an item, and the phone layouts for these have not been clicked through yet. | Design review | Open |

---

## Session log

Newest entries first.

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
| | | | | No code files yet. Repo setup starts with task 0.2. |

---

## Doc changes log

Every change to a doc. Newest first.

| Date | Doc | Change |
|---|---|---|
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
