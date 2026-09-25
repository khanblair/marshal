/**
 * Runs everything CI runs, in order, and prints one summary.
 * Steps stop the run on failure unless `--keep-going` is passed.
 */
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const keepGoing = process.argv.includes("--keep-going");

const STEPS = [
  ["generate tokens and ui index", "pnpm", ["gen"]],
  ["format check", "pnpm", ["format:check"]],
  ["lint", "pnpm", ["lint"]],
  ["type check", "pnpm", ["typecheck"]],
  ["code smells", "pnpm", ["smells"]],
  ["token contrast", "pnpm", ["--filter", "@marshal/tokens", "check:contrast"]],
  ["unit tests", "pnpm", ["test"]],
  ["build and budgets", "sh", ["-c", "pnpm build && pnpm budgets"]],
];

const results = [];
for (const [label, cmd, args] of STEPS) {
  console.log(`\n== ${label}`);
  const started = Date.now();
  const run = spawnSync(cmd, args, { cwd: root, stdio: "inherit" });
  const ok = run.status === 0;
  results.push({ label, ok, seconds: Math.round((Date.now() - started) / 1000) });
  if (!ok && !keepGoing) break;
}

console.log("\nSummary");
for (const r of results) console.log(`  ${r.ok ? "pass" : "FAIL"}  ${r.label} (${r.seconds}s)`);
const failed = results.some((r) => !r.ok) || results.length < STEPS.length;
process.exit(failed ? 1 : 0);
