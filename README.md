# Marshal

Marshal is a local-first desktop app for running AI coding agents across your Git projects. Each project has a board, and each card holds a task, its agent conversation, and the changes the agent made.

Bring your existing Claude Code or Codex CLI account. Start and resume sessions, respond to supported permission requests, and inspect diffs from the card. Marshal also has a built-in agent for configured model providers.

## Install on macOS

The current desktop release supports macOS 12 or later on Apple silicon and Intel Macs.

1. Download the [installer script](install.sh) with the command below and review it before running it.

   ```sh
   curl -fsSL https://raw.githubusercontent.com/khanblair/marshal/main/install.sh -o marshal-install.sh
   ```

2. Run the saved script.

   ```sh
   sh ./marshal-install.sh
   ```

3. Open **Marshal** from **Applications**.

The script checks the download against GitHub’s checksum and installs it in `/Applications`. You can also download the universal DMG from [GitHub Releases](https://github.com/khanblair/marshal/releases/latest). That DMG uses ad-hoc signing, so macOS may block the first launch after a browser download. For alternate install folders and details on how updates work, see the [installation and update guide](docs/app-updates.md).

Already installed? Open **Settings → Updates** to check for and install a newer version.

## Start your first task

1. Install and sign in to the agent CLI you want to use, or set up a provider for Marshal’s built-in agent.
2. In Marshal, choose **New project** and select a folder. Marshal sets up local Git if the folder has no repository.
3. On the board, choose **Add a card**, describe the task, and start its agent session.
4. Follow the conversation, respond to approvals, and review the **Diff** tab. Use **Finish card** when the work is ready for the project’s configured checks and review.

Agents edit the project folder you added. Marshal reserves that folder for one main editing session at a time. That session can coordinate its agent’s native subagents, while other projects can run in parallel. [How shared project folders work](docs/direct-workspace.md)

## Develop locally

You’ll need Git, Go (see [go.mod](daemon/go.mod)), Node.js 24 or later, and Corepack for the pinned pnpm version.

```sh
git clone https://github.com/khanblair/marshal.git
cd marshal
corepack enable
pnpm install --frozen-lockfile
pnpm setup:tools
pnpm gen
pnpm dev
```

Open [localhost:3210](http://localhost:3210). This runs the web interface and the dev daemon with separate data from an installed copy. The dev launcher uses real agents when you start a session.

| Command | What it does |
| --- | --- |
| `pnpm test:web` | Run web tests |
| `pnpm test:daemon` | Run daemon tests |
| `pnpm check` | Run the repository checks |
| `pnpm desktop:dev` | Run the Tauri desktop shell with the desktop prerequisites installed |

See the [development guide](docs/development.md) for prerequisites, build commands, and troubleshooting.

## Documentation and feedback

- [Documentation index](docs/README.md) for the architecture, product scope, and current status.
- [Agent sessions](docs/agent-sessions.md) for Claude Code and Codex support and limits.
- [GitHub Issues](https://github.com/khanblair/marshal/issues) for bug reports and feature requests. For an install or agent issue, include your OS, Marshal version, agent CLI version if relevant, and steps to reproduce.
