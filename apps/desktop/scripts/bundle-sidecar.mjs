#!/usr/bin/env node
// Builds the daemon and its CLI for the host's own target triple, and copies them into
// src-tauri/binaries/ with the suffix Tauri's sidecar loader expects
// (<name>-<target-triple>[.exe]). tauri.conf.json's bundle.externalBin lists the two
// binaries by their bare name; Tauri appends the triple itself when it resolves them, both
// in `tauri dev`/`tauri build` and inside the built app.
//
// This only builds for the machine running it. The release workflow cross-builds all four
// targets before calling `tauri build`, one job per OS/arch, so this script only ever needs
// the host triple in CI too.
import { execFileSync } from "node:child_process";
import { chmodSync, existsSync, mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../../..", import.meta.url));
const binariesDir = new URL("../src-tauri/binaries/", import.meta.url);

function hostTriple() {
  const out = execFileSync("rustc", ["-vV"], { encoding: "utf8" });
  const line = out.split("\n").find((l) => l.startsWith("host:"));
  if (!line) throw new Error("rustc -vV did not report a host triple");
  return line.slice("host:".length).trim();
}

function build(daemonDir, pkg, name, triple, outDir) {
  const ext = triple.includes("windows") ? ".exe" : "";
  const out = new URL(`${name}-${triple}${ext}`, outDir);
  execFileSync("go", ["build", "-o", fileURLToPath(out), `./cmd/${pkg}`], {
    cwd: daemonDir,
    stdio: "inherit",
  });
  chmodSync(fileURLToPath(out), 0o755);
  console.log(`built ${name}-${triple}${ext}`);
}

const triple = hostTriple();
mkdirSync(binariesDir, { recursive: true });
const daemonDir = new URL("daemon/", `file://${repoRoot}`);
build(fileURLToPath(daemonDir), "marshald", "marshald", triple, binariesDir);
build(fileURLToPath(daemonDir), "marshal", "marshal", triple, binariesDir);

// The web UI is what the shell shows; build.beforeBuildCommand in tauri.conf.json already runs
// the web build, but `tauri dev` also needs a dist/ to exist the first time it starts. A dev
// run with no dist yet gets one cheaply here rather than failing with a confusing Rust error.
const webDist = new URL("../../web/dist/index.html", import.meta.url);
if (!existsSync(webDist)) {
  console.log("apps/web/dist is missing; building it once so the shell has something to show");
  execFileSync("pnpm", ["--filter", "web", "build"], { cwd: repoRoot, stdio: "inherit" });
}

console.log(`sidecars ready for ${triple}`);
