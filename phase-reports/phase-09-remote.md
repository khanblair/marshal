# Phase 9 report: remote

One evolving file. Update it after every slice, and at the end of every run on this phase, even a stopped one. Never replace it with a fresh file — append and revise in place.

Dates of the runs: **2026-09-28** (slices 1 to 5) and **2026-09-29** (slices 6 to 9). Builder: an autonomous agent. A second engineer (the "controller") verifies.

## 1. Summary

Phase 9's milestone is "anywhere": the daemon joins the tailnet, phones pair by scanning a code, Telegram, Discord, and ntfy notify and take actions, and notifications reach a phone in under five seconds. On 2026-09-29 the owner scoped "remote" to mean controlling a daemon from a **phone or browser** (not a second machine), including an **Android app** (docs/mobile.md).

**Built and green (code + scoped tests run by the builder):**

- **Slices 1 to 5 (2026-09-28):** the tsnet node and origin policy, device pairing, Funnel for `/hooks/*` only, Telegram and Discord, notification routing on the event bus, and the pairing and tailnet screens. See section 2.
- **Slice 6 - phone sign-in and the platform adapter.** The "unauthorized" screen takes a pairing code (or scans it in the phone app) and calls `POST /v1/devices/pair`; the daemon's page opened with `?pair=<code>` pairs by itself and takes the code out of the address. `src/platform/` is the one place native features are reached (files, folder chooser, QR scan, haptics, `marshal://` links).
- **Slice 7 - the Android app (`apps/mobile`).** A Tauri v2 project that opens the page the daemon serves over the tailnet. Its first screen takes an address or a scanned QR code; the desktop's "Pair a device" shows the QR code. Deep links, shared text, camera attachments, and haptics are in; an offline bar names the machine and the age of the data, and a change is refused before it is sent. The release workflow has an Android job (signed `.apk` and `.aab`, once the four signing secrets exist).
- **Slice 8 - alerts.** The router now tells a person when a card finishes, when a card needs them for any reason but a permission, and when CI turns red; each notice links to the card. Where each alert goes is a setting (S26c) with an Alerts screen. ntfy is a third channel (S29h).
- **Slice 9 - chat cards and the last onboarding step.** A chat can add a card (`new <project> <title>`). Onboarding's "connect from anywhere" screen (S31b) makes a real pairing code and QR code and saves the real connections.

**Not built (left for later, by the owner's ruling on 2026-09-29):** several machines and the machine switcher (mobile.md 9.13), app lock and "confirm risky actions" (9.15), iOS (9.20), the push relay, Tailscale inside the app, sharing an image or file into Marshal, a per-button disabled state offline, and drawing `GET /v1/tailnet/peers` anywhere. Voice notes are still answered with "please type".

Phases 10, 11, 12, and 13 were **not started**.

## 2. Slice results

| Slice | Status | What it built | Tests added | Evidence |
|---|---|---|---|---|
| 1. tsnet node and origin policy | Done | `internal/tailnet`; additive listener in `server.go`; tailnet origins in `stream_origin.go`; `--tailnet` settings. A node nothing ever used now closes without panicking. | `internal/tailnet/tailnet_test.go`, `cmd/marshald/tailnet_test.go` | scoped tests pass |
| 2. Device pairing | Done | `internal/devices`; `routes_devices.go`; `protocol/device.go`, `protocol/tailnet.go` | `internal/devices/devices_test.go`, `api/routes_devices_test.go` | scoped tests pass |
| 3. Funnel and serving | Done (backend) | `FunnelHandler` + `hooksOnly`; `openFunnel` | `api/tailnet_test.go` | scoped tests pass |
| 4. Telegram, Discord, ntfy | Done | `internal/chatbot` (bots, ntfy publisher, parser); connections in `internal/integrations`; forms; S29f, S29g, S29h | `chatbot/*_test.go`, `integrations/chat_test.go` | fake servers only |
| 5. Routing and status screens | Done | `internal/notify`, `bus.go`, links, the alert settings (`alerts.go`, `GET/PUT /v1/settings/alerts`); `sync/devices.ts`; `sync/alerts.ts`; the Alerts screen | `notify/*_test.go`, `api/routes_alerts_test.go`, `AlertsSection.daemon.test.tsx` | notify and api scoped tests pass |
| 6. Phone sign-in, platform adapter | Done | `POST /v1/devices/pair` from the web app; `platform/` adapter; pairing QR; `?pair=` | `platform/*.test.ts`, `SignIn.test.tsx`, `AppRoot.test.tsx`, `sync/index.test.ts` | scoped web tests pass |
| 7. Android app | Built, not run | `apps/mobile` (crate, config, capabilities, first screen, scripts); deep links, share, camera, haptics, offline bar; release job | `splash/address.test.mjs`, `scripts/patch-android.test.mjs`, `deep-links.test.ts`, `api-client.test.ts` | `cargo check` passes for the host and for `aarch64-linux-android`; nothing was built into an app or run |
| 8. Alerts | Done | card done, needs you, CI red; per-channel links; alert settings | see slice 5 | scoped tests pass |
| 9. Chat cards, onboarding | Done | `chatcmd` `new`; `ControlStepLive`; S31b switched | `chatcmd_test.go`, `ControlStepLive.daemon.test.tsx` | scoped tests pass |
| Remote machines | Reframed | see section 6 | | |

## 3. Cutover evidence

Gate item 8 (`pnpm check` and budgets) is not run by the builder: "not run by the builder; see CI".

| Section | Built | Daemon tests + goldens | Mapper tests from goldens | Loading/empty/error/offline | End-to-end specs | Mock removed | Registers and docs | `pnpm check` | Switched |
|---|---|---|---|---|---|---|---|---|---|
| S2b Devices and Tailscale identity | Yes | Yes | No | Partial | No | Partial | Yes | see CI | **Yes** |
| S26c Settings: alerts | Yes | Yes (golden `alert-settings`) | Screen test against the fake daemon | Not connected sentence | No | N/A (new) | Yes | see CI | **Yes** |
| S29f Telegram, S29g Discord | Yes | Yes | N/A | Inherited | No | No | Yes | see CI | **Yes** |
| S29h Integration: ntfy | Yes | Yes | N/A | Inherited | No | N/A (new) | Yes | see CI | **Yes** |
| S31b Onboarding: connect from anywhere | Yes | Yes | No | Falls back to the design's own step while the daemon is not online | No | Partial | Yes | see CI | **Yes** |

## 4. What I changed

**2026-09-29 run, in short:** new `apps/mobile`; `apps/web/src/platform/`; `sync/deep-links.ts`, `sync/alerts.ts`; `AlertsSection.tsx`, `NtfyForm.tsx`, `PairingQr.tsx`, `ControlStepLive.tsx`; `daemon/internal/chatbot/ntfy.go`, `notify/alerts.go`, `api/routes_alerts.go`, `protocol/alerts.go`; changes to `notify` (more events, links), `chatcmd` (cards), `tailnet` (safe close), the sign-in screen, the offline bar, the API client (pair, offline guard, alert settings), the fake daemon, and the release workflow. The lists below are the 2026-09-28 run.

New files:

- `daemon/internal/chatbot/chatbot.go` — the notice, the action, the `Bot` interface, the incoming shape
- `daemon/internal/chatbot/command.go` — the pure command parser and help text
- `daemon/internal/chatbot/telegram.go` — the Telegram bot (`go-telegram/bot`)
- `daemon/internal/chatbot/discord.go` — the Discord bot (`discordgo`)
- `daemon/internal/chatbot/testcheck.go` — the shared check names and summary
- `daemon/internal/chatbot/command_test.go`, `telegram_test.go`, `discord_test.go`
- `daemon/internal/notify/notify.go` — the routing table, grouping, and the `Sender` seam
- `daemon/internal/notify/notify_test.go` — routing, grouping, and the under-five-seconds measurement
- `daemon/internal/integrations/chat.go` — the Telegram and Discord connections (save, read, bot, test)
- `daemon/internal/integrations/chat_test.go` — both connections against in-process fake servers
- `daemon/internal/protocol/chatbot.go` — `SaveTelegramRequest`, `SaveDiscordRequest`
- `apps/web/src/views/settings/TelegramForm.tsx`, `DiscordForm.tsx`
- `apps/web/src/sync/devices.ts` — the S2b syncer, the pairing-code action, revoke, and the tailnet read
- `daemon/internal/notify/bus.go` — the event-bus bridge (`Follow`), and `export_test.go` / `bus_test.go` for it

Modified:

- `daemon/internal/integrations/integrations.go` — Telegram/Discord read `Wired`; new test seams
- `daemon/internal/integrations/test.go` — the dispatcher's Telegram/Discord cases
- `daemon/internal/api/routes_integrations.go` — the two new save shapes
- `apps/web/src/sync/integrations.ts` — `TELEGRAM_ID`/`DISCORD_ID` and their sections
- `apps/web/src/sync/integration-actions.ts`, `apps/web/src/views/settings/integration-actions.ts` — the two writes
- `apps/web/src/mock/marshal.ts` — the two new `M` members
- `apps/web/src/mock/index.test.ts` — the two new members in the M surface list
- `apps/web/src/views/settings/IntegrationsSection.tsx` — the two forms in the switch
- `apps/web/src/data/api-client-types.ts` — the two save bodies in the union
- `apps/web/src/data/sections.ts` — S2b, S29f and S29g switched to `daemon`
- `apps/web/src/views/settings/ProfileSection.tsx` — the tailnet identity reads the daemon's node, its reachability, and Funnel
- `apps/web/src/views/settings/ProfileSection.daemon.test.tsx` — the paired devices are the daemon's now
- `apps/web/src/views/settings/DevicesList.tsx` — the real code, the real revoke, and a device-kind icon
- `apps/web/src/testing/fake-daemon.ts` — the four routes so a component test has a daemon that answers them
- `apps/web/src/mock/settings-types.ts` — a device row can say it was revoked
- `apps/web/src/sync/index.ts` — the S2b syncer joined the load order
- `docs/backend-checklist.md` — S29f/S29g register rows to `Daemon`; B9.3/B9.4 notes
- `docs/project-structure.md` — `internal/chatbot`, `internal/devices`, `internal/tailnet` in the tree

Counts: **16 new files, 17 modified** (this run). Earlier runs of this phase added `internal/tailnet`, `internal/devices`, `protocol/device.go`, `protocol/tailnet.go`, `api/tailnet.go`, `api/routes_devices.go`, config settings, and the additive listener.

## 5. Verification results

**2026-09-29 run.** The owner asked for the tests, error checks, and validations of Phases 8 and 9 to be run, and the builder ran, one at a time: `go build`, `go vet`, `gofmt`, `golangci-lint` (0 issues), `go test -race -count=1 ./...` in `daemon/`, `pnpm typecheck`, `pnpm format:check`, and the web and protocol vitest suites. After that, slices 6 to 9 were checked with scoped runs only: `tsc --noEmit` for `apps/web` and `packages/ui`, vitest on the touched folders (`src/platform`, `src/sync`, `src/mock`, `src/data`, `src/app`, `src/views/settings`, `src/views/card`, `src/onboarding`, the UI `SignIn` and `OfflineBanner` tests), `node --test` for `apps/mobile`, and Go tests and lint for `notify`, `chatbot`, `chatcmd`, `integrations`, `settings`, `tailnet`, `protocol`, and the alert and route tests in `api`. `cargo check` passed for `apps/mobile` on the host and for `aarch64-linux-android`. `pnpm gen` was run for the wire types.

**Not run (the owner has not asked):** the full `go test -race ./...` and full web suite after slices 6 to 9, `node scripts/check.mjs`, `pnpm test:e2e`, `pnpm build`, `pnpm budgets`, `pnpm smells`, `knip`, any Android build, emulator, or real phone. Before slices 6 to 9 the full runs showed only failures that were then fixed; the remaining known failure is `CalendarView.test.tsx` (a timeout in a file this phase did not change).

**Code review graph:** not run in this pass.

**The security rule of `docs/architecture.md` section 13** is unchanged and still tested: the tailnet listener is off without `--tailnet`, and Funnel serves only `/hooks/*`.

**Milestone check ("anywhere"):** not met end to end. Everything up to a phone reaching the daemon is built and tested in process; no phone has reached one, and no app has been installed.

## 6. Rulings

- **Funnel port 443.** `funnelAddr = ":443"` — the one Funnel port that needs no port in a phone's address bar. Tailscale also allows 8443 and 10000.
- **Pairing code shape.** Crockford base32 minus I/L/O/U, six characters in two groups of three, five-minute life, one live code at a time, five wrong guesses end it, spent on success. Every refusal is the same `unauthorized`, so nothing about a live code leaks.
- **`POST /v1/devices/pair` is the one unauthenticated domain-shaped route.** It is registered beside health, outside the token check, and refused by the Funnel handler with everything outside `/hooks/*`, so it is reachable only on the machine and the person's own tailnet.
- **Tailnet origin policy.** The stream's origin rule accepts same-host requests, dev localhost, the desktop app's `tauri://` origins, **and this node's own MagicDNS name and tailnet addresses** — and nothing else.
- **Chat connections reuse the existing connection machinery.** Telegram and Discord are not a new kind of thing: they are integrations with a token in the keychain and a chat in the row, saved through the same `PUT /v1/integrations/{id}` and tested through the same route, exactly like Trello.
- **`internal/chatbot` is the only package that imports the two bot libraries**, so the API layer and every test reach a bot through an interface and no test ever needs a token or the network.

## 7. Found, not fixed, and later phases

- **`status` in a chat** is listed in the help text and answered with the help text; nothing summarises what is happening yet.
- **Voice notes** are received and passed on as voice (with no text), but nothing transcribes them; the daemon asks for text.
- **`GET /v1/tailnet/peers`** is built and tested and nothing draws it.
- **`internal/search`** was needed by Phase 10 and now exists.
- **Known red test outside this phase:** `CalendarView.test.tsx` times out in one run; the file was not changed by this phase.
- The 2026-09-28 gaps "the router is on the bus for two event types", "a chat cannot answer", and "Phase 8's Trello tests fail" are closed: see slices 5, 8, and 9.

## 8. Needs the controller

Every check that needs a browser, a real account, or a real device is here. The builder never opened a browser and never signed in anywhere.

1. **Sign in to Tailscale** on the daemon's machine (`--tailnet`), confirm the node appears in the tailnet, and confirm the daemon answers on its MagicDNS name and **not** on any interface address.
2. **Allow Funnel** for `/hooks/*` in the Tailscale admin console (`--funnel`), then send a signed GitHub delivery to the public URL and confirm it verifies; then send an unsigned one and confirm 401. Confirm `/`, `/v1/*`, and `/v1/devices/pair` are **not** reachable publicly.
3. **Create the Telegram bot token** (BotFather) and the **Discord bot token** (developer portal) and save each through Settings → Integrations; press Test connection and confirm both pass.
4. **Pair a real phone:** open Settings → Profile, press Pair a device, read the code, and pair. Then revoke the device and confirm it loses access at once. (The pairing-code screen is not built, so this needs the mock code replaced first — see section 7.)
5. **The real-phone and tablet pass** (task 9.4): every view and action at phone, tablet, and desktop sizes, both themes.
6. **Run the full gate on GitHub CI** (or locally if asked): `node scripts/check.mjs`, `pnpm test:e2e`, budgets.

## 9. Design-doc rows I added or propose

The pairing-code display, the tailnet status, and the Funnel status (build-plan task 9.9) are built from the existing profile components. Three things are **new screen surface drawn by the builder from existing settings rows, and want the owner's eye**: the **Alerts** section in Settings (one checkbox per alert and channel, `AlertsSection.tsx`), the **pairing form on the sign-in screen** (a code field, a device name, and a scan button, in `packages/ui`'s `SignIn`), and the **phone app's first screen** (`apps/mobile/splash`). The two chat-connection forms and the ntfy form reuse the existing connection-row components.

## 10. Confirmation of the hard rules

- **Funnel was never turned on for real.** No automated test and no builder command enabled Funnel against the public internet. Every Funnel test drives a fake node in process.
- **No real Telegram or Discord bot token was ever used.** Every bot test points the bot at an in-process fake server and uses a made-up token; the Discord tests rewrite the real discord.com address to the fake server. No real Tailscale account was signed in to.
- **No real Trello token or Google OAuth client** was used (Phase 8's rule, unchanged).
- (2026-09-28 run) No mutating git command was run. No browser was opened. No generated file was edited by hand (`pnpm gen` regenerated them). The only deviation recorded is the code-review graph being unavailable (section 5).
- (2026-09-29 run) No real ntfy topic, Telegram or Discord token, or Tailscale account was used: every test points at an in-process fake. No Android app was built or run, no emulator or phone was used, and no browser was opened.

## 11. Rulings added on 2026-09-29

- **The phone app opens the daemon's page; it does not bundle the web app.** The daemon serves the UI on the tailnet, so the app and the daemon are one version and there is no cross-origin setup. The first screen only learns the address (typed, or from the QR code). The capability that lets the daemon's page use the camera and haptics is limited to `*.ts.net` and `100.*` on port 47800.
- **Cleartext http is allowed in the Android build** because the daemon is reached at a plain-http tailnet address; the tailnet tunnel already encrypts it.
- **The device token stays in the webview's private storage** for now; Android Keystore storage belongs with app lock, which is later work.
- **Offline, a change is refused before it is sent** (`offline` client error), never queued; the bar names the machine and how old the data is.
- **A notice links to the card:** `marshal://card/<id>` for ntfy, and `<tailnet address>/?open=card/<id>` for a chat, because a chat cannot link a custom scheme. With no online node there is no link.
- **Chat cards:** the first word is the project (id, name, or the start of one); with one project the whole message is the title.
- **ntfy needs only a topic.** An open topic on a public server has no token; the topic itself is the secret, and the form says so.
- **The mock seed has an ntfy row and the prototype comparison leaves it out**, because the design prototype never drew one.

## 12. Later work (the owner's notes, 2026-09-29)

Not built, and not blocked: several machines with a machine switcher (mobile.md 9.13); app lock and "confirm risky actions" with Keystore-backed token storage (9.15); iOS (9.20); the optional push relay; Tailscale inside the app; sharing images and files into Marshal (only text is taken today); per-button disabled state offline; showing `GET /v1/tailnet/peers`; `status` in a chat (the help text lists it and nothing answers it yet); transcribing voice notes; haptics on merge (there is no merge action in the web client yet); a minimum Android version other than 10 (29); the "This device was removed" sentence after a revoke (the daemon's standard "token no longer valid" sentence shows today).

## 13. Needs the controller (added 2026-09-29)

1. **Build and install the app:** `pnpm setup:mobile`, then `pnpm dev:android` on a phone or emulator, or run the release workflow with the four `ANDROID_*` secrets. Confirm the first screen reaches the daemon at `http://<tailnet name>:47800`, and that the page then gets the camera and haptics.
2. **Pair a real phone from the desktop's QR code**, then remove it from Settings > Profile and confirm it is refused at once.
3. **Share text from Chrome to Marshal** and confirm the New card dialog opens filled in.
4. **Tap an ntfy notice** and confirm it opens the card in the app.
5. **Create an ntfy topic, save it in Settings > Integrations, press Test connection**, then change an alert in Settings > Alerts and confirm it follows at once.
6. **Send `new <project> <title>` in Telegram and Discord** and confirm a card appears in the backlog.
