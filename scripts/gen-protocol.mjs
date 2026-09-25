/**
 * Generates packages/protocol/src/generated from the Go wire types with tygo, adds a readonly
 * `<Type>Values` array for every union of allowed values, then formats the result with Biome so
 * it passes the same checks as hand-written code. Run through `pnpm gen`. The output is never
 * edited by hand.
 */
import { spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("..", import.meta.url));
const generated = join(root, "packages", "protocol", "src", "generated", "index.ts");
const useShell = process.platform === "win32";

/** The union tygo writes for a Go const block: `export type T = typeof A | typeof B;`. */
const UNION = /^export type (\w+) = (typeof \w+(?: \| typeof \w+)*);$/;

/** A doc comment with one line of text, which tygo writes over three lines. */
const ONE_LINE_DOC = /^( *)\/\*\*\n *\* (.*)\n *\*\/$/gm;

/**
 * Cleans up tygo's output. Every `export type T = typeof A | ...` gets an array of its values
 * right after it, and `<T extends any>` loses the `extends any` that the lint rules reject.
 * A union in any other shape stops the build, so a value list can never be left without its array.
 * One-line doc comments are folded onto one line so the file stays short.
 */
function addValueArrays(source) {
  const tidy = source.replace(/ extends any(?=[>,])/g, "").replace(ONE_LINE_DOC, "$1/** $2 */");
  const out = [];
  for (const line of tidy.split("\n")) {
    out.push(line);
    if (!line.startsWith("export type ") || !line.includes("typeof ")) continue;
    const match = UNION.exec(line);
    if (!match) throw new Error(`Cannot add a values array for: ${line}`);
    const [, name, union] = match;
    const values = union.replaceAll("typeof ", "").split(" | ");
    out.push(`/** Every ${name}, in the order the Go list gives them. */`);
    out.push(`export const ${name}Values: readonly ${name}[] = [${values.join(", ")}];`);
  }
  out.splice(1, 0, "// The <Type>Values arrays are added by scripts/gen-protocol.mjs.");
  return out.join("\n");
}

function run(label, command, args, cwd) {
  const result = spawnSync(command, args, { cwd, stdio: "inherit", shell: useShell });
  if (result.status !== 0) {
    console.error(`${label} failed.`);
    process.exit(result.status ?? 1);
  }
}

run(
  "tygo",
  "node",
  [join(root, "scripts", "go-tool.mjs"), "tygo", "generate"],
  join(root, "daemon"),
);
writeFileSync(generated, addValueArrays(readFileSync(generated, "utf8")));
run(
  "biome format",
  "pnpm",
  [
    "exec",
    "biome",
    "check",
    "--write",
    "--linter-enabled=false",
    "packages/protocol/src/generated",
  ],
  root,
);
