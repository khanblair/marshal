# Project structure

This document explains the shape of the Marshal repo: what each folder is for, and what the files are whose names do not explain themselves. It is the map to check before creating, moving, or deleting a file. The full file list is `git ls-files`; this document does not repeat it. The layout rules are in `code-standards.md` section 1.

---

## How to keep this document true

- **On every file change, check this document.** Edit it when a folder is added, moved, renamed, or removed, or when a file of a kind listed here changes (an entry point, a config home, a generated file, a file other code imports from). Always add a row to the file changes log in `progress-tracker.md`. A plain new file in an existing folder needs no edit here.
- **Verified against `git ls-files` on 2026-10-13.** A folder marked (planned) is not built yet. When real work shows a better split, change the plan here first.
- **Tests** sit next to the file they test: `session_test.go` next to a Go `session.go`, and `CardItem.test.tsx` next to `CardItem.tsx`. They are not listed here. Exceptions: `packages/tokens/test/` and `packages/protocol/test/` hold their package's tests; browser tests are `apps/web/e2e/*.spec.ts` (Playwright); `*.daemon.test.tsx` runs a screen against the in-memory fake daemon through a `daemon-*-store.ts` helper. Test helpers live in `daemon/internal/testutil/`, `daemon/testdata/`, and the `testing/` folders of `apps/web/src/` (never in a bundle).
- **Naming:** Go files use lower case with underscores. OS-specific Go files end in `_unix.go`, `_windows.go`, `_darwin.go`, or `_linux.go`. Component files use PascalCase. Other TypeScript files use kebab-case. In `packages/ui`, a file that starts with `_` is private and is left out of the public index.
- **No Go file may be named `test.go` or end in `_test.go` unless it holds tests.** Connection test code lives in files named `conntest.go`.
- **Generated files are never edited by hand.** `pnpm gen` rebuilds them, and `scripts/check-generated.mjs` fails when a second run changes the committed ones.

| Path | Made by | In Git |
|---|---|---|
| `daemon/internal/store/db/` | sqlc, from `store/queries/` and `store/migrations/` (`daemon/sqlc.yaml`) | yes |
| `packages/protocol/src/generated/` | tygo from `daemon/internal/protocol` (`daemon/tygo.yaml`), then `scripts/gen-protocol.mjs` adds the `<Type>Values` arrays | yes |
| `packages/ui/src/index.ts` | `packages/ui/scripts/gen-index.mjs` (re-exports every public export; skips tests and `_` files) | yes |
| `packages/tokens/dist/` | `packages/tokens/scripts/build-css.ts` (CSS variables and the Tailwind theme) | no |
| `apps/mobile/splash/theme.css`, `apps/desktop/src-tauri/splash/theme.css` | `apps/mobile/scripts/build-splash-theme.mjs` from the tokens (`pnpm --filter mobile theme`); a test fails on drift | yes |
| `daemon/testdata/golden/` | The Go tests, with `UPDATE_GOLDEN=1`; the TypeScript tests read them | yes |
| `daemon/internal/webui/dist/` | `scripts/copy-web-dist.mjs` copies `apps/web/dist` in; only `.gitkeep` is committed, so `go:embed` works on a fresh checkout | no |
| `apps/desktop/src-tauri/icons/` | `pnpm --filter desktop icons` | yes |
| `apps/desktop/src-tauri/binaries/` | `apps/desktop/scripts/bundle-sidecar.mjs` | no |
| `apps/mobile/src-tauri/gen/` | `tauri android init` | no |
| `daemon/internal/integrations/googleclient/files/client.json` (Marshal's own Google OAuth client) | The release build | no |
| `pnpm-lock.yaml`, `Cargo.lock` | pnpm, cargo | yes |

---

## 1. Root

```
marshal/
  install.sh                          First-time macOS installer for the latest public release (app-updates.md)
  package.json, pnpm-workspace.yaml   Root pnpm scripts (development.md section 4) and the workspace packages. tsconfig.base.json: shared TypeScript settings
  biome.json, .golangci.yml           Blocking lint and smell limits (TypeScript, Go)
  biome.warn.json, .golangci.warn.yml Soft limits (the warning levels), run by pnpm smells
  knip.json, .jscpd.json              Unused code check, duplicated code check
  .smells-baseline.json               Accepted smells, each with a reason
  .gitattributes                      Line endings; marks generated and vendored paths (design/)
  .mcp.json                           MCP server settings for AI agents working in this repo
  .claude/launch.json                 Dev server entries (web, daemon) for the preview tool
  .github/
    workflows/                        ci.yml (the fast release gate), ci-full.yml (macOS daemon tests and browser tests, not a release gate) and e2e-macos.yml (started by hand): the gates in development.md section 4. release.yml: build, sign, and publish installers and daemon packages. nightly-agent-smoke.yml: the real agent CLIs through one small card each; off until a repository variable turns it on, and needs a self-hosted Mac with the CLIs signed in (planned: pin the CLI versions)
  design/                             The original design prototype (.dc.html, store.js). Read-only reference
  .tools/, dist/                      Local Go tool binaries, and build output (both ignored by Git)
  README.md                           Project overview, installation, first task, and local development
```

---

## 2. Docs

```
docs/
  README.md      Index of all docs and the reading order. Start there; one .md file per topic
  archive/       Older progress-tracker entries, moved out to keep the live file short
  research/      Research notes (the Paperclip comparison and adoption plan). Not required reading
```

---

## 3. Daemon

```
daemon/                       Go module (go.mod pins the toolchain). package.json holds pnpm scripts only (no Node code)
  tools.go                    Keeps pinned libraries in go.mod until code imports them. Delete a line when its library is used
  .air.toml, sqlc.yaml, tygo.yaml   Reload settings for pnpm dev; generation settings (see the generated files table)
  third_party/certstore/      Local cgo-free stand-in for tailscale/certstore, wired by a replace line in go.mod. The daemon must build without cgo
  testdata/                   repos/ (small-repo and monorepo fixtures; tests copy them to temp folders, so they are never changed; they also feed the dev fixture), hooks/github/ (recorded webhook bodies for tools/hooks-replay), golden/ (written by Go tests, read by TypeScript tests)
  cmd/
    marshald/                 Daemon entry point. main.go: flags, config, start modules, and the dev fixture (--fixture prototype). modules.go builds and wires every internal module. mcp.go is the daemon binary's mcp mode: the stdio MCP server an agent runs. terminals.go registers the PTY adapter. The other files wire one feature each
    marshal/                  Command line tool: status, token, keys (provider API keys in the OS keychain), service (installs marshald, not marshal itself, as the login service), dev reset (only ever deletes the dev data folder)
  internal/                   All daemon modules (architecture.md). Nothing outside the daemon imports them. A module uses another through its interface or its events, never its tables
    # Wire, API, events, storage
    protocol/                 Wire types and their small rules: Timestamp is UTC, RFC 3339, milliseconds; ids are ULIDs. Generated into packages/protocol (architecture.md 11.5). conventions_test.go checks the package against these rules
    api/                      HTTP and WebSocket API under /v1, on loopback only. router.go and routes.go register routes (a route exists only when the service it needs does). routes_<area>.go is one file per area. stream*.go serves GET /v1/events. auth.go checks bearer tokens. tailnet.go defines the node interface, so the API builds and is tested without Tailscale. The other small files are middleware
    events/                   In-process publish and subscribe: sequence numbers, replay ring, topic filters, bounded queues that cut slow subscribers
    store/                    SQLite in WAL mode: one writer and a pool of readers. migrate.go runs the embedded goose migrations and checks they are forward-only
      migrations/, queries/   One numbered .sql file per change (each module adds its own file for its own tables); hand-written SQL by area, one file each
      db/                     Generated by sqlc. Do not edit
    history/, cardhistory/    The typed history index of card sessions, and the read routes over it (a card's chat and activity, paged)
      history/conversation.go   Bounded, ordered replay of a built-in card/chat transcript; conversation_test.go covers isolation and paging
    webui/                    Embeds the built web app so the daemon serves it itself (desktop and plain browsers use this)
    # Projects, cards, and people
    projects/                 Projects, boards, cards, labels, saved views, forks. Owns card state moves. Removing a project never deletes the repo folder
    cardpanel/                A card's checks, checklists, comments with attachments, and members. The agent may not tick a person-only item
    chats/, search/, dashboard/, diff/   A project's chats; search for the palette; Home (needs you, tiles, activity); a card's diff over internal/gitx
    accounts/, devices/       The person (profile, avatar, preferences, progress); paired clients and their pairing codes
    settings/, zone/          The daemon's own settings; the time zone the clock runs in
    roles/, quality/          Role templates; code-smell checks on a card's diff and each project's profile
    memory/, codemap/         Card notes, file claims, and note search; the ctags-based index of each project's symbols
    schedules/, briefs/, runctx/   Scheduled jobs and their cron; the morning and evening briefs; why a schedule is running
    # Agents and sessions
    session/                  One-shot review failure checks (oneshot_{internal,failure}_test.go), card/chat sessions, direct project folder reservations (direct_workspace.go), native subagent coordination, snapshots, send, stop, resume, logs, pause and sleep, terminal view
    agents/                   The Agent interface, events, and registry. Adapters below it:
      acp/, pty/, claude/, gemini/   Agent Client Protocol; a CLI in a pseudo-terminal (ring of the last 256 KiB); Claude Code over stream-json (its session ids are UUIDs, not Marshal's ULIDs); Gemini CLI over ACP
      codex/                  Codex app-server over stdio JSON-RPC: lifecycle, settings, approval/event routing, and MCP results
      claude/permission.go    Native Claude permission callbacks and control responses; permission_test.go exercises the subprocess round trip
      builtin/, catalog/, trust/   Marshal's own agent over a provider API; which agents are installed and tested (cached five minutes); marks a folder trusted in a CLI's settings
      builtin/resume_test.go    Refuse missing history and preserve tool replies across turns; cmd/marshald/builtin_history_test.go checks production reader wiring and internal/api/builtin_resume_test.go checks HTTP restart/resume
    agentprefs/               What the person set for each agent in Settings: a chosen program path and an off switch, one settings row each. The catalog reads it (architecture.md 19.7)
    harness/, security/, secrets/   The permission decision for each agent request; the blocklist, permission profile, and deploy rule; the secret scan of committed text
    providers/                Model provider APIs, rate limits, usage, pricing. Thinking modes are mapped per provider here and nowhere else
    mcpserver/, mcpattach/    The internal MCP server for one card's agent; gives a session that server and its starting context
    proc/, keyedlock/         The one place that starts child processes, never through a shell (stop is polite first, then the whole process tree); one lock per key
    gitx/                     Every module reaches Git only through this (Git 2.38 or newer): temporary-index snapshots, direct.go snapshot refs, diff_revisions.go saved diffs, plus legacy worktree/merge primitives
    # Shipping and CI
    github/, pullrequest/, review/   The forge client; a card's branch as a pull request; the Reviewer role over it
    integrator/               Direct task completion through review/checks (direct.go); legacy branch merge/recovery helpers retained for compatibility
    ci/, localci/, preview/   CI monitor and fix loop (the only writer of ci_runs); a project's workflow steps run locally; a card's live preview dev server
    # Connections and notices
    integrations/             Saved connections: github/, trello/, googleclient/ (Marshal's own OAuth client), googlecal/, gmailread/, googlefiles/ (Drive, Docs, Sheets, Slides)
    connectiontest/           The shared connection-test machinery: time limit, cooldown, saved result
    netwatch/, outbox/, resend/   Whether the internet is reachable, checked only after a failure; notices and briefs saved while it was down and sent when it is back; the Resend email client (architecture.md 19.5, 19.6)
    notify/, chatbot/, chatcmd/   Which notices reach a person and where; Telegram and Discord bots; control.go routes coding commands to the production services through cmd/marshald/chat_control.go; cmd/marshald/chat_watch.go restarts changed connections; conversations.go saves reply links and delivered revisions, forwards completed answers, and routes notices to the card conversation
    audit/, auditlog/         The append-only audit log, and reading it back
    tailnet/                  Joins the daemon to the person's Tailscale tailnet from inside the process
    # Platform and support
    platform/                 Per-OS pieces: data folders, token files, the single-instance lock, and the login service installers (launchd, systemd, schtasks)
    config/, buildinfo/       Settings from flags, MARSHAL_* variables, and defaults; the version the daemon, tool, and app share
    fixture/, sample/         Dev sample data, safe to load on every start (--fixture prototype: the api, web, and mobile projects made from testdata/repos); the sample repository that ships in the daemon
    testutil/                 Test-only helpers: golden files, fixture repos, isolated empty schema copies from real migrations, builders for tools/stub-agent, the terminal helper, and api/testdata/approvalcli
```

### Planned for later phases (not built yet)

Not in the tree yet: card commands in `cmd/marshal` (`marshal cards`: list, new, start, send) and `internal/budgets/` (RAM, CPU, and disk self-measurement; `tools/budgets` measures from outside today). `build-plan.md` and `backend-checklist.md` give the phase of each. Check `git ls-files` before adding anything, because most planned names were built under other names: `schedules` for `scheduler`, `webui` for `web`, `cardpanel` for `comments`, `devices` for pairing, and `routes_*.go` for `handlers_*.go`.

---

## 4. Web app

```
apps/web/                     SolidJS app
  vite.config.ts              Vite and Solid setup, proxy to the dev daemon
  vite/dev-token.ts           Dev server route that gives the dev token to this machine only
  vitest.config.ts            Unit and component tests (jsdom) and the coverage floor (code-standards.md section 6)
  playwright.config.ts        Browser tests at phone, tablet, and desktop sizes; starts the built dev daemon (scripts/e2e-daemon.mjs) and Vite
  e2e/                        Playwright specs (daemon-lifecycle.spec.ts stops and starts the real daemon, so it runs alone after the rest, and daemon-onboarding.spec.ts runs last; see playwright.config.ts). support/ has the sizes, themes, openApp, daemon API calls, temp Git repos; global-setup.ts stops the run when the Vite server on the test port is not the one made for it
  src/
    main.tsx                  App entry: boots the store, then the app
    app/                      The shell: AppRoot (top bar, sidebar or bottom navigation, the screen for each connection state), Workspace, filter bar, project list. keyboard.ts holds global shortcuts. dialogs/, notices/, palette/, toasts/
    views/                    One folder per screen, each with an XView.tsx entry: home, chats, agents, board, list, timeline, calendar, card (panel, terminal), integration, settings, google-export
    features/                 Logic shared across views: agents (picker rows), chat (thread parts), markdown, tutorial (the Home tour)
    onboarding/               First-run flow. commit-step.ts turns the finished draft into a project on the daemon
    data/                     The client data layer (architecture.md 11.6). index.ts createData; api-client.ts (one method per route); event-stream.ts (the one WebSocket: hello, seq, resync, reconnect); connection*.ts; sections.ts (the per-section switch of the cutover register); mappers/ (wire shapes to screen shapes); testing/ (fake fetch, fake WebSocket)
    sync/                     Mirrors the daemon into the store (architecture.md 11.6). index.ts startSync; syncer.ts is the shape of one section's sync; <area>.ts loads and applies, <area>-actions.ts sends changes to the daemon (through data/optimistic.ts); reservoir.ts keeps mock records of projects the daemon does not have out of sight
    mock/                     The prototype's store M (index.ts, also on window.M for the console and browser tests). Its actions/, seed/, and sim/ are deleted section by section as the daemon takes over (backend-checklist.md sections 2.2 and 2.5)
    platform/                 Platform adapter: web, desktop (Tauri), phone
    testing/                  Test helpers: fake-daemon.ts (an in-memory daemon that answers the real client and pushes events on the real stream), daemon-*-store.ts, fake-*.ts, builders from the golden files
    styles/app.css            Imports tokens and Tailwind
```

Planned, not built: `layouts/` (AppRoot picks the layout by breakpoint today), `stores/` (theme, layout, and tour state live in the mock store), `data/queries/` (the sync layer calls the API client directly), a `lib/` of formatters (the mock's `format.ts` holds them), and browser specs for the tour, card flow, checklists, comments, and members.

---

## 5. Desktop shell

The phone app is its own Tauri v2 project (`docs/mobile.md`). It does not embed the web app or the daemon: it opens the page the daemon serves over the tailnet, the way the desktop shell does.

```
apps/desktop/                 Tauri v2 shell. Scripts: dev, build (bundles the daemon sidecar first), icons
  scripts/bundle-sidecar.mjs  Builds the daemon and CLI for the host target and copies them to src-tauri/binaries/ as <name>-<target-triple>
  src-tauri/
    tauri.conf.json           App settings, sidecar (externalBin), updater, deep links. capabilities/ holds the allowed Tauri permissions; rust-toolchain.toml pins Rust; clippy.toml holds the smell limits
    splash/                   Shown until the daemon answers its health route; then main.rs sends the window to the daemon's own address, which serves the app (internal/webui). theme.css is generated
    src/                      main.rs (window setup), daemon.rs (check and start the daemon), notifications.rs, deeplink.rs (marshal:// links), updater.rs (app and daemon updates)
apps/mobile/
  scripts/                    setup.mjs (Rust Android targets, Android tools), patch-android.mjs (fits the generated Android project: cleartext, shared text, signing; build runs it after init), build-splash-theme.mjs
  splash/                     The first screen: the computer's address, or scan its QR code. address.js reads a typed address or scanned code (pure, tested)
  src-tauri/
    tauri.conf.json           Identifier, deep link scheme, minimum Android
    capabilities/             mobile.json (the first screen: camera, deep links); remote-daemon.json (the daemon's page: camera, haptics, deep links, on the tailnet only)
    src/lib.rs                The app: plugins and the mobile entry point
```

---

## 6. Component library

`packages/ui` is `@marshal/ui`. `ui-registry.md` lists every component and what it is for; this section gives only the folders. Components that are not built, were cut, or live in `apps/web` are tracked in `ui-registry.md` sections 10, 12, and 13.

```
packages/ui/
  scripts/gen-index.mjs       Writes src/index.ts (the logic is in index-source.mjs)
  src/
    index.ts                  Public exports. Generated, do not edit
    base/                     Buttons, inputs, menus, tables, skeletons, toasts, empty and error states, and small helpers (cx, table-sort, virtual-window)
    layout/                   Dialog, Sheet, NavItem, and the connection screens (ConnectionLost, OfflineBanner, SignIn)
    board/, card/, chat/, dashboard/, settings/   Parts shared by those screens (CiStatus, DiffStat, Composer, charts, SettingsPanel)
    icons/                    Icon (a Lucide wrapper with token sizes) and the status icons. lucide-icons.ts lists the Lucide icons the design uses and must match design/. _design-source.ts is a test helper that reads design/
```

Files that start with `_` (`_Details.tsx`, `_ScreenFrame.tsx`, `_ChartParts.tsx`, `_RetryCountdown.tsx`) are private to their folder. Screen-level components (`HomeView`, `ChatsView`, `AgentsView`, `ListView`, `TimelineView`, `CalendarView`, `ProfilePage`, and `OnboardingFlow`) live in `apps/web` (section 4), because they are screens, not reusable parts.

---

## 7. Tokens

```
packages/tokens/
  src/
    tokens.ts, themes.ts      Source of truth for every value in ui-tokens.md; the light and dark theme maps
    contrast.ts               The contrast rules as code
    contrast-exceptions.ts    The design-approved pairs that miss the targets, and why (CI fails on a new miss or a stale entry)
    fonts.css                 The typeface faces and their fallbacks
  scripts/                    build-css.ts makes dist/ (generated); check-contrast.ts runs the rules in CI and for custom themes
```

The typefaces come from the `@fontsource/atkinson-hyperlegible-next` and `@fontsource/atkinson-hyperlegible-mono` packages, imported by `src/fonts.css`, so nothing loads from a CDN and the app works offline.

---

## 8. Protocol

```
packages/protocol/
  src/                        index.ts exports; generated/ is made from Go by tygo (do not edit)
  test/                       golden.ts reads the daemon's golden files; the tests check each golden file and the <Type>Values arrays against the generated types and the Go lists
```

---

## 9. Tools

Each program is its own Go module, with no `go.work`, and each has a `package.json` with scripts only (`build`, `test`, `lint:go`, `format:go`, `format:check:go`).

```
tools/
  stub-agent/                 A fake ACP agent for tests. scenarios/ is embedded in the program as JSON (not YAML, to add no dependency): default, approve, fail-test, stuck-loop, resume (says its turn number, so tests can prove context survived a restart), smells
  budgets/                    Starts the daemon, lets it idle, and reports memory and processor use against architecture.md section 14. sample_<os>.go reads the numbers per OS
  hooks-replay/               Sends a recorded webhook from daemon/testdata/hooks to the dev daemon, optionally signed with a secret
```

---

## 10. Scripts

```
scripts/
  setup-tools.mjs, go-tool.mjs   Install the pinned Go tools into .tools/; run one the same way on every platform
  gen-protocol.mjs               Runs tygo, adds the <Type>Values arrays, then formats packages/protocol
  copy-web-dist.mjs              Copies apps/web/dist into daemon/internal/webui/dist before the daemon compiles
  check.mjs, check-generated.mjs Run everything CI runs, the light steps first; check the generated files
  smells.mjs, budgets.mjs, coverage.mjs   Smell tools and the file length check; web build size and daemon idle use; per-package Go coverage floors (code-standards.md)
  agent-smoke.mjs                Runs the real agent CLIs (or, with --stub, the stub agent) through one small card each on a throwaway daemon
  direct-workspace-smoke.mjs     Checks direct folder ownership, snapshots, completion, and safe deletion against the built daemon using only the stub agent
  agent-lifecycle-smoke.mjs      Opt-in real Claude/Codex lifecycle through the built daemon API: MCP, approve/deny, file effects, and conversation resume
  coding-task-smoke.mjs          Opt-in complete feature tasks in a temporary copy of the Go budget module, with local CI, actual model review, completion, and a saved report
  e2e-daemon.mjs                 Runs the built dev daemon for the browser tests, on port 47811 with a data folder of its own
```

There is no `gen.mjs`, `dev-reset.mjs`, or `build.mjs`: `pnpm gen` and `pnpm build` are chains of package scripts, and `marshal dev reset` does the dev reset in Go, so the folder rules live in one place.

App updates: `apps/web/src/features/updates/` owns the shared native update state and automatic notice; `views/settings/UpdatesSection.tsx` presents the update controls. Desktop `src-tauri/src/updater.rs` owns signed check/install/restart commands, scoped by `permissions/updates.toml`. Desktop `scripts/prepare-release.mjs` produces the release config and updater manifest, with fixture tests alongside it. See `app-updates.md`.

Desktop `src-tauri/entitlements.plist` grants the executable-memory exception needed by the secret scanner while retaining hardened runtime. `scripts/check-bundle.mjs` gates publication by executing the signed archive’s Go helpers and checking its isolated daemon and embedded web app.

## November 2026 runtime and mobile fixes

- `daemon/internal/agents/catalog/runtime_env.go`: bounded login PATH recovery and project tool directories; `runtime_env_test.go` covers fallback, caching, and tool order.
- `daemon/internal/localci/run_test.go`: GUI service PATH, explicit workflow PATH, and bounded error diagnostics.
- `daemon/internal/session/workspace_lock*.go`: cross-daemon project leases and process-exit regression coverage.
- `daemon/internal/preview/connect.go`, `internal/previewrelay/`: isolated live preview gateways, one-use bootstrap authorization, root-path HTTP/WebSocket forwarding, and security regression cases.
- `daemon/internal/tailnet/preview_exposure*.go`: additive private Serve exposure, safe cleanup, and embedded-node forwarding.
- `apps/web/src/features/chat/{AgentActivity.tsx,agent-activity*.ts}`: state-driven activity feedback.
- `apps/web/src/views/card/{details-collapsed*.ts,preview-link*.ts}`: device-specific disclosure preferences and local preview links. The old `BypassBanner.tsx` was removed.
- `apps/web/src/platform/native-system-bars*.ts`, `apps/mobile/src-tauri/src/system_bars.rs`, `native/system-bars/SystemBarsPlugin.kt`: narrow Android system-bar theme bridge. `patch-android.mjs` copies the Kotlin source into generated Android output.

### Release sequencing

- `.github/release.json` records the requested version. It remains pending until publication succeeds.
- `apps/desktop/scripts/release-plan.mjs` selects the exact successful main CI commit, skips stale/already published requests, and guards manual tag retries. Its fixture tests run in the fast CI gate and desktop test script.
- `.github/workflows/release.yml` receives CI completion events, builds both platforms, stages artifacts, and publishes them together. `.github/workflows/e2e-macos.yml` is an optional manual browser check; the browser tests run in `ci-full.yml`.

- `daemon/cmd/marshald/keychain{,_test}.go`: fixture-only credential isolation and mode guards; the shared keychain is wired through the core daemon modules. Browser startup assertions live in `apps/web/e2e/support/{global-setup,daemon-api}.ts`.
