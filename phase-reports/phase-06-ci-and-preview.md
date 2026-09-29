# Phase 6 report: CI and preview

One evolving file. Update it after every slice, and at the end of every run on this phase, even a stopped one. Never replace it with a fresh file — append and revise in place.

## 1. Summary

**Code-complete.** All five slices are in the working tree with their tests: the GitHub App and the webhook route (slice 1), the CI monitor with its fix loop (slice 2), both modes of "Simulate CI failure" (slice 3), local CI from workflow files (slice 4), and live preview with screenshots (slice 5). The connection service behind the App (install, save, test, list, remove) is part of slice 1, and the three cutover sections the phase switches are registered `Daemon` in `docs/backend-checklist.md` §2.2 and `apps/web/src/data/sections.ts`: **S13** Card preview (`:74` / `sections.ts:85`), **S21** Home: CI health (`:83` / `:94`), and **S29a** Integration: GitHub (`:93` / `:104`). The web half of S29a was built this phase (mapper, syncer, actions, screen, fakes, the register flip, and the e2e spec).

The new route category the prompt asked for — a public, unauthenticated-but-signed webhook route — is `signedWebhook` in `daemon/internal/api/router.go`, used by `POST /hooks/github`; the signature is verified against the raw body **before** the JSON-body/size-limit wrapping runs. The webhook is proven against recorded fixtures replayed with their original headers, and `tools/hooks-replay` gained `-secret` so a fixture recorded without a signature can still be signed the same way the daemon checks it.

What is **not** done here, and is not the builder's to do: the real mode of "Simulate CI failure" was **never run** (rule 2; §10), the end-of-phase full gate (`node scripts/check.mjs`), the whole-repo suites, `pnpm build`/budgets/smells, knip/jscpd, `pnpm test:e2e`, the hands-on pass, the real GitHub App, and any real sign-in. Owner rule 1d forbids the builder running the full gate; CI runs it on the pull request. Section 5 lists exactly what was and was not run.

This file previously read `Not started` while the whole phase existed in the tree; it is now brought up to date for the whole phase, in place.

## 2. Slice results

| Slice | Status | What is in the tree | Tests / evidence |
|---|---|---|---|
| 1. GitHub App auth and the webhook route (B6.1, B6.7) | Done | `daemon/internal/integrations/github/` — `app.go` (installation tokens, `runTest`), `webhook.go` (delivery parsing, the `ingress` sink), `app_test.go`, `webhook_test.go`. `daemon/internal/integrations/` — `integrations.go` (the connection service: `List`, `Save`, `Test`, `Remove` over `KindGitHub`), `test.go` (GitHub's own test: three read-only calls, the webhook check, and the five named checks `Summary`, `App installed`, `Repositories`, `Permissions`, `Webhook`), `integrations_test.go`. The fourth router category `signedWebhook` in `daemon/internal/api/router.go`; `daemon/internal/api/routes_webhook.go` (`POST /hooks/github`, `needsWebhook`) and `routes_webhook_test.go` + `routes_ci_webhook_test.go`. Migration `0015_integrations.sql`, `store/queries/integrations.sql` + generated `db/integrations.sql.go`, `protocol/integration.go` + `integration_test.go`, `api/routes_integrations.go` + `routes_integrations_test.go`. Goldens `integration-list`, `save-github-request`; hooks fixtures `testdata/hooks/github/{ping,workflow-run}.json`. `tools/hooks-replay/{main,replay}.go` gained `-secret` signing. Pinned in `go.mod`: `ghinstallation/v2 v2.19.0`, `go-github/v68` + `go-github/v88`. | `internal/integrations` 39 tests, `internal/integrations/github` 24 tests; `api/routes_integrations_test.go` 8 route tests; `api/routes_webhook_test.go` 7 signature/no-token tests; `api/routes_ci_webhook_test.go` (`TestAReplayedWorkflowRunUpdatesTheCardsBadgeAndTheCIHealth`); `protocol/integration_test.go` 5 tests including `TestIntegrationListGolden` and `TestSaveGitHubRequestGolden`; web `sync/integrations.test.ts`, `data/mappers/integrations.test.ts`, `views/settings/IntegrationsSection.daemon.test.tsx`, `e2e/daemon-integrations.spec.ts`. |
| 2. CI monitor and its fix loop (B6.2, B6.3) | Done | `daemon/internal/ci/{ci,delivery,fix,poll}.go` — the run record, the delivery from a `workflow_run` webhook, the fix loop (rerun the failed jobs once; on a second failure fetch only the failed step's log, trim it, send it to the card's session, counted against the card's loop limits; on the limit move the card to Needs you with the existing `NeedsReasonKindCIFailed`), and conditional polling as the backup. Migration `0018_ci_runs.sql`, `store/queries/ci.sql` + generated `db/ci.sql.go`, `protocol/ci.go` + `ci_test.go`, `api/routes_ci.go` + `routes_ci_test.go`. Events `ci.updated` on `project:<id>` and `home`. Goldens `ci-run`, `ci-snapshot`, `ci-event-project`, `ci-event-home`. | `internal/ci` 38 tests, including `TestAFailedRunRerunsTheFailedJobsOnce`, `TestASecondFailureSendsTheFailedStepsLogToTheCard`, `TestTheLoopStopsWhenTheCardIsAtItsRoleLimit`, the four `TestThePoll*` tests, and `TestARunTellsTheProjectAndHomeThatItsStateChanged`; `api/routes_ci_test.go` — 2 of its 7 tests are the two CI-route tests (the other 5 are slice 3's); `protocol/ci_test.go` 9 tests including the four goldens; web `data/mappers/ci.test.ts`, `sync/ci.test.ts`, `views/home/ci-rows.test.ts`. |
| 3. Simulate CI failure, both modes (B6.4) | Done | `daemon/internal/ci/simulate.go` — the synthetic mode injects a failed run through the CI monitor's own normal path (same rerun, same trimmed log, same loop limits, same notice) and never touches GitHub; the real mode marks the branch with a deliberately failing commit and pushes it to the card's own branch. `POST /v1/cards/{id}/ci-failure`, gated so it is only on a dev daemon (dev mode or Developer options), refused for a card with no branch; both modes write one audit row (`audit.ActionCISimulated` = `ci.simulated`, `daemon/internal/audit/audit.go:61-64`, used at `ci/simulate.go:228`). Goldens `simulate-ci-failure-request`, `simulate-ci-failure-result`. | `internal/ci` 11 simulate tests (`TestTheSyntheticModeRunsTheWholeFixLoop`, `TestTheSyntheticModeNeverAsksTheRealForge`, `TestTheSyntheticModeStopsAtTheRoleCeiling`, `TestTheRealModeMarksTheBranchAndPushesIt`, `TestTheRealModeNeedsTheAppConnected`, `TestBothModesWriteOneAuditRow`, `TestASimulatedFailureLeavesWhatARealOneLeaves`, …); `api/routes_ci_test.go` (`TestSimulatingACIFailureInjectsARunAndRecordsIt`, `TestTheSimulatedFailureRouteIsOnlyOnADevDaemon`, …); `protocol/ci_test.go` (both simulate goldens); web `sync/ci-actions.test.ts`. |
| 4. Local CI from workflow files (B6.5) | Done | `daemon/internal/localci/{service,run,workflow,classify}.go` — parse workflow files, run the test and lint steps locally, and **mark** unsupported steps rather than failing on them (the four kinds and four statuses), with a step ceiling, a runner reason, the shell a step names, and a tail writer. `protocol/localci.go` + `localci_test.go`, `api/routes_localci.go` + `routes_localci_test.go`, golden `local-ci-result`. **Not** wired as the merge queue's `Tester`: `integrator.Deps.Tests` is still nil in `cmd/marshald` and the two `Run` signatures differ (see §7). | `internal/localci` 27 tests (`TestRunRunsWhatItCanAndMarksTheRest`, `TestRunMarksEveryStepOfAJobItWillNotRun`, `TestRunKeepsOtherJobsGoing`, `TestRunStopsAtTheStepCeiling`, the seven `TestParseWorkflow*`, the two `TestReadWorkflows*`); `api/routes_localci_test.go` 6 route tests; `protocol/localci_test.go` 3 tests including the golden. |
| 5. Live preview and screenshots (B6.6, B6.7) | Done | `daemon/internal/preview/{service,browser,chromedp,ports,process}.go` — a per-card dev server on its own port with an isolated browser profile, driven by the project's dev command; the before/after screenshot pair; the exact `stopped`/`starting`/`running` state names; a missing browser **skips the check with its own sentence** rather than passing silently. `protocol/preview.go`, `protocol/previewshot.go` (+ tests), `api/routes_preview.go` + `routes_preview_test.go`. Events `preview.state_changed` on `card:<id>`. Goldens `preview`, `preview-snapshot`, `preview-event`, `preview-shot-result`. Pinned `chromedp v0.16.0` (+ `cdproto`, `sysutil`). | `internal/preview` 19 tests (`TestTwoCardsPreviewAtOnceWithoutSharingState`, `TestEachCardGetsItsOwnBrowserProfile`, `TestNoBrowserSkipsTheCheckInASentence`, `TestPreviewMovesThroughTheThreeStatesTheTabReads`, `TestEveryStateChangeIsPublishedOnTheCardsTopic`, …); `api/routes_preview_test.go` 12 route tests; `protocol/preview_test.go` 8 tests including the four goldens; web `sync/preview.test.ts`, `views/card/PreviewTab.daemon.test.tsx`. |
| S29a cutover (web half) | Done | `apps/web/src/data/mappers/integrations.ts` + test, `sync/integrations.ts` + test, `sync/integration-actions.ts`, `views/settings/{IntegrationsSection,GitHubAppForm,TestChecks,integration-actions}.tsx`, `testing/fake-integrations.ts`, `testing/daemon-integrations-store.ts`, `data/api-client.ts` (the connection calls), `e2e/support/daemon-api.ts` (`WireIntegration`, `integrationsViaApi`) and `e2e/daemon-integrations.spec.ts`. `actions.ts`/`types.ts`/`settings-types.ts` in the mock re-export the mapper's own row types. Register flipped in `docs/backend-checklist.md` §2.2 (`:93`) and `apps/web/src/data/sections.ts` (`:104`). | `data/mappers/integrations.test.ts` (3, from the `integration-list` golden), `sync/integrations.test.ts` (15, from `integration-list` + `save-github-request`), `views/settings/IntegrationsSection.daemon.test.tsx` (7), `e2e/daemon-integrations.spec.ts` (2, written and not run by the builder). |

## 3. Cutover evidence

Sections switched, with their `docs/backend-checklist.md` §2.2 register row and `apps/web/src/data/sections.ts` entry changed **together** (one switch per section, never half a section). Gate item 8 (`pnpm check` and budgets) is not run by the builder (owner rule 1d; the full gate runs on GitHub CI).

| Section | 1. Inventory rows built | 2. Daemon tests + goldens | 3. Mapper tests from goldens | 4. Loading/empty/error/offline | 5. End-to-end specs | 6. Mock removed, knip clean | 7. Registers and docs | 8. `pnpm check` + budgets | Switched |
|---|---|---|---|---|---|---|---|---|---|
| S13 Card preview | yes (N10): screenshots under the daemon's data folder (no table), `api/routes_preview.go`, `preview.state_changed`, generated types from `protocol/preview.go`/`previewshot.go` | yes: `internal/preview` 19 + `routes_preview_test.go` 12; goldens `preview`, `preview-snapshot`, `preview-event`, `preview-shot-result` | partly: **no web mapper exists for preview** (the model reads the wire type directly), so the wire shape is checked against the Go goldens in `packages/protocol/test/golden-ci.test.ts` (`preview`, `preview-snapshot`, `preview-shot-result`); the screen is driven by `PreviewTab.daemon.test.tsx` (6) and `sync/preview.test.ts` (4) against the fake daemon | not verified by the builder (needs a browser): `preview-model.ts` maps the three states and turns a refused start into `stopped` with the daemon's sentence; offline handled by the sync seam | **not present** — no preview e2e spec was written (see §7) | the Preview tab was mock-only and now reads the daemon when S13 is `daemon`; the mock path is pinned, not deleted; knip **not run by the builder** | **yes**: `docs/backend-checklist.md:74` and `sections.ts:85` → `Daemon`; `docs/architecture.md` §10/§11.1/§11.2 | **not run by the builder; see CI** | **Yes** |
| S21 Home: CI health | yes: `ci_runs` (`0018`), `store/queries/ci.sql`, `api/routes_ci.go`, `ci.updated` on `project:<id>` and `home`, generated types from `protocol/ci.go` | yes: `internal/ci` 38 + `routes_ci_test.go` 2; goldens `ci-run`, `ci-snapshot`, `ci-event-project`, `ci-event-home` | yes: `data/mappers/ci.test.ts` (8) and `sync/ci.test.ts` (8) read the `ci-snapshot` golden the Go contract test writes | not verified by the builder (needs a browser): `CiSection`'s `<Show … fallback={<CiNotConnected/>}>` is the empty/not-connected state, `ci-rows.hasCi` filters projects with no CI, the error/offline path is the sync seam | **not present** — no CI-health e2e spec was written (see §7) | mock CI health now comes from the daemon snapshot; the mock's `testing/prototype-ci.ts` overlay stays as the differential oracle; knip **not run by the builder** | **yes**: `docs/backend-checklist.md:83` and `sections.ts:94` → `Daemon`; `docs/architecture.md` §10/§11.2 | **not run by the builder; see CI** | **Yes** |
| S29a Integration: GitHub | yes: `integrations` (`0015`), `store/queries/integrations.sql`, `internal/integrations/{integrations,test}.go` + `github/{app,webhook}.go`, `api/routes_integrations.go`, `POST /hooks/github`; the section follows no topic | yes: `internal/integrations` 39 + `github` 24 + `routes_integrations_test.go` 8; goldens `integration-list`, `save-github-request` | yes: `data/mappers/integrations.test.ts` (3) and `sync/integrations.test.ts` (15) read the `integration-list` and `save-github-request` goldens | not verified by the builder (needs a browser): `IntegrationsSection` shows the daemon row status, a `Not connected` row when nothing is stored, and the fix sentence on a row whose test found something wrong | written, **not run by the builder**: `e2e/daemon-integrations.spec.ts` (2 tests: lists the daemon's connections; opens the App) | the mock's GitHub row keeps its own words (id/name/icon); the daemon overwrites `st`/`detail`/`lastTest`, so the connection is not invented twice; knip **not run by the builder** | **yes**: `docs/backend-checklist.md:93` and `sections.ts:104` → `Daemon`; `docs/architecture.md` §10/§11.1/§11.2, `docs/backend-inventory.md` | **not run by the builder; see CI** | **Yes** |

Where a section switched, the mock-mode tests were **pinned explicitly** in their test section tables (reworded or deleted: never), and the register pin is repeated after the spreads so a register change cannot silently flip it.

## 4. What I changed

Slice 1 — the GitHub App, the webhook route, and the connection service:

- `daemon/internal/integrations/github/app.go`, `webhook.go` (+ `app_test.go`, `webhook_test.go`) — installation tokens via `ghinstallation/v2`; the delivery parse and the `ingress` sink; `runTest` is the App's own connectivity check.
- `daemon/internal/integrations/integrations.go`, `test.go` (+ `integrations_test.go`) — the connection service (`List`, `Save`, `Test`, `Remove`) and GitHub's own test (three read-only calls, the webhook check, the five named checks, and the cooldown refusal).
- `daemon/internal/api/router.go` — the new `signedWebhook` category (skips the bearer check like `public`, but reads the raw body for the HMAC check **before** the JSON/size wrapping).
- `daemon/internal/api/routes_webhook.go` (+ `routes_webhook_test.go`, `routes_ci_webhook_test.go`) — `POST /hooks/github` and its signature/no-token tests.
- `daemon/internal/api/routes_integrations.go` (+ `routes_integrations_test.go`) — the save, test, list, and remove routes.
- `daemon/internal/store/migrations/0015_integrations.sql`, `store/queries/integrations.sql` + generated `db/integrations.sql.go`.
- `daemon/internal/protocol/integration.go` + `integration_test.go`; goldens `integration-list`, `save-github-request`.
- `daemon/testdata/hooks/github/{ping,workflow-run}.json`; `tools/hooks-replay/{main,replay}.go` (+ `replay_test.go`) — the `-secret` signing path.
- `daemon/go.mod`, `go.sum` — `ghinstallation/v2 v2.19.0`, `go-github/v68`, `go-github/v88`.
- `daemon/cmd/marshald/main.go` — build the integrations service and the webhook route.

Slice 2 — the CI monitor and its fix loop:

- `daemon/internal/ci/ci.go`, `delivery.go`, `fix.go`, `poll.go` (+ `ci_test.go`).
- `daemon/internal/store/migrations/0018_ci_runs.sql`, `store/queries/ci.sql` + generated `db/ci.sql.go`.
- `daemon/internal/protocol/ci.go` + `ci_test.go`; goldens `ci-run`, `ci-snapshot`, `ci-event-project`, `ci-event-home`.
- `daemon/internal/api/routes_ci.go` + `routes_ci_test.go`.
- `daemon/cmd/marshald/main.go` — the monitor, the poller, and the `ci.updated` publisher.

Slice 3 — Simulate CI failure, both modes:

- `daemon/internal/ci/simulate.go` (+ `simulate_test.go`) — the two modes, the dev-mode gate, and the audit row.
- `daemon/internal/api/routes_ci.go` — `POST /v1/cards/{id}/ci-failure` and the dev-only gate.
- `daemon/internal/audit/audit.go` — `ActionCISimulated`.
- `daemon/internal/protocol/ci.go`; goldens `simulate-ci-failure-request`, `simulate-ci-failure-result`.

Slice 4 — local CI:

- `daemon/internal/localci/{service,run,workflow,classify}.go` (+ tests).
- `daemon/internal/protocol/localci.go` + `localci_test.go`; golden `local-ci-result`.
- `daemon/internal/api/routes_localci.go` + `routes_localci_test.go`.
- `daemon/internal/integrator/service.go` (Phase 5) — **not** changed: the merge queue's `Tester` is left nil (see §7).

Slice 5 — preview and screenshots:

- `daemon/internal/preview/{service,browser,chromedp,ports,process}.go` (+ `service_test.go`).
- `daemon/internal/protocol/preview.go`, `previewshot.go` (+ tests); goldens `preview`, `preview-snapshot`, `preview-event`, `preview-shot-result`.
- `daemon/internal/api/routes_preview.go` + `routes_preview_test.go`.
- `daemon/go.mod` — `chromedp v0.16.0` (+ `cdproto`, `sysutil`).

S29a cutover (web half):

- `apps/web/src/data/mappers/integrations.ts` + `.test.ts`; `sync/integrations.ts` + `.test.ts`; `sync/integration-actions.ts`; `views/settings/{IntegrationsSection,GitHubAppForm,TestChecks,integration-actions}`, `IntegrationsSection.daemon.test.tsx`; `testing/fake-integrations.ts`, `testing/daemon-integrations-store.ts`.
- `apps/web/src/data/api-client.ts` — the connection calls; `data/sections.ts` (`S29a: "daemon"`); `mock/settings-types.ts`/`types.ts`/`actions/settings.ts` re-export the mapper's own row types.
- `apps/web/e2e/daemon-integrations.spec.ts`, `e2e/support/daemon-api.ts`.

Docs (this pass and earlier in the phase):

- `docs/architecture.md` — §10's `integrations` and `ci_runs` rows rewritten to the columns actually built; §11.1's `POST /v1/integrations/{id}/test` row expanded and the "routes built today" sentence widened; §11.1 gained the five preview route rows and the missing Phase 3/5 rows (bypass, plan decisions, checkpoints, audit, roles, pull-request/merge/review, notices, sleep settings); §11.2's published/reserved event lists corrected and a new "State today (2026-09-27)" paragraph; §18 gained the GitHub-test bullet. `docs/backend-inventory.md:99` corrected `notice.updated` → `notice.dismissed`.
- `docs/development.md`, `docs/progress-tracker.md`, `docs/project-structure.md`, `docs/ui-rules.md`.
- `docs/backend-checklist.md` §2.2 — `S13`/`S21`/`S29a` → `Daemon`.

A formatting fix made along the way: `gofmt -l internal cmd` flagged `cmd/marshald/main.go` (import order, `preview` after `platform`); `gofmt -w` applied, now clean. No behaviour change.

## 5. Verification results

Run from `daemon/` in the final pass of this phase (fast checks only, per owner rule 1d):

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `gofmt -l internal cmd` — flagged only `cmd/marshald/main.go`; `gofmt -w` applied; now empty.
- `go test -count=1 ./internal/integrations/... ./internal/ci/ ./internal/preview/ ./internal/localci/ ./internal/protocol/ ./internal/connectiontest/` — all ok.
- `go test -count=1 -run '<the 43 CI/preview/integration/webhook/registration route-test names>' ./internal/api/` — ok (10.500s). The whole `internal/api` package is a slow whole-package suite the builder does not run (rule 1d); only these named tests were run.
- `packages/protocol`: `pnpm exec vitest run --no-coverage` — 13 files / 78 tests passed.
- `apps/web`: `pnpm exec tsc --noEmit` — exit 0. `src/sync/integrations.test.ts` + `src/data/mappers/integrations.test.ts` + `src/mock/index.test.ts` + `src/mock/testing` — 3 files / 23 tests passed.

**CI behaviour tested with recorded webhooks only.** Every webhook test replays `daemon/testdata/hooks/github/{ping,workflow-run}.json` (through the route, with the signature computed by `hooks-replay`'s own `-secret` path). No test needs a live GitHub account; the GitHub client and the connection test are exercised against a fake HTTP server and fake doubles.

**Rule 1c end-of-phase graph check (run this phase).** `build_or_update_graph_tool` (incremental, base `0562fc3`) → "324 files re-parsed, 605 nodes and 6456 edges updated", `fts_indexed: 5107`, no errors. `detect_changes_tool` (base `0562fc3`, minimal) → **158 changed files, 420 changed functions/classes, 0 affected flows, 284 test gaps, overall risk score 0.60**. This report is the record of that summary, as rule 1c asks:

- **Untested names it named:** `integrationsViaApi`, `roleQuery`, `routeMethods`, `cardMethods`, `Machine`. `integrationsViaApi` is the e2e helper in `apps/web/e2e/support/daemon-api.ts` (exercised by the e2e spec, which the builder does not run); `roleQuery` is a SQL query constant reached through a tested service method; `routeMethods` and `cardMethods` are small dispatch helpers on already-covered paths; `Machine` is the connection state machine covered by `data/connection.test.ts`. None is a silent gap in the tested behaviour of this phase.
- **Review priorities it named:** `roleQuery`, `commit`, `createConnection`. `roleQuery` as above; `commit` and `createConnection` are Phase 3/4 surfaces already covered by their own suites (`sync/checkpoint-actions.test.ts`, `data/connection.test.ts`).

**Not run by the builder** (owner rule 1d; CI runs these on the pull request): the full gate (`node scripts/check.mjs`), the whole-repo Go suites, `go test -race ./...`, the slow whole-package suites (`internal/api`, `internal/projects`, `internal/session`), `pnpm build`, `pnpm budgets`, the smell budget, knip/jscpd, `pnpm test:e2e`, and anything that starts a process (the daemon, the stub-agent smoke, `scripts/agent-smoke.mjs`, `scripts/check-generated.mjs`, `scripts/coverage.mjs`). No browser was opened and nothing was signed in to. The chromedp shooter is never invoked by a test — it is tested against a fake driver.

## 6. Rulings

- **The Integration row's transient state is client-side only (the ruling the prompt asked for).** `IntegrationStatus` has exactly three values — `connected`, `none`, `error` — and the wire carries only those (`daemon/internal/protocol/integration.go`; `packages/protocol/src/generated/index.ts:2558-2562`). An App install under way is the browser's own fact (it is a redirect the browser performs), and a screen that must keep work-in-progress state keeps it in its own state, not in the wire type. Concretely, `GitHubAppForm`'s `busy()` holds the in-progress state; the daemon never sends a fourth value. No fourth value was added.
- **`IntegrationTest` stays `ProviderTest`.** The connection-test answer is the provider-test shape (`data/mappers/providers.ts:7-19`), reused rather than duplicated. The daemon answers a `TestCheck` with `name`/`state`/`message`/`fix`; `ProviderTest` drops the connection id and the run time, so it cannot reproduce the daemon's **derived row status/sentence**. Decision: keep the shared type and make the fake **mirror the daemon** (`rowToWire`), so the two sides agree on what the checks say; the divergence is a modelling gap, recorded in §7.
- **`TestChecks` drops the check named "Summary".** That check's message is the row's own sentence, drawn above the list, so showing it again inside the list would print it twice.
- **The private key is sent exactly as pasted** (no `.trim()`), because the golden request's PEM has a trailing newline; ids and the shared secret are trimmed.
- **`preview.state_changed` and `quality.checked` are ordinary (non-critical) events.** Each screen reads its snapshot back when it opens, so a slow client may lose an old one and be told to reload; the rest of this phase's events (`ci.updated`, `notice.created`, `notice.dismissed`) are critical and use the replay ring.
- **`ci.updated` is published on two topics.** One `workflow_run` webhook publishes on `project:<id>` (that project's runs) and on `home` (the whole CI snapshot), so a card's badge, the board, and Home's CI health all follow one event (`internal/ci/ci.go`).
- **S29a is a row-mirroring syncer.** The daemon lists every connection it knows (`known()` — github, trello, gcal, gmail, telegram, discord, obsidian; `Wired: true` only for github); the app keeps its own words for each row and `applyIntegrationStates` overwrites only `id`/`st`/`detail`/`lastTest`, and only for connections whose section is `daemon`.
- **`startPreview`/`stopPreview` answer a `PreviewSnapshot`,** not a bare state, so the screen applies a start/stop exactly as it applies the snapshot it loaded.
- **`ciSyncer` filters runs to the default branch,** so a card's own branch runs are the card's badge and are not mistaken for the project's CI health.
- **Local CI is not the merge queue's `Tester` yet.** `integrator.Deps.Tests` is still nil in `cmd/marshald`, and `integrator.Tester.Run(ctx, worktree, changed)` does not match `localci.Service.Run(ctx, cardID, workflow)`, so an adapter is still owed (§7). Phase 5's placeholder therefore remains unclosed.
- **A missing browser is a skip, not a pass.** `NoBrowserSkipsTheCheckInASentence` asserts the check reports its own sentence and never silently succeeds (`docs/library-docs.md`'s own note).

## 7. Found, not fixed, and later phases

- **The merge queue's `Tester` is still nil (an unclosed Phase 5 → Phase 6 hand-off).** Phase 5 recorded that Phase 6's local CI (build-plan 6.5) would supply the merge queue's `Tester`; that adapter was **not** written. `integrator.Deps.Tests` is left nil in `cmd/marshald`, so the queue still moves the target forward on a clean merge alone. Wiring it needs an adapter over `localci.Service.Run(ctx, cardID, workflow)` to match `integrator.Tester.Run(ctx, worktree, changed)`.
- **The connection-test model is lossy (see §6).** `ProviderTest` carries only `{ ok, checks: [{ name, state, message, fix? }] }` and drops the connection id and the run time, so it cannot reproduce the daemon's derived row status/sentence. The fake mirrors the daemon today; a fuller type (or a daemon-side summary field) is a later pass.
- **B6.6's "screenshots attach to the pull request" is not built as an image upload.** `internal/github/github.go` has no contents/upload API and the PR body is literally `card.Body` (`internal/pullrequest/service.go:118-124`), so a screenshot cannot be pushed to GitHub today. Screenshots attach to the **card** (kept under the daemon's data folder and served back); the PR half is unbuilt.
- **There is no notices module and no notices table.** `NoticeKindCI`/`NoticeKindCIMain` exist unused, and `internal/ci` does not create a notice — on a second failure it moves the card to Needs you (`ci/fix.go:187`). A CI notice is a later-phase item.
- **There is no preview migration** (screenshots live on disk, not in a table) and **no CI/preview wire section** in `architecture.md` beyond the §11.1 table rows added this pass.
- **The web has no preview mapper** (the model reads the wire type directly), so S13 has no golden-driven mapper test on the web side; the protocol golden test covers the shape instead (§3).
- **No e2e spec exists for preview or for Home's CI health** (only `daemon-integrations.spec.ts` and the existing `daemon-cards.spec.ts`). The preview and CI-health flows are covered by unit/component tests only.
- **The mock's "Simulate CI failure" hidden menu item is retained,** not deleted: it still backs a card the mock owns (the daemon path applies only to a card the daemon made and a dev daemon), so B6.4's "replace the mock's hidden menu item" is a routing decision at the item level, not a removal.
- **`robfig/cron/v3` is still unpinned** — `docs/library-docs.md` names it "pin at setup", and Phase 8 is where the scheduler that needs it lands.
- **`go-github` (v68 and v88) and `ghinstallation` are marked `// indirect`** because GitHub calls go through `net/http`; nothing imports `go-github` directly. Two major versions are in the graph; a later pass should retire the unused one.
- **The `ci.simulated` audit action and the `notice.dismissed` wire contract** were verified against code this pass (`audit.go:61-64`; `session/notices.go`); the inventory row `backend-inventory.md:99` was corrected to `notice.dismissed`.
- **The Activity-rows-cannot-carry-a-result divergence** (Phase 5) still stands.
- **The reset-to-starter semantics mismatch** (Phase 5) still stands: the UI confirm copy promises replacing edits with the starter template, but the code only clears the per-project override.
- **The restore-dialog backup-ref divergence** (Phase 5) still stands: the prototype promises a backup ref; the daemon keeps none, so `disconnectConnection`'s copy says the change is discarded.
- **Reset-to-starter and the restore dialog** are the two copy divergences carried forward from Phase 5.
- **Maintenance gaps (Phase 5, unchanged):** `openai-go v1.12.0` is behind; `pricing.go`'s numbers are unverified against published list prices.
- **One unexplained Phase 4 failure (unchanged):** `internal/api`'s `TestAReconnectingClientAsksForTheScreenInsteadOfBeingReplayedOutput` (`terminal_test.go:592`, ~15.34s) failed once and was not diagnosed.
- **S8b is not switched** (Phase 5): a stored chat approval block has no approval id and no route lists a card's waiting approvals.
- **The unresolved instruction conflict (Phase 3/Phase 5, recorded reading: 1d wins).** Rule 1d forbids `node scripts/check.mjs` while §5/§6 of the prompt say to run it once at the end of the phase. The full gate runs on CI.
- **The two required reviews of Phase 3's `security`/`harness` cannot both come from the builder.**
- **A Phase 5 documentation debt cleared this pass:** `architecture.md` §11.1 gained the Phase 5 pull-request/review/merge/notices/sleep-settings route rows, and §11.2 gained `notice.dismissed` and `plan.updated`.

## 8. Needs the controller

Every check that needs a browser, a real App, a real sign-in, or a privileged action goes here (the builder never opens a browser: owner rule 1a).

- **Create and install the real GitHub App** (`docs/backend-checklist.md` §3) and save it through the Settings GitHub row so `POST /hooks/github`, the connection test, and the CI badge can be exercised against a throwaway repository. Nothing in this phase installed anything real, and no test used a live installation.
- **Confirming the real mode of "Simulate CI failure", only after reading exactly what it will push.** It pushes a deliberately failing commit to the card's own branch and uses real GitHub Actions minutes. It was **never run by the builder** (§10). It must only be run with the owner watching and approving each use; it needs the GitHub App connected, a card with its own branch, and a worktree.
- **The hands-on pass for S13, S21, and S29a** at phone, tablet, and desktop sizes, in both themes, with no console errors, no sideways scrolling, and no wrapped button labels (cutover gate item 4; not verified by the builder).
- **The real-browser check for slice 5:** with Chrome or Edge present, preview two fixture cards at once and confirm the before/after screenshots; with neither present, confirm the check is skipped in a sentence. The chromedp shooter is not driven by any test.
- **Run the full gate** (`node scripts/check.mjs`), `pnpm build`, budgets/smells, knip/jscpd, and `pnpm test:e2e` (including `e2e/daemon-integrations.spec.ts`) on the pull request as CI.
- **Tick the `docs/backend-checklist.md` §4 `B6.x` rows** (B6.1, B6.5, B6.6, B6.7) and move the `docs/progress-tracker.md` rows (controller-only; the builder does not tick these).
- **The owner's sign-off on the two screens named in §9.**

## 9. Design-doc rows I added or propose

- **Added this pass:** `docs/architecture.md` §10's `integrations` and `ci_runs` rows rewritten to the columns actually built; §11.1's `POST /v1/integrations/{id}/test` row expanded, the "routes built today" sentence widened, the five preview route rows added, and the missing Phase 3/5 rows added (bypass, plan decisions, checkpoints, audit, roles, pull-request/merge/review, notices, sleep settings); §11.2's event lists corrected with a new "State today (2026-09-27)" paragraph; §18's GitHub-test bullet. `docs/backend-inventory.md:99` corrected.
- **Propose:** a `backend-inventory.md` row (or an `architecture.md` §17.4 "wire") for the **CI and preview** routes, which today exist only as §11.1 table rows; and a **notices table / notices module** row, since `NoticeKindCI`/`NoticeKindCIMain` exist but nothing stores or creates a CI notice.
- **Needs the owner's sign-off before they are drawn:** the **checkpoints** screen (Phase 5's backend) and the **smell-findings** screen (Phase 5's slice 7). Neither has a prototype to extract from, so the daemon-path behaviour is written up but the screens are not drawn. Carried forward unchanged.

## 10. Confirmation of the hard rules

- **The real mode of "Simulate CI failure" was never run.** It was built and gated (dev mode only, refused for a card with no branch or no App), and it is proven only by unit tests against a fake forge (`TestTheRealModeMarksTheBranchAndPushesIt`, `TestTheRealModeNeedsTheAppConnected`, `TestTheRealModeIsRefusedWithoutAWorktree`, `TestARealModeThatCannotPushSaysSoAndKeepsItsMark`). No real commit was pushed anywhere and no Actions minutes were spent.
- No mutating git command was run; only `status`, `diff`, `log`, `show`, `ls-files`, and `blame` were read. `HEAD` is unchanged at `0562fc3`.
- No browser, preview, simulator, screenshots of a running app, or `pnpm test:e2e` was used.
- **No live GitHub account, installation, or sign-in was used in any test.** CI behaviour is tested with the recorded webhooks under `daemon/testdata/hooks/github/` and fake HTTP servers; the App is proven against a fixture, never real.
- No file was deleted; no document was created beyond this report and what the slices need. No scratch files were left in the repo.
- No generated file was edited by hand (`db/*.sql.go` came from `sqlc`; the protocol bindings came from `gen-protocol`).
- No test was weakened or deleted to make it pass.
- Fast checks only: `go build`, `go vet`, scoped `go test`, `gofmt`, `tsc --noEmit`, and scoped vitest. Everything heavier is left to CI.
- No credentials or secrets were used or recorded; every key, token, and signature in the tree is synthetic.
