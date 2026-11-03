/** Checks the built daemon's shared-folder flow with the free stub agent, without a browser. */
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const bin = fileURLToPath(new URL("../dist/bin/marshald", import.meta.url));
const sleep = (ms) => new Promise((done) => setTimeout(done, ms));

async function freePort() {
  return new Promise((done, fail) => {
    const server = createServer();
    server.once("error", fail);
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      server.close(() => done(port));
    });
  });
}

async function until(check, label) {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const result = await check();
    if (result) return result;
    await sleep(200);
  }
  throw new Error(`Timed out waiting for ${label}.`);
}

function client(base, token) {
  return async (method, path, body, allowError = false) => {
    const response = await fetch(base + path, {
      method,
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(60_000),
    });
    const text = await response.text();
    const data = text ? JSON.parse(text) : null;
    if (!response.ok && !allowError) {
      throw new Error(`${method} ${path}: HTTP ${response.status}: ${text}`);
    }
    return { status: response.status, data };
  };
}

async function checkWorkspace(call, dataDir) {
  const { data: project } = await call("POST", "/v1/projects", { source: "sample" });
  const git = (...args) =>
    execFileSync("git", ["-C", project.path, ...args], { encoding: "utf8" }).trim();
  const file = (name, content) => writeFileSync(join(project.path, name), content);
  file("owner.txt", "staged owner content\n");
  git("add", "owner.txt");
  file("owner.txt", "unstaged owner content\n");
  const head = git("rev-parse", "HEAD");
  const index = git("diff", "--cached");
  await call("PUT", `/v1/projects/${project.id}/integration/settings`, {
    review: false,
    checks: false,
    autoShip: false,
  });
  const makeCard = async (title) =>
    (await call("POST", `/v1/projects/${project.id}/cards`, { title, agent: "claude" })).data;
  const first = await makeCard("First direct card");
  const second = await makeCard("Second direct card");
  const cardPath = (id, suffix = "") => `/v1/cards/${id}${suffix}`;
  const idle = (id) =>
    until(async () => (await call("GET", cardPath(id))).data.session === "awake", "idle agent");
  const started = await call("POST", cardPath(first.id, "/start"));
  assert.equal(started.data.worktree, project.path);
  const blocked = await call("POST", cardPath(second.id, "/start"), undefined, true);
  assert.equal(blocked.status, 422);
  assert.match(JSON.stringify(blocked.data), /project_editor_busy/);
  await idle(first.id);
  file("agent-change.txt", "direct card change\n");
  await call("POST", cardPath(first.id, "/messages"), { text: "Read the folder and reply OK." });
  await idle(first.id);
  await until(async () => {
    const result = await call("POST", cardPath(first.id, "/send-to-merge"), undefined, true);
    if (result.status === 200) return true;
    assert.equal(result.status, 422);
    assert.match(JSON.stringify(result.data), /still working/);
    return false;
  }, "completion to start after snapshotting");
  await until(async () => (await call("GET", cardPath(first.id))).data.state === "done", "Done");
  assert.equal(git("rev-parse", "HEAD"), head);
  assert.equal(git("diff", "--cached"), index);
  assert.equal(readFileSync(join(project.path, "owner.txt"), "utf8"), "unstaged owner content\n");
  const before = (await call("GET", cardPath(first.id, "/diff"))).data;
  assert.ok(before.files.some((entry) => entry.path === "agent-change.txt"));
  assert.ok(before.files.every((entry) => entry.path !== "owner.txt"));
  await call("POST", cardPath(second.id, "/start"));
  await idle(second.id);
  file("later.txt", "later card content\n");
  const after = (await call("GET", cardPath(first.id, "/diff"))).data;
  assert.deepEqual(after.files, before.files);
  await call("POST", cardPath(second.id, "/stop"));
  assert.equal((git("worktree", "list", "--porcelain").match(/^worktree /gm) ?? []).length, 1);
  assert.equal(existsSync(join(dataDir, "worktrees")), false);
  await call("DELETE", cardPath(first.id));
  assert.equal(
    readFileSync(join(project.path, "agent-change.txt"), "utf8"),
    "direct card change\n",
  );
  assert.equal(git("rev-parse", "HEAD"), head);
  assert.equal(git("diff", "--cached"), index);
}

const dataDir = mkdtempSync(join(tmpdir(), "marshal-direct-smoke-"));
const port = await freePort();
const daemon = spawn(
  bin,
  [
    "--dev",
    "--agent",
    "stub",
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
try {
  const base = `http://127.0.0.1:${port}`;
  await until(async () => {
    try {
      return (await fetch(`${base}/v1/health`)).ok;
    } catch {
      return false;
    }
  }, "daemon health");
  const token = readFileSync(join(dataDir, "dev-token"), "utf8").trim();
  await checkWorkspace(client(base, token), dataDir);
  console.log(
    "PASS: original folder, one editor, completion in place, owner edits/index/HEAD preserved, frozen diffs, safe card deletion, no extra worktrees.",
  );
} finally {
  const exited = new Promise((done) => daemon.once("exit", done));
  daemon.kill("SIGTERM");
  await Promise.race([exited, sleep(10_000)]);
  if (daemon.exitCode === null && daemon.signalCode === null) {
    daemon.kill("SIGKILL");
    await exited;
  }
  rmSync(dataDir, { recursive: true, force: true });
}
