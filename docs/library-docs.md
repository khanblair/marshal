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
| Go standard library (`net/http`, `log/slog`, `context`, `os/exec`) | Pin at setup | HTTP server and routing, logging, processes | Modern routing patterns are built in. No web framework needed. |
| `golang.org/x/sync/errgroup` | Pin at setup | Groups of goroutines | Standard way to run and stop related goroutines together |
| `golang.org/x/time/rate` | Pin at setup | Rate limiting per provider | Small, standard token bucket |
| `github.com/coder/websocket` | Pin at setup | WebSocket event stream | Small, maintained, supports context well |
| `github.com/pelletier/go-toml/v2` | Pin at setup | `config.toml` | Fast, correct TOML |
| `github.com/fsnotify/fsnotify` | Pin at setup | Watching the vault and worktrees | The standard cross-platform file watcher |

### 2.2 Storage

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `modernc.org/sqlite` | Pin at setup | SQLite driver | Pure Go, no cgo, so cross-compiling stays simple |
| `github.com/pressly/goose/v3` | Pin at setup | Migrations | Simple, supports embedded SQL files |
| `sqlc` (tool) | Pin at setup | Generating typed Go from SQL queries | Real SQL, type safety, no ORM |

**Notes:**

- Open SQLite with WAL mode, `busy_timeout`, and foreign keys on. Use one writer connection and a small pool of readers.
- `modernc.org/sqlite` is slower than the cgo driver for heavy writes. Our write load is light, so this is fine. If a benchmark shows it matters, we revisit.

### 2.3 Processes, terminals, and Git

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `github.com/aymanbagabas/go-pty` | Pin at setup | PTY on macOS, Linux, and Windows (ConPTY) | One API for all three platforms |
| Git command line (not a library) | 2.38 or newer | Worktrees, sparse checkout, merge-tree | Go Git libraries do not fully support these features |
| `github.com/kardianos/service` | Pin at setup | Installing the daemon as a user service | Covers launchd and systemd. Windows uses a per-user scheduled task through our own small helper. |

**Notes:**

- Always pass Git arguments as a list, never through a shell.
- Check the Git version on start. Below 2.38, show a clear message saying which version is needed.

### 2.4 Agents and models

| Library | Version | Used for | Why this one |
|---|---|---|---|
| `github.com/modelcontextprotocol/go-sdk` | Pin at setup | The internal MCP server, and MCP client for the built-in agent | The official Go SDK |
| ACP (Agent Client Protocol) | Pin at setup | Talking to ACP agents | Use an official or well-maintained Go SDK if one is available at setup. Otherwise, implement the JSON-RPC over stdio layer ourselves, which is small. Decide in task 1.6. |
| `github.com/anthropics/anthropic-sdk-go` | Pin at setup | Anthropic adapter | Official SDK, supports thinking and tool use |
| `github.com/openai/openai-go` | Pin at setup | OpenAI and all OpenAI-compatible providers (OpenRouter, DeepSeek, Ollama, LM Studio) | Official SDK, base URL can be changed per provider |
| `google.golang.org/genai` | Pin at setup | Gemini adapter | Official Google Gen AI SDK |

**Notes:**

- All provider calls go through the `providers` module, which adds the queue, retries, fallback, and usage tracking. Agent code never calls an SDK directly.
- Thinking modes are mapped per provider in one place in `providers`. Do not spread provider-specific settings through other modules.

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
| `github.com/gzuidhof/tygo` (tool) | Pin at setup | Generating TypeScript types from Go | Keeps `packages/protocol` in sync with the API |

**Notes:**

- Screenshot checks need Chrome or Edge on the machine. If neither is found, the check is skipped with a clear notice, never silently passed.

### 2.8 Development tools (daemon)

| Tool | Version | Used for |
|---|---|---|
| `air` | Pin at setup | Restarting the daemon when Go files change, in `pnpm dev` |
| `golangci-lint` | Pin at setup | Go linting and code smell checks (see `code-standards.md` section 12) |

These, plus `sqlc` and `tygo`, are installed into `.tools/` by `pnpm setup:tools`.

### 2.9 Testing (daemon)

| Library | Version | Used for |
|---|---|---|
| Go `testing` package | Pin at setup | All tests |
| `go.uber.org/goleak` | Pin at setup | Catching leaked goroutines |
| `github.com/google/go-cmp` | Pin at setup | Comparing structs in tests |

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
