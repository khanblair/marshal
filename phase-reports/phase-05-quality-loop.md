# Phase 5 report: quality loop

One evolving file. Update it after every slice, and at the end of every run on this phase, even a stopped one. Never replace it with a fresh file — append and revise in place.

## 1. Summary

**Code-complete.** All seven slices are in the working tree with their tests: roles, plan-first mode, limits/stuck detection/checkpoints (slices 1–3, from an earlier run), the pull-request service with the Reviewer role on every PR (slice 4a + 4b), the Integrator merge queue (slice 5), automatic sleep with notices (slice 6), and the quality module (slice 7). The four code sections the phase switches are registered `Daemon` in `docs/backend-checklist.md` §2.2 and `apps/web/src/data/sections.ts`: `S8c` Plans and `S27` Settings: Roles from the earlier run, and `S23` Notices and `S26a` Settings: sleep switched **this phase** with slice 6.

What is **not** done here, and is not the builder's to do: the end-of-phase full gate (`node scripts/check.mjs`), the whole-repo suites, `pnpm build`/budgets/smells, knip/jscpd, `pnpm test:e2e`, the hands-on pass, and any real GitHub sign-in. Owner rule 1d forbids the builder running the full gate; CI runs it on the pull request. Section 5 lists exactly what was and was not run.

This file previously read `Not started` while slices 1–3 existed in the tree, then mid-phase while slices 4–5 landed; both were corrected in place as the work arrived. It is now brought up to date for the whole phase.

## 2. Slice results

| Slice | Status | What is in the tree | Tests / evidence |
|---|---|---|---|
| 1. Roles (B5.1) | Present (earlier run) | `daemon/internal/roles/` (`roles.go`, `limits.go`, `seed.go`, `service.go`, `errors.go`), migration `0016_roles_quality.sql` (`roles`, `role_overrides`), `store/queries/roles.sql`, `protocol/role.go`, `api/routes_roles.go`, and the web side `data/mappers/roles.ts`, `sync/roles.ts`, `sync/role-actions.ts`, `views/settings/RolesSection.tsx`, `views/settings/RolesTransfer.tsx`, `testing/fake-roles.ts`, `testing/daemon-roles-store.ts`. `S27` reads `Daemon`. | `roles/service_test.go`, `roles/limits_test.go`, `api/routes_roles_test.go`, and the web `*.daemon.test.tsx` suites. |
| 2. Plan-first mode (B5.2, N6) | Present (earlier run) | `session/plan.go`, `history/plan.go`, the plan wire types in `protocol/history.go` (+ `protocol/plan_test.go`), `api/routes_plan.go`, web `sync/plan-actions.ts`, `testing/fake-card-plan.ts`. `S8c` reads `Daemon`. | `api/routes_plan_test.go`, `history/plan_test.go`, `sync/plan-actions.test.ts`. |
| 3. Limits, stuck detection, checkpoints (B5.3) | Present (earlier run) | `harness/limits.go`, `harness/stuck.go`, `session/limits.go`, `session/checkpoint.go`, `gitx/checkpoint.go`, `protocol/checkpoint.go`, `api/routes_checkpoints.go`, web `sync/limit-actions.ts`, `sync/checkpoint-actions.ts`, `data/mappers/checkpoints.ts`, `mock/actions/checkpoints.ts`, `views/settings/LimitsSection.tsx`. | `harness/limits_test.go`, `harness/stuck_test.go`, `session/limits_test.go`, `session/checkpoint_test.go` + `checkpoint_internal_test.go`, `gitx/checkpoint_test.go`, `api/routes_checkpoints_test.go`, web `sync/limits.test.ts`, `sync/checkpoint-actions.test.ts`, `ActivityTab.daemon.test.tsx`, `LimitsSection.daemon.test.tsx`. |
| 4. Pull requests and review (B5.4) | **Done** | 4a: `internal/github/github.go` — the `Client` interface and `TokenClient`, a personal-token HTTP adapter over `net/http`; `internal/pullrequest/service.go` — opens a card's branch as a PR against the default branch, records the link, moves the card to In review, idempotent, forge failures translated to plain sentences (401 → "sign in again"); `internal/projects/pullrequest.go` (`SetPullRequest`); `api/routes_pullrequest.go` + `needsPullRequests`; `cmd/marshald` builds it only when a `github` keychain token is saved. 4b: `internal/review/` (`service.go`, `checks.go`) — the Reviewer role runs on every PR. `ChecksReviewer` reads the checks the forge reports (the role's "approve only when every check passes") and a later phase swaps in a model-backed reader behind the same `Reviewer` interface. `Service.Review` requires the card to be In review and to have a PR, posts each comment (`Comment` with an optional `Path`) on the pull request, and sends the verdict back to the worker that wrote the branch as one message; nothing is sent when nothing ran (it waits, never approves); an approval is not sent back. `pullrequest.Open` triggers the review through an injected `After` callback whose failure is logged and never undoes the PR. `api/routes_review.go` + `needsReview`. | `github/github_test.go` (fake `httptest` server: create/read PR, both comment shapes, checks, `APIError`/`errors.Is`, the token is never repeated, validation); `pullrequest/service_internal_test.go` (create+move, idempotence, no-branch refusal, non-GitHub-remote refusal, 401 translation, remote-URL parsing, and the quality-blocking hold added in slice 7); `review/checks_test.go` + `review/service_internal_test.go` (approve when every check passes, fail on a failed check with a comment per failure, wait when nothing ran, worker text and verdict, `originRepository`). |
| 5. Integrator merge queue (B5.5) | **Done** | `internal/gitx/merge.go` — `DryRunMerge` (in-memory `git merge-tree --write-tree`, plus changed files and conflicts), `BackupBranch`, `AddMergeWorktree` (detached, so only the target moves), `MergeInto` (`--no-ff`, `ErrMergeConflict`), `FastForwardRef` (forward-only compare-and-swap on the old tip, `ErrNotFastForward`), `AbortMerge`, and a `runOutput` helper that reads Git exit codes 0/1 as answers. `internal/integrator/service.go` — one card at a time per project (`keyedlock`), dry-run, backup branch, merge in a temp worktree under `<data>/merge/<project>/<card>`, an injectable `Tester` (nil today; Phase 6's local CI supplies one), fast-forward only after tests pass, and `SetNeeds` + a plain sentence with the target untouched on every failure. `api/routes_integrator.go` + `needsIntegrator`; `cmd/marshald` wires it. | `gitx/merge_test.go` against real temporary repositories; `integrator/service_internal_test.go` with fake Git and cards (clean merge marks Done and moves the target, conflict leaves the target and sends the card to Needs, `ErrMergeConflict` aborts, failing tests keep the target and send `ci-failed`, passing tests move it, a non-ready card is refused). |
| 6. Automatic sleep (B5.6) | **Done** | `session/sleep.go` — one 15-second idle sweep started explicitly by `StartSleepWatch` (exported as `Manager.CheckIdle`), the warning (`session state sleep-warning`), the project awake limit that never warns the newest card, and `SetSleepSettings`/`SetAwakeLimits`/`SetResumeMode` (`--resume` overridden by the stored "after a restart" setting before `RestoreAll`). `session/notices.go` — the sleep notice as **runtime state in the manager's memory**, not a table: one reminder per project, `Notice`/`NoticeList`, four actions (`keep-awake`, `dismiss`, `keep-all-awake`, `sleep-all`), a kept-awake card held off the idle timer for the keep-awake length, and a refused sleep clearing the warning. `protocol/notice.go` (`Notice`, `NoticeList` via `NewNoticeList` — array, never null; `DefaultSleepSettings()`); `settings/settings.go` stores sleep settings under key `sleep`; `api/routes_notices.go` (`GET /v1/notices`, `POST /v1/notices/{id}/action`) + `needsNotices`; web `data/mappers/notices.ts`, `data/mappers/sleep-settings.ts`, `sync/notices.ts`, `sync/notice-actions.ts`, `sync/sleep-settings.ts`, `sync/sleep-actions.ts`, `views/settings/sleep-actions.ts`, `testing/fake-notices.ts`, `testing/daemon-notices-store.ts`, `testing/fake-sleep.ts`, `testing/daemon-sleep-store.ts`. `S23` and `S26a` switched to `Daemon`. | `session/sleep_test.go`, `settings/settings_test.go`, `protocol/notice_test.go`, `api/routes_notices_test.go`, web `NoticesPanel.daemon.test.tsx`, `sync/notices.test.ts`, `sync/notice-actions.test.ts`, `sync/sleep-settings.test.ts`, `data/mappers/notices.test.ts`, `data/mappers/sleep-settings.test.ts`, `GeneralSection.daemon.test.tsx`. |
| 7. Quality module (B5.8) | **Done** | Protocol `protocol/smell.go` (+ `smell_test.go`, goldens `smell-finding`, `smell-finding-list`, `smell-profile`, `dismiss-finding-request`); migrations `0016_roles_quality.sql` (`smell_profiles`, `smell_findings`) and `0017_smell_checks.sql` (`smell_checks (card_id, commit_sha, checked_at)` so a check that found nothing is remembered); `store/queries/quality.sql` + generated `db/quality.sql.go`; `internal/quality/` — `service.go` (`Check`, `Findings`, `BeforeReview`, `BlockingFindings`, `Fix`, `Dismiss`, `Profile`, `SetProfile`), `diff.go`, `builtin.go` (the nine families), `lang.go`, `linter.go`, `profile.go`. The new-or-worse rule: a finding blocks only if it is on a line the card added or made worse, resolved at save time. A linter's findings follow the same rule. `Deps.NewLinter` injects the builder so no test starts a process. `Fix` with no worker → `refused` / `reason=quality_no_agent`; `Dismiss` with a blank reason → `invalid_argument` field `reason`. The quality gate lives in **both** move paths (`projects.MoveCard` for the person's drag and `projects.SetState` for the daemon's own move from `pullrequest.Open`) as an interface (`ReviewGate.BlockingFindings`) set after construction via `proj.SetReviewGate(...)`; a check that could not run never refuses a move. New refusal reason `move_quality_blocking` joins `MoveRefusalReasonValues()`; `SmellFindingList.Checked` is a `*Timestamp` so a never-checked card encodes `"checked":null`. `api/routes_quality.go` (findings list/dismiss/fix + profile get/put, all 404 without the bit) + `needsQuality`. **No web code for this slice:** the daemon side is complete, but there is no quality mapper, no sync module, and no smell screen — the screens are **not** drawn (see §9). | `quality/service_test.go`, `quality/internal_test.go`, `protocol/smell_test.go`, `api/routes_quality_test.go` (the five routes at HTTP level: empty list probes the literal bytes `"findings":[]`/`"checked":null`, family/severity/blocking round-trip, findings read as of the card's current commit, dismiss keeps the reason and clears blocking, blank reason 400, fix with no session manager 422 `quality_no_agent` with the finding still open, four not-found cases, `withoutQuality()` → all five 404, `PUT` answered and read back by `reflect.DeepEqual`, six refusal cases each asserting `details.field`, unknown/uppercase project 404), `projects/quality_test.go` (refusal carries `move_quality_blocking` + the verbatim sentence with the card unchanged and one event-free, `SetState`-to-Review refused, other columns not asked, move proceeds when the gate is absent/finds nothing/could not run, `SetReviewGate` replaces the gate), `pullrequest/service_internal_test.go` (a PR opened while a blocking smell holds the card in Working skips the Reviewer and still succeeds). |

## 3. Cutover evidence

Sections switched, with their `docs/backend-checklist.md` §2.2 register row and `apps/web/src/data/sections.ts` entry changed **together** (one switch per section, never half a section):

| Section | 1. Inventory rows built | 2. Daemon tests + goldens | 3. Mapper tests from goldens | 4. Loading/empty/error/offline | 5. End-to-end specs | 6. Mock removed, knip clean | 7. Registers and docs | 8. `pnpm check` + budgets | Switched |
|---|---|---|---|---|---|---|---|---|---|
| S8c Plans | yes (earlier run) | yes (earlier run) | yes (earlier run) | not verified by builder | not run by builder | not verified | yes (earlier run) | **not run by the builder; see CI** | **Yes** |
| S23 Notices | yes | yes (`notice` goldens + `routes_notices_test.go`) | yes (`mappers/notices.test.ts`) | not verified by builder | not run by builder | not verified | **yes (this phase)** | **not run by the builder; see CI** | **Yes** |
| S26a Settings: sleep | yes | yes (`sleep-settings` golden + `settings_test.go`) | yes (`mappers/sleep-settings.test.ts`) | not verified by builder | not run by builder | not verified | **yes (this phase)** | **not run by the builder; see CI** | **Yes** |
| S27 Settings: Roles | yes (earlier run) | yes (earlier run) | yes (earlier run) | not verified by builder | not run by builder | not verified | yes (earlier run) | **not run by the builder; see CI** | **Yes** |

Where a section switched, the mock-mode tests were **pinned explicitly** in their test section tables (reworded or deleted: never), and `MOCK_CARDS_AND_HISTORY` repeats its `S5a`/`S23`/`S26a` pins after both spreads, so a register change cannot silently flip them.

The register move for `S23`/`S26a` is in `docs/backend-checklist.md` §2.2 (`| S23 Notices | … | Daemon |`, `| S26a Settings: sleep | … | Daemon |`) and `apps/web/src/data/sections.ts` (`S23: "daemon"`, `S26a: "daemon"`).

## 4. What I changed

This phase (slices 4–7; slices 1–3 are the earlier run and are listed in §2).

Slice 4a — the forge client and opening a PR:

- `daemon/internal/github/github.go` (new) — the `Client` interface and the `TokenClient` personal-token HTTP implementation.
- `daemon/internal/github/github_test.go` (new).
- `daemon/internal/pullrequest/service.go` (new) — open a card's branch as a PR, record the link, move the card to In review.
- `daemon/internal/pullrequest/service_internal_test.go` (new).
- `daemon/internal/projects/pullrequest.go` (new) — `SetPullRequest`.
- `daemon/internal/api/routes_pullrequest.go` (new) — `POST /v1/cards/{id}/pull-request`.
- `daemon/internal/api/server.go`, `daemon/internal/api/routes.go` — `needsPullRequests`, the `Deps.PullRequests` field, the route, and the `routeNeeds` widening to `uint32` (a 17th bit no longer fits in `uint16`).
- `daemon/internal/api/stack_test.go`, `daemon/internal/api/routes_auth_test.go` — a `withoutPullRequests()` option, the route group, and the register-presence assertion.
- `daemon/cmd/marshald/main.go` — build the pull-request service from the keychain's `github` token, or leave it out.

Slice 4b — the Reviewer role on every PR:

- `daemon/internal/review/service.go`, `daemon/internal/review/checks.go` (new) — the `Service`, the `Reviewer` interface, `ChecksReviewer`, `Target`/`Comment`/`Verdict`/`Request`.
- `daemon/internal/review/service_internal_test.go`, `daemon/internal/review/checks_test.go` (new).
- `daemon/internal/api/routes_review.go` (new) — `POST /v1/cards/{id}/review` and the `needsReview` route bit.
- `daemon/internal/api/server.go`, `daemon/internal/api/routes.go`, `daemon/internal/api/stack_test.go`, `daemon/internal/api/routes_auth_test.go` — the `Deps.Review` field, the route, a `withoutReview()` option, and the register-presence assertion.
- `daemon/internal/pullrequest/service.go` — the injected `After` callback that triggers a review when a PR opens, with the failure logged and the PR never undone.
- `daemon/cmd/marshald/main.go` — build the review service and hand it to the pull-request service.

Slice 5 — the Integrator merge queue:

- `daemon/internal/gitx/merge.go` (new) — `DryRunMerge`, `BackupBranch`, `AddMergeWorktree`, `MergeInto`, `FastForwardRef`, `AbortMerge`, `ErrMergeConflict`, `ErrNotFastForward`, and the `runOutput`/`revParse` helpers.
- `daemon/internal/gitx/merge_test.go` (new).
- `daemon/internal/integrator/service.go` (new) — the merge queue.
- `daemon/internal/integrator/service_internal_test.go` (new).
- `daemon/internal/api/routes_integrator.go` (new) — `POST /v1/cards/{id}/merge`.
- `daemon/internal/api/server.go`, `daemon/internal/api/routes.go`, `daemon/internal/api/stack_test.go`, `daemon/internal/api/routes_auth_test.go` — `needsIntegrator`, `Deps.Integrator`, a `withoutIntegrator()` option, and the register-presence assertion.
- `daemon/cmd/marshald/main.go` — build the merge queue (a nil `Tester` until Phase 6's local CI).

Slice 6 — automatic sleep and notices:

- `daemon/internal/session/sleep.go`, `daemon/internal/session/notices.go` (new) — the idle sweep, warning, awake limit, resume mode, and the in-memory sleep notice with its four actions.
- `daemon/internal/session/sleep_test.go` (new).
- `daemon/internal/protocol/notice.go` + `notice_test.go` (new) / `daemon/testdata/golden/notice.json`, `notice-list.json`, `notice-action-request.json`, `notice-action-result.json`, `sleep-settings.json` (new).
- `daemon/internal/settings/settings.go` + `settings_test.go` (new) — the `sleep` settings key.
- `daemon/internal/api/routes_notices.go` + `routes_notices_test.go` (new) — the two notice routes.
- `daemon/internal/api/server.go`, `daemon/internal/api/routes.go`, `daemon/internal/api/stack_test.go`, `daemon/internal/api/routes_auth_test.go` — `needsNotices`, the `Deps.Notices` wiring, a `withoutNotices()` option, and the register-presence assertion.
- `daemon/internal/session/manager.go`, `config.go`, `restore.go`, `live.go`, `hold.go`, `start.go`, `pump.go`, `send.go` — the sleep watch start, the settings/limits/resume hooks, and `Sleep` accepting `sleep-warning`.
- `daemon/cmd/marshald/main.go` — start the sleep watch and wire the notice routes.
- Web: `data/mappers/notices.ts`, `data/mappers/sleep-settings.ts`, `sync/notices.ts`, `sync/notice-actions.ts`, `sync/sleep-settings.ts`, `sync/sleep-actions.ts`, `views/settings/sleep-actions.ts`, `testing/fake-notices.ts`, `testing/daemon-notices-store.ts`, `testing/fake-sleep.ts`, `testing/daemon-sleep-store.ts`, `app/notices/NoticesPanel.daemon.test.tsx`, and the mapper/sync/`GeneralSection.daemon` test files (all new or extended).
- `apps/web/src/data/sections.ts` and `docs/backend-checklist.md` §2.2 — `S23`/`S26a` → `Daemon`.

Slice 7 — the quality module:

- `daemon/internal/protocol/smell.go` + `smell_test.go`, and the four smell goldens (new).
- `daemon/internal/store/migrations/0017_smell_checks.sql` (new); `0016_roles_quality.sql` extended for `smell_profiles`/`smell_findings`.
- `daemon/internal/store/queries/quality.sql` + generated `daemon/internal/store/db/quality.sql.go`.
- `daemon/internal/quality/{service,diff,builtin,lang,linter,profile}.go` + `service_test.go`, `internal_test.go` (new).
- `daemon/internal/projects/quality.go` (new) and `service.go`/`move.go`/`cardstate.go` — the `ReviewGate` interface, `SetReviewGate`, and the gate called in both move paths.
- `daemon/internal/api/routes_quality.go` (new) + `routes.go`/`server.go`/`stack_test.go`/`routes_auth_test.go` — the `needsQuality` bit, the five routes, a `withoutQuality()` option, and the register-presence assertion.
- `daemon/internal/api/stack_test.go` — a **real defect fixed**: `withoutSessions()` handed `quality.New` a typed-nil `*session.Manager` in its `Worker` interface, so the field was non-nil, the "no agent" refusal was unreachable, and a call panicked (500). The worker is now set only when `st.mgr != nil`, with a comment explaining the typed-nil trap.
- `daemon/cmd/marshald/main.go` — build the quality service and set it as the projects module's review gate.
- Docs: `docs/architecture.md` §6.1 gained refusal **order 8** (`move_quality_blocking`, the exact sentence) plus why it runs last and only asks another module; §11.1's route table gained the four quality rows; §17 gained **§17.4 "The wire"** for the five quality routes; `docs/ui-rules.md` §14 gained the refusal-sentence bullet naming `move_quality_blocking` and `quality_no_agent`.

A correctness fix made along the way (not a weakening): `resolveProfile` was dropping the `Linters` field; the pass-through was restored.

## 5. Verification results

Run from `daemon/` in the final pass of this phase (fast checks only, per owner rule 1d):

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `gofmt -l internal cmd` — flagged `internal/settings/settings_test.go`; `gofmt -w` applied; now empty.
- `go test -count=1 ./internal/quality/ ./internal/pullrequest/ ./internal/protocol/ ./internal/settings/` — all ok (1.903s / 1.564s / 1.011s / 1.591s).
- `go test -count=1 -run 'Quality|ReviewGate' ./internal/projects/` — ok (2.048s).
- `go test -count=1 -run 'TestARouteIsRegisteredOnlyWhenItsServiceIsThere|TestEveryRouteRequiresAToken|TestNoDomainRoutesWithoutAStore' ./internal/api/` — ok.
- `packages/protocol`: `pnpm exec vitest run --no-coverage` — 12 files / 69 tests passed; `pnpm exec tsc --noEmit -p packages/protocol` — clean.
- Earlier in the phase, per slice, `go test` was green for `internal/github`, `internal/pullrequest`, `internal/integrator`, `internal/gitx`, `internal/projects`, `internal/review`, `internal/session`, `internal/api` (route/auth/registration), and the web suites named in §2.

**Rule 1c end-of-phase graph check (run this phase).** `build_or_update_graph_tool` (incremental, base `0562fc3`) → "309 files re-parsed, 952 nodes and 7777 edges updated", `fts_indexed: 5003`, no errors. `detect_changes_tool` (base `0562fc3`, minimal) → **143 changed files, 344 changed functions, 0 affected flows, 234 test gaps, overall risk score 0.60**. This report is the record of that summary, as rule 1c asks:
- **Untested names it named:** `roleQuery`, `routeMethods`, `cardMethods`, `toStoredCard`, `approvePlan`. (`roleQuery` is a SQL query constant reached through a tested service method; the other four are small mapping/dispatch helpers on already-covered paths. None is a silent gap in a tested behaviour.)
- **Review priorities it named:** `roleQuery`, `readOpenCard`, `SessionSettings`. `readOpenCard` and `SessionSettings` are on the review surface and are covered by the `internal/review` and `internal/settings` tests above; `roleQuery` as above.

**Not run by the builder** (owner rule 1d; CI runs these on the pull request): the full gate (`node scripts/check.mjs`), the whole-repo Go suites, `go test -race ./...`, the slow whole-package suites (`internal/api`, `internal/projects`, `internal/session`), `pnpm build`, `pnpm budgets`, the smell budget, knip/jscpd, `pnpm test:e2e`, and anything that starts a process (the daemon, the stub-agent smoke, `scripts/agent-smoke.mjs`, `scripts/check-generated.mjs`, `scripts/coverage.mjs`). No browser was opened and nothing was signed in to.

## 6. Rulings

Slices 1–3 (earlier run, recorded here so this file is self-contained):

- **Limit semantics.** Ceilings come from the role a card names, read at run time; only a ceiling somebody set is enforced; time is checked after the turn ends; cost is `SumUsageForCard / 1e6` rounded down; rounds are the count of `KindUser` session events; priority is time → cost → rounds.
- **Stuck semantics.** Only a failed tool call (failure text, whitespace-collapsed, cut to 200 chars) and an edit (key = path) count; three identical signals **in a row**; a stopped card stays awake.
- **Checkpoint semantics (design ruling; no prototype exists).** A checkpoint is a real commit on the card's branch kept on `refs/marshal/checkpoints/<id>`, newest 20; restore is `reset --hard` + `clean -fd`; refused while a turn runs (`422`, `details.reason = restore_turn_running`); conversation "restore" is a boundary, not a rewind; checkpoints live in the existing S10 Activity tab — no new cutover section.

Slices 4–7 (this phase):

- **The Phase 5 / Phase 6 GitHub boundary.** One `github.Client` interface; Phase 5 ships `TokenClient` (a personal-token HTTP adapter over `net/http`), and Phase 6's GitHub App goes behind the same interface with no change to any caller. `internal/pullrequest` and `internal/review` depend on the interface, never on `*TokenClient`. **Deviation from the prompt:** the prompt named `google/go-github` and `golang.org/x/oauth2`; `go-github` is not in this machine's module cache and adding it needs network the owner did not grant, so the token client is written directly on `net/http`. A personal token needs no OAuth flow (it is a bearer header), and the interface keeps `go-github` swappable behind `TokenClient` later.
- **A GitHub token with no connection screen.** `cmd/marshald` reads a `github` keychain entry and builds the pull-request and review services only when one is present; otherwise the routes do not exist and nothing is pretended. The screen that saves the token (and its connection test) is Phase 6's 6.8/6.11.
- **Review semantics.** The Reviewer reads a *pull request*, so the card must be In review; nothing ran ⇒ wait, never approve; an approval is **not** sent to the worker; the review is triggered inside `pullrequest.Open` via an injected `After` callback whose failure is logged, never undoes the PR; shared remote parsing lives in `github`.
- **Slice 6 decisions.** The sleep notice is runtime state in the manager's memory, not a table; one reminder per project; every call that takes a card off a notice also holds it off the idle timer for the keep-awake length; only the **sleep** notice kind is produced in Phase 5; sleep settings live in the `settings` table under key `sleep`; the idle timer is one 15-second sweep started explicitly by `StartSleepWatch` and exported as `Manager.CheckIdle`; a warned card's state is `sleep-warning`; `Sleep` accepts `sleep-warning` as awake; a card is warned only if `Sleep` would accept it; a refused sleep clears the warning; the "after a restart" setting overrides `--resume` via `SetResumeMode` before `RestoreAll`; the notice-action body is one call with four action names, and an unknown action is `invalid_argument`; `GET /v1/notices` is wrapped in `protocol.NewNoticeList` (array, never null); dismiss/keep-all on a gone notice answers 0, not 404; the defaults live only in `protocol.DefaultSleepSettings()`.
- **Slice 6 web decisions.** The daemon names notice cards by opaque ids, so `toNotices` takes a `keyOf` lookup and drops a sleep group left with no known card; sleep settings keep the daemon's wire value for an unknown channel/restore; `saveSleepSettings` sends the whole record; a notice's four buttons prefer the daemon path (`S23`) over `card-hold` (`S7c`) over the mock; `noticesSyncer` is ordered after `cardsSyncer`; the notice/sleep store types live in `data/mappers/` and are re-exported by `mock/types.ts`.
- **Slice 7 decisions.** `smell_checks (card_id, commit_sha, checked_at)` is stored in `0017` so a check that found nothing is remembered; `smell_findings.family` is one of the nine families and `smell` is the rule's own name; severity is resolved at save time; `quality.checked` carries `SmellCheckedEventData`; `BeforeReview` is advisory; a linter's findings follow the same new-or-worse rule; `Profile`/`SetProfile` take a project id; `Deps.NewLinter` injects the builder so tests start no process; `addedLines` treats a card-added file as entirely its own; `CountSmellFindingsForCardCommit` was deleted rather than kept as dead SQL.
- **Slice 7 wiring decisions.** The quality gate lives in **both** move paths (`projects.MoveCard` and `projects.SetState`); it is an interface on the **projects** side set after construction (`proj.SetReviewGate(...)`); **a check that could not run never refuses a move**; the refusal sentence and its ownership stay in `projects` while `quality.BlockingFindings` answers only a count; the new reason is `move_quality_blocking`; when a PR opens and the card is held in Working by a blocking smell, `pullrequest.Open` skips the Reviewer entirely and still returns success; `SmellFindingList.Checked` is a `*Timestamp` so a never-checked card encodes `"checked":null`; the findings read route runs nothing.
- **Duplication scope.** The codebase map that would compare a card's code with the rest of its repository is Phase 7's, so slice 7's built-in duplication is judged **within the card's own new code**. Stated here rather than left implicit.
- **Cutover testing decision.** When a section switches, mock-mode tests are pinned explicitly in their test section tables (never reworded or deleted), and `MOCK_CARDS_AND_HISTORY` repeats its `S5a`/`S23`/`S26a` pins after both spreads so a register change cannot silently flip them.
- **The typed-nil interface defect.** A typed-nil concrete pointer in an interface field (the test stack passing `st.mgr` when there is no manager) is a real defect, not a test-only annoyance: it made an intended refusal unreachable and turned it into a panic. Fixed in the harness, not papered over.

## 7. Found, not fixed, and later phases

- **`S8b` is not switched.** A stored chat approval block has no approval id and no route lists a card's waiting approvals, so that half of the approvals surface stays on the mock.
- **A watcher is not built.** `POST /v1/cards/{id}/merge` is the entry point; something that picks up every card that becomes Ready to merge does not exist yet.
- **After a merge the card's own worktree and branch are left in place** (task 5.11 cleanup is not built).
- **The merge queue's `Tester` is nil in `cmd/marshald`.** A clean merge moves the target forward; the `Tester` that makes the queue wait for a real test run is Phase 6's local CI (build-plan 6.5).
- **`TokenClient` talks to `api.github.com` only.** A GitHub Enterprise base URL is supported through `WithBaseURL` but is untested against a real Enterprise server.
- **`ListChecks` feeds the Reviewer now, but a CI monitor does not.** The monitor that polls checks to gate a merge is Phase 6.
- **Reset-to-starter semantics mismatch (decided, needs recording in the design docs).** The UI confirm copy says "This replaces your edits … with the starter template" but the code only clears the per-project override. The copy and the code must be reconciled in a later pass; recording it here so it is not lost.
- **A second copy divergence to record.** The prototype's restore dialog promises a backup ref; the daemon keeps none, so the daemon-path copy says the change is discarded.
- **Activity rows cannot carry a result unless the record is a tool call** — a mock/daemon divergence to record.
- **Doc gaps still to record.** `plan.updated` is named in `backend-inventory.md:107` but missing from `architecture.md` §11.2; the `B5.x` rows live in `backend-checklist.md` §5 (line 407 for B5.8), not §4; §7 still owes the checkpoint-restore semantics and the GitHub-client boundary as written-up prose; `architecture.md` has no notice **wire contract** (only a `notices` table row) and `notice.dismissed` still needs recording in §11.2; and §11.1's route table still lacks rows for the Phase 5 pull-request, review, merge, notices, and sleep-settings routes (only the quality rows were added this pass). These are documentation follow-ups, not code gaps.
- **Maintenance gaps.** `openai-go v1.12.0` is behind; `pricing.go`'s numbers are unverified against published list prices.
- **One unexplained Phase 4 failure:** `internal/api`'s `TestAReconnectingClientAsksForTheScreenInsteadOfBeingReplayedOutput` (`terminal_test.go:592`, ~15.34s) failed once and was not diagnosed. Recorded here because it touches the same test stack.
- **A duplicated module** — both `apps/web/src/mock/index.test.ts` (tracked, modified) and an untracked copy exist; only the untracked one runs.
- **An unresolved instruction conflict** (Phase 3 / Phase 5): rule 1d forbids `node scripts/check.mjs`, while §5/§6 of the phase prompt say to run the full gate once at the end of the phase. The recorded reading is **1d wins**; CI runs the full gate.
- **The two required reviews of Phase 3's `security`/`harness` cannot both come from the builder.**

## 8. Needs the controller

Every check that needs a browser, a real sign-in, or a privileged action goes here (the builder never opens a browser: owner rule 1a).

- **Sign in to GitHub for real** (checklist section 3) and save a personal token so the `github` keychain entry, `POST /v1/cards/{id}/pull-request`, and `POST /v1/cards/{id}/review` can be exercised against a throwaway repository. Nothing in this phase pushed anywhere real, opened a real PR, or signed in.
- **Tick the `docs/backend-checklist.md` §5 `B5.x` rows** and move the `docs/progress-tracker.md` rows (controller-only; the builder does not tick these).
- **Run the full gate** (`node scripts/check.mjs`), `pnpm build`, budgets/smells, knip/jscpd, and `pnpm test:e2e` on the pull request as CI.
- The hands-on pass for `S23`, `S26a`, `S8c`, and `S27`.
- The owner's sign-off on the two screens named in §9.

## 9. Design-doc rows I added or propose

- **Added this pass:** `docs/architecture.md` §6.1 refusal order 8 (`move_quality_blocking`), §11.1's four quality route rows, §17.4 "The wire"; `docs/ui-rules.md` §14's quality refusal sentence. `docs/backend-checklist.md` §2.2's `S23`/`S26a` rows and `apps/web/src/data/sections.ts` moved together.
- **Needs the owner's sign-off before they are drawn:** the **checkpoints** screen (build-plan 5.20 — its backend exists from slice 3) and the **smell-findings** screen (build-plan 5.21). Neither has a prototype to extract from, so the daemon-path behaviour is written up (§17.4) but the screens are not drawn.

## 10. Confirmation of the hard rules

- No mutating git command was run; only `status`, `diff`, `log`, `show`, `ls-files`, and `blame` were read. `HEAD` is unchanged at `0562fc3`.
- No browser, preview, simulator, or `pnpm test:e2e` was used.
- No real GitHub sign-in, no real pull request, and no push anywhere; the GitHub client and the Reviewer are tested against a fake HTTP server and fake doubles.
- No file was deleted; no document was created beyond this report and what the slices need.
- No generated file was edited by hand (`db/quality.sql.go` and the protocol bindings came from `sqlc`/`gen-protocol`).
- Fast checks only: `go build`, scoped `go test`, `gofmt`, `go vet`, the protocol `tsc`/vitest. Everything heavier is left to CI.
