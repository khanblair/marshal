import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import type { Health } from "../src";

/** Reads a golden file that the Go tests wrote from the real wire types. */
function golden(name: string): unknown {
  const path = fileURLToPath(
    new URL(`../../../daemon/testdata/golden/${name}.json`, import.meta.url),
  );
  return JSON.parse(readFileSync(path, "utf8"));
}

describe("golden files from the daemon", () => {
  it("has the health answer the generated type describes", () => {
    // The sample below is checked against the generated type by the compiler. If the Go type
    // gains or loses a field, the golden file changes, and this stops compiling until the
    // sample matches again, so the two sides cannot drift apart unnoticed.
    const sample: Health = {
      status: "ok",
      version: "0.0.0",
      mode: "dev",
      serverTime: "2026-09-25T10:00:00Z",
    };
    expect(golden("health")).toEqual(sample);
  });
});
