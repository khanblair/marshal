import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

/** The repository root, found from this file. It is a string path, because jsdom replaces `URL`. */
export const REPO_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "../../../../..");

/** Reads a golden file that the Go tests wrote from the real wire types. */
export function golden<T = unknown>(name: string): T {
  return JSON.parse(
    readFileSync(resolve(REPO_ROOT, "daemon", "testdata", "golden", `${name}.json`), "utf8"),
  ) as T;
}
