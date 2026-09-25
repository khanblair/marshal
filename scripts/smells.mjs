/**
 * Code smell checks for our own code (code-standards.md section 12).
 *
 * Blocking: file length over 800 lines, unused code (knip), duplicated code of
 * 30 lines or more (jscpd). Warnings: file length over 400 lines, and the soft
 * limits for function length, complexity, and parameters. The hard limits for
 * function length, complexity, parameters, and `any` run in `pnpm lint`.
 *
 * There is no git history here yet, so this always checks the whole repo.
 * Accepted smells live in .smells-baseline.json, each with a reason and a task.
 */
import { spawnSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const root = new URL("..", import.meta.url).pathname;
const FILE_WARN_LINES = 400;
const FILE_BLOCK_LINES = 800;
/** Folders that exist only once something is added to them (`tools/`) are skipped until then. */
const SOURCE_ROOTS = ["apps", "packages", "scripts", "tools"].filter((dir) =>
  existsSync(join(root, dir)),
);
const SOURCE_EXT = /\.(ts|tsx|css|mjs)$/;
const SKIP_DIR = new Set(["node_modules", "dist", "coverage", "test-results", "playwright-report"]);
const SKIP_FILE = /(\.d\.ts|\/dist\/)/;

const baseline =
  JSON.parse(readFileSync(join(root, ".smells-baseline.json"), "utf8")).accepted ?? [];
const isAccepted = (rule, file) => baseline.some((b) => b.rule === rule && b.file === file);

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIR.has(name)) continue;
    const path = join(dir, name);
    if (statSync(path).isDirectory()) yield* walk(path);
    else if (SOURCE_EXT.test(name) && !SKIP_FILE.test(path)) yield path;
  }
}

function checkFileLengths() {
  const blocking = [];
  const warnings = [];
  for (const top of SOURCE_ROOTS) {
    for (const path of walk(join(root, top))) {
      const file = relative(root, path);
      const lines = readFileSync(path, "utf8").split("\n").length;
      if (lines > FILE_BLOCK_LINES && !isAccepted("file-length", file))
        blocking.push(`${file}: ${lines} lines`);
      else if (lines > FILE_WARN_LINES) warnings.push(`${file}: ${lines} lines`);
    }
  }
  return { blocking, warnings };
}

function biomeSoftLimits() {
  const run = spawnSync(
    "pnpm",
    [
      "exec",
      "biome",
      "lint",
      "--config-path=biome.warn.json",
      "--only=complexity/noExcessiveCognitiveComplexity",
      "--only=complexity/noExcessiveLinesPerFunction",
      "--only=complexity/useMaxParams",
      "--reporter=json",
      "--max-diagnostics=1000",
      ...SOURCE_ROOTS,
    ],
    { cwd: root, encoding: "utf8" },
  );
  const start = run.stdout.indexOf("{");
  if (start < 0) return [];
  const report = JSON.parse(run.stdout.slice(start));
  return report.diagnostics
    .filter((d) => !/\.test\.tsx?$/.test(d.location?.path ?? ""))
    .map(
      (d) =>
        `${d.location.path}:${d.location.start.line} ${d.category.split("/").pop()}: ${d.message}`,
    );
}

function runTool(label, args) {
  const run = spawnSync("pnpm", ["exec", ...args], { cwd: root, encoding: "utf8" });
  return { label, ok: run.status === 0, output: `${run.stdout}${run.stderr}`.trim() };
}

function main() {
  let failed = false;
  const lengths = checkFileLengths();
  const soft = biomeSoftLimits();
  const knip = runTool("unused code (knip)", ["knip", "--no-progress"]);
  const dupes = runTool("duplicated code (jscpd)", [
    "jscpd",
    ...SOURCE_ROOTS,
    "--config",
    ".jscpd.json",
  ]);

  console.log("Code smell checks\n");
  const report = (label, blocking) => {
    console.log(`${blocking.length ? "FAIL" : "ok  "}  ${label}`);
    for (const line of blocking) console.log(`        ${line}`);
    if (blocking.length) failed = true;
  };
  report("file length over 800 lines", lengths.blocking);
  for (const tool of [knip, dupes]) {
    console.log(`${tool.ok ? "ok  " : "FAIL"}  ${tool.label}`);
    if (!tool.ok) {
      failed = true;
      console.log(
        tool.output
          .split("\n")
          .map((l) => `        ${l}`)
          .join("\n"),
      );
    }
  }
  console.log(
    `\nWarnings (fix, or explain in the change): ${lengths.warnings.length + soft.length}`,
  );
  for (const line of [...lengths.warnings, ...soft].slice(0, 60)) console.log(`  warn  ${line}`);
  process.exit(failed ? 1 : 0);
}

main();
