/**
 * Runs a Go tool from `.tools/bin` (or `go` itself) with that folder first on PATH, so pnpm
 * scripts work the same on every platform.
 *
 *   node scripts/go-tool.mjs golangci-lint run ./...
 *   node scripts/go-tool.mjs go test ./...
 *
 * The working folder is the caller's, so a package script can run it inside its own folder.
 */
import { spawnSync } from "node:child_process";
import { delimiter, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const binDir = join(root, ".tools", "bin");

const [tool, ...args] = process.argv.slice(2);
if (!tool) {
  console.error("Usage: node scripts/go-tool.mjs <tool> [arguments]");
  process.exit(2);
}

const run = spawnSync(tool, args, {
  stdio: "inherit",
  env: { ...process.env, PATH: `${binDir}${delimiter}${process.env.PATH ?? ""}` },
});
if (run.error) {
  const hint = tool === "go" ? "Install Go first." : "Run `pnpm setup:tools` first.";
  console.error(`Could not start ${tool}: ${run.error.message}. ${hint}`);
  process.exit(1);
}
process.exit(run.status ?? 1);
