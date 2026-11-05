#!/usr/bin/env node
// Builds the daemon and its CLI for the host's own target triple, and copies them into
// src-tauri/binaries/ with the suffix Tauri's sidecar loader expects
// (<name>-<target-triple>[.exe]). tauri.conf.json's bundle.externalBin lists the two
// binaries by their bare name; Tauri appends the triple itself when it resolves them, both
// in `tauri dev`/`tauri build` and inside the built app.
//
// By default it builds for the machine running it, which is all `tauri dev` and a plain
// `tauri build` need. With --universal it builds the macOS sidecars for both chips and joins them with
// lipo, which is what `tauri build --target universal-apple-darwin` looks for: one file per chip
// (-aarch64-apple-darwin and -x86_64-apple-darwin, used when Tauri compiles each half) and the joined
// one (-universal-apple-darwin, used when it bundles the app). The daemon builds without cgo, so
// either chip can be built from the other.
import { execFileSync } from "node:child_process";
import { chmodSync, existsSync, mkdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../../..", import.meta.url));
const binariesDir = new URL("../src-tauri/binaries/", import.meta.url);
const tauriConfigPath = fileURLToPath(new URL("../src-tauri/tauri.conf.json", import.meta.url));
const tauriVersion = JSON.parse(readFileSync(tauriConfigPath, "utf8")).version;

function hostTriple() {
  const out = execFileSync("rustc", ["-vV"], { encoding: "utf8" });
  const line = out.split("\n").find((l) => l.startsWith("host:"));
  if (!line) throw new Error("rustc -vV did not report a host triple");
  return line.slice("host:".length).trim();
}

// How Go names the two macOS chips, for the targets that are built from another machine's chip.
const GO_TARGETS = {
  "aarch64-apple-darwin": { GOOS: "darwin", GOARCH: "arm64" },
  "x86_64-apple-darwin": { GOOS: "darwin", GOARCH: "amd64" },
};

function build(daemonDir, pkg, name, triple, outDir) {
  const ext = triple.includes("windows") ? ".exe" : "";
  const out = new URL(`${name}-${triple}${ext}`, outDir);
  const cross = GO_TARGETS[triple];
  const releaseVersion = process.env.MARSHAL_RELEASE_VERSION || tauriVersion;
  if (
    typeof releaseVersion !== "string" ||
    !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/.test(
      releaseVersion,
    )
  ) {
    throw new Error(
      "MARSHAL_RELEASE_VERSION or tauri.conf.json version must be a semantic version.",
    );
  }
  const ldflags = [
    "-ldflags",
    `-X github.com/khanblair/marshal/daemon/internal/buildinfo.Version=${releaseVersion}`,
  ];
  execFileSync("go", ["build", ...ldflags, "-o", fileURLToPath(out), `./cmd/${pkg}`], {
    cwd: daemonDir,
    stdio: "inherit",
    env: cross ? { ...process.env, ...cross, CGO_ENABLED: "0" } : process.env,
  });
  chmodSync(fileURLToPath(out), 0o755);
  console.log(`built ${name}-${triple}${ext}`);
}

// Joins the two chips' builds of one program into the universal file Tauri bundles.
function join(name, outDir) {
  const parts = Object.keys(GO_TARGETS).map((triple) =>
    fileURLToPath(new URL(`${name}-${triple}`, outDir)),
  );
  const universal = fileURLToPath(new URL(`${name}-universal-apple-darwin`, outDir));
  execFileSync("lipo", ["-create", "-output", universal, ...parts], { stdio: "inherit" });
  chmodSync(universal, 0o755);
  console.log(`joined ${name}-universal-apple-darwin`);
}

const universal = process.argv.includes("--universal");
const triples = universal ? Object.keys(GO_TARGETS) : [hostTriple()];
mkdirSync(binariesDir, { recursive: true });
const daemonDir = fileURLToPath(new URL("daemon/", `file://${repoRoot}`));

// Build and copy the frontend before compiling Go so both the packaged shell and the daemon's
// embedded web UI use the same current assets, including when dist already exists but is stale.
const webDist = new URL("../../web/dist/index.html", import.meta.url);
console.log("building the web app before the daemon sidecars");
execFileSync("pnpm", ["--filter", "web", "build"], { cwd: repoRoot, stdio: "inherit" });
if (!existsSync(webDist))
  throw new Error("Web build completed without creating apps/web/dist/index.html.");
execFileSync("node", ["scripts/copy-web-dist.mjs"], { cwd: repoRoot, stdio: "inherit" });

for (const triple of triples) {
  build(daemonDir, "marshald", "marshald", triple, binariesDir);
  build(daemonDir, "marshal", "marshal", triple, binariesDir);
}
if (universal) {
  join("marshald", binariesDir);
  join("marshal", binariesDir);
}

console.log(`sidecars ready for ${universal ? "both Mac chips" : triples[0]}`);
