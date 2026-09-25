/**
 * Code smell checks for our own code (code-standards.md section 12).
 *
 * Blocking: file length over 800 lines, unused code (knip), duplicated code of
 * 30 lines or more (jscpd). Warnings: file length over 400 lines, and the soft
 * limits for function length, complexity, and parameters, for TypeScript (Biome) and
 * Go (golangci-lint, `.golangci.warn.yml`). The hard limits for function length,
 * complexity, parameters, and `any` run in `pnpm lint`.
 *
 * There is no git history here yet, so this always checks the whole repo.
 * Accepted smells live in .smells-baseline.json, each with a reason and a task.
 */
import { spawnSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
/** On Windows, pnpm is a .cmd file, which Node can only start through a shell. */
const useShell = process.platform === "win32";
const FILE_WARN_LINES = 400;
const FILE_BLOCK_LINES = 800;
/** Folders that exist only once something is added to them (`tools/`) are skipped until then. */
const SOURCE_ROOTS = ["apps", "daemon", "packages", "scripts", "tools"].filter((dir) =>
  existsSync(join(root, dir)),
);
const SOURCE_EXT = /\.(ts|tsx|css|mjs|go)$/;
const SKIP_DIR = new Set([
  "node_modules",
  "dist",
  "coverage",
  "test-results",
  "playwright-report",
  "testdata",
  "tmp",
]);
/** Generated code is not written by hand, so its size is not a smell (`generated/` from tygo, `store/db/` from sqlc). */
const SKIP_FILE = /(\.d\.ts|\/dist\/|\/generated\/|\/store\/db\/)/;

const baseline =
  JSON.parse(readFileSync(join(root, ".smells-baseline.json"), "utf8")).accepted ?? [];
const isAccepted = (rule, file) => baseline.some((b) => b.rule === rule && b.file === file);

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIR.has(name)) continue;
    const path = join(dir, name);
    if (statSync(path).isDirectory()) yield* walk(path);
    else if (SOURCE_EXT.test(name) && !SKIP_FILE.test(path.replaceAll("\\", "/"))) yield path;
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
    { cwd: root, encoding: "utf8", shell: useShell },
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

/** The Go modules in the repo: the daemon and each tool that has a go.mod. */
function goModules() {
  const found = [];
  if (existsSync(join(root, "daemon", "go.mod"))) found.push("daemon");
  const tools = join(root, "tools");
  if (existsSync(tools)) {
    for (const name of readdirSync(tools)) {
      if (existsSync(join(tools, name, "go.mod"))) found.push(`tools/${name}`);
    }
  }
  return found;
}

/** The soft Go limits, from `.golangci.warn.yml`. A missing tool is reported, not skipped. */
function goSoftLimits() {
  const warnings = [];
  for (const dir of goModules()) {
    const run = spawnSync(
      "node",
      [
        join(root, "scripts", "go-tool.mjs"),
        "golangci-lint",
        "run",
        "--config",
        join(root, ".golangci.warn.yml"),
        "--show-stats=false",
        "--output.json.path",
        "stdout",
        "--output.text.path",
        "stderr",
        "./...",
      ],
      { cwd: join(root, dir), encoding: "utf8" },
    );
    const start = (run.stdout ?? "").indexOf("{");
    if (start < 0) {
      warnings.push(`${dir}: the Go soft limits could not run. Run \`pnpm setup:tools\` first.`);
      continue;
    }
    for (const issue of JSON.parse(run.stdout.slice(start)).Issues ?? []) {
      warnings.push(`${issue.Pos.Filename}:${issue.Pos.Line} ${issue.FromLinter}: ${issue.Text}`);
    }
  }
  return warnings;
}

function runTool(label, args) {
  const run = spawnSync("pnpm", ["exec", ...args], {
    cwd: root,
    encoding: "utf8",
    shell: useShell,
  });
  return { label, ok: run.status === 0, output: `${run.stdout}${run.stderr}`.trim() };
}

function main() {
  let failed = false;
  const lengths = checkFileLengths();
  const soft = [...biomeSoftLimits(), ...goSoftLimits()];
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
