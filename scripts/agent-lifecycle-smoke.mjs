/** Real installed CLIs, reached only through the built Marshal daemon and its public API. */
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

function option(name, fallback) {
  const at = process.argv.indexOf(`--${name}`);
  return at < 0 ? fallback : process.argv[at + 1];
}
if (!process.argv.includes("--real")) {
  throw new Error("Pass --real to verify installed CLIs using their signed-in accounts.");
}
const kinds = option("agents", "claude,codex").split(",");
assert.ok(kinds.every((kind) => ["claude", "codex"].includes(kind)));
const sleep = (ms) => new Promise((done) => setTimeout(done, ms));
const bin = fileURLToPath(new URL("../dist/bin/marshald", import.meta.url));
assert.ok(existsSync(bin), "Build the daemon first.");
const port = await new Promise((done, fail) => {
  const server = createServer();
  server.once("error", fail);
  server.listen(0, "127.0.0.1", () => {
    const { port } = server.address();
    server.close(() => done(port));
  });
});
const dataDir = mkdtempSync(join(tmpdir(), "marshal-real-agents-"));
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
async function call(method, path, body) {
  const response = await fetch(base + path, {
    method,
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(180_000),
  });
  const text = await response.text();
  const data = text ? JSON.parse(text) : null;
  if (!response.ok) throw new Error(`${method} ${path}: HTTP ${response.status}: ${text}`);
  return data;
}
async function until(check, label, timeout = 180_000) {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const result = await check();
    if (result) return result;
    await sleep(500);
  }
  throw new Error(`Timed out: ${label}`);
}
let sampleProject;
async function verify(kind) {
  // Marshal has one sample repository per data folder. Each agent uses a
  // separate card in that repository, sequentially, matching the ownership rule.
  sampleProject ??= await call("POST", "/v1/projects", {
    source: "sample",
    name: "CLI lifecycle",
  });
  const project = sampleProject;
  await call("PUT", `/v1/projects/${project.id}/integration/settings`, {
    autoShip: false,
    review: false,
    checks: false,
  });
  const approvedFile = `marshal-${kind}-approved.txt`;
  const deniedFile = `marshal-${kind}-denied.txt`;
  const marker = `MARSHAL_${kind.toUpperCase()}_${Date.now()}`;
  const write = (name) =>
    kind === "claude"
      ? `Use the Write tool to create ${name} in the current project folder with the exact content VERIFIED.`
      : `Run python3 -c "from pathlib import Path; Path('${name}').write_text('VERIFIED')" in the project folder. Request approval if needed.`;
  const card = await call("POST", `/v1/projects/${project.id}/cards`, {
    title: `Verify ${kind} approval transport`,
    agent: kind,
    permissionMode: "ask",
    model: option(`model-${kind}`, kind === "claude" ? "haiku" : ""),
    body: `This is a bounded Marshal integration check. Remember ${marker} in this conversation. First call Marshal's board_status MCP tool once to verify its connection. Then ${write(approvedFile)} Do not use other tools, do not finish the card, do not commit or change branches. If a tool is denied, stop and report that.`,
  });
  const path = `/v1/cards/${card.id}`;
  const history = () => call("GET", `${path}/messages?limit=100`);
  const read = () => call("GET", path);
  const approval = () =>
    until(async () => {
      const current = await read();
      if (current.session === "stopped")
        throw new Error(`${kind} stopped: ${current.needsReason?.text}`);
      if (current.session === "awake" && !current.needsReason?.approvalId) {
        const page = await history();
        const failed = page.items.find((message) => message.tool?.state === "failed");
        if (failed) {
          const detail = await call("GET", `${path}/messages/${failed.id}`);
          throw new Error(
            `${kind} tool failed before approval: ${detail.tool?.title}: ${detail.tool?.content}`,
          );
        }
      }
      return current.needsReason?.approvalId;
    }, `${kind} approval request`);
  const settle = () =>
    until(async () => {
      const current = await read();
      if (current.session === "stopped")
        throw new Error(`${kind} stopped: ${current.needsReason?.text}`);
      return current.session === "awake";
    }, `${kind} turn to finish`);
  const started = await call("POST", `${path}/start`);
  assert.equal(started.worktree, project.path);
  let id = await approval();
  assert.equal(existsSync(join(project.path, approvedFile)), false, "write ran before approval");
  await call("POST", `/v1/approvals/${id}`, { decision: "approved" });
  await settle();
  assert.equal(readFileSync(join(project.path, approvedFile), "utf8"), "VERIFIED");
  let page = await history();
  assert.ok(
    page.items.some(
      (message) => message.approval?.id === id && message.approval.state === "approved",
    ),
  );
  assert.ok(
    page.items.some(
      (message) =>
        message.tool?.name?.includes("board_status") ||
        message.tool?.title?.includes("board_status"),
    ),
    "Marshal MCP board_status call missing",
  );
  await call("POST", `${path}/messages`, {
    text: `${write(deniedFile)} Do not use a different method or retry after a denial.`,
  });
  id = await approval();
  await call("POST", `/v1/approvals/${id}`, { decision: "denied" });
  await settle();
  assert.equal(existsSync(join(project.path, deniedFile)), false, "denied write ran");
  page = await history();
  assert.ok(
    page.items.some(
      (message) => message.approval?.id === id && message.approval.state === "denied",
    ),
  );
  await call("POST", `${path}/stop`);
  // The app's Start/Resume agent action continues a stopped session through /start.
  // /resume is reserved for sessions left waiting after a daemon restart.
  await call("POST", `${path}/start`);
  const before = (await history()).items[0]?.seq ?? 0;
  await call("POST", `${path}/messages`, {
    text: "Without using tools or reading any files, reply only with the MARSHAL_ marker I asked you to remember earlier in this conversation.",
  });
  await settle();
  page = await history();
  assert.ok(
    page.items.some(
      (message) =>
        message.seq > before && message.kind === "agent" && message.text.includes(marker),
    ),
    "resumed agent lost its conversation",
  );
  await call("POST", `${path}/stop`);
  console.log(
    `PASS ${kind}: built Marshal -> real CLI -> MCP -> approval and denial -> file effects -> stop/resume with conversation retained.`,
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
  for (const kind of kinds) await verify(kind);
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
