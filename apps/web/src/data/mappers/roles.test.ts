import type { RoleList } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "../testing/golden";
import {
  type Role,
  readRoleExports,
  toExport,
  toExports,
  toRole,
  toRoleRows,
  toRoleSpec,
} from "./roles";

/* The list the real daemon answers with, written by its own Go tests, so the two sides cannot drift. */
const list = golden<RoleList>("role-list");
const worker = toRole(list.roles[0] as RoleList["roles"][number]);

describe("one role as a screen holds it", () => {
  it("lifts the spec onto the row, so the editor reads role.model rather than role.spec.model", () => {
    expect(worker).toEqual({
      name: "Worker",
      starter: true,
      overridden: false,
      skills: ["conventional-commits"],
      mcp: ["marshal", "github"],
      limits: { time: 60, cost: 5, rounds: 12 },
      backup: "gpt-5",
      desc: "Does the coding on a card",
      agent: "Claude Code",
      model: "claude-sonnet-4-5",
      think: "Medium",
      perm: "Auto-accept edits",
      strength: "Your choice",
      instr: "You do the coding on one card.",
    });
  });

  it("keeps the overridden flag the daemon answered, and copies its lists", () => {
    const rows = toRoleRows(list);
    expect(rows.roles.map((row) => row.overridden)).toEqual([false, true, false]);
    expect(rows.roles[1]?.name).toBe("Reviewer");
    expect(rows.roles[2]?.skills).toEqual([]);
    rows.roles[0]!.skills.push("later");
    expect(list.roles[0]?.spec.skills).toEqual(["conventional-commits"]);
  });

  it("turns a row back into the wire's body, which is what a save and an export send", () => {
    expect(toRoleSpec(worker)).toEqual(list.roles[0]?.spec);
    expect(toExport(worker)).toEqual({ name: "Worker", spec: list.roles[0]?.spec });
    expect(toExports(list.roles.map(toRole))).toEqual(
      list.roles.map((row) => ({ name: row.name, spec: row.spec })),
    );
  });
});

describe("reading a pasted export", () => {
  const good = {
    name: "Nightly janitor",
    spec: { ...toRoleSpec(worker), desc: "Tidies the backlog overnight" },
  };

  it("reads one document, and an array of them, exactly as Export wrote them", () => {
    expect(readRoleExports(JSON.stringify(good))).toEqual([good]);
    expect(readRoleExports(JSON.stringify([good, { ...good, name: "Sweeper" }]))).toEqual([
      good,
      { ...good, name: "Sweeper" },
    ]);
  });

  it("ignores the id and the two flags, which are the daemon's to set", () => {
    const pasted = { ...good, id: "01JD7Q4M2X8K9V0P5T3RB6NHAE", starter: true, overridden: true };
    expect(readRoleExports(JSON.stringify(pasted))).toEqual([good]);
  });

  it("defaults every field a partial document left out, so a hand-written role still imports", () => {
    expect(readRoleExports('{"name": "Bare"}')).toEqual([
      {
        name: "Bare",
        spec: {
          skills: [],
          mcp: [],
          limits: { time: 0, cost: 0, rounds: 0 },
          backup: "",
          desc: "",
          agent: "",
          model: "",
          think: "",
          perm: "",
          strength: "",
          instr: "",
        },
      },
    ]);
  });

  it("trims the name, and refuses text that is not an export with one plain sentence", () => {
    expect(readRoleExports('{"name": "  Spaced  "}')[0]?.name).toBe("Spaced");
    expect(() => readRoleExports("not json")).toThrow(
      "That is not JSON. Paste what Export gave you, or a role's own export.",
    );
    expect(() => readRoleExports("[]")).toThrow("That export has no roles in it.");
    expect(() => readRoleExports("17")).toThrow("Entry 1 is not a role.");
    expect(() => readRoleExports('[{"name": "  "}]')).toThrow(
      "Entry 1 has no name, and a role needs one.",
    );
  });

  it("refuses more roles than one import may carry", () => {
    const many = Array.from({ length: 201 }, (_, at) => ({ name: `Role ${at}` }));
    expect(() => readRoleExports(JSON.stringify(many))).toThrow(
      "That export has more than 200 roles in it. Import them in parts.",
    );
    expect(readRoleExports(JSON.stringify(many.slice(0, 200)))).toHaveLength(200);
  });
});

describe("a role the screens hold", () => {
  it("carries every field the editor writes, so a save cannot lose one", () => {
    const edited: Role = { ...worker, name: "Builder", model: "gpt-5-mini" };
    expect(Object.keys(toRoleSpec(edited)).sort()).toEqual(
      Object.keys(list.roles[0]?.spec ?? {}).sort(),
    );
  });
});
