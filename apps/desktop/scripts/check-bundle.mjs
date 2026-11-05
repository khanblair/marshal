// Exercise the signed updater archive before publication. Do not launch the desktop window.
import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, statSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { parseReleaseTag } from "./prepare-release.mjs";

const COMMAND_TIMEOUT_MS = 10_000;
const READY_TIMEOUT_MS = 30_000;
const HTTP_TIMEOUT_MS = 5000;
const POLL_MS = 100;
const SHUTDOWN_MS = 5000;
const { values } = parseArgs({
  options: {
    "bundle-dir": { type: "string" },
    version: { type: "string" },
    universal: { type: "boolean", default: false },
  },
});
assert.equal(process.platform, "darwin", "Mac bundle checks require macOS");
assert.ok(values["bundle-dir"] && values.version, "Pass --bundle-dir and --version");
const expected = parseReleaseTag(values.version);
const bundle = resolve(values["bundle-dir"]);
const archive = join(bundle, "Marshal.app.tar.gz");
assert.ok(statSync(archive).size > 0, "Missing updater archive");
assert.ok(readFileSync(`${archive}.sig`, "utf8").trim(), "Missing updater signature");
const work = mkdtempSync(join(tmpdir(), "marshal-signed-bundle-check-"));
let child;
let stopped;
let ended = false;
const run = (command, args) =>
  execFileSync(command, args, { encoding: "utf8", timeout: COMMAND_TIMEOUT_MS }).trim();
try {
  // Match an updater extraction: extended attributes must not be needed for code signatures.
  run("tar", ["--no-xattrs", "--no-mac-metadata", "-xzf", archive, "-C", work]);
  const app = join(work, "Marshal.app");
  const binaries = join(app, "Contents/MacOS");
  run("codesign", ["--verify", "--deep", "--strict", app]);
  assert.equal(
    run("plutil", [
      "-extract",
      "CFBundleShortVersionString",
      "raw",
      "-o",
      "-",
      join(app, "Contents/Info.plist"),
    ]),
    expected,
  );
  for (const executable of ["marshal-desktop", "marshald", "marshal"]) {
    const architectures = run("lipo", ["-archs", join(binaries, executable)]).split(/\s+/);
    assert.ok(architectures.includes(process.arch === "arm64" ? "arm64" : "x86_64"));
    if (values.universal)
      assert.ok(
        architectures.includes("arm64") && architectures.includes("x86_64"),
        `${executable} must be universal`,
      );
  }
  assert.equal(run(join(binaries, "marshald"), ["--version"]), expected);
  assert.equal(run(join(binaries, "marshal"), ["version"]), expected);
  const listener = createServer();
  await new Promise((done, reject) => {
    listener.once("error", reject);
    listener.listen(0, "127.0.0.1", done);
  });
  const port = listener.address().port;
  await new Promise((done) => listener.close(done));
  child = spawn(
    join(binaries, "marshald"),
    ["--dev", "--agent", "real", "--port", String(port), "--data-dir", join(work, "data")],
    { stdio: "ignore", env: { ...process.env, MARSHAL_FIXTURE: "" } },
  );
  stopped = new Promise((done) => {
    child.once("exit", () => {
      ended = true;
      done();
    });
    child.once("error", (error) => {
      ended = true;
      done();
      console.error(error.message);
    });
  });
  const base = `http://127.0.0.1:${port}`;
  const deadline = Date.now() + READY_TIMEOUT_MS;
  let health;
  while (Date.now() < deadline && !ended) {
    try {
      const response = await fetch(`${base}/v1/health`, {
        signal: AbortSignal.timeout(HTTP_TIMEOUT_MS),
      });
      if (response.ok) {
        health = await response.json();
        break;
      }
    } catch {
      /* Wait for the owned daemon to listen. */
    }
    await new Promise((done) => setTimeout(done, POLL_MS));
  }
  assert.equal(
    health?.version,
    expected,
    "The signed packaged daemon must start and answer health",
  );
  const html = await (await fetch(base, { signal: AbortSignal.timeout(HTTP_TIMEOUT_MS) })).text();
  const entry = html.match(/src="([^"]+\.js)"/)?.[1];
  assert.ok(entry, "Missing embedded web entry");
  const entryUrl = new URL(entry, base);
  assert.equal(entryUrl.origin, base, "The web entry must come from the bundled daemon");
  const js = await (await fetch(entryUrl, { signal: AbortSignal.timeout(HTTP_TIMEOUT_MS) })).text();
  assert.ok(js.includes("Download and install"), "The bundled daemon must serve the update UI");
  console.log(
    `PASS: signed updater archive starts on ${process.arch}, reports ${expected}, and serves the update UI.`,
  );
} finally {
  if (child && !ended) {
    child.kill("SIGTERM");
    const timer = setTimeout(() => child.kill("SIGKILL"), SHUTDOWN_MS);
    await stopped;
    clearTimeout(timer);
  }
  rmSync(work, { recursive: true, force: true });
}
