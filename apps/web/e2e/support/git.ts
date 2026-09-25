import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readdirSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { devNull, tmpdir } from "node:os";
import { basename, join } from "node:path";

/**
 * Git with none of the developer's own settings (a global hook or a signing rule could refuse the
 * commit), and the same author every time.
 */
function git(cwd: string, ...args: string[]): string {
  return execFileSync(
    "git",
    ["-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main", ...args],
    {
      cwd,
      encoding: "utf8",
      env: {
        ...process.env,
        GIT_CONFIG_GLOBAL: devNull,
        GIT_CONFIG_NOSYSTEM: "1",
        GIT_AUTHOR_NAME: "Marshal E2E",
        GIT_AUTHOR_EMAIL: "e2e@example.invalid",
        GIT_COMMITTER_NAME: "Marshal E2E",
        GIT_COMMITTER_EMAIL: "e2e@example.invalid",
      },
    },
  );
}

export interface Folder {
  /** The folder's full path, with symbolic links resolved (macOS keeps temp folders under one). */
  dir: string;
  /** The folder's own name, unique for the run. */
  name: string;
  /** Every file and folder in it, top level only, sorted. */
  entries(): string[];
  /** What `git status --porcelain` says: empty when nothing changed. */
  status(): string;
  /** Deletes the folder. It only ever deletes what this made. */
  remove(): void;
}

function folderAt(dir: string): Folder {
  return {
    dir,
    name: basename(dir),
    entries: () => readdirSync(dir).sort(),
    status: () => git(dir, "status", "--porcelain"),
    remove: () => rmSync(dir, { recursive: true, force: true }),
  };
}

/** A new folder with one file and no Git repository, which the daemon refuses. */
export function makeFolder(prefix: string): Folder {
  const dir = mkdtempSync(join(realpathSync(tmpdir()), `${prefix}-`));
  writeFileSync(join(dir, "notes.txt"), "not a repository\n");
  return folderAt(dir);
}

/** A new Git repository with one commit, as the daemon needs to add a project. */
export function makeRepo(prefix: string): Folder {
  const dir = mkdtempSync(join(realpathSync(tmpdir()), `${prefix}-`));
  mkdirSync(join(dir, "src"));
  writeFileSync(join(dir, "README.md"), "# A repository for the end-to-end tests\n");
  writeFileSync(join(dir, "src", "main.go"), "package main\n\nfunc main() {}\n");
  writeFileSync(join(dir, "go.mod"), "module example.invalid/e2e\n\ngo 1.22\n");
  git(dir, "init");
  git(dir, "add", "--all");
  git(dir, "commit", "-m", "First commit");
  return folderAt(dir);
}
