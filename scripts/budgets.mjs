/**
 * Budgets from docs/architecture.md section 14 (`pnpm build` first). Fails when one is exceeded.
 * The web build size is measured here. The daemon's idle memory and processor use are measured
 * by tools/budgets, which starts the built daemon and lets it sit idle. CI sets
 * MARSHAL_BUDGET_IDLE to 60s, which is how long the budget says to watch.
 */
import { spawnSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { extname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";

const DIST = fileURLToPath(new URL("../apps/web/dist", import.meta.url));
const BYTES_PER_KB = 1024;
const KB_PER_MB = 1024;
/** How long the daemon sits idle while it is measured. CI uses 60s. */
const DEFAULT_IDLE = "10s";

const BUDGETS = [
  { name: "JavaScript, gzip", ext: [".js"], gzip: true, maxKb: 300 },
  { name: "CSS, gzip", ext: [".css"], gzip: true, maxKb: 40 },
  { name: "Whole build, raw", ext: null, gzip: false, maxKb: 2 * KB_PER_MB },
];

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) yield* walk(path);
    else yield path;
  }
}

function measure(budget, files) {
  let bytes = 0;
  for (const file of files) {
    if (budget.ext && !budget.ext.includes(extname(file))) continue;
    const data = readFileSync(file);
    bytes += budget.gzip ? gzipSync(data).length : data.length;
  }
  return bytes / BYTES_PER_KB;
}

function main() {
  let files;
  try {
    files = [...walk(DIST)];
  } catch {
    console.error("No build found. Run `pnpm build` first.");
    return 1;
  }
  let failed = false;
  for (const budget of BUDGETS) {
    const kb = measure(budget, files);
    const over = kb > budget.maxKb;
    failed ||= over;
    console.log(
      `${over ? "FAIL" : "ok  "}  ${budget.name.padEnd(20)} ${kb.toFixed(1)} kB of ${budget.maxKb} kB`,
    );
  }
  return daemonBudgets() || (failed ? 1 : 0);
}

/** Starts the built daemon and measures its idle memory and processor use. */
function daemonBudgets() {
  const binary = join(DIST, "..", "..", "..", "dist", "bin", "marshald");
  const built = existsSync(binary) || existsSync(`${binary}.exe`);
  if (!built) {
    console.error("No daemon build found. Run `pnpm build` first.");
    return 1;
  }
  const idle = process.env.MARSHAL_BUDGET_IDLE ?? DEFAULT_IDLE;
  const run = spawnSync("pnpm", ["--filter", "budgets", "measure", "-idle", idle], {
    stdio: "inherit",
    shell: process.platform === "win32",
  });
  return run.status ?? 1;
}

process.exit(main());
