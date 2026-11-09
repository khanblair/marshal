# Direct project workspaces

Accepted direction, 2026-11-02. This replaces the earlier per-card worktree and Integrator worktree design.

## Editing model

You can add the root of a local Git repository or a standalone folder. When you add a plain folder, Marshal creates a local `.git` directory and one empty commit so card diffs work. Your files stay unstaged. GitHub is optional. If the folder is inside an existing repository, choose that repository’s root.

Marshal runs a card's main coding session in the repository folder the owner added. It does not create a separate checkout, change the current branch, install another copy of dependencies, or merge a card branch back into the folder.

One main coding session owns each project folder until it stops or sleeps. An idle or paused session still owns it. Other independent cards and coding chats receive a refusal naming the current owner. Different project folders can run together. Folder aliases share the same reservation.

The main session can delegate to the selected CLI's native subagents. Its instructions require separate file ownership, sequential edits to shared files, and Git coordination by the main agent. This is cooperation within one session, not filesystem isolation. Native subagents can still conflict if they ignore their assignments. Marshal does not offer a new native subagent dashboard or a Codex adapter as part of this change.

The Orchestrator and temporary reviewers remain restricted to reading and planning. Integrator chats from the earlier merge design cannot start an editing workspace.

## Snapshots and completion

Starting a card captures the existing folder as a baseline. Ending a turn captures its result. These are Git objects under `refs/marshal/direct/`, written through a temporary index. They do not stage files in the owner's index or add commits to the current branch. Existing dirty and untracked files are part of the baseline, so they are not presented as new card changes.

The Diff tab compares the card's baseline and result. A completed result stays fixed after later cards change the same files. While a turn is running, the diff can read a current snapshot.

Finish card runs the configured review and checks in the project folder, then stops the main session and marks the card Done. Review uses the exact snapshot being checked. If the folder changes after that snapshot or while checks run, completion stops and asks for another review. No automatic merge, branch switch, push, or commit of all source changes happens.

The project folder stays reserved during completion. Failed review or checks leave the work in place for correction. Checks can still create ignored build outputs through the project's own commands.

When review or checks request fixes, the completion reservation ends before the correction message reaches the main session. That session keeps its editing ownership while it makes the fixes. A process still shutting down keeps the folder reserved until its event stream drains and the final snapshot is saved.

Checkpoints use snapshots too. Automatic restoration of one card is unavailable in a shared folder because it could overwrite the owner's work or another card's later changes. Snapshots are kept for deliberate recovery. The old merge Undo route refuses direct mode.

## Existing cards

Older worktree folders, branches, and sessions are not automatically moved or deleted by this change. Starting an old card refuses with a recovery explanation and its recorded location. Recover its changes deliberately into the project, then start a new card. Existing sessions are not silently relocated, because CLI resume state can retain the old working directory.

A direct card can resume only when the project is still on its recorded branch and its files match the card's last saved snapshot. If another card or the owner changed the files after it stopped, start a new card for that work. The previous conversation and snapshots remain available.

Deleting a direct card removes Marshal's task/session data and keeps the project folder and its borrowed branch. Removing a project also keeps the source folder and direct-card branches. Explicit deletion of an old card retains its existing legacy cleanup rules and confirmation.

## Compatibility

The database and API still use the older `worktree` path field to identify a card's workspace. Direct cards store the original project folder in it. Existing merge route names remain compatible but route direct cards through completion. Internal legacy code remains for tests and recovery compatibility; application wiring always selects direct workspaces, with no user-facing worktree option.

Only one dev-server preview may run for the same source folder at a time. Cards share the source and dependencies; their screenshot profiles remain separate.

## Installed and development instances

Normal and development daemons use different ports and data folders, but editing sessions now acquire an OS lock in the user's shared Marshal workspace-lock folder. The lock identifies the physical project directory, so symlink aliases do not bypass it. Another card or coding chat in another updated daemon must wait until the owner stops or sleeps. The main agent's coordinated native subagents remain part of the same owner. The OS releases the lock if the daemon exits or crashes; lock files are not deleted during release.

Both instances must run the updated daemon for this protection. An older installed version does not acquire these locks. Do not restart a daemon underneath an active coding session merely to apply this change.
