/**
 * Generates packages/protocol/src/generated from the Go wire types with tygo, then formats the
 * result with Biome so it passes the same checks as hand-written code. Run through `pnpm gen`.
 * The output is never edited by hand.
 */
import { spawnSync } from "node:child_process";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const useShell = process.platform === "win32";

function run(label, command, args, cwd) {
  const result = spawnSync(command, args, { cwd, stdio: "inherit", shell: useShell });
  if (result.status !== 0) {
    console.error(`${label} failed.`);
    process.exit(result.status ?? 1);
  }
}

run(
  "tygo",
  "node",
  [join(root, "scripts", "go-tool.mjs"), "tygo", "generate"],
  join(root, "daemon"),
);
run(
  "biome format",
  "pnpm",
  [
    "exec",
    "biome",
    "check",
    "--write",
    "--linter-enabled=false",
    "packages/protocol/src/generated",
  ],
  root,
);
