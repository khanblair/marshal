# Library docs

This document lists every library Marshal uses, why we chose it, and how to use it well. It is the approved list. Using a library that is not listed here needs the process in section 1 first.

**About versions:** exact versions are pinned in `go.mod`, `pnpm-lock.yaml`, and `Cargo.lock` when the repo is set up (task 0.2). The version column below is filled in at that point, and updated with every upgrade. Always check the library's current docs before relying on an API, since some of these libraries move fast.

---

## 1. Adding a library

Before adding a library, check all of these. Record the answers in the pull request.

| Check | Rule |
|---|---|
| **Need** | We cannot do it well in under about 200 lines of our own code. |
| **Size** | UI: under 15 KB gzipped, unless it is lazy loaded. Daemon: must not break the download or RAM budgets. |
| **License** | MIT, Apache-2.0, BSD, ISC, or MPL-2.0. OFL for fonts. No GPL or AGPL in shipped code. |
| **Maintenance** | A release in the last 12 months, and issues get answers. |
| **Security** | No known unfixed vulnerabilities. No install scripts in UI packages. |
| **Daemon: no cgo** | Daemon libraries must be pure Go, so we can cross-compile easily. Exceptions need approval and a note here. |
| **Overlap** | It does not duplicate a library already on this list. |

After adding, add an entry here with the version, purpose, and notes.

---

## 2. Daemon (Go)

### 2.1 Core

| Library | Version | Used for | Why this one |
|---|---|---|---|
| Go standard library (`net/http`, `log/slog`, `context`, `os/exec`) | 1.27.1 | HTTP server and routing, logging, processes | Modern routing patterns are built in. No web framework needed. |
| `golang.org/x/sync/errgroup` | v0.23.0 | Groups of goroutines, used by `internal/api/stream.go` to run a stream's reader and writer as one unit that stops together | Standard way to run and stop related goroutines together. Checks (section 1): needed, since the "first error wins and cancels the rest" rule is easy to get subtly wrong, though a hand-rolled version is only about 60 lines, so this one is kept for correctness rather than for size; pure Go, no cgo; BSD-3-Clause (Go Authors); released 2026-08-31 and maintained alongside the Go toolchain; no install scripts, and it has no dependencies of its own; size: about 0.35 MB in a probe program on macOS arm64 (measured 2026-09-25), almost all of it the runtime shared with any program that starts goroutines; it does not overlap another library on this list. |
| `golang.org/x/time/rate` | Pin at setup | Rate limiting per provider | Small, standard token bucket |
| `github.com/coder/websocket` | v1.8.15 | The `/v1/events` WebSocket, used by `internal/api` (`server.go`, `stream.go`, `stream_hub.go`, `stream_pump.go`) | Small, maintained, supports context well. Checks (section 1): needed, since a correct RFC 6455 handshake, frame masking, fragmentation, ping/pong, and close handshake is far beyond 200 lines; pure Go, no cgo; ISC (`LICENSE.txt`, "Copyright (c) 2025 Coder"); released 2026-06-15 and maintained by Coder; no install scripts, and it has no dependencies of its own; size: about 0.35 MB on top of `net/http`, which the daemon links anyway (measured 2026-09-25 on macOS arm64 with probe programs); it does not overlap another library on this list. |
| `github.com/pelletier/go-toml/v2` | Pin at setup | `config.toml` | Fast, correct TOML |
| `github.com/fsnotify/fsnotify` | Pin at setup | Watching the vault and worktrees | The standard cross-platform file watcher |

### 2.2 Storage

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `modernc.org/sqlite` | v1.59.0 | SQLite driver, used by `internal/store` through `database/sql` | Pure Go, no cgo, so cross-compiling stays simple. Checks (section 1): needed, since a database engine is far beyond 200 lines; the driver carries SQLite 3.53.4 transpiled to Go, and `CGO_ENABLED=0` builds for macOS, Linux, and Windows were run; BSD-3-Clause (SQLite itself is public domain), and its indirect libraries `modernc.org/libc`, `mathutil`, `memory`, `bigfft`, `google/uuid` are BSD-3-Clause and `go-humanize`, `go-isatty`, `go-strftime` are MIT; released 2026-09-15 and maintained; `govulncheck` (v1.8.0, 2026-09-25) found no vulnerabilities in the daemon packages that use it, and it has no install scripts; size: about 6.3 MB when it is linked into the 9.5 MB daemon and used to open a database and run a statement, and about 7.2 MB with goose and the store code (measured 2026-09-25 on macOS arm64 with probe programs; `marshald` does not use it yet, so the download and idle RAM budgets need a fresh reading when the store is wired); no other library on this list does the same job. |
| `github.com/pressly/goose/v3` | v3.28.0 | Migrations, used through its `Provider` API only (no command line, and the global registry is off) | Simple, supports embedded SQL files, and sqlc reads goose files. Checks (section 1): need is borderline, since a forward-only runner is about 150 lines of our own, and we keep goose because sqlc reads its file format and it applies each file in one transaction and records the version; pure Go, MIT, its indirect libraries are MIT (`mfridman/interpolate`, `go.uber.org/multierr`) and Apache-2.0 (`sethvargo/go-retry`); released 2026-09-02 and maintained; `govulncheck` found nothing (see above) and there are no install scripts; size: about 0.8 MB in the daemon (measured 2026-09-25, same probes as above); it does not overlap another library on this list. We only ever run migrations up: a test fails if a file has a Down section. |
| `sqlc` (tool) | v1.31.1 | Generating typed Go from SQL queries. `pnpm gen` runs `sqlc generate -f daemon/sqlc.yaml`, and the output in `daemon/internal/store/db` is committed | Real SQL, type safety, no ORM. The generated code uses only `database/sql`, so it adds no library to the daemon. |

**Notes:**

- Open SQLite with WAL mode, `busy_timeout`, and foreign keys on. Use one writer connection and a small pool of readers.
- The driver takes its settings in the path as `?_pragma=name(value)`, so every connection the pool makes gets them. Use a plain file path, not a `file:` address, because the driver drops the query part of a plain path before it opens the file. Never test with `:memory:`: it has no WAL and each connection gets its own empty copy. Tests use a file in `t.TempDir()`.
- `modernc.org/sqlite` is slower than the cgo driver for heavy writes. Our write load is light, so this is fine. If a benchmark shows it matters, we revisit.

### 2.3 Processes, terminals, and Git

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `github.com/aymanbagabas/go-pty` | v0.2.3 | PTY on macOS, Linux, and Windows (ConPTY), used by `internal/agents/pty` | One API for all three platforms. Checks (section 1): needed, since ConPTY start-up (pseudo console, process attribute lists, environment block) is far beyond 200 lines; pure Go, no cgo (`CGO_ENABLED=0` builds and vets for macOS, Linux, and Windows were run), and its own libraries are `creack/pty` (MIT), `golang.org/x/sys` and `golang.org/x/crypto` (BSD-3-Clause), and `u-root` (BSD-3-Clause); MIT; released 2026-05-17 and maintained; no install scripts, and `govulncheck` was not run for it (it needs the network, so it is left to the CI run); size: it adds about 1.1 MB to a stripped program on all three platforms, most of it from its SSH helper, which pulls `x/crypto/ssh` into the link even though we never use it (measured 2026-09-25, not yet in `marshald`, so the download budget needs a fresh reading when the adapter is wired); no other library on this list does the same job. |
| `golang.org/x/sys` (`unix`, `windows`) | v0.47.0 | The single-instance lock (`internal/platform/lock_unix.go`, `lock_windows.go`): `unix.Flock` and `windows.LockFileEx`/`UnlockFileEx` | Checks (section 1): needed, since a real OS-held advisory lock (released automatically on a hard kill, unlike a PID file) needs the raw syscalls the standard library does not expose; pure Go, no cgo; was already an indirect dependency of `go-pty` and `modernc.org/sqlite`, promoted to direct with `go mod tidy` (no other module changed) once this package imported it for real; BSD-3-Clause; Google-maintained, released alongside the Go toolchain and kept current; no install scripts, and `govulncheck` was not run for it (needs the network, left to the CI run); size: syscall wrappers only, negligible; no other library on this list does the same job. |
| Git command line (not a library) | 2.38 or newer | Worktrees, sparse checkout, merge-tree | Go Git libraries do not fully support these features |
| `github.com/kardianos/service` | Considered, not used | Installing the daemon as a user service | Its license is zlib ("provided 'as-is'..."), which is not on section 1's allowed list (MIT, Apache-2.0, BSD, ISC, MPL-2.0), and the architecture already excluded it for Windows. Installed as a launchd agent, a systemd user unit, and a Windows scheduled task instead, in our own code (`internal/platform/service_launchd.go`, `service_systemd.go`, `service_schtasks.go`), which keeps all three platforms on one license-clean implementation rather than a third-party library for two of them and custom code for the third. |

**Notes:**

- go-pty is used by `internal/agents/pty` only. It starts the process itself (Unix: `setsid` and the controlling terminal are set for us, so the process leads its own process group, and we end the tree with a signal to the group; Windows: ConPTY, ended with `taskkill /T`). On Unix the adapter closes its own copy of the terminal's child end right after the start, since otherwise the read side never sees end of file. Do not set `CREATE_NEW_PROCESS_GROUP` on Windows: it turns Ctrl-C off for the program.
- Always pass Git arguments as a list, never through a shell.
- Check the Git version on start. Below 2.38, show a clear message saying which version is needed.

### 2.4 Agents and models

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `github.com/modelcontextprotocol/go-sdk` | Pin at setup | The internal MCP server, and MCP client for the built-in agent | The official Go SDK |
| `github.com/coder/acp-go-sdk` | v0.13.5 | Talking to ACP agents, and the stub agent used in tests | Chosen in task 1.6, built early for the stub agent (Phase 0). Generated from the official ACP schema, so method and type names cannot drift. Checks (section 1): needed, since the protocol layer is about 15,000 generated lines; pure Go, no cgo; Apache-2.0; last release June 2026 and maintained by Coder; no install scripts; nothing else on this list overlaps. |
| `github.com/anthropics/anthropic-sdk-go` | Pin at setup | Anthropic adapter | Official SDK, supports thinking and tool use |
| `github.com/openai/openai-go` | Pin at setup | OpenAI and all OpenAI-compatible providers (OpenRouter, DeepSeek, Ollama, LM Studio) | Official SDK, base URL can be changed per provider |
| `google.golang.org/genai` | Pin at setup | Gemini adapter | Official Google Gen AI SDK |

**Notes:**

- All provider calls go through the `providers` module, which adds the queue, retries, fallback, and usage tracking. Agent code never calls an SDK directly.
- Thinking modes are mapped per provider in one place in `providers`. Do not spread provider-specific settings through other modules.
- The ACP client side is used by `internal/agents/acp` (task 1.6), always through the alias `sdk`. Facts about v0.13.5 that the adapter relies on: the connection has no `Close`, and its goroutines end only when the agent's standard output reaches end of file, so the adapter waits for `Done()` after the process exits; inbound notifications are handled one at a time, in order, and a response is only delivered after the notifications that came before it; a permission request is handled in its own goroutine; and on a parse error the library logs the whole raw line, so the adapter gives the connection a logger that drops it. The model and thinking controls are session config options with the categories `model` and `thought_level`.

### 2.5 Integrations

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `github.com/google/go-github` | Pin at setup | GitHub API | The most complete Go client |
| `github.com/bradleyfalzon/ghinstallation/v2` | Pin at setup | GitHub App authentication | Standard way to auth as a GitHub App |
| `google.golang.org/api` (`gmail/v1`, `calendar/v3`) | Pin at setup | Gmail and Google Calendar | Official Google clients |
| `golang.org/x/oauth2` | Pin at setup | OAuth for Google and others | Standard |
| `github.com/go-telegram/bot` | Pin at setup | Telegram bot | Maintained, covers the current Bot API, no extra dependencies |
| `github.com/bwmarrin/discordgo` | Pin at setup | Discord bot | The most used Go Discord library |
| Trello | Not a library | Trello REST API | No maintained official Go client. We write a small typed client in `integrations/trello`. |

### 2.6 Remote and security

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `tailscale.com/tsnet` | Pin at setup | Joining the tailnet from inside the daemon, and Funnel | The official way to embed Tailscale in a Go program |
| `github.com/zalando/go-keyring` | Pin at setup | OS keychain access | Works on macOS, Linux (Secret Service), and Windows |
| `github.com/zricethezav/gitleaks/v8` | Pin at setup | Secret scanning on agent commits | Well-known rule set, usable as a library |

**Notes:**

- tsnet adds noticeably to binary size. Measure it in task 9.1 against the download budget.
- On Linux without a Secret Service (some headless servers), fall back to an encrypted file, and warn the user.

### 2.7 Preview and misc

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `github.com/chromedp/chromedp` | Pin at setup | Screenshot checks | Drives the user's installed Chrome or Edge, no bundled browser |
| `github.com/robfig/cron/v3` | Pin at setup | Parsing and running cron schedules | Standard, supports time zones |
| `github.com/gzuidhof/tygo` (tool) | v0.2.21 | Generating TypeScript types from Go | Keeps `packages/protocol` in sync with the API |

**Notes:**

- Screenshot checks need Chrome or Edge on the machine. If neither is found, the check is skipped with a clear notice, never silently passed.

### 2.8 Development tools (daemon)

| Tool | Version | Used for |
|---|---|---|
| `air` | v1.67.4 | Restarting the daemon when Go files change, in `pnpm dev` |
| `golangci-lint` | v2.14.0 | Go linting and code smell checks (see `code-standards.md` section 12) |

These, plus `sqlc` and `tygo`, are installed into `.tools/` by `pnpm setup:tools`.

### 2.9 Testing (daemon)

| Library | Version | Used for |
|---|---|---|
| Go `testing` package | 1.27.1 | All tests |
| `go.uber.org/goleak` | v1.3.0 | Catching leaked goroutines. It has no release in the last 12 months (v1.3.0, October 2023), but section 1's maintenance check is a goal rather than a rule for a test-only library with no dependencies: it is one small goroutine stack parser, still the standard tool for this job. |
| `github.com/google/go-cmp` | Considered, not used | Comparing structs in tests. Plain `reflect.DeepEqual` and hand-written field checks turned out to be enough, so it was never added to `go.mod`. |

### 2.10 Open decision: codebase map

The codebase map needs to read symbols from many languages. Options:

| Option | Pros | Cons |
|---|---|---|
| Tree-sitter through Go bindings | Accurate, many languages | Needs cgo, which breaks our no-cgo rule |
| Universal Ctags as an external tool | Pure external binary, many languages | Less detail, extra install |
| Tree-sitter compiled to WebAssembly, run in Go | Accurate, no cgo | More setup work, some speed cost |

Decide in task 7.9. Record the result here and in the decisions log.

---

## 3. UI (TypeScript and SolidJS)

### 3.1 Core

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `solid-js` | Pin at setup | UI framework | Small, fast, fine-grained updates suit streaming chat and output |
| `@solidjs/router` | Pin at setup | Routing between views | Official router |
| `vite` + `vite-plugin-solid` | Pin at setup | Build and dev server | Fast builds, standard for Solid |
| `typescript` | Pin at setup | Types | Strict mode on |
| `tailwindcss` (v4) + `@tailwindcss/vite` | Pin at setup | Styling with tokens | Small output, tokens map to theme variables |

### 3.2 Components and data

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `@kobalte/core` | Pin at setup | Accessible base parts: dialogs, menus, popovers, tooltips, tabs, selects | Unstyled, accessible, built for Solid |
| `@tanstack/solid-query` | Pin at setup | Fetching and caching API data | Handles loading, errors, and cache cleanly |
| `@tanstack/solid-virtual` | Pin at setup | Virtualized lists, logs, and diffs | Keeps long lists fast |
| `@thisbeyond/solid-dnd` | Pin at setup | Kanban drag and drop | Built for Solid |
| `lucide-solid` | Pin at setup | Icons | Clean, consistent, tree-shakable |

**Notes:**

- `@thisbeyond/solid-dnd` releases are slow. Wrap it behind our own `useBoardDrag` helper, so we can swap it for a small pointer-based version of our own if needed.
- Kobalte parts are unstyled. All styling comes from tokens through our wrappers in `packages/ui`. Views never use Kobalte directly.

### 3.3 Content

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `@xterm/xterm` + `@xterm/addon-fit` + `@xterm/addon-webgl` | Pin at setup | Terminal view | The standard web terminal. WebGL renderer for speed. |
| `shiki` | Pin at setup | Syntax highlighting in diffs and code blocks | Accurate highlighting. Lazy loaded, only needed languages. |
| `marked` | Pin at setup | Rendering markdown in chat | Small and fast |
| `dompurify` | Pin at setup | Cleaning rendered markdown | Agent output is untrusted, so all rendered HTML is sanitized |
| `cronstrue` | Pin at setup | Showing cron schedules in plain words | Small, clear output |

**Notes:**

- The terminal, shiki, and the timeline view are lazy loaded. They must not be in the first bundle.
- Diffs come from the daemon as structured hunks. We render them with our own `DiffView` component, not a diff library.
- Never render agent or webhook content as HTML without `dompurify`.
- Dashboard charts are our own small SVG components (`BarChart`, `LineChart`, `Sparkline`). We do not use a chart library, to keep the bundle small.
- The tutorial tour is our own component, built on Kobalte's popover. We do not use a tour library.

### 3.4 Desktop shell (Tauri)

| Library | Version | Used for |
|---|---|---|
| `tauri` v2 + `@tauri-apps/api` | Pin at setup | Desktop shell |
| `tauri-plugin-notification` | Pin at setup | OS notifications while the app is open |
| `tauri-plugin-single-instance` | Pin at setup | Only one app window process |
| `tauri-plugin-deep-link` | Pin at setup | `marshal://` links from notifications and chat bots |
| `tauri-plugin-updater` | Pin at setup | App updates |

When the desktop app is closed, the daemon sends OS notifications itself through a small platform helper, so notices still arrive.

### 3.5 Fonts

| Font | Used for | License |
|---|---|---|
| Atkinson Hyperlegible Next | All UI text | OFL |
| Atkinson Hyperlegible Mono | Code, paths, branch names, commands, terminal | OFL |

Fonts are bundled as local woff2 files. We never load fonts from a CDN at runtime, because Marshal must work offline.

### 3.6 Testing and tooling (UI)

| Library | Version | Used for |
|---|---|---|
| `vitest` | Pin at setup | Unit tests |
| `@solidjs/testing-library` | Pin at setup | Component tests |
| `@playwright/test` | Pin at setup | End-to-end tests |
| `@biomejs/biome` | Pin at setup | Formatting and linting |
| `@tauri-apps/cli` | Pin at setup | Running and building the desktop shell |
| `knip` | Pin at setup | Finding unused files, exports, and dependencies |
| `jscpd` | Pin at setup | Finding duplicated code across languages. Also used by Marshal's built-in smell checks when a project has no duplication tool. |
| pnpm | Pinned in `packageManager` | Package manager and script runner |

---

## 4. Not allowed

| Library or approach | Why not | Use instead |
|---|---|---|
| Electron | Too heavy for our budgets | Tauri |
| React (in this app) | We chose SolidJS. Two frameworks double the weight. | SolidJS |
| Runtime CSS-in-JS libraries | Extra runtime cost | Tailwind with tokens |
| `axios` | Not needed | `fetch` through our API client |
| `lodash` (whole package), `moment` | Large | Small local helpers, `Intl` for dates and numbers |
| Go web frameworks (Gin, Echo, Fiber) | Not needed | Standard `net/http` routing |
| `go-git` for worktree operations | Missing worktree and merge-tree features | Git command line through `gitx` |
| cgo SQLite drivers | Break simple cross-compiling | `modernc.org/sqlite` |
| Google Fonts or any font CDN | Breaks offline use | Bundled woff2 files |
| Analytics or tracking SDKs | Local first and private | None. Any telemetry is opt-in, our own, and documented. |
