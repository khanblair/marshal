/**
 * Runs the built dev daemon for the end-to-end tests, and stops it when Playwright stops this script.
 *
 *   node scripts/e2e-daemon.mjs
 *
 * It needs `pnpm build` first (that builds `dist/bin/marshald` and `dist/bin/stub-agent`). It uses a
 * data folder of its own, made new on every start, and its own port, so nothing of the developer's
 * dev daemon (the default port 47801 and the real dev data folder) is ever touched. The prototype's
 * three projects are loaded from a fixture, and the agent is the stub.
 *
 * A spec that must stop and start the daemon signals this script and never the daemon: SIGUSR2 stops
 * the daemon (this script keeps running), SIGHUP starts it again on the same data folder, and SIGINT
 * or SIGTERM stops the daemon and exits. (SIGUSR1 is not used: Node keeps it for its debugger.) The
 * script writes its own process id to a file for that. There is no control endpoint.
 */
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(fileURLToPath(new URL("..", import.meta.url)));
const binDir = join(root, "dist", "bin");
const suffix = process.platform === "win32" ? ".exe" : "";
const daemonPath = join(binDir, `marshald${suffix}`);
const stubAgentPath = join(binDir, `stub-agent${suffix}`);

// The Playwright config passes its own values (apps/web/e2e/support/e2e-env.ts); these are the same.
const PORT = process.env.E2E_DAEMON_PORT ?? "47811";
const testResults = join(root, "apps", "web", "test-results");
const dataDir = resolve(process.env.E2E_DATA_DIR ?? join(testResults, "daemon-data"));
const pidFile = join(testResults, "e2e-daemon.pid");
const STOP_GRACE_MS = 5000;
const KEEP_ALIVE_MS = 60_000;
// The ports of the real daemon (47800) and of the developer's dev daemon (47801): never these.
const FORBIDDEN_PORTS = ["47800", "47801"];

if (FORBIDDEN_PORTS.includes(PORT)) {
  console.error(`Port ${PORT} belongs to a daemon that is not this test's. Use another port.`);
  process.exit(1);
}

for (const path of [daemonPath, stubAgentPath]) {
  if (!existsSync(path)) {
    console.error(`${relative(root, path)} is missing. Run pnpm build first.`);
    process.exit(1);
  }
}

// Deleting is only safe inside the test output folder, so refuse anything else.
if (!dataDir.startsWith(testResults + sep)) {
  console.error("The data folder is not inside the test results folder, so it was not deleted.");
  process.exit(1);
}
rmSync(dataDir, { recursive: true, force: true });
mkdirSync(dataDir, { recursive: true });
writeFileSync(pidFile, String(process.pid));

let child = null;
let stopping = false;
// While the daemon is stopped nothing else keeps this script alive, and it must wait for the signal that starts it.
const keepAlive = setInterval(() => {}, KEEP_ALIVE_MS);

function startDaemon() {
  if (child) return;
  const env = { ...process.env, MARSHAL_FIXTURE: "prototype", MARSHAL_AGENT: "stub" };
  // Quiet unless something goes wrong: the daemon logs every request at its dev level.
  child = spawn(
    daemonPath,
    ["--dev", "--port", PORT, "--data-dir", dataDir, "--log-level", "warn"],
    {
      env,
      stdio: "inherit",
    },
  );
  const started = child;
  child.on("exit", (code, signal) => {
    if (child === started) child = null;
    // A daemon that ends by itself is a failure the run must see. One this script stopped is not.
    if (!stopping && !started.commanded) process.exit(code ?? (signal ? 1 : 0));
  });
}

function stopDaemon() {
  const current = child;
  if (!current) return Promise.resolve();
  current.commanded = true;
  return new Promise((done) => {
    const kill = setTimeout(() => current.kill("SIGKILL"), STOP_GRACE_MS);
    current.once("exit", () => {
      clearTimeout(kill);
      done();
    });
    current.kill("SIGTERM");
  });
}

async function shutDown() {
  stopping = true;
  clearInterval(keepAlive);
  await stopDaemon();
  rmSync(pidFile, { force: true });
  process.exit(0);
}

process.on("SIGINT", shutDown);
process.on("SIGTERM", shutDown);
process.on("SIGUSR2", () => void stopDaemon());
process.on("SIGHUP", () => startDaemon());

startDaemon();
