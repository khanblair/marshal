/**
 * Smoke test of the real agent CLIs. It starts the built daemon on a throwaway data folder and a free
 * port, makes the sample project, and for each agent starts a card, sends one small message, and
 * waits for the agent's reply to appear in the card's history.
 *
 *   node scripts/agent-smoke.mjs --agents claude,codex,gemini
 *   node scripts/agent-smoke.mjs --agents claude --stub     (checks this script against the stub agent)
 *
 * It needs `pnpm build` first. The real run needs each CLI installed and signed in on the machine
 * that runs it (the daemon passes an agent only a short list of environment variables, so an API
 * key in the environment does not reach it), and it spends a few cents of that account. Only the
 * nightly workflow runs it for real: no builder and no unit test ever calls a real agent. `--stub`
 * uses the stub agent, so it is free and safe to run anywhere.
 */
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(fileURLToPath(new URL("..", import.meta.url)));
const suffix = process.platform === "win32" ? ".exe" : "";
const daemonPath = join(root, "dist", "bin", `marshald${suffix}`);

const PROMPT = "Reply with the single word OK and do nothing else. Do not use any tools.";
const START_WAIT_MS = 200_000;
const HEALTH_WAIT_MS = 30_000;
const POLL_MS = 1500;

function option(name, fallback) {
  const at = process.argv.indexOf(`--${name}`);
  return at === -1 ? fallback : (process.argv[at + 1] ?? fallback);
}
const agents = option("agents", "claude")
  .split(",")
  .map((one) => one.trim())
  .filter(Boolean);
const stub = process.argv.includes("--stub");
const replyWaitMs = Number(option("timeout", "180")) * 1000;

const sleep = (ms) => new Promise((done) => setTimeout(done, ms));

function freePort() {
  return new Promise((done, fail) => {
    const server = createServer();
    server.once("error", fail);
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address();
      server.close(() => done(port));
    });
  });
}

/** One call to the daemon. A refusal or a failure throws with the daemon's own sentence. */
async function call(base, token, method, path, { body, waitMs = 30_000 } = {}) {
  const response = await fetch(`${base}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(waitMs),
  });
  const text = await response.text();
  const data = text ? JSON.parse(text) : null;
  if (!response.ok) {
    throw new Error(`${method} ${path}: ${response.status} ${data?.error?.message ?? text}`);
  }
  return data;
}

async function waitForHealth(base) {
  const until = Date.now() + HEALTH_WAIT_MS;
  while (Date.now() < until) {
    try {
      if ((await fetch(`${base}/v1/health`)).ok) return;
    } catch {
      // Not listening yet.
    }
    await sleep(300);
  }
  throw new Error("The daemon did not answer /v1/health in time.");
}

/** Runs one agent through start, message, and reply. It returns what to print for it. */
async function smokeOne(base, token, projectId, agent) {
  const started = Date.now();
  const seconds = () => Math.round((Date.now() - started) / 1000);
  try {
    const catalog = await call(base, token, "GET", "/v1/agents");
    const entry = catalog.agents.find((one) => one.kind === agent);
    if (!entry) return { agent, ok: false, seconds: seconds(), detail: "not in the agent catalog" };
    if (!stub && entry.status === "missing") {
      const hint = entry.installHint ? ` (${entry.installHint})` : "";
      return { agent, ok: false, seconds: seconds(), detail: `not installed${hint}` };
    }
    const card = await call(base, token, "POST", `/v1/projects/${projectId}/cards`, {
      body: { title: `Smoke test: ${agent}`, body: PROMPT, agent },
    });
    await call(base, token, "POST", `/v1/cards/${card.id}/start`, { waitMs: START_WAIT_MS });
    await call(base, token, "POST", `/v1/cards/${card.id}/messages`, { body: { text: PROMPT } });
    const until = Date.now() + replyWaitMs;
    let reply = "";
    while (Date.now() < until && !reply) {
      await sleep(POLL_MS);
      const page = await call(base, token, "GET", `/v1/cards/${card.id}/messages?limit=50`);
      reply = page.items.find((one) => one.kind === "agent" && one.text.trim())?.text.trim() ?? "";
    }
    await call(base, token, "POST", `/v1/cards/${card.id}/stop`).catch(() => undefined);
    if (!reply) return { agent, ok: false, seconds: seconds(), detail: "no reply in time" };
    return {
      agent,
      ok: true,
      seconds: seconds(),
      detail: `${entry.version} replied: ${reply.slice(0, 40)}`,
    };
  } catch (error) {
    return { agent, ok: false, seconds: seconds(), detail: String(error.message ?? error) };
  }
}

async function main() {
  if (!existsSync(daemonPath)) {
    console.error("dist/bin/marshald is missing. Run `pnpm build` first.");
    return 1;
  }
  const dataDir = mkdtempSync(join(tmpdir(), "marshal-smoke-"));
  const port = await freePort();
  const env = { ...process.env, ...(stub ? { MARSHAL_AGENT: "stub" } : {}) };
  const daemon = spawn(
    daemonPath,
    ["--dev", "--port", String(port), "--data-dir", dataDir, "--log-level", "warn"],
    { env, stdio: "inherit" },
  );
  let results = [];
  try {
    const base = `http://127.0.0.1:${port}`;
    await waitForHealth(base);
    const token = readFileSync(join(dataDir, "dev-token"), "utf8").trim();
    const project = await call(base, token, "POST", "/v1/projects", {
      body: { source: "sample" },
      waitMs: 60_000,
    });
    // One agent after another: they share the machine and the account's rate limit.
    for (const agent of agents) results.push(await smokeOne(base, token, project.id, agent));
  } catch (error) {
    console.error(String(error.message ?? error));
    results = [
      { agent: "(daemon)", ok: false, seconds: 0, detail: String(error.message ?? error) },
    ];
  } finally {
    daemon.kill("SIGTERM");
    await sleep(500);
    rmSync(dataDir, { recursive: true, force: true });
  }
  console.log(`\nAgent smoke test${stub ? " (stub agent)" : ""}`);
  for (const r of results) {
    console.log(`  ${r.ok ? "pass" : "FAIL"}  ${r.agent.padEnd(8)} ${r.seconds}s  ${r.detail}`);
  }
  return results.length > 0 && results.every((r) => r.ok) ? 0 : 1;
}

process.exit(await main());
