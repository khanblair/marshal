/**
 * Writes src/index.ts, which re-exports every public export in src/.
 * Run with `pnpm gen` (root) or `pnpm --filter @marshal/ui gen`.
 * Pass `--check` to fail instead of writing when the file is out of date.
 */
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { buildIndex } from "./index-source.mjs";

const src = fileURLToPath(new URL("../src", import.meta.url));
const indexPath = join(src, "index.ts");

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) yield* walk(path);
    else yield path;
  }
}

const modules = [...walk(src)].map((path) => ({
  path: relative(src, path).split(sep).join("/"),
  code: readFileSync(path, "utf8"),
}));
const next = buildIndex(modules);
const current = readFileSync(indexPath, "utf8");

if (process.argv.includes("--check")) {
  if (next !== current) {
    process.stderr.write("ui: src/index.ts is out of date. Run `pnpm gen`.\n");
    process.exit(1);
  }
} else if (next !== current) {
  writeFileSync(indexPath, next);
  process.stdout.write("ui: wrote src/index.ts\n");
} else {
  process.stdout.write("ui: src/index.ts is up to date\n");
}
