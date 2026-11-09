# Claude and Codex sessions

Marshal starts both agents through its production session manager, in the original project folder. One main editing session owns a project at a time. See [direct-workspace.md](direct-workspace.md) for coordination and snapshots.

## Coding through Telegram or Discord

Connect the bot in **Settings → Integrations** and keep Marshal running on the computer that holds your projects. Commands are accepted only from the configured chat or channel. Members of that chat share control of Marshal.

Send `new web Fix the login bug` to add a card to the project named `web`. Marshal replies with its key, for example `web#12`. Use that key in the following commands.

| Command | What it does |
| --- | --- |
| `agent web#12 codex` | Choose Claude, Codex, Gemini, or the built-in agent before the first start |
| `start web#12` | Start the task using the card's instructions and permissions |
| `message web#12 Also check password reset.` | Send instructions to the card's session |
| `reply web#12` | Read the latest answer, or the current partial answer while it runs |
| `status web#12` | Read the task state, progress, and any waiting approval |
| `status` | List up to 20 open cards with their keys |
| `stop web#12` | Stop the session |
| `resume web#12` | Continue the saved session |
| `finish web#12` | Request the project's configured review, checks, and card completion |
| `approve <approval-id>` / `reject <approval-id>` | Answer a permission request |

Telegram also accepts slash commands, including `/start web#12`. Follow-up instructions keep their spacing and line breaks. After a card is created or addressed through a bot, completed agent answers arrive automatically in that conversation. `reply` still reads an answer on demand. Answers are split into chat-sized messages and bounded to an excerpt for long answers. The app retains the full transcript. Voice notes still ask you to type.

Reply to a linked Marshal message in Telegram or Discord to send instructions to its card without typing the key again. Existing Discord threads and Telegram topics become linked when you address a card there. A linked thread stays with that card; use another thread or the main channel for another card. Inside a linked reply or thread, `start`, `stop`, `resume`, `reply`, `finish`, `status`, `agent codex`, and `message <instructions>` use that card. Unknown slash commands show help. Ordinary unlinked text never starts an editing task.

Reply links and delivered answer revisions are saved in Marshal's private database, scoped to the bot credentials and configured destination. The service checks linked cards every two seconds and forwards completed answers, with a maximum of 64 tracked cards and 512 message links per connection. Older links expire as these limits are reached; explicit card commands still work. Failed sends retry while the daemon and connection remain active. Delivery is at least once: a crash after a successful send but before its database save can repeat that answer. Changing bot credentials or destination starts a separate set of links. The receive loop checks connection changes every ten seconds.

Approval, completion, and needs-you notices keep the existing notification settings and agent-provided buttons. For a linked card, they arrive as replies in its conversation. The same approval ID and option reach the normal session service. No second approval flow is added.

These commands use the same services as the app. The project editor reservation, permission requests, resume checks, and completion checks apply to bot commands too. A second card cannot take over a project folder that another session owns.

## Claude Code

The adapter runs a persistent `claude -p` process with stream JSON input/output. Permission requests use `--permission-prompts=host` and `--permission-prompt-tool=stdio`. A native `can_use_tool` control request becomes Marshal's existing approval block. Approve returns the original tool input unchanged; deny returns a refusal. Request cancellation, turn completion, and process exit resolve waiting blocks so their buttons do not stay active.

The native permission mode still applies: Ask uses `default`, Auto-accept edits uses `acceptEdits`, Plan uses `plan`, Full auto uses `auto`, and Bypass uses `bypassPermissions`. Marshal's harness also checks requests that reach the host. Explicit native deny rules still take precedence.

## Built-in agent

The production registry now supplies the history store's conversation reader to the built-in agent. A resume resolves the native ID to one durable card or project-chat session, reads every saved page in order, joins streamed answer chunks, and restores user messages and recorded tool activity. Tool activity is supplied as text context because the display history does not contain the original native tool inputs or signed thinking blocks. It is never replayed as an executable call. The role instructions are supplied again for each stateless provider request.

Missing, unreadable, ambiguous, or oversized saved history refuses resume instead of silently starting an empty conversation. The read has a default 4 MiB bound, including row overhead, configurable through `history.WithConversationBytes`. Live conversations also retain tool results across subsequent turns. Regression checks cover card/chat isolation, multiple history pages, tool output, read failures, and two HTTP stop/restart/resume cycles with a fixture provider; no paid provider calls are used by these tests.

## Codex

The **Test** button in Settings starts the same app-server command and checks that it answers Marshal’s startup greeting. It does not create a thread or send a model prompt. Expand an agent row to see the version. The tested-version badge reflects the versions verified through the full card lifecycle.

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

The stuck guard compares completed edits using a SHA-256 fingerprint of the actual arguments or reported diff. Distinct changes to one file count as progress; repeated inputs and alternating edit patterns still pause the card. Failed attempts continue to feed the repeated-error rule. Missing comparison data keeps the conservative guard. Pending edit metadata and detector history are bounded.

The real check uses the built daemon and its public HTTP API, with a fresh data folder, sample repository, and free port. It does not call a CLI directly or touch the owner's projects. It exercises the same adapters, working folder, MCP attachment, card approval rows, history, and start/stop actions used by the app. A stopped session continues through `POST /v1/cards/{id}/start`, matching the app's Resume agent action; `/resume` is reserved for sessions waiting after a daemon restart.

Run only when the owner requests a real account check:

```sh
pnpm --filter daemon build
node scripts/agent-lifecycle-smoke.mjs --real --agents claude,codex --model-codex default
```

It calls Marshal's `board_status` MCP tool, proves a file does not exist before approval, approves its creation, denies a second write and checks that it did not run, checks durable approval history, then stops/resumes and asks for a marker retained only in the conversation. It cleans up its daemon and temporary data. Browser checks and automatic CI model calls are excluded.

On 2026-11-03 this flow passed with Claude Code **2.1.299** and Codex CLI **0.166.0-alpha.5.1**. Automated tests additionally exercise the actual Claude adapter through HTTP with a protocol fixture, cancellation and duplicate replies, Codex JSON-RPC subprocess fixtures, failed/interrupted turns, startup cancellation, grouped edits, and path aliases.

On 2026-11-05 the same real lifecycle check passed with Codex CLI **0.166.0-alpha.6**.

Protocol references: [Claude SDK permission transport](https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/_internal/transport/subprocess_cli.py), [Claude control responses](https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/_internal/query.py), [Codex app-server](https://developers.openai.com/codex/app-server), and [Codex MCP configuration](https://developers.openai.com/codex/config-reference).

For the owner-requested complete coding check, `node scripts/coding-task-smoke.mjs --real` copies the real Go budget module and its daemon/stub-agent test dependencies into a temporary workspace. It first runs the unchanged baseline tests, then uses separate sequential Claude and Codex cards to add two CLI features. Codex runs short tests inside its native sandbox because the full suite needs listening sockets; Marshal local CI runs the unchanged full suite on the host. Completion uses the app’s asynchronous `send-to-merge` action. Each card must call Marshal MCP, pass the project's full Go suite through local CI, pass independent CLI output assertions, expose a card diff, pass the actual model Reviewer, and reach Done with its session stopped. The source module and branch are checked for changes. Results, bounded patches, local-CI output, and review comments are saved to `dist/coding-task-smoke.json`; the temporary project and daemon are removed. The test features are not added to Marshal's source module.
