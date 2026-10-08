import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const SRC_DIR = join(__dirname, "..");

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

/** The files that may read dates in the device's own zone, each with the reason. */
const ALLOWED: ReadonlyArray<{ path: RegExp; why: string }> = [
  // The one place that reads dates; every other file asks it.
  { path: /\/data\/zone\.ts$/, why: "the zone helper itself" },
  // Fixtures for tests build their dates on the device's clock, as the test's expectations do.
  { path: /\/(testing)\//, why: "test support builds dates, it does not show them" },
];

const files = sourceFiles(SRC_DIR).filter(
  (path) => !ALLOWED.some((allowed) => allowed.path.test(path)),
);
const offenders = (pattern: RegExp): string[] =>
  files
    .filter((path) => pattern.test(readFileSync(path, "utf8")))
    .map((p) => p.slice(SRC_DIR.length));

describe("one time zone for every date on screen", () => {
  it("reads and sets no date part by the device's zone: `data/zone.ts` does it", () => {
    expect(
      offenders(/\.(setHours|setDate|getHours|getDay|getDate|getMonth|getFullYear)\(/),
    ).toEqual([]);
  });

  it("formats no date by the device's zone: `formatDate` and its kin do", () => {
    expect(offenders(/\.toLocale(Date|Time)String\(/)).toEqual([]);
  });

  it("calls toLocaleString with no date option", () => {
    const withDateOption =
      /\.toLocaleString\([^)]*\b(dateStyle|timeStyle|month|weekday|hour)\b[^)]*\)/;
    expect(offenders(withDateOption)).toEqual([]);
  });
});
