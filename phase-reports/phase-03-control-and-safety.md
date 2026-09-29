# Phase 3 report: control and safety

One evolving file. Update it after every slice, and at the end of every run on this phase, even a stopped one. Never replace it with a fresh one — append and revise in place.

_Last updated: 2026-09-28, a further run on S8b. Slices 1–5 are Done. This run fixed the two daemon prerequisites section 3 recorded (the approval id on the read path, and rewriting the stored row's state on resolve) and cut the web app's S8b over from mock to daemon for the card and Home halves; the project-chat half of S8b is not done and is recorded honestly in sections 3 and 7 below. No check, test, or build was run by this run (rule 1d/constraint of this pass) — see section 5's new rows and section 8 for exactly what the controller must verify first._

## 1. Summary

Slices 1–5 are **Done** and verified. **Slice 1 (approvals**, B3.4/N7): the session pump records a permission request, holds the session at `waiting-approval`, and announces it; `Manager.Respond` delivers the person's answer to the agent, records the decision, audits it, and announces `approval.resolved`; `POST /v1/approvals/{id}` exposes it; migration `0013` adds the `approvals` and `audit_log` tables. **Slice 2 (bypass mode**, B3.2/N8): bypass *is* `permissionMode: "bypass"`, set through `POST`/`DELETE /v1/cards/{id}/bypass` with a typed acknowledgement, a project lock that refuses turning it on, containment rules in `gitx`, auto-answered permission requests, and `bypass.on`/`bypass.off` audit rows; the web S7b section was cut over. **Slice 3 (permission modes and the safety rules**, B3.1/B3.3/B3.7): two new packages — `internal/security` (profile, command blocklist, deploy rule) and `internal/harness` (the decision that puts the mode, the profile, the blocklist, the deploy rule, and the card's worktree together) — plus the session wiring that consults them. **Slice 4 (secret scanner and the audit log**, B3.5): a new `internal/secrets` package wraps gitleaks and never keeps the matched text; the session reads every commit an agent made at the end of each turn and stops the card (`needs` with reason `secret-detected`) the moment one holds a credential, holding the queued message with it; and a new `internal/auditlog` service with read-only `GET /v1/audit`, `/v1/audit/search`, and `/v1/audit/export` (JSON or CSV) routes exposes the trail. **Slice 5 (mid-session model and thinking switch**, part of B3.6): a running session reads the card's current model, thinking mode, and permission mode at the start of every turn and hands a change to the agent through an optional `agents.SettingsApplier` (which the ACP adapters implement), and each changed setting is written once into the card's history as a system note that serves both the chat and the activity list. The phase's write path is complete; the phase's remaining step is the web **S8b** switch, which is built but not flipped, with the reason and the two blocking daemon gaps in section 3. The `Approval`/`DecideApprovalRequest`/approval-event goldens the cutover will need are added this run.

Slice 3 is the phase's high-risk surface and has had **two independent review rounds**; the second round's findings are all addressed except one conservative-direction limitation recorded in section 7. The prompt requires **two reviews of `security`/`harness`**, and the builder cannot supply both — this is called out in section 8.

**This run (2026-09-28): the S8b cutover.** The two daemon gaps section 3 recorded against S8b are both fixed: `ChatApproval` (and `NeedsReason`) carry the approval's own id now, `history.RecordsOf` no longer writes the approval's history row before an id exists (`history.AppendApproval` does, explicitly, from `holdPermission` and the harness auto-answer path), and new `history.ResolveApproval`/`ResolveChatApproval` calls rewrite the stored row's state when the approval is answered. Separately, nothing moved a card to Needs you for a waiting approval at all before this run — a gap the phase report's own section 3 undersold as "no id on the read path" when the real gap was "no path at all" — so `holdPermission` now calls `projects.SetNeeds` and `Respond`/`withdrawApprovals` call a new `clearApprovalNeeds` to move the card back. The web app's card and Home halves of S8b are cut over (`data/sections.ts`'s `S8b: "daemon"`); the project-chat half is not — see section 3's revised bullet and section 7. Design rulings for both daemon fixes are new entries in section 6. Nothing was run: see section 5's new row and section 8.

## 2. Slice results

| Slice | Status | What it built | Tests | Evidence |
|---|---|---|---|---|
| 1 — approvals, through the path that already works (B3.4, N7) | **Done** | See section 4 | `internal/session/approval_test.go` (3), `internal/protocol/opaque_id_test.go` (2), `internal/api/routes_approvals_test.go` (1) | Section 5 |
| 2 — bypass mode (B3.2, N8) | **Done** | See section 4 | `internal/projects/bypass_test.go`, `internal/session/bypass_test.go`, `internal/api/routes_bypass_test.go`, `internal/gitx/containment_test.go`, `internal/protocol/bypass_test.go` | Section 5 |
| 3 — permission modes, profiles, blocklist, deploy rule (B3.1, B3.3, B3.7) | **Done** | See section 4 | `internal/security/security_test.go`, `internal/harness/harness_test.go`, `internal/session/harness_test.go` | Section 5 |
| 4 — secret scanner and audit log (B3.5) | **Done** | See section 4 | `internal/secrets/secrets_test.go` (5), `internal/gitx/log_test.go` (6), `internal/session/secret_test.go` (2), `internal/api/routes_audit_test.go` (6), `internal/protocol/audit_test.go` (3 goldens) | Section 5 |
| 5 — mid-session model and thinking switch (part of B3.6) | **Done** | See section 4 | `internal/session/settings_test.go` (7), `internal/agents/acp/fake_test.go` (3 new live-change tests), `internal/api/routes_card_settings_test.go` (2) | Section 5 |

## 3. Cutover evidence

- **S7b (bypass in the web app) — cut over.** `packages/web/src/data/sections.ts` now has `S7b: "daemon"` and the register row was moved with it, in the same change. The mock dispatch for bypass lives behind `isDaemon("S7b")` in `packages/web/src/mock/marshal.ts`, so the daemon path serves it.
- **S8b (approvals in the web app) — the card and Home halves are cut over; the project-chat half is not.** This run (2026-09-28) fixed both daemon gaps this section used to record:
  1. **The stored approval block now carries the approval's own id.** `protocol.ChatApproval` gained an `ID` field, `internal/history/agents.go`'s `RecordsOf` no longer writes the approval's history row at all (see section 6's ruling on why the automatic write had to go, not just gain a field), and a new `history.AppendApproval`/`AppendChatApproval` write it explicitly, with the id, from `internal/session/approval.go`'s `holdPermission` once the id exists and from the harness auto-answer path (already resolved, with no id, since nobody is going to answer it). A new `history.ResolveApproval`/`ResolveChatApproval` rewrite the stored row's `state` when the approval is answered (`Respond` and `withdrawApprovals` both call it), found by paging the owner's own approval events and matching the id inside the stored detail — not a new column, not `json_extract`; see section 6's ruling for why. So a chat opened *after* the request was made now shows the right buttons, and a chat left open across an answer given elsewhere updates without a reload.
  2. **Home can now find a card's waiting approval without opening it.** The real gap was bigger than "no id on the read path": nothing moved a card to Needs you for a waiting approval at all, so it was never on Home's needs-you list to begin with. `holdPermission` now calls `projects.SetNeeds` (kind `approval-needed`), and `protocol.NeedsReason` gained an `ApprovalID` field, populated by `session.StoredStates.CardSession`/`ProjectSessions` from a fresh read of the `approvals` table (never stored on the card row, so it cannot go stale once the approval is answered) and carried onto the card by `projects.withSessionInfo`. `Respond` and `withdrawApprovals` both call a new `clearApprovalNeeds` to move the card back to `working` once its approval is answered. Section 6 has both design rulings in full.
  The web app is cut over for what these two fixes serve: `data/mappers/card.ts` maps `Card.approvalId`, `sync/chat-mapper.ts` maps `ChatApproval.id`, a new `sync/approval-actions.ts` calls `POST /v1/approvals/{id}` and applies the live `approval.requested`/`approval.resolved` pair to an open card's chat, `mock/actions/approvals.ts`'s `approve`/`deny` and `views/home/needs.ts`'s `canApproveHere` are daemon-aware the way bypass and plans already are, and `testing/fake-approvals.ts` gives the component-test suite a fake `POST /v1/approvals/{id}`. `data/sections.ts` now has `S8b: "daemon"`, moved with the `docs/backend-checklist.md` §2.2 register row in this same change, and `mock/testing/twin.ts` pins S8b to `mock` the way it already pins S7b and S8c.
  **What is not done: the project-chat half.** B3.4's own first sentence is "one approval row shared by the card and any project chat that asked" — the daemon side of that is real (`AppendChatApproval`/`ResolveChatApproval`, tested in `internal/history/approvals_test.go`'s last case), but nothing on the web side resolves a *chat's* own topic for a live approval event (`sync/approval-actions.ts`'s `cardKeyOf` only ever finds a card), and the mock's own cross-chat mirror (`syncProjectApproval` in `mock/actions/approvals.ts`) has no daemon-backed twin. A person answering from a project chat, or watching one live, is not covered. This is why B3.4's checkbox stays unticked — section 1 and section 8 say so plainly.
- **The audit screen is not built, and was not cut over.** `docs/backend-inventory.md` §6 and build-plan 3.13 list it as needing design work before it exists, so Slice 4 built the read routes and nothing above them.
- Gate item 8 (`pnpm check` and budgets) is not run by the builder; see CI and section 8.

## 4. What I changed

### Slice 1 — approvals

New files (8, all in the daemon):

- `internal/store/migrations/0013_approvals.sql` — the `approvals` table (id, session_id, request_json, decision, decided_by; architecture §10) with a partial index on pending rows, and the `audit_log` table (id, session_id, actor, action, target, detail_json, created_at) with two indexes. Forward only, STRICT, UTC-ms times.
- `internal/store/queries/approvals.sql`, `internal/store/queries/audit.sql` — `CreateApproval`, `GetApproval`, `DecideApproval` (guarded so a second answer changes nothing), `InsertAuditLog`, `ListAuditLog`. Regenerated `internal/store/db/{approvals,audit}.sql.go` and `models.go` with sqlc v1.31.1.
- `internal/protocol/approvals.go` — the wire types `Approval`, `ApprovalOption`, `ApprovalDecision` (+ values/valid/state), `DecideApprovalRequest`, `ApprovalRequestedEventData`, `ApprovalResolvedEventData`.
- `internal/audit/audit.go` — the append-only audit recorder (`Recorder.Record`, `LogAndForget`, actor and action constants), shared by the slices that follow.
- `internal/session/approval.go` — the in-memory pending-approval registry, `onPermissionRequested` (record, hold, announce), `Respond` (answer, record, audit, announce, restore), `withdrawApprovals`, `chooseOption`, `wireApprovalOptions`.
- `internal/api/routes_approvals.go` — `POST /v1/approvals/{id}`.
- Tests: `internal/session/approval_test.go`, `internal/protocol/opaque_id_test.go`, `internal/api/routes_approvals_test.go`.

Changed files:

- `internal/session/pump.go` — added `case agents.PermissionRequested: m.onPermissionRequested(ls, e)`, replacing the "approvals are Phase 3 (B3.4), which does not exist yet" marker; `finishPump` now withdraws a session's waiting approvals.
- `internal/session/manager.go` — `audit` field and an `approvals` map on `Manager`; `NewManager` builds an `audit.Recorder` when the config has none.
- `internal/session/config.go` — `Config.Entropy` (default `crypto/rand.Reader`) and `Config.Audit` (default built on the store).
- `internal/protocol/opaque_id.go` — `IDTime`, which reads an id's creation time back (so an approval reports when it was asked without a `created_at` column).
- `internal/protocol/enums_test.go`, `packages/protocol/test/enums.test.ts`, `daemon/testdata/golden/enums.json` — registered the `ApprovalDecision` enum and regenerated the golden and the TypeScript types (`node scripts/gen-protocol.mjs`).
- `internal/session/fake_agent_test.go` — the fake agent now really answers approvals (`Respond` records the choice, `ask` emits a request).
- `internal/api/routes.go`, `internal/api/routes_auth_test.go` — registered the route, and added it to the session group the route-coverage test checks by hand.

### Slice 2 — bypass mode

New files:

- `internal/gitx/containment.go` — the containment a card's worktree carries (`NewContainment`, `Containment`, `CheckWorktrees`, `CheckCommand`): a path outside the worktree, and a Git command that would leave the worktree or touch the main branch, each with a stable rule name.
- `internal/protocol/bypass.go` — the wire types `BypassRequest{Acknowledged bool}` and `BypassState`. Golden `internal/…/testdata/golden/bypass-request.json`.
- `internal/projects/bypass.go` — `SetBypass`/`ClearBypass`: the typed-acknowledgement refusal (`reason: "unacknowledged"`), the project-lock refusal (`reason: "locked"`), and the mode change.
- `internal/session/bypass.go` — the bypass auto-answer path in the session pump, plus `containmentOf`.
- `internal/api/routes_bypass.go` — `POST` and `DELETE /v1/cards/{id}/bypass`.
- Tests: `internal/projects/bypass_test.go`, `internal/session/bypass_test.go`, `internal/api/routes_bypass_test.go`, `internal/gitx/containment_test.go`, `internal/protocol/bypass_test.go`.

Changed files:

- `internal/projects/{update.go,cards.go,fork.go}` — a locked project refuses a card edit; a new card and a fork both refuse bypass, the same refusal closing all three doors.
- `internal/session/approval.go` — a permission request consults bypass first, so a bypassed card is answered by the daemon (actor `daemon`) rather than the person.
- `internal/audit/audit.go` — added the `ActionBypassOn`/`ActionBypassOff` actions.
- Web: `packages/web/src/data/api-client.ts`, `data/mappers/card.ts`, `sync/cards.ts`, `sync/card-actions.ts`, `mock/marshal.ts` — the daemon path for bypass, behind `isDaemon("S7b")`; `data/sections.ts` moved `S7b` to `daemon` in the same change.

### Slice 3 — permission modes and the safety rules

New files:

- `internal/security/action.go` — what a request would do (`Action`, `Classify`), the two readings of a command line (`Words` for the blocklist, `Argv`/`ArgvTail` for argument shape), the segment splitter (`Segments`/`splitOperators`), shell-script expansion (`shellScript`, `envScript`), and the "cannot be read" test (`Substitute`, `Readable`, `namesByVariable`).
- `internal/security/blocklist.go` — the blocklist and its rules (`DefaultBlocklist`, `Check`, `Rule*`, `matchForkBomb`…`matchProductionCredentials`, `wideDeleteTarget`, `foldExpansion`, `statementArguments`, `destructiveStatement`).
- `internal/security/profile.go` — the permission profile (`Profile`, `DefaultProfile`, `NothingAllowed`, `Check`).
- `internal/security/deploy.go` — the deploy rule (`DeployWorkflow`, `deploysInSegment`).
- `internal/harness/harness.go` — `Decide`, `Config`, `Request`, `Decision`, `Outcome`, `ContainmentBreach`, `InsideWorktree`, `GitCommandBreach`, `RuleMode`, `RuleBypass`, `RuleUnreadable`.
- `internal/session/harness.go` — builds a `harness.Config` from the session's card (mode, profile, blocklist, worktree), failing closed when a lookup fails.
- Tests: `internal/security/security_test.go`, `internal/harness/harness_test.go`, `internal/session/harness_test.go`.

Changed files:

- `internal/gitx/containment.go` — `trimRef` strips only a real remote prefix (so `feature/main` is not read as `main`); read-only escape hatches (`readsOnly`, `refValueFlags`) so `git branch --contains main` and `git tag -l main` are not false positives; `globalTakesValue` gained `--attr-source` and `--super-prefix`.
- `internal/session/{config.go,manager.go,approval.go}` — the harness is consulted for a permission request, and its decision (allow/ask/deny) is applied and audited.
- `internal/session/{helpers_test.go,approval_test.go,bypass_test.go,harness_test.go,internal_test.go}` — test harness for the new path.
- `internal/store/migrations/0013_approvals.sql`, `internal/store/queries/audit.sql`, `internal/store/db/audit.sql.go` — `ListAuditLog` now orders by `rowid DESC` (true write order), because rows written in the same millisecond share a `created_at`; see section 6.

### Slice 4 — secret scanner and the audit log

New files:

- `internal/secrets/secrets.go` — the gitleaks wrapper (`Scanner`, `New`, `Scan`, `Finding`). The detector is built lazily behind a `sync.Once`, its `Redact` is raised to 100, and gitleaks' own zerolog output is disabled once before the detector is built. `Finding` carries only the rule, its description, the file, and the line — **never the matched text**.
- `internal/secrets/secrets_test.go` — five tests: a known key and its rule name, a private key and a GitHub PAT, the property that a finding never holds the secret, ordinary work left alone, and shared use from many goroutines. All keys are synthetic (the AWS key is Amazon's own documented example).
- `internal/gitx/log.go` — reading the commits an agent made: `Commit{SHA, Subject}`, `CommitsOnBranch(ctx, dir, base)` (`git log --reverse --format=%H\x1f%s base..HEAD`, an empty base refused), `parseCommits`, `ChangedPaths(ctx, dir, sha)` (`diff-tree --name-only --diff-filter=d --root`), `FileAtCommit(ctx, dir, sha, path)`, and `safePath`.
- `internal/protocol/audit.go`, `internal/protocol/audit_test.go` — the wire types `AuditEntry` and `AuditExport`, with goldens `audit-entry`, `audit-entry-list`, and `audit-export`.
- `internal/auditlog/service.go` — the read service over the audit log: `Deps`, `New`, `WithClock`, `WithLogger`, `Cursor{CreatedAt, ID}`, `Page{Items, Next, More}`, `Entries(ctx, query, cursor, limit)` (asks for `limit+1` to learn `More`), `Export(ctx, query)`, and the constants `DefaultPageSize` (50), `MaxPageSize` (200), `MaxExport` (10000).
- `internal/session/secret.go` — the scanner's place in a session: `scanTurnForSecrets(ls) bool`, `unscanned`, `scanCommit`, and `blockCommitForSecret` (audit row `commit.blocked`, `projects.SetNeeds` with `NeedsReasonKindSecret`, a system note, and a warning log).
- `internal/session/secret_test.go` — two tests: a commit holding a credential stops the card, is audited without the value, and leaves a system note (also without the value); and the message queued behind that commit is never delivered.
- `internal/gitx/log_test.go` — six tests for the commit-reading functions, including the empty-base refusal and the `--root` first-commit case.
- `internal/api/routes_audit.go` — `GET /v1/audit`, `GET /v1/audit/search` (requires `q`), and `GET /v1/audit/export` (`format` `json`/`csv`, 400 otherwise). `writeAuditCSV` sets the download headers, and `csvField` prefixes a cell that begins with `=`, `+`, `-`, `@`, tab, or CR with an apostrophe.
- `internal/api/routes_audit_test.go` — six tests: newest-first list, paging without repeating a row, search narrowing plus the empty-search 400, export total and shape, the CSV download, and the formula guard.

Changed files:

- `internal/protocol/enums.go` — added `NeedsReasonKindSecret` (`"secret-detected"`).
- `internal/audit/audit.go` — added `ActionCommitBlocked` (`"commit.blocked"`).
- `internal/projects/cardstate.go` — added `(*Service).SetNeeds(ctx, id, reason)`: validates the kind, writes state and reason in one write, and publishes `card.updated` or `card.moved`.
- `internal/session/{config.go,live.go,pump.go}` — `Config.Secrets` (defaulted to `secrets.New()`), the pump-goroutine-only `scannedThrough` watermark on a live session, and the `onTurnEnded` call that reads the turn's commits and releases the session without delivering the next queued message when a commit is blocked.
- `internal/store/queries/audit.sql` (+ regenerated `internal/store/db/audit.sql.go`) — `ListAuditLogPage`, `SearchAuditLogPage`, `ExportAuditLog`, `SearchAuditLogExport`, all cursored on `(created_at, id)`; see the ruling in section 6.
- `internal/api/{server.go,routes.go}`, `internal/api/{stack_test.go,routes_auth_test.go}`, `cmd/marshald/main.go` — wired the service, registered the three routes, and taught the test stack to build without it (`withoutAudit`).
- `daemon/go.mod`, `daemon/go.sum` — pinned `github.com/zricethezav/gitleaks/v8 v8.22.1`.
- `internal/protocol/enums_test.go`, `daemon/testdata/golden/enums.json`, `packages/protocol/src/generated/index.ts`, `packages/protocol/test/enums.test.ts` — registered the new enum value and regenerated the golden and the TypeScript types.

### Slice 5 — mid-session model and thinking switch

New files:

- `internal/session/settings.go` — the live-settings path: `applyCardSettings(ctx, ls)` reads the card each turn, hands a change to the running agent when it can take one, and never fails the turn; `settingCalls` (the five settings the chat speaks about — agent, role, model, thinking mode, permission mode; bypass is excluded because it has its own call and its own note), `NoteSettingsChanged(ctx, before, after)` (one `history.KindSystem` + `StateOK` record per changed setting), `settingRecords`, and `settingSentence`. `ErrUnsupportedSetting` is recorded so a value the agent will never offer is not re-sent; any other error is *not* recorded, so a failure that could be passing is retried.
- `internal/session/settings_test.go` — seven tests: a change while the session runs reaches the next turn; settings the session already has are not handed over again; a value the agent cannot take is offered once, not every turn; a change that could be passing is retried; the change is written into the card's history (checked as both a chat message and an activity row); nothing is written when nothing changed or when the card has no session; and a cleared setting is said too.
- `internal/api/routes_card_settings_test.go` — two tests: a `PATCH` that changes the model writes one system note into the card's chat, and an edit that changes no setting writes none.

Changed files:

- `internal/agents/agent.go` — added `SessionSettings{Model, Thinking, PermissionMode}` and the **optional** `SettingsApplier` interface (`ApplySettings(ctx, h, SessionSettings) (Applied, error)`), deliberately *not* folded into `Agent` so an adapter that cannot honour a live change does not carry the method.
- `internal/agents/acp/{session.go,handshake.go,settings.go,adapter.go}` — the handshake's controls are retained on the session (`ctl`, guarded by `mu`); `applyOption`/`applyMode` now write the accepted value back into the retained controls, so a later change is compared against what the session is *really* running with; `applyLiveSettings` reuses `applySettings` with a `StartSpec` built from `SessionSettings`; and `(*Adapter).ApplySettings` goes through `usable`, so a dead session errors rather than silently doing nothing.
- `internal/session/{live.go,manager.go}` — a live session remembers the settings it started with (`settingsMu`/`settings`, `settingsNow`, `setSettings`), seeded from the handle in `newLiveSession`.
- `internal/session/{send.go,pump.go}` — `applyCardSettings` runs with the turn claimed, both for a message sent into an idle session (`deliver`) and for a message that waited in the queue (`deliverQueued`), so two sends cannot each hand the agent a change at once.
- `internal/session/pump.go` — `onTurnEnded` now settles the turn's bookkeeping (the commit scan, the pause hold, and the queue's `next`) **before** announcing `awake`. See section 7.
- `internal/api/routes_cards.go` — `updateCard` reads the card as it was, and after a successful `UpdateCard` calls `s.sessions.NoteSettingsChanged(before, card)`. The note is written here rather than inside `projects.UpdateCard` because a history write that must succeed would fail every edit; an unrecorded change is logged, not fatal.
- `internal/session/fake_agent_test.go` — the fake agent now implements `SettingsApplier` (`settings`, `applyErr`, `applyCalls`, with `appliedSettings`/`settingsCalls`/`setApplyErr`), so the manager's bookkeeping is checked against what the agent was really handed.

### S8b cutover (2026-09-28 run)

Daemon — new files:

- `daemon/internal/history/approvals.go` — `AppendApproval`/`AppendChatApproval` (write an approval's history row, with its id, once it exists) and `ResolveApproval`/`ResolveChatApproval` (rewrite the stored row's `state` when the approval is answered, found by paging the owner's approval events and matching the id inside the detail; section 6's ruling).
- `daemon/internal/history/approvals_test.go` — the id is stored and read back; a no-id row (the daemon's own auto-answer) reads as already resolved; resolve rewrites the state; an unmatched id and an empty id are both no-ops, not errors; a card's row is never touched by another card's resolve; the chat-side calls mirror the card-side ones.

Daemon — changed files:

- `daemon/internal/protocol/history.go` — `ChatApproval` gained `ID string`.
- `daemon/internal/protocol/card.go` — `NeedsReason` gained `ApprovalID string` (`omitempty`).
- `daemon/internal/history/agents.go` — removed the `agents.PermissionRequested` case from `RecordsOf` (the row it wrote had no id to give the approval; see the case's own new comment) and the now-unused `approvalDetailOf`; `approvalDetail` gained an `ID` field.
- `daemon/internal/history/agents_test.go` — moved the "a permission request" case from `TestRecordsOfTurnsAnAgentEventIntoOneRecord` into `TestRecordsOfIgnoresEventsThatAreNotHistory`.
- `daemon/internal/history/wire.go` — `approvalOf` reads `ID: stored.ID` onto the wire block.
- `daemon/internal/session/approval.go` — added the `ApprovalHistory` interface (mirrors `PlanStore`'s own pattern); `holdPermission` now calls `appendApprovalHistory` (writes the row, with the id, after the approvals-table row exists) and, for a card, `projects.SetNeeds` (kind `approval-needed`) at the end, so the card actually reaches Needs you — nothing did this before this run; `Respond` and `withdrawApprovals` both call the new `resolveApprovalHistory` and `clearApprovalNeeds` (which moves the card back to `working`, guarded so it only ever undoes this exact reason).
- `daemon/internal/session/approval_test.go` — two new tests: holding a permission request moves the card to Needs you with the right approval id; answering it moves the card back to working.
- `daemon/internal/session/harness.go` — `answerWithoutAsking` now calls `appendApprovalHistory` with an empty id and an already-resolved state, so a daemon-auto-answered request still gets a history row (it did before this run too, through the automatic write this run removed; this replaces that, rather than silently dropping it).
- `daemon/internal/session/config.go`, `daemon/internal/session/manager.go` — `Config.Approvals ApprovalHistory`, defaulted in `NewManager` the same way `History` and `Plans` are.
- `daemon/internal/projects/service.go` — `SessionInfo` gained `ApprovalID string`.
- `daemon/internal/projects/card_labels.go` — `withSessionInfo` sets `card.NeedsReason.ApprovalID` from it when the reason is `approval-needed`.
- `daemon/internal/session/states.go` — `StoredStates.CardSession`/`ProjectSessions` read the card's (or project's) pending approval from the `approvals` table, only when a session's state is `waiting-approval` (section 6's ruling: derived fresh, never stored).
- `daemon/internal/store/queries/session_events.sql`, `daemon/internal/store/db/session_events.sql.go` — new, additive-only queries: `ListChatEventsByKind`, `UpdateSessionEventState`, `UpdateChatEventState`.
- `daemon/internal/store/queries/approvals.sql`, `daemon/internal/store/db/approvals.sql.go` — new, additive-only queries: `GetPendingApprovalBySession`, `ListPendingApprovalsByProject`.
- `daemon/internal/protocol/history_test.go` — the `chatMessages()` golden fixture's approval block gained an `ID`.
- `daemon/testdata/golden/chat-messages.json` — hand-edited to add `"id": "app_01JQZ0000000000000000000AD"` to the one non-null approval block (matching `approval.json`'s own id format).

TypeScript protocol — changed files:

- `packages/protocol/src/generated/index.ts` — hand-edited (cannot run `gen-protocol.mjs`): `ChatApproval.id: string`, `NeedsReason.approvalId?: string`.
- `packages/protocol/test/golden-chat.test.ts` — the matching literal gained `id: "app_01JQZ0000000000000000000AD"`.

Web app — new files:

- `apps/web/src/sync/approval-actions.ts` — `approvalsOnDaemon`, `pendingApprovalID` (card field first, open chat as a fallback), `approveOnDaemon`/`denyOnDaemon` (one `POST /v1/approvals/{id}` call), `applyApprovalEvent` (a live `approval.requested` adds a block to an open card's chat; a live `approval.resolved` rewrites one already there).
- `apps/web/src/sync/approval-actions.test.ts`, `apps/web/src/testing/fake-approvals.ts` — the fake daemon's `POST /v1/approvals/{id}` (a top-level route, found by searching every card's history for the approval id, since it is never a card's own id) and the tests against it.

Web app — changed files:

- `apps/web/src/data/mappers/card.ts`, `apps/web/src/mock/types.ts` — `Card`/`DaemonCard` gained `approvalId?: string`, mapped from `card.needsReason?.approvalId`.
- `apps/web/src/data/mappers/card.test.ts` — a test for the new field.
- `apps/web/src/sync/chat-mapper.ts`, `apps/web/src/mock/types.ts` — `approvalMessage` maps `ChatApproval.id` onto `ApprovalMsg.approvalId` (`undefined`, not `""`, for a daemon-auto-answered row with no id).
- `apps/web/src/sync/chat-mapper.test.ts` — two tests for the new field.
- `apps/web/src/data/api-client.ts` — added `decideApproval`.
- `apps/web/src/mock/actions/approvals.ts` — `approve`/`deny` are daemon-aware the way `approvePlan`/`rejectPlan` already are (`approveLocally`/`denyLocally` are the old bodies, renamed and given the found `ApprovalMsg` rather than re-finding it).
- `apps/web/src/views/home/needs.ts` — `canApproveHere` also reads `card.approvalId`, which is what makes a *closed* card answerable from Home (the gap section 3 records as the bigger of the two).
- `apps/web/src/sync/index.ts` — registered `applyApprovalEvent` beside `applyPlanUpdated`.
- `apps/web/src/testing/fake-cards.ts` — routes `/v1/approvals/{id}` to the new `approvalRoute`.
- `apps/web/src/data/sections.ts` — `S8b: "daemon"`.
- `apps/web/src/mock/testing/twin.ts` — pinned `S8b: "mock"` for the twin, the way `S7b`/`S8c` already are.
- `docs/backend-checklist.md` §2.2 — the S8b register row moved to `Daemon`, in the same change as the code (rule 3 in §2.3, and `data/sections.test.ts`).

**Total: 38 implementation files this run (5 new, 33 changed), across the daemon, the generated protocol package, and the web app, plus this report. The repo's working tree also carries a large number of other files modified by other, unrelated concurrent sessions (dependencies, handoff, notes, lessons, memory, MCP servers, and more) — none of those are this run's, and this list names only the files this run actually touched.**

## 5. Verification results

Baseline before this run: HEAD `0562fc3`, working tree clean except the untracked `phase-prompts/` and `phase-reports/`; `go build ./...` clean.

| Check | Command | Result |
|---|---|---|
| Build | `go build ./...` (daemon) | clean |
| Vet | `go vet ./...` (daemon) | clean |
| Format | `gofmt -l internal/` | no files |
| Approvals flow | `go test -run 'TestPermissionRequestHolds\|TestRespondAnswers\|TestRespondRefuses' ./internal/session/` | `ok` (1.1s) |
| Approvals flow, races | `go test -race -run 'TestPermissionRequestHolds\|TestRespond' ./internal/session/` | `ok` (3.7s) |
| ID time | `go test -run 'TestIDTime' ./internal/protocol/` | `ok` |
| Protocol (Go) | `go test ./internal/protocol/` | `ok` |
| Protocol (TS) | `pnpm --filter @marshal/protocol test` | 11 files, 66 passed |
| Wire goldens | `UPDATE_GOLDEN=1 go test -run TestEnumsGolden ./internal/protocol/` | wrote `enums.json` (+4 lines: the `ApprovalDecision` list) |
| Store (migration applies) | `go test ./internal/store/ ./internal/audit/` | `ok` |
| Route registration | `go test -run 'TestEveryRouteRequiresAToken\|TestARouteIsRegisteredOnlyWhenItsServiceIsThere\|TestNoDomainRoutesWithoutAStore' ./internal/api/` | `ok` |
| Route behaviour | `go test -run TestDecideApprovalRoute ./internal/api/` | `ok` |
| Bypass (projects) | `go test ./internal/projects/ -run Bypass` | `ok` |
| Containment (gitx) | `go test ./internal/gitx/` | `ok` (13.2s) |
| Blocklist, profile, deploy | `go test ./internal/security/` | `ok` |
| Harness decision table | `go test ./internal/harness/` | `ok` |
| Session: bypass/approval/harness/permission | `go test -run 'Bypass\|Approval\|Harness\|Containment\|Profile\|Permission\|Refus' ./internal/session/` | `ok` (5.9s), run twice, each clean |
| Build after Slices 2–3 | `go build ./...` (daemon) | clean |
| Vet after Slices 2–3 | `go vet ./internal/security/... ./internal/harness/... ./internal/session/... ./internal/gitx/...` | clean |
| Format after Slices 2–3 | `gofmt -l internal/security internal/harness` | no files |
| Audit read order | `go test ./internal/audit/` | `ok` (the `rowid DESC` change) |
| Secret scanner | `go test -count=1 ./internal/secrets/` | `ok` (0.9s) |
| Commit reading (gitx) | `go test -count=1 -run 'TestCommitsOnBranch\|TestChangedPaths\|TestFileAtCommit' ./internal/gitx/` | `ok` |
| Secret block in a session | `go test -count=1 -run 'TestACommitWithACredential\|TestTheMessageQueuedBehindTheCommit' ./internal/session/` | `ok` (1.8s) |
| Audit routes | `go test -count=1 -run 'TestTheAudit' ./internal/api/` | `ok` (6 tests) |
| Route registration (all groups) | `go test -count=1 -run 'TestEveryRouteRequiresAToken\|TestARouteIsRegisteredOnlyWhenItsServiceIsThere\|TestNoDomainRoutesWithoutAStore\|TestATokenIsNeverLogged' ./internal/api/` | first run **failed** (see section 7), then `ok` |
| Build after Slice 4 | `go build ./...` (daemon) | clean |
| Vet after Slice 4 | `go vet ./internal/secrets/ ./internal/gitx/ ./internal/auditlog/ ./internal/audit/ ./internal/protocol/ ./internal/projects/ ./internal/session/ ./internal/api/ ./cmd/marshald/` | clean |
| Format after Slice 4 | `gofmt -l` over the packages above | no files (two import blocks fixed with `gofmt -w`) |
| Wire goldens after Slice 4 | `UPDATE_GOLDEN=1 go test -run 'TestEnumsGolden\|TestAuditEntryGolden\|TestAuditEntryListGolden\|TestAuditExportGolden' ./internal/protocol/` | wrote `enums.json` (+1 value), `audit-entry.json`, `audit-entry-list.json`, `audit-export.json` |
| Regenerated types | `node scripts/gen-protocol.mjs` | updated `packages/protocol/src/generated/index.ts` and `packages/protocol/test/enums.test.ts` |
| sqlc after the new queries | `PATH="$PWD/.tools/bin:$PATH" sqlc generate -f daemon/sqlc.yaml` | clean (the one `rowid`-in-`WHERE` error was fixed by moving the read cursor to `(created_at, id)`) |
| Build after Slice 5 | `go build ./...` (daemon) | clean |
| Vet after Slice 5 | `go vet ./internal/session/ ./internal/agents/... ./internal/api/ ./internal/secrets/ ./internal/auditlog/ ./internal/harness/ ./internal/security/` | clean |
| Format after Slice 5 | `gofmt -l internal/agents internal/session internal/api` | no files |
| Live settings, session layer | `go test -count=1 -run 'TestA(Live\|nUnchanged\|Setting\|Failing\|Cleared)\|TestTheChange\|TestNothingSaid' ./internal/session/` | `ok` (7 tests) |
| Live settings, ACP adapter | `go test -count=1 -run 'TestALiveChange\|TestSettingsAreApplied\|TestSettingsTheAgentDoesNotOffer' ./internal/agents/acp/` | `ok` (3 new + the 2 handshake tests) |
| Agent packages, full | `go test -count=1 ./internal/agents/ ./internal/agents/acp/` | `ok` (0.4s / 2.5s) |
| Card settings through the route | `go test -count=1 -run 'TestChangingASettingIsSaid\|TestAnEditThatChangesNoSetting' ./internal/api/` | `ok` (2 tests) |
| Phase 3 routes, re-run | `go test -count=1 -run 'TestBypass\|TestChangingASettingIsSaid\|TestAnEditThatChangesNoSetting\|TestAudit\|TestApproval' ./internal/api/` | `ok` |
| Turn-end paths (view, queue, pause, secrets, settings) | `go test -count=1 -run 'TestASwitch\|TestACommitWithACredential\|TestTheMessageQueuedBehind\|TestA(ChatQueues\|FailedQueued\|QueuedMessage\|PauseHolds\|MessageWaits\|Setting\|Cleared)\|TestBypassApplies\|TestBypassWritesTheNote\|Test(Sleep\|Wake\|Pause\|Hold)\|TestNothingSaid\|TestTheChange' ./internal/session/` | `ok` (6.5s), run three times |
| Approval wire goldens | `UPDATE_GOLDEN=1 go test -run 'TestApproval\|TestDecideApproval\|TestAnApproval' ./internal/protocol/` | wrote `approval.json`, `decide-approval-request.json`, `approval-requested-event.json`, `approval-resolved-event.json` |
| Protocol package | `go test -count=1 ./internal/protocol/` | `ok` (0.4s) |
| Format after the golden work | `gofmt -l internal/protocol` | no files |
| End-of-phase graph update | `code-review-graph update` | 230 files re-parsed, 187 nodes, 1,341 edges, FTS 4,387, `head_matches_build: true` |
| End-of-phase blast radius | `code-review-graph detect-changes --base HEAD` | 48 files, 96 changed functions, risk 0.55, 72 test gaps, 0 affected flows |

Not run by the builder (rule 1d; the owner has not asked): the full gate (`pnpm check`), the whole-repo test run, the slow Go packages (`internal/api`, `internal/projects`, `internal/session` in full), `pnpm build`, budgets, knip/jscpd, and the browser suite. See CI and section 8.

**Nothing in the 2026-09-28 S8b run was run at all — no build, no vet, no gofmt, no test, in Go or TypeScript, of any scope, including a single package.** This was a constraint of that run, not a decision the usual rules made: no `go build`, `go test ./internal/history/`, `vitest run`, or anything else was executed for any file this run touched or added. Every line above this note is Slices 1–5's own evidence, from before that run; none of it re-verifies what the run changed. Section 8 has the exact commands the controller should run first, in order, and section 3 and section 7 name the two places most likely to need a real fix once they are.

**Slice 3 reviews.** Slice 3 changed `security` and `harness`, which the prompt requires **two reviews** of. Seven independent verifier passes ran over this slice (rounds 1–3 plus re-reviews); the last two returned `DO NOT SHIP` and every finding in them is addressed except the one limitation in section 7. All of these were dispatched and read by the builder; see section 8 for why the builder cannot itself satisfy the "two reviewers" rule.

End-of-slice graph review (rule 1c):

- `code-review-graph update` — incremental: 19 files re-parsed, 118 nodes and 1,017 edges updated, FTS rebuilt (4,128), `head_matches_build: true`.
- `code-review-graph detect-changes --base HEAD` — 9 files, 18 changed functions, **risk 0.50**, 16 test gaps; review priorities `Config`, `withDefaults`, `fakeAgent`. `IDTime` was flagged untested and is now covered by `opaque_id_test.go`. `Config`/`withDefaults` gaps are the two new config fields, exercised through `NewManager` in every session test that runs.

End-of-slice graph review, Slice 4 (rule 1c):

- `code-review-graph update` — incremental: 228 files re-parsed, 183 nodes and 1,257 edges updated, FTS rebuilt (4,289), `head_matches_build: true`.
- `code-review-graph detect-changes --base HEAD` — 40 files, 72 changed functions, **risk 0.55**, 51 test gaps, 0 affected flows; review priorities `forkPermissionMode`, `Config`, `withDefaults`. `withDefaults` is the session config normaliser that now defaults `Config.Secrets`. The new Slice 4 code is covered by the tests in the table above; the remaining gaps are the web-side functions the graph lists, which belong to the S8b cutover.

End-of-phase graph review (rule 1c), covering Slices 1–5 together (the Slice 5 edits came after the last per-slice pass):

- `code-review-graph update` — incremental: 230 files re-parsed, 187 nodes and 1,341 edges updated, FTS rebuilt (4,387), `head_matches_build: true`.
- `code-review-graph detect-changes --base HEAD` — 48 files, 96 changed functions, **risk 0.55**, 72 test gaps, 0 affected flows; review priorities `forkPermissionMode`, `TestALiveChangeOnAnUnknownSessionIsRefused`, `handshake`; flagged untested: `routeMethods`, `toStoredCard`, `cardActions`, `changeOf`, `grantBypass`. Checked each: `forkPermissionMode` **is** covered, through its exported path by `TestForkNeverInheritsBypass` (`internal/projects/fork_test.go`), so the flag is the graph's function-level heuristic, not a real gap; `grantBypass` is web-side (`apps/web/src/testing/fake-card-actions.ts`, `apps/web/src/sync/card-actions.ts`) and belongs to the S8b cutover; `routeMethods`, `toStoredCard`, `cardActions`, and `changeOf` are the web mappers/route helpers the S8b work will touch. No new daemon-side gap is left open by Slices 1–5.

## 6. Rulings

- **Ruling: an approval's time comes from its own id, not a stored column — architecture §10 gives the `approvals` table only id, session_id, request_json, decision, decided_by, and the UI still needs a time — cost if wrong: a reader that wants a second time (when it was decided) reads it from the audit row, which every decision also writes.**
- **Ruling: the decision vocabulary is `approved`/`denied` — the same two words as `ChatApprovalState` and the mock (`syncProjectApproval`), so a decided approval needs no translation between the request body, the stored column, and the chat block — cost if wrong: a client that expected the verbs `approve`/`deny` in the body must use the states instead.**
- **Ruling: answering an approval that is no longer waiting is 409 (`conflict`) when it was already answered and 404 (`not_found`) when there is no such row, rather than silently succeeding — a second press tells the person what happened — cost if wrong: a client that treats 409 as a hard error must treat it as "already answered".**
- **Ruling: the agent is given the exact option the person chose (`optionId`), defaulting to `allow_once`/`reject_once` — the ACP options are passed through unchanged so the agent obeys the person, not a paraphrase — cost if wrong: an agent offering only "always" forms still works, via the fallback.**
- **Ruling (Slice 2): bypass is not a card flag, it *is* `permissionMode: "bypass"` — one field means one thing in the store, the wire, and the UI, so there is no second source of truth to drift — cost if wrong: a reader that expected a separate `bypassed` boolean must read the mode.**
- **Ruling (Slice 2): acknowledgement must be typed (`Acknowledged: true`); missing or false is `Refused` 422 `reason: "unacknowledged"`, and the same refusal closes PATCH, card creation, and forks — one gate, not four — cost if wrong: a client that sent no body must send the acknowledgement explicitly.**
- **Ruling (Slice 2): the project lock refuses turning bypass *on* but never turning it *off*; turning it off lands in full auto — a person can always leave bypass — cost if wrong: a locked project's card that was already bypassed cannot be re-bypassed after being turned off.**
- **Ruling (Slice 3): an independent Marshal-side gate exists.** `internal/harness` decides from the mode, the profile, the command blocklist, the deploy rule, and the card's worktree, for every permission request, regardless of what the agent does. For Claude the CLI flags stay as they are and the screens say what the capability flag says. **Cost if wrong: policy is duplicated between the harness and the adapters, so a mode change in Claude Code's own flags would not necessarily be refused by Marshal's own reading.**
- **Ruling (Slice 3): the order of the rules is the safety property.** The worktree rule runs first in every mode (bypass included), then bypass allow, then the profile, then the blocklist, then the deploy ask, then the mode. A refusing rule is never widened by a mode. **Cost if wrong: a rule that is correct in isolation can be undone by the order it runs in, so the order is as load-bearing as any rule and is tested as its own set of expectations.**
- **Ruling (Slice 3): an unreadable command is never allowed on its own words.** A substitution, a backtick, an ANSI-C quote, a variable or brace reference, and a command whose program is a variable all fall to `RuleUnreadable`, which is an **ask**, never an allow; and `Classify` reads the command whenever one is present, ignoring the agent's kind word. Fail closed. **Cost if wrong: a person is asked about a request that was harmless (a false ask), which is the direction the package always errs in.**
- **Ruling (Slice 3): the two readings of a command line are deliberate.** `Words` is blunt and quote-stripped for the blocklist (so a quote cannot hide a command); `Argv`/`ArgvTail` are quote-aware where an argument's shape matters (so a commit message is one argument and is not read as a git subcommand). **Cost if wrong: a rule added to the wrong reading either over-asks or can be dodged.**
- **Ruling (Slice 3): deploy detection is deliberately narrow** — `echo deploy` is not a deploy, and the rule reads every command in a line. **Cost if wrong: a deploy spelled in a way the rule does not know is left to the mode, which is the conservative direction only because full auto is not the shipped mode.**
- **Ruling (Slice 3): the audit log is ordered by `rowid DESC`, not `created_at`/`id`.** Rows written in the same millisecond share a `created_at` and differ only in random ULID bits, so ordering by time and id is nondeterministic; `rowid` is the true write order. `rowid` cannot be named in an index, so the indexes stay on `(created_at DESC, id DESC)`. **Cost if wrong: a reader that wanted a stable order across a vacuum must re-read; the ordering is still total and stable for the life of the file.**
- **Ruling (Slice 4): "blocked" means the card is stopped, not the commit intercepted.** The daemon cannot intercept `git commit` inside an agent's own worktree, and the harness only sees a tool call an agent chooses to ask about. So the daemon reads the branch it gave the card after the turn and moves the card to `needs` with reason `secret-detected` the moment a commit holds a credential. **Cost if wrong: the offending commit already exists in the worktree when the person sees the block; nothing is rewritten for them, and the note says so.**
- **Ruling (Slice 4): a credential is never recorded.** `secrets.Finding` drops the matched text, and the audit row, the system note, and the wire entry carry only the rule, the file, and the line. gitleaks' own `Redact` is also raised as a second guard. **Cost if wrong: a person must open the file and look; the trail names where, but not what.**
- **Ruling (Slice 4): the secret block holds the queue.** `onTurnEnded` releases the session without delivering the next queued message, so the agent does not build on top of the offending commit; the message waits for the person. **Cost if wrong: a queued message that had nothing to do with the commit also waits until the card is resumed.**
- **Ruling (Slice 4): the audit read cursor is `(created_at, id)`, not `rowid`.** SQLite allows `rowid` to be ordered by but not compared in a `WHERE`, and sqlc refuses it there, so the *read* paging cursor is the pair. The pair is unique because the id is. The *write-order* rationale that forced `rowid DESC` on `ListAuditLog` (section 6, Slice 3) still stands for the newest-first screen list. **Cost if wrong: the two orderings are not the same one; a reader that needs the exact write order must use `ListAuditLog`, not the cursor page.**
- **Ruling (Slice 4): the CSV export guards formula injection.** A cell that begins with `=`, `+`, `-`, `@`, a tab, or a carriage return is prefixed with an apostrophe, so a spreadsheet reads it as text. **Cost if wrong: a cell whose real content began with one of those characters carries a leading apostrophe in the download.**
- **Ruling (Slice 4): gitleaks' own logging is disabled globally, once.** gitleaks narrates every rule it considers through zerolog at trace; Marshal logs through slog and uses zerolog for nothing else, so the level is set to `Disabled` before the detector is built. **Cost if wrong: a future component that wanted zerolog would find it off and must set its own level.**
- **Ruling (Slice 5): a running session picks up a setting change by reading the card at the start of each turn, and hands the change to the agent only through an optional `SettingsApplier`.** `agents.Agent` is left unchanged; the ACP adapters implement `SettingsApplier` (through `SetSessionConfigOption`/`SetSessionMode`, reusing the handshake's `applySettings`), and an adapter that cannot take a live change simply does not implement it. A session whose agent has no live controls keeps what it started with, and the change takes effect at the next `Start`/`Resume`, which already reads the card. **Cost if wrong: a caller that wants to know whether live changes are possible must type-assert to `SettingsApplier`, and an agent without live controls applies a mid-session change only on the next start.**
- **Ruling (Slice 5): a setting the agent does not offer is recorded as given once, but a failure that could be passing is retried.** `applyCardSettings` advances the session's remembered settings when `ApplySettings` returns `ErrUnsupportedSetting` (a permanent "not this value"), and leaves them alone on any other error (a transient failure re-attempts on the next turn). **Cost if wrong: a genuinely transient `ErrUnsupportedSetting` would stop being retried until something else about the session changes.**
- **Ruling (Slice 5): one `KindSystem` + `StateOK` history record per changed setting.** The chat draws it as a system message and the activity list draws it as an ok tool row (`history.ChatMessageOf` / `history.ActivityItemOf`), which is how approvals and tool calls already serve both views. **Cost if wrong: both views read the same row, so a client that wanted two separate records would have to split it.**
- **Ruling (Slice 5): the settings note is written in the `PATCH` handler, not inside `projects.UpdateCard`.** An audit/history write that had to succeed would make every card edit fail when the trail could not be written; here a note that cannot be written is logged, and the edit still lands. **Cost if wrong: a settings change whose note cannot be written is unrecorded in the chat (it is still logged) — the change itself is never lost.**
- **Ruling (Slice 5): bypass is not one of the settings `settingCalls` reports.** Turning bypass on or off has its own call and its own note (Slice 2/§6), so it is excluded here to keep one sentence per action. **Cost if wrong: a bypass change is announced by its own path, not by the card-settings note.**
- **Ruling (S8b, 2026-09-28 run): the resolved approval's stored row is found by paging and decoding, not by a new column or `json_extract`.** Three designs were open: (a) add an `approval_id` column to `session_events` via a migration, threaded through `Record`/`InsertSessionEvent`/`InsertChatEvent`; (b) match inside `detail_json` with SQLite's `json_extract` in a hand-written `WHERE` clause; (c) page the owner's own `KindApproval` events with the existing `ListSessionEventsByKind` (plus a new, trivial `ListChatEventsByKind`), decode each detail in Go, and match on the id already inside it. (a) touches `InsertSessionEvent`/`InsertChatEvent`'s call sites and every `Record` literal in the codebase for one field only approvals use. (b) is a query shape nothing in this codebase uses today, so sqlc's generated parameter type for `json_extract(...) = ?` could not be hand-written with any confidence without running `sqlc generate`, which this pass cannot do. (c) reuses an existing, already-generated query shape almost verbatim, needs one trivial new query (`ListChatEventsByKind`, a straight `chat_id` twin of `ListSessionEventsByKind`) and one narrow `:execrows` update (`UpdateSessionEventState`/`UpdateChatEventState`, touching only the `state` column), and is the one of the three whose generated-looking Go code (`internal/store/db/session_events.sql.go`) is closest to something already proven to compile. Approvals are rare enough per card or chat (a handful over a whole life, not thousands) that a page-and-scan is not a real cost. **Cost if wrong: a card or chat that somehow accumulated far more than one page of approvals pays for more than one round trip inside the same write transaction before finding the one it wants; the loop is written to keep paging rather than give up, so it is still correct, just slower.**
- **Ruling (S8b, 2026-09-28 run): a card's waiting-approval id is derived fresh from the `approvals` table on every read, through `session.StoredStates`, never stored on the card row.** The alternative (recorded as the controller's own suggestion, and worth naming because it does not fit this codebase) would have added a persisted `NeedsReason.ApprovalID` written once when the card enters Needs you. That needs either a new `cards` column (touching every hand-written `SELECT`/`UPDATE` that already lists `needs_reason_kind`/`needs_reason_text` explicitly in `internal/store/db/projects.sql.go` — four or five call sites, all hand-edited without `sqlc generate` to check them) or a live-computed join injected into `cardWithLabels`/`toCards`, which the codebase's own doc comments say a board must never do per card. Deriving it in `session.StoredStates.CardSession`/`ProjectSessions` instead reuses the exact seam that already carries a card's session state into the wire card (`projects.SessionInfo`, `withSessionInfo`), costs one query only when a card's state is (or, for the batched project read, might be) `waiting-approval`, and can never go stale: once the approval is answered the `approvals` row's `decision` column stops matching, and the field reads empty on the very next read with no extra bookkeeping to keep it in sync. **Cost if wrong: `StoredStates` now depends on the `approvals` table as well as `sessions`, which it did not before; a future change to either table's shape must keep both queries in mind.**

## 7. Found, not fixed, and later phases

- **The stored chat-message approval block is not yet updated on re-open — fixed in the 2026-09-28 run.** `history.ResolveApproval`/`ResolveChatApproval` now rewrite the stored row's state when the approval is answered (section 3, section 6).
- **The `Approval` wire type had no golden file — fixed this run.** The prompt's §3 lists `Approval` and `ApprovalDecision` "each with a golden file"; the enum golden existed (in `enums.json`), but a full `Approval` example golden did not. `internal/protocol/approvals_test.go` now goldens `Approval`, `DecideApprovalRequest`, and both approval events (`approval.json`, `decide-approval-request.json`, `approval-requested-event.json`, `approval-resolved-event.json`), so the web mapper's tests will have files to read from when S8b is switched (section 3).
- **The approval read path could not answer an approval (Slice 5 run) — fixed in the 2026-09-28 run.** See section 3 for the fix and section 6 for the design rulings behind it.
- **The project-chat half of S8b is not cut over (2026-09-28 run).** The daemon writes and rewrites a chat's own approval history correctly (`AppendChatApproval`/`ResolveChatApproval`), but nothing on the web side resolves a project chat's own topic for a live approval event, and the mock's own cross-chat mirror (`syncProjectApproval`) has no daemon-backed twin. See section 3's S8b bullet and section 8.
- **This run's daemon changes were not compiled, vetted, or tested (2026-09-28 run; constraint of this pass, not a choice).** The controller must run `go build ./...`, `go vet`, and the new tests before trusting any of it. Two places are flagged as the most likely to have a mistake a compiler would catch: `internal/history/approvals.go`'s pagination loop in `findApprovalEvent` (the termination condition on a page exactly `approvalEventPageSize` long), and the hand-written sqlc-shaped generated code in `internal/store/db/session_events.sql.go` and `internal/store/db/approvals.sql.go` (new functions only, nothing existing touched, but never run through `sqlc generate`). See section 5 and section 8.
- **`Funnel`/remote and `AgentCapabilities.Approvals`.** Nothing in this slice changed which agents can ask; an agent that cannot ask never emits `PermissionRequested`, so the flow is inert for it.
- **A relative path is assumed to be relative to the worktree (Slice 3).** `harness.InsideWorktree` reads a relative path against the card's worktree, because that is where the agent runs, without knowing the agent's real cwd. A request that names `../..` is refused, but one that names a path the agent would resolve elsewhere is read as if it were in the worktree. This is the permissive direction, and the last review's finding 12. Left in place because the conservative fix — refusing every relative path — would refuse ordinary edits; recorded as a known limitation, not silently.
- **The `security`/`harness` two-review rule is not satisfied by the builder alone (Slice 3).** The prompt requires two reviews of a change to `security`/`harness`. The builder dispatched and read seven verifier passes, but a builder's own dispatched verifiers are not the "two independent reviewers" the gate means. The controller must arrange the second reviewer; this is repeated in section 8.
- **The route-registration test had been failing since Slice 2, and is now fixed (Slice 4).** `TestARouteIsRegisteredOnlyWhenItsServiceIsThere` counts the routes it knows by hand against `api.RoutePatterns()`; Slice 2 added `POST`/`DELETE /v1/cards/{id}/bypass` to the server but never added them to the test's `sessionRoutes` group, so the count was two short. Slice 4's own scoped runs were the first to execute this test, which is how it surfaced. Both routes are now in the group and the test passes. Recorded because it means Slice 2's verification was incomplete: that milestone's evidence did not include this test.
- **The audit screen does not exist (Slice 4).** The read routes have no UI, and none was built. `docs/backend-inventory.md` §6 and build-plan 3.13 require the screen to be designed first. The `AuditEntry`/`AuditExport` wire types are goldened and ready for it.
- **`onTurnEnded` announced `awake` before it settled the turn — fixed in Slice 5.** Slice 4 inserted the (git-backed) commit scan between `publishState(awake)` and the code that releases the turn, which widened a pre-existing gap into a deterministic one: a client that heard `session.state.changed` → `awake` and then tried to switch the card's view was told a turn was still running, and a paused card holding a message was told the same instead of `holding-messages`. Two baseline tests (`TestASwitchWaitsForARunningTurn`, `TestASwitchIsRefusedWhileAPauseHoldsAMessage`) caught it once the turn-end paths were run. The fix moves `publishState(awake)` to *after* the scan, the pause hold, and the queue's `next`, so the announced state always matches the turn bookkeeping. The secret block's ordering guarantee is unchanged: the turn stays claimed while the scan runs, so a message cannot be delivered on top of an offending commit. Recorded because Slice 4's own scoped runs did not exercise these two tests.
- **`FileAtCommit` drops a trailing newline.** It returns `Git.Run`'s output, which trims trailing `\r`/`\n`; that is harmless for a credential scan and is asserted in `internal/gitx/log_test.go`. Recorded because a caller that needed byte-exact content would need a different reader.

## 8. Needs the controller

Everything that needs a browser (the owner never opens one for the builder) or the real gate, once S8b is cut over:

- **The second reviewer for `security`/`harness` (Slice 3).** The prompt's gate needs two reviews of this change; the builder supplied its own verifier passes only. The controller must arrange the independent second reviewer before the slice is considered shipped.
- **Verify the 2026-09-28 S8b run before anything else.** No check, test, or build was run for any of it (constraint of that pass). In order:
  1. `go build ./...` and `go vet ./...` in `daemon/`. The two spots flagged as most likely to need a fix: `internal/history/approvals.go` (new file; the pagination loop and the two new `db.Queries` calls it makes) and the hand-written generated code in `internal/store/db/session_events.sql.go` and `internal/store/db/approvals.sql.go` (new functions only — nothing existing was touched, but none of it has been through `sqlc generate` or the compiler).
  2. `PATH="$PWD/.tools/bin:$PATH" sqlc generate -f daemon/sqlc.yaml` and diff the result against what this run hand-wrote in those two files, the way Slice 4's own verification did for its new queries (section 5). A mismatch means the hand-written version was a guess in the wrong place and should be replaced with sqlc's own output.
  3. `go test ./internal/history/... ./internal/session/... ./internal/protocol/...` — this run added `internal/history/approvals_test.go` (six cases: id stored, no-id row, resolve rewrites state, unmatched id is a no-op, empty id is a no-op, card isolation, and the chat-side mirror) and two cases in `internal/session/approval_test.go` (holding an approval moves the card to Needs you with the right approval id; answering it moves the card back). None of these have been run.
  4. `pnpm --filter @marshal/web test` (or the narrower `vitest run src/data/mappers/card.test.ts src/sync/chat-mapper.test.ts src/sync/approval-actions.test.ts src/data/sections.test.ts`) and `pnpm --filter @marshal/protocol test` (the hand-edited `golden-chat.test.ts` literal must match the hand-edited `daemon/testdata/golden/chat-messages.json` exactly, including the fixture id `app_01JQZ0000000000000000000AD` chosen to match the format `approval.json`'s own golden already uses).
  5. Once green: the project-chat half of S8b is still missing (section 3, section 7) — decide whether that is its own follow-up slice or blocks B3.4's checkbox indefinitely, and tick B3.4 in `docs/backend-checklist.md` only after the hands-on pass below also covers a project chat, not only a card and Home.
- Hands-on: open a card whose agent asks for permission; approve and deny from the card; confirm the project chat that asked shows the same approval and updates (not built this run — see above); confirm Home's needs-you list follows (built this run, unverified).
- Hands-on: turn bypass on from a card, confirm the acknowledgement is required and the audit log records `bypass.on`; turn it off; confirm a locked project refuses it on.
- Hands-on (Slice 4): with an agent that commits, put a credential in a file and commit it; confirm the card moves to Needs you with a sentence naming the rule, file, and line, that the audit trail has a `commit.blocked` row without the value, and that the card's history carries the daemon's note. Then read `GET /v1/audit`, `/v1/audit/search?q=…`, and `/v1/audit/export?format=csv` and confirm the CSV opens as a download.
- Hands-on (Slice 5): start a card against an agent with live controls (an ACP agent that offers model/thinking/mode), change the model on the card while it runs, and confirm the next turn runs with the new model and that the card's chat shows "Model set to …. It takes effect on the next turn."; put a queued message behind a running turn and confirm it too runs with the new setting.
- The audit screen has no UI to click (Slice 4); the read routes are the whole surface.
- The full gate, budgets, `pnpm build`, knip/jscpd, and the browser suite — on CI, not the owner's machine.

## 9. Design-doc rows I added or propose

- `docs/architecture.md` §10 already specifies both tables; the migration matches it, and adds the audit indexes without changing the columns.
- Proposed: §11.1 already lists `POST /v1/approvals/{id}`; no new row needed. A short note that an approval's time is read from its id (section 6) belongs in §10 with the approvals table.
- `docs/backend-inventory.md`/`docs/progress-tracker.md` are the controller's to tick; B3.4 stays unticked until the controller verifies.

## 10. Confirmation of the hard rules

- No mutating git command was run (no add, commit, checkout, stash, branch, push). `git status --short` and `git log` were read.
- No browser was opened.
- The code-review graph was used before changing shared code (impact/symbol queries), kept fresh (`update`), and reviewed after (`detect-changes`); grep was used only for docs, SQL text, and string literals.
- Only fast checks ran. The full gate, whole-repo tests, budgets, and the browser suite were not run.
- No real agent ran; no real provider key, GitHub account, Trello token, Tailscale account, or Telegram/Discord token was used. Tests use the fake agent and a temporary store.
- Untrusted content was treated as data; nothing was executed from it.
