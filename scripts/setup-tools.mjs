/**
 * Installs the pinned Go tools into `.tools/bin`, which Git ignores, so everyone uses the
 * same versions. Run it through `pnpm setup:tools`. A tool that is already at the pinned
 * version is skipped.
 */
import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const binDir = join(root, ".tools", "bin");
const markerFile = join(root, ".tools", "versions.json");

/** Exact versions, also listed in docs/library-docs.md section 2.8. */
const TOOLS = [
  { name: "air", pkg: "github.com/air-verse/air", version: "v1.67.4" },
  { name: "sqlc", pkg: "github.com/sqlc-dev/sqlc/cmd/sqlc", version: "v1.31.1" },
  { name: "tygo", pkg: "github.com/gzuidhof/tygo", version: "v0.2.21" },
  {
    name: "golangci-lint",
    pkg: "github.com/golangci/golangci-lint/v2/cmd/golangci-lint",
    version: "v2.14.0",
  },
];

function readMarker() {
  try {
    return JSON.parse(readFileSync(markerFile, "utf8"));
  } catch {
    return {};
  }
}

function exeName(name) {
  return process.platform === "win32" ? `${name}.exe` : name;
}

function install(tool) {
  console.log(`Installing ${tool.name} ${tool.version}`);
  const run = spawnSync("go", ["install", `${tool.pkg}@${tool.version}`], {
    cwd: root,
    stdio: "inherit",
    // No cgo: the tools then need no C toolchain, and the daemon uses none either.
    env: { ...process.env, GOBIN: binDir, CGO_ENABLED: "0" },
  });
  return run.status === 0;
}

function main() {
  mkdirSync(binDir, { recursive: true });
  const marker = readMarker();
  let failed = false;
  for (const tool of TOOLS) {
    const present = existsSync(join(binDir, exeName(tool.name)));
    if (present && marker[tool.name] === tool.version) {
      console.log(`${tool.name} ${tool.version} is already installed`);
      continue;
    }
    if (install(tool)) marker[tool.name] = tool.version;
    else failed = true;
  }
  writeFileSync(markerFile, `${JSON.stringify(marker, null, 2)}\n`);
  return failed ? 1 : 0;
}

process.exit(main());
