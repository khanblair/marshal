#!/usr/bin/env node
// Copies the built web app (apps/web/dist) into daemon/internal/webui/dist, where
// internal/webui embeds it (//go:embed all:dist). Every real build runs this before compiling
// the daemon, so the compiled binary can serve the app itself (internal/api/webui.go): the
// desktop shell points its window at the daemon's own address, and a plain browser can do the
// same thing over Tailscale, instead of a second copy of the UI living at another origin.
//
// A bare `go build`/`go test` in daemon/, run without this step first, still works: the embedded
// dist/ then holds only its placeholder, internal/webui.Available() says so, and the daemon
// answers every address that is not "/" the same as it always did (webUIFS() in cmd/marshald
// returns nil, so api.Deps.WebUI is unset).
import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { fileURLToPath } from "node:url";

const webDist = fileURLToPath(new URL("../apps/web/dist", import.meta.url));
const embedDir = fileURLToPath(new URL("../daemon/internal/webui/dist", import.meta.url));

if (!existsSync(webDist)) {
  console.error(`copy-web-dist: ${webDist} does not exist. Run "pnpm --filter web build" first.`);
  process.exit(1);
}

// Clear everything except the placeholder, so a file removed from a later web build does not
// linger in the embedded copy and get served as if it still existed.
mkdirSync(embedDir, { recursive: true });
for (const entry of readdirSync(embedDir)) {
  if (entry !== ".gitkeep") rmSync(`${embedDir}/${entry}`, { recursive: true, force: true });
}
cpSync(webDist, embedDir, { recursive: true });
console.log(`copied ${webDist} -> ${embedDir}`);
