/**
 * Checks that the generated files are what `pnpm gen` makes.
 *
 *   node scripts/check-generated.mjs              runs `pnpm gen` again and fails if it changes any
 *                                                 generated file (a second run must change nothing)
 *   node scripts/check-generated.mjs --committed  fails if the generated files differ from what Git
 *                                                 holds (CI runs `pnpm gen` first, then this)
 *
 * The first form works on a working tree with uncommitted changes, so `scripts/check.mjs` uses it.
 * The second form needs the generated files to be committed, which is the case in CI.
 */
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const useShell = process.platform === "win32";

/** The generated files that Git holds. Tokens' own output is not committed, so it is not here. */
const GENERATED = [
  "packages/protocol/src/generated",
  "packages/ui/src/index.ts",
  "daemon/internal/store/db",
];

function* filesUnder(path) {
  const full = join(root, path);
  if (!statSync(full).isDirectory()) {
    yield path;
    return;
  }
  for (const name of readdirSync(full).sort()) yield* filesUnder(`${path}/${name}`);
}

function snapshot() {
  const hashes = new Map();
  for (const path of GENERATED) {
    for (const file of filesUnder(path)) {
      hashes.set(
        file,
        createHash("sha256")
          .update(readFileSync(join(root, file)))
          .digest("hex"),
      );
    }
  }
  return hashes;
}

function git(args) {
  return spawnSync("git", args, { cwd: root, encoding: "utf8", shell: useShell });
}

function fail(lines) {
  for (const line of lines) console.error(line);
  return 1;
}

function checkCommitted() {
  const changed = git(["diff", "--name-only", "--", ...GENERATED])
    .stdout.split("\n")
    .filter(Boolean);
  const untracked = git(["ls-files", "--others", "--exclude-standard", "--", ...GENERATED])
    .stdout.split("\n")
    .filter(Boolean);
  const stale = [...changed, ...untracked];
  if (stale.length === 0) {
    console.log("ok    the generated files are what Git holds");
    return 0;
  }
  return fail([
    "FAIL  these generated files differ from the committed ones. Run `pnpm gen` and commit the result:",
    ...stale.map((file) => `        ${file}`),
  ]);
}

function checkSecondRun() {
  const before = snapshot();
  const run = spawnSync("pnpm", ["gen"], { cwd: root, stdio: "inherit", shell: useShell });
  if (run.status !== 0) return fail(["FAIL  `pnpm gen` failed on its second run."]);
  const after = snapshot();
  const changed = [...new Set([...before.keys(), ...after.keys()])].filter(
    (file) => before.get(file) !== after.get(file),
  );
  if (changed.length === 0) {
    console.log("ok    a second `pnpm gen` changes nothing");
    return 0;
  }
  return fail([
    "FAIL  a second `pnpm gen` changed these files, so generation is not repeatable:",
    ...changed.map((file) => `        ${file}`),
  ]);
}

process.exit(process.argv.includes("--committed") ? checkCommitted() : checkSecondRun());
