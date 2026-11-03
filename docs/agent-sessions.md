# Claude and Codex sessions

Marshal starts both agents through its production session manager, in the original project folder. One main editing session owns a project at a time. See [direct-workspace.md](direct-workspace.md) for coordination and snapshots.

## Claude Code

The adapter runs a persistent `claude -p` process with stream JSON input/output. Permission requests use `--permission-prompts=host` and `--permission-prompt-tool=stdio`. A native `can_use_tool` control request becomes Marshal's existing approval block. Approve returns the original tool input unchanged; deny returns a refusal. Request cancellation, turn completion, and process exit resolve waiting blocks so their buttons do not stay active.

The native permission mode still applies: Ask uses `default`, Auto-accept edits uses `acceptEdits`, Plan uses `plan`, Full auto uses `auto`, and Bypass uses `bypassPermissions`. Marshal's harness also checks requests that reach the host. Explicit native deny rules still take precedence.

## Codex

The adapter runs `codex app-server --listen stdio://` and performs `initialize`, `initialized`, and `thread/start`. Messages use `turn/start`; interrupt uses `turn/interrupt`; stop ends the owned process. Resume starts another app-server and calls `thread/resume` with the saved thread ID. A missing or different returned ID is refused rather than starting an empty conversation.

| Marshal mode | Codex approval policy | Codex sandbox |
|---|---|---|
| Ask | `untrusted` | `read-only` |
| Auto-accept edits | `untrusted` | `workspace-write` |
| Plan | `never` | `read-only` |
| Full auto | `never` | `workspace-write` |
| Bypass | `never` | `danger-full-access` |

Native command and file-change approvals use Marshal's existing approve/deny flow. Each grouped file approval carries every affected path to containment checks. File changes with no inspectable paths are denied. A turn-start timeout ends the process, because the server might already have accepted the message; the saved thread remains resumable.

The `default` model choice inherits the installed Codex CLI's configured model. Existing named model choices remain available, subject to account access. Thinking settings and MCP configuration travel in the thread request; server credentials are passed through stdin, not command-line arguments.

Only the injected `marshal` MCP server has its native transport preapproved. Its tools still enforce Marshal's session permissions on the server. Other MCP servers retain their own consent policy. MCP tool names, successful text results, and failures appear in the transcript.

Limits: arbitrary MCP input forms, URL/device verification, and additional native permission profiles are not interactive in Marshal yet. Unsupported requests are refused rather than silently granted. Codex token/cost usage events are not mapped yet. These checks do not establish feature parity with the standalone CLIs.

## Verification through Marshal

The real check uses the built daemon and its public HTTP API, with a fresh data folder, sample repository, and free port. It does not call a CLI directly or touch the owner's projects. It exercises the same adapters, working folder, MCP attachment, card approval rows, history, and start/stop actions used by the app. A stopped session continues through `POST /v1/cards/{id}/start`, matching the app's Resume agent action; `/resume` is reserved for sessions waiting after a daemon restart.

Run only when the owner requests a real account check:

```sh
pnpm --filter daemon build
node scripts/agent-lifecycle-smoke.mjs --real --agents claude,codex --model-codex default
```

It calls Marshal's `board_status` MCP tool, proves a file does not exist before approval, approves its creation, denies a second write and checks that it did not run, checks durable approval history, then stops/resumes and asks for a marker retained only in the conversation. It cleans up its daemon and temporary data. Browser checks and automatic CI model calls are excluded.

On 2026-11-03 this flow passed with Claude Code **2.1.299** and Codex CLI **0.166.0-alpha.5.1**. Automated tests additionally exercise the actual Claude adapter through HTTP with a protocol fixture, cancellation and duplicate replies, Codex JSON-RPC subprocess fixtures, failed/interrupted turns, startup cancellation, grouped edits, and path aliases.

Protocol references: [Claude SDK permission transport](https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/_internal/transport/subprocess_cli.py), [Claude control responses](https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/_internal/query.py), [Codex app-server](https://developers.openai.com/codex/app-server), and [Codex MCP configuration](https://developers.openai.com/codex/config-reference).
