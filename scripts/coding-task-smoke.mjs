/** End-to-end coding checks using real agents through Marshal's public API. */
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import {
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

function option(name, fallback) {
  const at = process.argv.indexOf(`--${name}`);
  return at < 0 ? fallback : process.argv[at + 1];
}

if (!process.argv.includes("--real")) {
  throw new Error("Pass --real to run the end-to-end check with signed-in real coding agents.");
}

const sleep = (ms) => new Promise((done) => setTimeout(done, ms));
const root = fileURLToPath(new URL("..", import.meta.url));
const bin = join(root, "dist/bin/marshald");
const source = join(root, "tools/budgets");
assert.ok(existsSync(bin), "Build the daemon first.");
assert.ok(existsSync(source));

function run(command, args, cwd) {
  try {
    return execFileSync(command, args, {
      cwd,
      encoding: "utf8",
      timeout: 180_000,
      stdio: ["ignore", "pipe", "pipe"],
    });
  } catch (error) {
    throw new Error(
      `${command} ${args.join(" ")} failed: ${String(error.stderr ?? error.stdout ?? error.message).slice(-3000)}`,
    );
  }
}

function sourceManifest(dir) {
  const files = [];
  const visit = (path) => {
    for (const entry of readdirSync(path, { withFileTypes: true })) {
      const full = join(path, entry.name);
      if (entry.isDirectory()) visit(full);
      else if (entry.isFile()) files.push([relative(dir, full), readFileSync(full)]);
    }
  };
  visit(dir);
  return JSON.stringify(files.map(([name, bytes]) => [name, bytes.toString("base64")]));
}

const sourceBefore = sourceManifest(source);
const sourceHeadBefore = run("git", ["rev-parse", "HEAD"], root).trim();
const temp = mkdtempSync(join(tmpdir(), "marshal-coding-check-"));
const workspace = join(temp, "workspace");
const repo = join(workspace, "tools/budgets");
const dataDir = join(temp, "marshal-data");
mkdirSync(repo, { recursive: true });
cpSync(source, repo, {
  recursive: true,
  filter: (path) => !/(^|\/)(?:\.git|node_modules|dist|build|coverage)(\/|$)/.test(path),
});
// The real module's integration test builds ../../daemon and ../stub-agent.
// Preserve that layout inside the temporary copy instead of skipping the test.
for (const module of ["daemon", "tools/stub-agent"]) {
  cpSync(join(root, module), join(workspace, module), {
    recursive: true,
    filter: (path) =>
      !/(^|\/)(?:\.git|node_modules|\.tmp|coverage)(\/|$)/.test(path) &&
      !path.endsWith("coverage.out"),
  });
}
mkdirSync(join(repo, ".github/workflows"), { recursive: true });
writeFileSync(
  join(repo, ".github/workflows/check.yml"),
  "name: Go checks\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - name: Go tests\n        run: go test ./...\n",
);
run("git", ["init", "-b", "main"], repo);
run("git", ["config", "user.name", "Marshal Coding Check"], repo);
run("git", ["config", "user.email", "marshal-coding-check@example.invalid"], repo);
run("git", ["add", "."], repo);
run("git", ["commit", "-m", "Baseline CLI"], repo);
const initialWorktrees = run("git", ["worktree", "list", "--porcelain"], repo)
  .trim()
  .split("\n\n").length;
assert.equal(initialWorktrees, 1);
console.log("Checking temporary project baseline tests before agent work...");
let baselineTests;
try {
  baselineTests = run("go", ["test", "./..."], repo);
} catch (error) {
  rmSync(temp, { recursive: true, force: true });
  throw error;
}
const initialRepoHead = run("git", ["rev-parse", "HEAD"], repo).trim();

const port = await new Promise((resolve, reject) => {
  const server = createServer();
  server.once("error", reject);
  server.listen(0, "127.0.0.1", () => {
    const chosen = server.address().port;
    server.close(() => resolve(chosen));
  });
});
const daemon = spawn(
  bin,
  [
    "--dev",
    "--agent",
    "real",
    "--port",
    String(port),
    "--data-dir",
    dataDir,
    "--log-level",
    "error",
  ],
  {
    env: { ...process.env, MARSHAL_FIXTURE: "", MARSHAL_TAILNET: "false", MARSHAL_FUNNEL: "false" },
    stdio: ["ignore", "ignore", "inherit"],
  },
);
const base = `http://127.0.0.1:${port}`;
let token;
const report = { cards: [], projectId: null, checks: [], baselineTests, sourceUnchanged: false };

async function call(method, path, body) {
  const response = await fetch(base + path, {
    method,
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(180_000),
  });
  const text = await response.text();
  let data;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = text;
  }
  if (!response.ok)
    throw new Error(`${method} ${path}: HTTP ${response.status}: ${String(text).slice(-3000)}`);
  return data;
}

async function until(check, label, timeout = 360_000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const result = await check();
    if (result) return result;
    await sleep(1_000);
  }
  throw new Error(`Timed out: ${label}`);
}

async function history(id) {
  return call("GET", `/v1/cards/${id}/messages?limit=200`);
}

async function verifyAgentWork(card, spec, cardRepo) {
  const page = await history(card.id);
  assert.ok(
    page.items?.some(
      (message) =>
        message.tool?.name?.includes("board_status") ||
        message.tool?.title?.includes("board_status"),
    ),
    `${spec.agent} did not call board_status through MCP`,
  );
  const localCI = await call("POST", `/v1/cards/${card.id}/local-ci`, {});
  report.localCI ??= [];
  report.localCI.push({ agent: spec.agent, cardId: card.id, result: localCI });
  const steps = (localCI.workflows ?? []).flatMap((workflow) => workflow.steps ?? []);
  assert.ok(
    steps.some((step) => step.status === "passed" && step.command?.includes("go test ./...")),
    `${spec.agent} local CI has no passed go test ./... step: ${JSON.stringify(steps).slice(0, 2000)}`,
  );
  assert.ok(
    !steps.some((step) => step.status === "failed"),
    `${spec.agent} local CI failed: ${JSON.stringify(steps.filter((step) => step.status === "failed")).slice(0, 2000)}`,
  );
  if (spec.agent === "claude") {
    assert.equal(run("go", ["run", ".", "-version"], cardRepo), "budgets dev\n");
  } else {
    assert.deepEqual(JSON.parse(run("go", ["run", ".", "-print-config"], cardRepo)), {
      idle: "10s",
      maxRssMb: 100,
      maxCpuPercent: 1,
    });
    assert.deepEqual(
      JSON.parse(
        run("go", ["run", ".", "-print-config", "-idle", "2s", "-max-rss-mb", "50"], cardRepo),
      ),
      { idle: "2s", maxRssMb: 50, maxCpuPercent: 1 },
    );
  }
  const changed = run("git", ["diff", "--no-ext-diff", "--name-only", "HEAD"], cardRepo).trim();
  const recordedDiff = await call("GET", `/v1/cards/${card.id}/diff`);
  assert.ok(recordedDiff.files?.length, `${spec.agent} changes missing from Marshal's card diff`);
  const added = run("git", ["ls-files", "--others", "--exclude-standard"], cardRepo).trim();
  const cardDiff = [...new Set(`${changed}\n${added}`.split("\n").filter(Boolean))];
  assert.ok(cardDiff.length, `${spec.agent} card produced no diff`);
  const trackedPatch = run("git", ["diff", "--binary", "HEAD"], cardRepo);
  const untrackedPatch = added
    .split("\n")
    .filter(Boolean)
    .map((path) => {
      const content = readFileSync(join(cardRepo, path), "utf8");
      return `--- /dev/null\n+++ b/${path}\n${content}`;
    })
    .join("\n");
  return { files: cardDiff, patch: `${trackedPatch}\n${untrackedPatch}`.slice(0, 20_000), page };
}

async function runCard(project, spec) {
  const testInstructions =
    spec.agent === "codex"
      ? "Inside the Codex sandbox run go test -short ./... with one reusable Go cache. The full suite starts a local daemon and needs a listening socket that the sandbox blocks; Marshal local CI will run that unchanged full suite outside the sandbox. Do not weaken or skip integration tests in the source, and do not keep creating fresh Go caches."
      : "Run the complete go test ./... suite.";
  const card = await call("POST", `/v1/projects/${project.id}/cards`, {
    title: spec.title,
    body: `${spec.task}\n\nCall Marshal's board_status MCP tool exactly once to confirm MCP works. Implement and test the requested behavior. ${testInstructions} Leave all changes uncommitted. Do not create branches, commits, or subagents; do not call finish_card. When completely done, reply with a concise summary.`,
    agent: spec.agent,
    model: spec.model,
    permissionMode: "full-auto",
  });
  const path = `/v1/cards/${card.id}`;
  const start = await call("POST", `${path}/start`);
  assert.ok(start.worktree, `${spec.agent} card did not receive a project folder`);
  assert.equal(
    realpathSync(start.worktree),
    realpathSync(repo),
    `${spec.agent} did not edit the isolated project folder`,
  );
  const final = await until(
    async () => {
      const current = await call("GET", path);
      if (current.needsReason)
        throw new Error(
          `${spec.agent} needs intervention: ${JSON.stringify(current.needsReason).slice(0, 2000)}`,
        );
      if (current.session === "stopped")
        throw new Error(
          `${spec.agent} stopped before review: ${JSON.stringify(current).slice(0, 2000)}`,
        );
      if (current.session !== "awake") return null;
      const page = await history(card.id);
      // The app considers a turn finished when it is awake. Streamed text can
      // split a marker across chunks; judge the actual work with tests below.
      if (!(page.items ?? []).some((message) => message.kind === "agent" && message.text))
        return null;
      const diff = await call("GET", `${path}/diff`);
      return diff.files?.length ? current : null;
    },
    `${spec.agent} card to finish`,
    600_000,
  ).catch(async (error) => {
    report.failedCard = await call("GET", path).catch(() => null);
    report.failedHistory = await history(card.id).catch(() => null);
    report.failedDiff = run("git", ["diff", "HEAD"], repo).slice(0, 24_000);
    throw error;
  });
  const cardRepo = final.worktree;
  assert.ok(cardRepo && existsSync(cardRepo), "Card worktree is unavailable for direct CLI check");
  const workEvidence = await verifyAgentWork(card, spec, cardRepo);
  const commentsBefore = await call("GET", `/v1/cards/${card.id}/comments`);
  // Match the app's Complete action: review/checks run asynchronously, avoiding
  // the normal HTTP write timeout during a longer model review.
  const merge = await call("POST", `${path}/send-to-merge`, {});
  assert.ok(merge, `${spec.agent} merge/review returned no result`);
  const done = await until(
    async () => {
      const current = await call("GET", path);
      if (current.state === "needs")
        throw new Error(
          `${spec.agent} review or merge needs intervention: ${JSON.stringify(current.needsReason).slice(0, 2000)}`,
        );
      if (current.state === "done" && current.session === "stopped") return current;
      return null;
    },
    `${spec.agent} review and merge`,
    360_000,
  );
  const commentsAfter = await call("GET", `/v1/cards/${card.id}/comments`);
  const comments = commentsAfter.items ?? commentsAfter.comments ?? [];
  const reviewComments = comments.filter(
    (comment) =>
      !(commentsBefore.items ?? commentsBefore.comments ?? []).some(
        (before) => before.id === comment.id,
      ),
  );
  assert.ok(reviewComments.length > 0, `${spec.agent} reviewer left no comment evidence`);

  report.cards.push({
    agent: spec.agent,
    model: spec.model,
    id: card.id,
    title: spec.title,
    status: done.state,
    session: done.session,
    diffFiles: workEvidence.files,
    diffPatch: workEvidence.patch,
    taskSummary: (workEvidence.page.items ?? [])
      .reduce(
        (latest, message) =>
          message.kind === "agent" && (!latest || message.seq > latest.seq) ? message : latest,
        null,
      )
      ?.text?.slice(0, 2000),
    checks: [
      "board_status MCP",
      "local Go tests passed",
      "direct CLI behavior",
      "Reviewer and completion flow",
    ],
    reviewComments: reviewComments.slice(0, 10).map(({ body, text, author, createdAt }) => ({
      body: (body ?? text ?? "").slice(0, 2000),
      author,
      createdAt,
    })),
  });
  console.log(
    `PASS ${spec.agent}: project folder, MCP, local CI, CLI behavior, and completion review.`,
  );
}

try {
  await until(
    async () => {
      try {
        return (await fetch(`${base}/v1/health`)).ok;
      } catch {
        return false;
      }
    },
    "daemon health",
    30_000,
  );
  token = readFileSync(join(dataDir, "dev-token"), "utf8").trim();
  const project = await call("POST", "/v1/projects", {
    source: "folder",
    path: repo,
    name: "Marshal CLI coding check",
  });
  report.projectId = project.id;
  await call("PUT", `/v1/projects/${project.id}/integration/settings`, {
    autoShip: false,
    review: true,
    checks: true,
    allowPush: false,
  });
  await runCard(project, {
    agent: "claude",
    model: option("claude-model", "haiku"),
    title: "Add version output to budgets CLI",
    task: "Implement `-version`: it must print exactly `budgets dev\\n` and exit successfully without requiring the daemon. Add Go feature tests and run the complete `go test ./...` suite.",
  });
  await runCard(project, {
    agent: "codex",
    model: option("codex-model", "default"),
    title: "Add config output to budgets CLI",
    task: "Implement `-print-config`: output one newline-terminated JSON object with defaults {idle:'10s',maxRssMb:100,maxCpuPercent:1}; reflect flag overrides, including `-idle 2s -max-rss-mb 50`. It must exit successfully without requiring the daemon. Add meaningful feature tests. Preserve the existing `-version` feature and its tests.",
  });
  assert.equal(
    run("git", ["worktree", "list", "--porcelain"], repo).trim().split("\n\n").length,
    initialWorktrees,
    "Temporary worktrees remain after merge",
  );
  assert.equal(
    run("git", ["rev-parse", "HEAD"], repo).trim(),
    initialRepoHead,
    "Temporary project HEAD changed",
  );
  report.checks.push("project folder HEAD unchanged");
} catch (error) {
  report.failure = String(error?.stack ?? error).slice(0, 4000);
  throw error;
} finally {
  const exited = new Promise((resolve) => daemon.once("exit", resolve));
  daemon.kill("SIGTERM");
  await Promise.race([exited, sleep(10_000)]);
  if (daemon.exitCode === null && daemon.signalCode === null) {
    daemon.kill("SIGKILL");
    await exited;
  }
  if (report.failure && process.argv.includes("--keep-failed")) {
    report.retainedTemporaryFolder = temp;
  } else {
    rmSync(temp, { recursive: true, force: true });
  }
  report.sourceUnchanged = sourceManifest(source) === sourceBefore;
  report.sourceHeadUnchanged = run("git", ["rev-parse", "HEAD"], root).trim() === sourceHeadBefore;
  if (report.sourceHeadUnchanged) report.checks.push("source repository HEAD unchanged");
  const reportPath = join(root, "dist/coding-task-smoke.json");
  mkdirSync(join(root, "dist"), { recursive: true });
  writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
  console.log(`Report: ${reportPath}`);
  assert.ok(report.sourceUnchanged, "Source tools/budgets content changed during the harness");
  assert.ok(report.sourceHeadUnchanged, "Source repository HEAD changed");
}
