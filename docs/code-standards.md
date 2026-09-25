# Code standards

This document says how we write code in the Marshal repo. It applies to people and AI agents equally. The goal is code that is easy to read, easy to change, safe, and light.

When a rule here conflicts with a habit, follow the rule. When a rule seems wrong for a case, raise it and update this document instead of quietly breaking it.

---

## 1. Repository layout

This is the folder overview. Every planned file, and what it is for, is listed in `project-structure.md`. Adding, moving, renaming, or deleting a file means updating that document and logging the change in `progress-tracker.md`.

```
marshal/
  package.json               root scripts, see development.md
  pnpm-workspace.yaml        workspace packages
  daemon/                    Go module (package.json holds scripts only)
    cmd/
      marshald/              daemon entry point
      marshal/               command line tool entry point
    internal/                all daemon modules (see architecture.md)
      api/
      events/
      projects/
      chats/
      accounts/
      dashboard/
      session/
      harness/
      agents/
        acp/
        pty/
        builtin/
      providers/
      gitx/
      integrator/
      ci/
      scheduler/
      mcpserver/
      memory/
      codemap/
      integrations/
        github/
        trello/
        gcal/
        gmail/
        telegram/
        discord/
      notify/
      security/
        permissions/
        secrets/
        audit/
        keychain/
      remote/
      preview/
      store/
        migrations/
        queries/
      budgets/
    testdata/                fixture repos and golden files
  apps/
    web/                     SolidJS app, used by desktop and web UI
      src/
        app/                 app shell, routes, top bar, navigation
        layouts/             desktop, tablet, and phone layouts
        views/               one folder per view
          home/              dashboard
          chats/             chat list and chat
          agents/
          board/
          list/
          timeline/
          calendar/
          card/              card detail
          settings/
            profile/
            projects/
            roles/
            providers/
            schedules/
            integrations/
        onboarding/          first-run screens
        features/            feature logic shared across views
          tutorial/          the Home tour
          projects/          create, rename, remove
          chats/             create, rename, archive, delete
        stores/              client state
        lib/                 small helpers
    desktop/                 Tauri v2 shell
      src-tauri/
  packages/
    ui/                      component library, see ui-registry.md
    tokens/                  design tokens, see ui-tokens.md
    protocol/                generated API types from Go
  tools/
    stub-agent/              fake ACP agent for tests
    budgets/                 budget measurement scripts
  docs/                      these documents
```

Rules:

- **Daemon code stays in `internal/`.** Nothing outside the daemon imports it.
- **One module, one folder.** A module does not read another module's tables. It calls the other module's interface or listens to its events.
- **UI components live in `packages/ui`.** Views in `apps/web` compose them. Views do not define reusable components.
- **Generated code is never edited by hand.** It lives in clearly named folders (`packages/protocol`, `store/queries`) and is rebuilt by a script.

---

## 2. General rules

- **Clear over clever.** Write the obvious version first.
- **Small functions, small files.** If a file passes about 400 lines, consider splitting it.
- **No dead code.** Delete it. Git remembers.
- **No commented-out code** in commits.
- **Comments explain why, not what.** The code says what.
- **Names say what things are.** Avoid short names except for loop indexes and very local values.
- **No magic numbers.** Put them in named constants, with units in the name, for example `idleTimeout` or `maxRounds`.
- **Every limit has a default and a setting.** Timeouts, sizes, rounds, and budgets are configurable.

---

## 3. Go standards

### 3.1 Tooling

- Go version: the latest stable release, pinned in `go.mod` and CI.
- Format with `gofmt` and `goimports`. CI fails on unformatted code.
- Lint with `golangci-lint` using the repo config.
- Run tests with the race detector in CI: `go test -race ./...`.

### 3.2 Style

- Follow Effective Go and the Go code review comments.
- Package names are short, lower case, one word: `session`, `gitx`, `notify`.
- Exported names only when another package needs them.
- **Interfaces belong to the consumer**, not the producer. Keep them small.
- **No global mutable state.** Pass dependencies in through constructors.
- **Every blocking call takes a `context.Context`** as its first argument, and respects cancellation.

### 3.3 Errors

- Return errors, do not panic. Panics are only for truly impossible states at start up.
- Wrap errors with context: `fmt.Errorf("start worktree for card %s: %w", cardID, err)`.
- Define sentinel errors or error types for cases callers must handle, such as `session.ErrResumeFailed`.
- Never swallow an error. If it is safe to ignore, say why in a comment.
- Errors shown to users are turned into plain messages at the API layer, following the copy rules in `ui-rules.md`.

### 3.4 Concurrency

- Every goroutine has a clear owner and a way to stop.
- Use `errgroup` for groups of related goroutines.
- Channels are owned by the sender, who closes them.
- Test for leaked goroutines with `goleak` in packages that start goroutines.
- Protect shared state with a mutex, or better, give it to one goroutine.

### 3.5 Logging

- Use `log/slog` with structured fields.
- Always include `project_id`, `card_id`, or `session_id` when relevant.
- Levels: `debug` for detail, `info` for normal events, `warn` for recoverable problems, `error` for failures needing attention.
- **Never log secrets**, tokens, full prompts, or full file contents. Log IDs and sizes instead.

### 3.6 Database

- Schema changes only through migrations in `store/migrations`. Forward only.
- Queries are written in SQL and generated into Go with sqlc. No hand-built SQL strings with user input.
- Keep transactions short. Never call a model or an outside service inside a transaction.

### 3.7 Processes and Git

- Start child processes through one helper that sets the working directory, environment, and process group, and always cleans up.
- Git is called through the `gitx` module only. Never shell out to Git from other modules.
- Never pass user or agent text into a shell. Pass arguments as a list.

---

## 4. TypeScript and SolidJS standards

### 4.1 Tooling

- TypeScript in strict mode. No `any`. Use `unknown` and narrow it.
- Format and lint with Biome using the repo config.
- Package manager: pnpm, pinned through the `packageManager` field and Corepack, with the lock file committed. Never use npm or yarn in this repo.
- Every command, including Go and Rust tasks, is started through a pnpm script. See `development.md`.
- Tests with Vitest and the Solid testing library. End-to-end tests with Playwright.

### 4.2 Style

- Components are function components in PascalCase files: `CardItem.tsx`.
- Other files use kebab-case: `use-board.ts`, `format-cost.ts`.
- One component per file, except tiny private helpers.
- Props are typed with an interface named after the component: `CardItemProps`.
- **Keep reactivity simple.** Use signals and stores. Do not copy props into signals. Do not destructure props, since it breaks reactivity in Solid.
- Side effects go in `createEffect` or event handlers, never in render.
- Data from the daemon comes through the shared API client and query layer, never through ad-hoc `fetch` calls.

### 4.3 Styling

- Use Tailwind classes that map to tokens from `packages/tokens`. **No raw hex colors, pixel values, or font names in components.**
- No inline `style` except for values that are truly dynamic, such as a drag position.
- Never use `text-transform: uppercase` or all-caps strings. See `ui-rules.md`.
- **Responsive by default.** Every view and component must work at phone, tablet, and desktop sizes, using the breakpoint tokens. A pull request that adds a view includes screenshots at all three sizes.

### 4.4 Performance

- Virtualize any list that can pass 100 items.
- Load heavy parts (terminal, diff highlighting, timeline) lazily.
- Only the open card subscribes to full output streams.
- Check bundle size in CI. A pull request that grows the main bundle by more than 20 KB gzipped needs a reason in its description.

---

## 5. Rust (Tauri shell)

- Keep the Rust shell thin: window management, daemon start, OS notifications, deep links, and updates.
- **No business logic in Rust.** If it is not about the window or the OS, it belongs in the daemon.
- Format with `rustfmt`, lint with `clippy`.

---

## 6. Testing

| Kind | Rule |
|---|---|
| Unit tests | Every package and component with logic has tests. Go tests are table-driven where it fits. |
| Integration tests | Use the stub agent and fixture repos. Never call real model APIs in CI. |
| Real agent tests | Run nightly against pinned CLI versions. |
| End-to-end tests | Cover the main flows: add project, start card, approve, review, merge. |
| Budget tests | Run on every pull request. Over budget fails the build. |

- New code comes with tests. Bug fixes come with a test that fails before the fix.
- Coverage floor: 70% for daemon modules, 60% for UI packages. Critical modules (`harness`, `security`, `integrator`, `session`) aim for 85%.
- Tests must be fast and reliable. A flaky test is a bug and gets fixed or removed within a week.
- Test names describe behavior: `TestSleepSkipsWorkingSessions`, `it("shows a banner in bypass mode")`.

---

## 7. Security rules

- No secrets in code, config files, tests, or logs. Use the keychain module.
- Treat all agent output, webhook bodies, integration data, imported skills, and vault content as untrusted input.
- Verify every webhook signature before reading its body.
- Every permission check happens in the harness, never only in the UI.
- New dependencies must pass the checks in `library-docs.md`.

---

## 8. Performance rules

- **No polling loops** unless the design in `architecture.md` calls for one, with a long interval and conditional requests.
- **Batch high-rate events** before sending them to clients.
- **Bound every buffer, queue, and cache**, with a documented size.
- **Large data goes to disk**, referenced by ID.
- Any change that affects a budget in `architecture.md` must include a before and after measurement in the pull request.

---

## 9. Git workflow

### 9.1 Branches

- `main` is always releasable.
- Branch names: `<type>/<short-description>`, for example `feat/sleep-reminders` or `fix/resume-windows`.
- Branches created by Marshal agents use `marshal/<card-id>-<short-title>`.

### 9.2 Commits

- Use Conventional Commits with lower case types: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `chore:`, `perf:`.
- The summary line is in the imperative, under 72 characters: `feat: add grouped sleep reminders`.
- Reference the task ID from `build-plan.md` in the body: `Task: 5.10`.

### 9.3 Pull requests

- One task or one clear change per pull request.
- The description says what changed, why, how it was tested, and any budget impact.
- CI must pass, including the code smell check. At least one review is needed, and reviewers use the checklist in section 12.5. For changes to `security`, `harness`, or `integrator`, two reviews are needed.
- Squash merge into `main`.

---

## 10. Writing and copy

- Use sentence case in docs, code comments, UI text, log messages, and error messages.
- Do not use all-caps words for emphasis anywhere. Standard acronyms (API, CI, MCP, PR, URL) are fine.
- UI copy follows `ui-rules.md`.

---

## 11. Rules for AI agents working in this repo

1. **Read first:** `project-overview.md`, `progress-tracker.md`, then the task in `build-plan.md`.
2. **Stay in scope.** Only change files the task needs. If you find something else that needs fixing, add a note to `progress-tracker.md` instead of fixing it.
3. **Check before creating.** Look in `ui-registry.md` before making a component, and in `library-docs.md` before adding a library.
4. **Never edit generated code.** Change the source and run the generator.
5. **Keep the structure true.** When you add, move, rename, or delete a file, update `project-structure.md` and add a row to the file changes log in `progress-tracker.md`.
6. **Never touch** migrations that have already been merged, CI secrets, or release config, unless the task says so.
7. **Run `pnpm check`** (format, lint, code smells, tests, budgets) before saying a task is done. Do not add new code smells. If your task touches code that already has a smell, you may fix it only if it is small and inside the task's scope. Otherwise, add a note to `progress-tracker.md`.
8. **Update the tracker** at the end: task status, what was done, and any open questions.
9. **Ask when unsure.** A short question is cheaper than a wrong change.

---

## 12. Code smells

A code smell is a sign that code, while it may work, will be hard to read, change, or test. We check for smells in our own code for the same reason Marshal checks for them in its users' code: they pile up quietly, and they are cheapest to fix right when they are written.

This section is both a rule and a reference. Tools catch most smells automatically. Reviewers catch the rest.

### 12.1 The families

We use the nine families from the catalog by Jerzyk and Madeyski (2023) at codesmells.org. The table says which ones matter for our languages and how each is caught.

| Family | Smells we watch for | How it is caught |
|---|---|---|
| Bloaters | Long functions, large files, long parameter lists, data clumps, primitive obsession | Tools, with the limits in 12.2 |
| Change preventers | Shotgun surgery (one change touching many modules), divergent change (one file changing for many unrelated reasons), deep callback nesting | Review. Nesting depth by tools. |
| Couplers | Feature envy, reaching into another module's internals, long message chains | Module rules in section 1, and review |
| Data dealers | Middle men, data passed through functions that never use it, global mutable data | Global state by tools, the rest by review |
| Dispensables | Dead code, duplicated code, speculative generality, lazy types, comments that explain unclear code instead of fixing it | Dead and duplicated code by tools, the rest by review |
| Functional abusers | Unexpected mutation, side effects a name does not suggest | Review |
| Lexical abusers | Magic numbers, mysterious names, misleading function names, outdated comments | Magic numbers by tools, the rest by review |
| Obfuscators | Complex boolean expressions, clever code, related code placed far apart | Cognitive complexity by tools, the rest by review |
| Object-oriented abusers | Repeated switches on the same value, similar types with different method names, unused inherited members | Review |

**Adapted to our languages:**
- **Go:** plain `for` loops are idiomatic and are not a smell. Type switches are fine in one place, but the same switch repeated across packages is a smell. Go has no inheritance, so inheritance smells do not apply.
- **TypeScript:** we mostly use functions and modules, not classes. If a class is used, inheritance smells apply.
- **Rust (Tauri shell):** Clippy's complexity and argument lints apply.

### 12.2 Limits

Warnings must be fixed or explained in the pull request. Blocking limits fail CI.

| Check | Warning | Blocking | Notes |
|---|---|---|---|
| Function length | Over 50 lines | Over 100 lines | Tests and table data are excluded |
| File length | Over 400 lines | Over 800 lines | Generated code is excluded |
| Parameters | Over 4 | Over 6 | Use an options struct or object instead |
| Nesting depth | Over 3 levels | Over 5 levels | Return early instead |
| Cognitive complexity | Over 15 | Over 25 | Per function |
| Duplicated code | 10 lines or more | 30 lines or more | Across the whole repo |
| Magic numbers | Any | Not blocking | Named constants, with units in the name |
| Boolean expressions | More than 3 operators | Not blocking | Pull parts into well-named variables or functions |
| Message chains | More than 3 calls | Not blocking | Builders and fluent APIs are fine |
| Global mutable state | Not allowed | Blocking | Go and TypeScript |
| Unused code, parameters, and exports | Not allowed | Blocking | |

### 12.3 Tools

| Language | Tool | Rules used |
|---|---|---|
| Go | `golangci-lint` | `funlen`, `gocognit`, `nestif`, `dupl`, `mnd`, `goconst`, `unparam`, `unused`, `gochecknoglobals`, `revive` (including argument limit and naming rules) |
| TypeScript | Biome | Complexity rules, including cognitive complexity, plus unused variable and import rules |
| TypeScript | `knip` | Unused files, exports, and dependencies |
| All | `jscpd` | Duplicated code across Go, TypeScript, and Rust |
| Rust | Clippy | `cognitive_complexity`, `too_many_arguments`, `too_many_lines` |

All of these run with `pnpm smells`, and as part of `pnpm check` and CI. Their config files live at the repo root and match the limits in 12.2.

### 12.4 New smells only

- CI fails only on **new** blocking smells in changed code, compared with `main`.
- If a smell ever has to be accepted, it goes in `.smells-baseline.json` with a reason and a linked task to fix it. The baseline starts empty, and should stay close to empty.
- Suppression comments in code (such as `//nolint`) need a reason on the same line, for example `//nolint:funlen // generated state table`.

### 12.5 Review checklist

Tools cannot judge meaning. Reviewers check:

- Do names say what things are and do? Does each function do what its name says, and nothing more?
- Are comments still true after this change?
- Does a single change spread across many modules? If yes, is there a missing abstraction?
- Does a function use another module's data more than its own?
- Is any data passed through layers that do not use it?
- Is anything added "for later" that nothing uses yet?
- Is there clever code where a standard approach exists?
- Is related code kept close together?

### 12.6 Refactoring

- Refactoring changes structure without changing behavior. Tests must pass before and after.
- Keep refactoring in its own commits, or its own pull request when large, so reviews stay clear.
- Fix smells in code you are changing. Note smells in code you are only passing through, in `progress-tracker.md`.
