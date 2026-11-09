# Development

This document explains how to set up Marshal on your machine, run it while developing, test it, and build it for release. It applies to people and AI agents.

We use **pnpm** as the package manager and as the one place where every command lives. Even Go and Rust tasks are started through pnpm scripts, so there is one way to do everything.

---

## 1. What you need installed

| Tool | Version | Why |
|---|---|---|
| Go | 1.27.1 (the `go` line in `daemon/go.mod`) | Daemon and command line tool |
| Node.js | Pinned in `package.json` `engines` (24 or newer) | UI build tools |
| pnpm | Pinned in `package.json` `packageManager` (11.4.0), enabled with Corepack | Package manager and script runner |
| Git | 2.38 or newer | Project detection, snapshots, and diffs |

For desktop development and builds, install Rust and the Tauri system prerequisites for your OS. Web and daemon development can run without those desktop tools.

Go tools are installed by one script, so everyone uses the same versions:

```sh
pnpm setup:tools
```

This installs `air` (reloads the daemon when Go files change), `sqlc`, `tygo`, and `golangci-lint` into a local `.tools/` folder that is ignored by Git, at the versions pinned in `scripts/setup-tools.mjs` and `library-docs.md`. They are built without cgo, so they need no C compiler. The race detector used by `pnpm test:daemon` does need one, which is already there on macOS, Linux, and the Windows CI runners.

### First-time setup

```sh
corepack enable
pnpm install
pnpm setup:tools
pnpm gen
```

`pnpm gen` runs every code generator: SQL queries, Go to TypeScript types, and design tokens. Run it again after changing SQL, API types, or tokens.

---

## 2. How the workspace is set up

The repo is a pnpm workspace. `pnpm-workspace.yaml` lists these packages:

| Package | Folder | What its scripts do |
|---|---|---|
| `daemon` | `daemon/` | Wraps Go commands: run, test, lint, build. It has a `package.json` with scripts only, no Node code. |
| `web` | `apps/web/` | The SolidJS app |
| `@marshal/ui` | `packages/ui/` | Component library |
| `@marshal/tokens` | `packages/tokens/` | Design tokens |
| `@marshal/protocol` | `packages/protocol/` | Generated API types |
| `stub-agent` | `tools/stub-agent/` | Fake agent for tests and development |
| `hooks-replay` | `tools/hooks-replay/` | Replays a recorded webhook through the daemon |
| `budgets` | `tools/budgets/` | Measures the built daemon's idle RAM and CPU |

`apps/*` and `tools/*` are workspace globs. The Tauri desktop shell is in `apps/desktop/`.

---

## 3. Running Marshal while developing

### 3.1 The everyday command

```sh
pnpm dev
```

This builds the stub agent first (the daemon looks for it next to itself, `dist/bin/stub-agent` beside `dist/bin/marshald`; `MARSHAL_STUB_AGENT` overrides the path), then runs two things in parallel with labeled output in one terminal:

1. **The daemon** in dev mode through `air`, so it restarts when you save a Go file. It starts with `--dev`, so it shows your own projects and nothing else. For the three prototype projects as well, run `MARSHAL_FIXTURE=prototype pnpm dev`.
2. **The Vite dev server** for the UI, which reloads the page as you edit.

Open the printed local address in your browser. Most UI work happens here, because it is the fastest loop.

**Gotcha:** `daemon/.air.toml` starts the daemon with `--agent real`, and a flag beats `MARSHAL_AGENT`. So `pnpm dev` and `pnpm dev:daemon` run real agents (section 3.6), not the stub agent that `--dev` uses by default.

### 3.2 With the desktop window

`pnpm desktop:dev` starts the Tauri shell in `apps/desktop/` (it needs Rust and the Tauri prerequisites for your OS). Most UI work still happens in a browser tab.

### 3.3 Only one part

`pnpm dev:daemon` runs only the daemon, with reload. `pnpm dev:web` runs only the Vite dev server, pointing at a daemon you started yourself. There is no `pnpm dev:stub`: the stub agent is a program, not a server. Build it with `pnpm --filter stub-agent build` and run `dist/bin/stub-agent` yourself when you want it on its own.

### 3.4 Dev mode

The daemon's `--dev` flag keeps development fully separate from a real Marshal install on the same machine.

| | Dev mode | Normal install |
|---|---|---|
| Data folder | `<data>-dev` | `<data>` |
| Port | 47801 | 47800 |
| Default agent | Stub agent | Real agents |
| Keychain entries | Stored under `marshal-dev` | Stored under `marshal` |
| Client auth | A dev token, accepted on localhost only | Paired tokens |
| Budget warnings | Shown in the daemon log | Shown in the app |

The Vite dev server forwards API and WebSocket requests to the dev daemon's port, so the browser talks to one address and there are no cross-origin issues.

The dev server also hands the browser the dev token, so there is nothing to paste. In the dev build only, the app asks `GET /__marshal/dev-token`, and `apps/web/vite/dev-token.ts` answers with the `dev-token` file in the dev daemon's data folder. It reads the file on every request, because `pnpm dev` starts Vite and the daemon at the same time, and answers 404 until the daemon has made it. It answers only requests from this machine, even with `--host`, and does not exist in a production build or in `vite preview`. The app keeps the token in memory and never stores it.

### 3.5 Settings through environment variables

| Variable | Default in dev | Purpose |
|---|---|---|
| `MARSHAL_DATA_DIR` | `<data>-dev` | Use another data folder |
| `MARSHAL_PORT` | 47801 | Use another port |
| `MARSHAL_AGENT` | `stub` | Set to `real` to use installed CLI agents |
| `MARSHAL_LOG_LEVEL` | `debug` | `debug`, `info`, `warn`, or `error` |
| `MARSHAL_NETWORK_PROBE` | `1.1.1.1:443,8.8.8.8:443` | Where the daemon connects to check the internet after a call failed (architecture.md 19.5). A comma-separated list of `host:port`, or `off` to turn the check off |
| `MARSHAL_FIXTURE` | none | Load a fixture on start (or pass `--fixture`). The only one is `prototype`. Dev mode only; a normal install ignores it. Safe on every start. `pnpm dev` does not pass it. |

`MARSHAL_FIXTURE=prototype` loads the three prototype projects (`daemon/internal/fixture/fixture.go`) as the projects `api`, `web`, and `mobile`, built from `daemon/testdata/repos`, which the daemon finds from the checkout it was built from, so run a daemon built from a checkout (`pnpm dev` does). It also writes the prototype's 29 cards (11 in `api`, 9 in `web`, 9 in `mobile`) with their numbers, states, labels, dates, pull requests, CI states, and needs-you reasons, and a session row for each of the 20 cards that has started.

- **Those sessions have no agent process, and a daemon with a fixture loaded does not restore sessions on start**, so nothing runs until a person starts a card themselves.
- If the fixture cannot load, the daemon logs a warning and starts without it.
- The project language and packages are what detection finds in those repositories (for example `JavaScript` for the two small ones), not the hand-picked labels of the mock data.

### 3.6 Using real agents and models

The `--dev` default is the stub agent, so development costs nothing and works offline. Real agents: `MARSHAL_AGENT=real` or `--agent real` (but see the `.air.toml` gotcha in section 3.1).

API keys for the built-in agent are stored in the OS keychain (the macOS Keychain, the Linux Secret Service, or the Windows Credential Manager), never in a file and never in the environment. Add one with `marshal keys`, which reads the key from standard input so it does not land in your shell history:

```sh
marshal keys set anthropic      # paste the key when asked, then press Enter
marshal keys list               # which providers have a key, each shown masked
marshal keys remove anthropic   # delete one
```

The list shows the key masked (`sk-ant-…4f2a`), never the key itself. A provider that runs on this machine (Ollama, LM Studio) stores a server address instead, shown as it is. Add `--dev` to use the dev daemon's own keychain entries, kept apart from a normal install's. The onboarding and settings screens save into the same keychain through the daemon's provider routes. Real-agent runs that need a signed-in CLI still use that CLI's own sign-in.

### 3.7 Webhooks in development

- **Replay recorded webhooks.** Fastest, and needs no internet setup.

  ```sh
  pnpm hooks:replay github ci-failed
  pnpm hooks:replay trello card-moved
  ```

  Recorded bodies live in `daemon/testdata/hooks/`. Add new ones when you add new event types.

- **Real webhooks through Funnel.** Start the daemon with `--tailnet --funnel` (or `MARSHAL_TAILNET=1 MARSHAL_FUNNEL=1`), sign in to Tailscale from Settings, Remote control, and allow Funnel for the tailnet in the Tailscale admin console. Only `/hooks/*` is public, and every request is still signed.

### 3.7a Connecting Google Calendar, Gmail, Drive, Docs, Sheets, and Slides

Marshal reads your Google Calendar (and, if you want, a Gmail label) read-only, makes files in Google Drive, Docs, Sheets, and Slides, and reads a doc, sheet, or deck from a link. Each of the six is its own row with its own consent, and the person pastes nothing. That works when the build has **Marshal's own Google client**. Without one, Settings, Integrations, Google Calendar shows the setup steps and a form for the id and secret of a **Desktop app** OAuth client the person makes; **Save and grant access** does both, and a client the person saved is always the one used.

**Making Marshal's own Google client (once, by whoever publishes Marshal)**

1. In the Google Cloud console, make a project for Marshal and turn on six APIs: Calendar, Gmail, Drive, Docs, Sheets, and Slides. Marshal's tests say which one is off.
2. Under **OAuth consent screen**, choose **External**. Give it Marshal's name, a homepage, a privacy policy address, and your contact address. On the **Data access** page add six scopes: `calendar.readonly`, `gmail.readonly`, `drive.file`, `documents.readonly`, `spreadsheets.readonly`, and `presentations.readonly`.
3. Under **Credentials**, make an **OAuth client ID** of type **Desktop app**. Google accepts any `http://127.0.0.1:<port>` return address for that type, so the dev daemon (47801) and the installed app (47800) both work. A Desktop app's secret is not confidential to Google, which is why it may ship inside the app.
4. Hand the client to the build:
   - **Release builds:** add the repository secrets `MARSHAL_GOOGLE_CLIENT_ID` and `MARSHAL_GOOGLE_CLIENT_SECRET`. `release.yml` writes them to `daemon/internal/integrations/googleclient/files/client.json` (not in git) before the daemon is compiled.
   - **Your own dev daemon:** set the same two variables, or put `{"clientId": "...", "clientSecret": "..."}` in `client.json` before building.

**Who can use it**

- Publishing status **Testing**: only the test users you list can connect, and Google expires their access after about a week (Marshal then says "Reconnect Google Calendar").
- **In production**: anyone can connect. Until Google **verifies** the app, each person sees an "unverified app" warning, and Google caps unverified apps at 100 users. `calendar.readonly`, `documents.readonly`, `spreadsheets.readonly`, and `presentations.readonly` are sensitive scopes: verification asks for the privacy policy, the homepage, and a short demo video of the consent and its use, but no yearly security assessment.
- `drive.file` is non-sensitive: Marshal makes files and sees only the ones it made, and never edits or deletes a file you already had.
- `gmail.readonly` is restricted: letting other people use it through Marshal's client needs Google's yearly security assessment. Until then Gmail works for you and your test users only; anyone else can use Gmail with a client of their own.

**Using it**

- Grant access in your own browser on the computer that runs Marshal, because Google sends the browser back to that computer's own daemon. A phone cannot finish it, and says so; it shows the events once connected.
- Calendar asks for `calendar.readonly` only. **Gmail** is a separate row with its own **Grant access** (`gmail.readonly` alone). **Google Drive** asks for `drive.file`; **Docs**, **Sheets**, and **Slides** each ask for their own `.readonly` scope and `drive.file`. If a person unticks a box on Google's page, Marshal refuses the grant, stores nothing, and asks them to connect again with every box ticked.
- Marshal puts what it makes in one Drive folder, named "Marshal" until the person names another on the Google Drive row. All four file connections share it, whether or not Drive itself is connected, and Marshal makes it when it saves the first file. Their tests never make a file or a folder.
- Once connected, the Calendar row lists every calendar you have, with a tick for each one Marshal reads. Until you choose, the ticks follow Google Calendar's own.
- The calendar feeds the calendar view, Home's "Coming up today", the morning and evening briefs, schedules with the **Event** trigger, and **Stay quiet during calendar events** under Settings, Alerts. An event marked available, and a working-location marker, does not count as busy. Events are read at most once a minute; if Google cannot be reached, the last events read are shown and marked as old.
- **Gmail's row** reads connected once it has a label and its own grant, in either order, and asks again while you grant access in the other tab.
- **Disconnect** removes Marshal's stored access. When it is the last Google connection holding a token, Marshal also asks Google to end the access; while another is still connected it leaves Google's grant alone, because Google ends every scope when one token is revoked. If Google cannot be reached, the access stays until it is removed at myaccount.google.com/permissions.

### 3.8 Testing on phones and tablets

- **In the browser:** use the browser's device mode for quick checks at phone and tablet sizes.
- **On a real device:** a daemon listens on `127.0.0.1` only until a phone has a way in. Settings, Remote control says which way is in place and what is missing, with your computer's real name and port.
  - If Tailscale already runs on your Mac, run the command that page shows (`tailscale serve --bg --http=47800 http://127.0.0.1:47801` for a dev daemon), press **Check again**, and type the address it shows into the phone app. The phone needs the Tailscale app, signed in to the same account.
  - Otherwise start the daemon on the tailnet itself: `MARSHAL_TAILNET=1 pnpm dev` (add `MARSHAL_TAILNET_HOSTNAME=marshal-dev` to name the node). The flags of the compound `dev` script do not reach the daemon, so use the variables; the `daemon` entry in `.claude/launch.json` already does. The node appears as a second device on your tailnet, and Remote control shows its sign-in link the first time.
  - The phone app lets the daemon's page use the camera and haptics on port 47800 only, which is why the Serve command uses 47800.
- **In tests:** `pnpm test:e2e` runs every view at phone, tablet, and desktop sizes, in both themes, against the built dev daemon. Run `pnpm build` first: Playwright starts `dist/bin/marshald` through `scripts/e2e-daemon.mjs` (port 47811, a throwaway data folder, the `prototype` fixture) and a Vite server beside it. It never uses the dev daemon on port 47801 or your dev data folder.

### 3.9 Resetting dev data

```sh
pnpm dev:reset
```

This runs `marshal dev reset`. It refuses to run while the dev daemon is up (stop it first), asks you to confirm, then deletes the dev data folder, including dev worktrees. It only deletes a folder whose name ends in `-dev` and that sits well below the home folder, so it can never touch the normal install. Pass `--yes` after `--` to skip the question.

### 3.10 Installing as a login service

For everyday use outside this dev loop, `marshald` runs as a per-user service that starts at login (architecture.md section 2, "Daemon as a user service"):

```sh
marshal service install     # installs marshald next to marshal's own program
marshal service status      # reports whether it is installed and running
marshal service uninstall   # stops and removes it
```

Add `--dev` to manage the dev daemon's own, separate service, so it can run beside a normal install. `install` finds `marshald` next to wherever `marshal` is running from, so both programs need to sit together, as they do in a built release. These commands register a real login item on your account (a launchd agent, a systemd user unit, or a Windows scheduled task), so no automated test runs them; only their argument and file generation and their status parsing are tested.

---

## 4. Everyday commands

| Command | What it does |
|---|---|
| `pnpm dev` | Build the stub agent, then daemon and web UI with reload (dev daemon, your own data; add `MARSHAL_FIXTURE=prototype` for the prototype projects) |
| `pnpm dev:daemon` | Only the daemon, with reload |
| `pnpm dev:web` | Only the Vite dev server |
| `pnpm gen` | Run all code generators (tokens, the UI index, the sqlc queries, the protocol types) |
| `pnpm lint` | Lint Go and TypeScript (no Rust yet: the Tauri shell comes after Phase 2) |
| `pnpm format` | Format all code |
| `pnpm format:check` | Check formatting without changing anything |
| `pnpm typecheck` | Type check every workspace that has a `typecheck` script |
| `pnpm test` | All unit and integration tests, one workspace at a time |
| `pnpm test:daemon` | Go tests with the race detector, for the daemon (`pnpm test` also runs the Go tools' tests) |
| `pnpm test:web` | UI unit and component tests |
| `pnpm test:e2e` | End-to-end tests in the browser against the built dev daemon (run `pnpm build` first) |
| `node scripts/direct-workspace-smoke.mjs` | When requested: checks the built daemon with the free stub agent and a temporary sample project. Verifies one editor, direct completion, preserved source/index/HEAD, frozen diffs, safe deletion, and no extra worktrees. No browser or real model calls; run `pnpm build` first. |
| `node scripts/agent-lifecycle-smoke.mjs --real --agents claude,codex --model-codex default` | Only when the owner requests a real account check: drives the installed CLIs through the built daemon's actual card/session API, MCP, approval/denial, file effects, and stop/resume. Build the daemon first. Details and limits: `agent-sessions.md`. |
| `node scripts/coding-task-smoke.mjs --real` | Only when requested: copies the Go budget module and test dependencies, proves the baseline, then runs separate Claude/Codex feature cards through implementation, full local tests, model review, and completion. Writes `dist/coding-task-smoke.json` and removes its temporary project. Build the daemon first. |
| `pnpm budgets` | Measure the web build size, then the built daemon's idle RAM and CPU (run `pnpm build` first). `MARSHAL_BUDGET_IDLE` sets how long the daemon idles: 10 seconds by default, 60 in CI. |
| `pnpm smells` | Code smell checks on changed code (see `code-standards.md` section 12) |
| `pnpm smells:all` | Code smell checks on the whole repo |
| `pnpm check` | The heavy full gate; CI runs it on every pull request, so run it locally only when you decide to. Everything CI's non-browser jobs run, in order: `gen`, format check, lint (including Go), type check, code smells, token contrast, all unit tests (Go with `-race`), the build (daemon, stub agent, web), and the budgets |
| `pnpm build` | Build for your OS: the daemon and the command line tool into `dist/bin/`, the stub agent, and the web UI |
| `pnpm hooks:replay <provider> <case>` | Replay a recorded webhook through the daemon |

**CI runs these checks on every pull request, so you do not have to run `pnpm check` first.** It is heavy: it uses every core for several minutes. Run it on your own machine only when you want the answer before you push. **AI agents never run it on the owner's machine unless the owner asks, and they start no process to check work by hand (no daemon, no smoke script, no dev server, no browser). They verify with the fast checks only: the typecheck of the workspace they changed, `biome check` on the files they changed, `golangci-lint` on the packages they changed, and the fast unit tests of the files they changed (`go test -race -run <TestName> ./internal/<pkg>/`, vitest with file filters and `--maxWorkers=4`).

**Where the full gate runs.** `pnpm check` and CI (`.github/workflows/ci.yml`) run the same steps, split differently:

The required macOS check and the separate macOS browser workflow use the standard `macos-15-intel` runner. On 2026-11-03 GitHub could not allocate the ARM runner selected by `macos-latest`; no test steps started. Tool-cache keys include both operating system and processor architecture, since the cached Go tools are executable binaries. Supported labels are listed in [GitHub's runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).

| | `pnpm check` (your machine, on request) | GitHub CI |
|---|---|---|
| Light steps (generated files, format, lint, type check, code smells, token contrast) | First, one after another, on your OS | First, in the `lint` job on Linux only, so a small mistake stops the run before the slow jobs |
| Generated files | Runs `pnpm gen` a second time and fails if it changes anything (works on an uncommitted tree) | Runs `pnpm gen`, then fails if the generated files differ from what Git holds |
| Tests, coverage floors, build, budgets | After the light steps, on your OS. The budgets watch the idle daemon for 10 seconds. | In the `check` job on macOS only (Linux and Windows are off while Marshal is macOS first; add them to the matrix in `ci.yml` to turn them back on), once `lint` passes. The budgets watch for 60 seconds. |
| Browser suite (`pnpm test:e2e`) | Not part of `pnpm check` | The `e2e` job on Linux after `check`; then `e2e-macos.yml` on macOS after `ci` passes on `main` (never on a pull request) |
| Real agent smoke test | Not part of `pnpm check` | `nightly-agent-smoke.yml`, off until you turn it on (see its header) |

Two commands this section used to list do not exist yet: `pnpm test:agents` (a real-agent smoke test; no real agent is started by any automated test) and `pnpm marshal <command>` (run the built CLI directly, as `dist/bin/marshal status --dev`).

---

## 5. Testing notes

- **Integration tests use the stub agent and fixture repos.** They never call real model APIs.
- **macOS tool setup is cached before tests run.** A later test failure no longer throws away successfully installed Go tools.
- **Fixture databases use a migrated empty schema.** `testutil.OpenStore` runs the real migrations once per test process, closes SQLite to checkpoint its WAL, and copies that empty image into a separate private file for each test. Existing files use the normal open path, so restart and persistence checks still read real saved data. The image is never cached between test binaries or CI runs. Store migration tests continue using `store.Open` directly. Database operations retain their ten second test limit; schema setup has a separate one minute limit.
- **Protocol fixture programs stay small.** Approval HTTP tests build a standalone Claude protocol fixture instead of launching the complete API test binary as a child. This keeps test binary initialization and coverage machinery out of the agent startup deadline. The production Claude adapter, permission decisions, file effects, and stored history remain under test.
- **The stub agent can be scripted** to act out cases: ask for approval, fail a test, get stuck in a loop, resume after a restart. Scripts live in `tools/stub-agent/scenarios/`.
- **Fixture repos** live in `daemon/testdata/repos/`: a small repo and a monorepo. Tests copy them into a temp folder, so fixtures are never changed.
- **Real agent tests are opt-in.** Automatic tests use fake programs and the stub agent. A real session uses the owner's paid account, so it requires their explicit request; the lifecycle script below keeps it on Marshal's production path.
- **The real-mode catalog check is read-only.** `dist/bin/marshald --dev --agent real --data-dir "$(mktemp -d)" --port <free port>` asks each installed CLI for its `--version` and nothing else. `GET /v1/agents` with that daemon's dev token shows what it found. Create/start real cards only when the owner requests a real agent check.

### The one real-agent check, done by hand

Real sessions use the owner's account, so they are excluded from automatic tests and run only when the owner explicitly requests them. The opt-in `agent-lifecycle-smoke.mjs` check above exercises Claude and Codex through Marshal itself. For a manual single-card check:

1. Build: `pnpm build`.
2. Start a throwaway daemon with real agents, a data folder of its own, and a free port that is not 47801:
   `dist/bin/marshald --dev --agent real --data-dir "$(mktemp -d)" --port 47899`
3. Read the token from `<data-dir>/dev-token` (do not print it) and call `POST /v1/projects` for a throwaway repository, then create a card, start it, and send one short message.
4. The expected answer is a normal reply from Claude Code, the card moving to working and then to needs or done, use of the original project folder, and an entry in `<data-dir>/logs/sessions/<session id>/`.
5. Undo: stop the daemon (SIGTERM), then delete the throwaway data folder. Nothing of the owner's dev daemon (port 47801, `Marshal-dev`) is touched, because the flag gave this daemon its own folder and port.

---

## 6. Building for production

Production builds happen in CI on every release. You can also build for your own OS locally to test an installer.

### 6.1 What gets built

1. **The UI** is built by Vite into static files.
2. **The daemon and command line tool** are built by Go for every target, with the UI files embedded, so the daemon can serve the web UI to phones even when no desktop app is open.
3. **The desktop app** is built by Tauri, with the UI and the daemon included as a sidecar binary.

### 6.2 Targets

| Platform | Daemon builds | Desktop installers |
|---|---|---|
| macOS (Apple Silicon and Intel) | `marshald`, `marshal` | `.dmg` |
| Windows (x64) | `marshald.exe`, `marshal.exe` | `.msi` and `.exe` |
| Linux (x64 and ARM64) | `marshald`, `marshal` | `.AppImage`, `.deb`, `.rpm` |

- **Go builds cross-compile from one machine**, since the daemon uses no cgo.
- **Tauri builds run on each OS**, so CI uses a macOS, a Windows, and a Linux runner.

### 6.3 Local production builds

| Command | Builds |
|---|---|
| `pnpm build` | The UI, the daemon and the command line tool (into `dist/bin/`), and the stub agent, for your OS |

The per-target commands this section used to list (`pnpm build:daemon`, `pnpm build:daemon:all`, `pnpm build:desktop`) do not exist yet: they arrive with the release pipeline. Build for another platform by calling Go directly, through the tool runner, for example
`node scripts/go-tool.mjs go build -o dist/bin/ ./cmd/...` from `daemon/` with `GOOS` and `GOARCH` set (the daemon uses no cgo, so it cross-compiles).

Output goes to `dist/`, which is ignored by Git.

### 6.4 Signing

Unsigned apps show "unknown developer" warnings, so every release is signed:

- **macOS:** signed with a Developer ID certificate and notarized with Apple.
- **Windows:** signed with a code signing certificate.
- **Linux:** packages are signed, and checksums are published.

Signing keys live only in CI secrets. They are never on developer machines or in the repo.

### 6.5 Versions and release channels

- One version number for everything: the desktop app, daemon, and command line tool always ship together with the same version.
- Semantic versioning: `major.minor.patch`.
- Two channels: **stable** and **beta**. Users pick one in settings.
- The Tauri updater updates the app and its daemon together. The daemon finishes active turns and saves sessions before it restarts for an update.

### 6.6 Daemon-only packages

For remote machines and servers that have no screen:

- Homebrew formula for macOS and Linux
- `.deb` and `.rpm` packages
- Plain archives with the binaries, for any other setup

These are managed from the web UI on another device, over Tailscale.

### 6.7 Release steps in CI

1. Run `pnpm check` on all three platforms.
2. Build the UI.
3. Build the daemon and command line tool for every target, with the UI embedded.
4. Build and sign desktop installers on each OS runner.
5. Check the download size budget.
6. Publish installers, daemon packages, checksums, and updater files.
7. Publish release notes.

**What exists today:** `.github/workflows/release.yml`. It runs when a tag like `v0.1.0` is pushed, or by hand from the Actions tab with a tag. A macOS job builds the desktop app as one universal app (Apple silicon and Intel) and its dmg, on `macos-latest`, and an Android job builds the phone app. When a build succeeds, the release is published on the repository's Releases page with notes made from the commits since the last release; a build that fails publishes nothing. To cut a release: `git tag v0.1.0 && git push origin v0.1.0`. The dmg is only ad-hoc signed, so macOS shows an "unidentified developer" prompt, until the six `APPLE_*` secrets listed at the top of `release.yml` exist. The Android job publishes only when its four keystore secrets exist. To try the build on your own Mac first: `pnpm gen && pnpm --filter web build && node scripts/copy-web-dist.mjs && pnpm --filter desktop build:universal` (it builds the daemon and CLI for both chips and joins them with `lipo`; the Intel Rust target is added with `rustup target add x86_64-apple-darwin`); the dmg lands in `apps/desktop/src-tauri/target/universal-apple-darwin/release/bundle/dmg/`, and `cargo clean` in `apps/desktop/src-tauri` frees the build data afterwards.

---

## 7. Troubleshooting

| Problem | Fix |
|---|---|
| Port 47801 is already in use | Another dev daemon is running. Stop it, or set `MARSHAL_PORT`. |
| "Git 2.38 or newer is needed" | Update Git. Worktree and merge features need it. |
| The desktop window is blank on Linux | Install the webview packages from the Tauri prerequisites for your distribution. |
| The UI shows "Can't reach the daemon" | Check that the daemon is running in dev mode, and that `pnpm dev:web` points at the same port. |
| Types in the UI don't match the API | Run `pnpm gen`. |
| Dev data looks broken | Run `pnpm dev:reset`. |
| A real agent is not found | Check it is on your `PATH`, and that you set `MARSHAL_AGENT=real`. |

### In-app update releases

The signed desktop update workflow and one-time signing setup are described in [app-updates.md](app-updates.md). Source builds show updates as disabled. The release workflow requires the updater signing secret and matching public-key variable before publishing the macOS desktop release; it ships the archive, signature, and manifest along with the DMG. A real signed upgrade still needs release acceptance testing.
