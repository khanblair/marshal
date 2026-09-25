import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

/** The folder where the Go tests write their golden files, with a trailing slash. */
export const GOLDEN_DIR = fileURLToPath(
  new URL("../../../daemon/testdata/golden/", import.meta.url),
);

/** Reads a golden file that the Go tests wrote from the real wire types. */
export function golden(name: string): unknown {
  return JSON.parse(readFileSync(`${GOLDEN_DIR}${name}.json`, "utf8"));
}
