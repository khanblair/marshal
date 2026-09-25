/**
 * Size budgets for the web build (`pnpm build` first). Fails when a budget is exceeded.
 * The daemon budgets in docs/architecture.md section 14 are measured when the daemon exists;
 * the ones here are the frontend's own.
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { extname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";

const DIST = fileURLToPath(new URL("../apps/web/dist", import.meta.url));
const BYTES_PER_KB = 1024;
const KB_PER_MB = 1024;

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
  return failed ? 1 : 0;
}

process.exit(main());
