import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { isDaemon, type SectionId, type SectionStatus, sectionStatus } from "./sections";
import { REPO_ROOT } from "./testing/golden";

interface RegisterRow {
  id: string;
  status: string;
}

/** The rows of the register in section 2.2 of the checklist, and nothing from other tables. */
function readRegister(): RegisterRow[] {
  const text = readFileSync(resolve(REPO_ROOT, "docs", "backend-checklist.md"), "utf8");
  const start = text.indexOf("### 2.2");
  const end = text.indexOf("### 2.3");
  expect(start).toBeGreaterThan(-1);
  expect(end).toBeGreaterThan(start);
  const rows: RegisterRow[] = [];
  for (const line of text.slice(start, end).split("\n")) {
    // A section row starts with its id and a name, such as "| S5a Cards |". The header row does not.
    const id = /^\| (S\d+[a-z]?) /.exec(line)?.[1];
    if (!id) continue;
    const cells = line.split("|").map((cell) => cell.trim());
    rows.push({ id, status: cells.at(-2) ?? "" });
  }
  return rows;
}

describe("the section register", () => {
  const register = readRegister();

  it("reads every section of the register", () => {
    expect(register.length).toBeGreaterThan(40);
    expect(new Set(register.map((row) => row.id)).size).toBe(register.length);
  });

  it("has the same sections in the code as in the doc", () => {
    expect(Object.keys(sectionStatus).sort()).toEqual(register.map((row) => row.id).sort());
  });

  it("has the same status in the code as in the doc, for every section", () => {
    const fromDoc = Object.fromEntries(register.map((row) => [row.id, row.status.toLowerCase()]));
    expect(sectionStatus).toEqual(fromDoc);
  });

  it("only uses the two words the register uses", () => {
    for (const row of register) expect(["Mock", "Daemon"]).toContain(row.status);
  });
});

describe("isDaemon", () => {
  const table: Record<SectionId, SectionStatus> = { ...sectionStatus, S1: "mock", S3: "daemon" };

  it("reads the given table", () => {
    expect(isDaemon("S3", table)).toBe(true);
    expect(isDaemon("S1", table)).toBe(false);
  });

  it("reads the real table by default", () => {
    expect(isDaemon("S3")).toBe(sectionStatus.S3 === "daemon");
  });
});
