import { readFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

/*
 * The data layer is the part of the app that talks to the daemon, and the mock is deleted in
 * Phase 13. A module in `src/data` must therefore never import from `src/mock`: if it did, the
 * dependency would point the wrong way and deleting the mock would break the real client. The
 * mock and the sync layer may import the data layer, never the reverse.
 */

/** This file's own folder, `src/data`. A plain path, because jsdom replaces the global `URL`. */
const DATA_DIR = dirname(fileURLToPath(import.meta.url));

/** Every TypeScript file under a folder, at any depth. */
function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    return /\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

/** The module specifiers a file imports, from `import ... from "x"` and `export ... from "x"`. */
function importsOf(source: string): string[] {
  const specifiers: string[] = [];
  const pattern = /(?:^|\n)\s*(?:import|export)[^;]*?from\s+["']([^"']+)["']/g;
  for (const match of source.matchAll(pattern)) {
    if (match[1]) specifiers.push(match[1]);
  }
  return specifiers;
}

describe("the data layer", () => {
  it("never imports the mock", () => {
    const offenders = sourceFiles(DATA_DIR)
      .filter((path) => !path.endsWith(".test.ts") && !path.endsWith(".test.tsx"))
      .flatMap((path) =>
        importsOf(readFileSync(path, "utf8"))
          .filter(
            (specifier) =>
              specifier === "~/mock" ||
              specifier.startsWith("~/mock/") ||
              specifier.includes("/mock/") ||
              specifier.startsWith("./mock/"),
          )
          .map((specifier) => `${path.slice(DATA_DIR.length)} imports ${specifier}`),
      );
    expect(offenders).toEqual([]);
  });
});
