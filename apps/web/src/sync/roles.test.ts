import type { RoleList } from "@marshal/protocol";
import { unwrap } from "solid-js/store";
import { describe, expect, it } from "vitest";
import type { ApiClient } from "~/data/api-client";
import { toRoleRows } from "~/data/mappers/roles";
import { golden } from "~/data/testing/golden";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { applyRoleList, applyRoleRows, roleNames, rolesSyncer } from "./roles";

// Section S27: the role templates. The daemon holds them, every route answers the whole list, and
// the syncer publishes no topic, so its snapshot is the same answer a save gets back.

const list = golden<RoleList>("role-list");

describe("the roles section", () => {
  it("is section S27, follows no topic, and loads the daemon's list with no project", async () => {
    expect(rolesSyncer.section).toBe("S27");
    expect(rolesSyncer.topics).toEqual([]);
    const asked: unknown[] = [];
    const api = {
      listRoles: async (project?: string) => {
        asked.push(project);
        return list;
      },
    } as unknown as ApiClient;
    // No project, because the roles themselves do not change with one: only whether each reads as
    // overridden does, and the Roles screen looks at no project.
    expect(await rolesSyncer.load(api, contextOf(createTestMarshal()))).toEqual(toRoleRows(list));
    expect(asked).toEqual([undefined]);
  });

  it("puts the daemon's whole list in the store, replacing the roles it had", () => {
    const ctx = contextOf(createTestMarshal());
    expect(ctx.S.roles).toEqual([]);
    rolesSyncer.apply(ctx, toRoleRows(list));
    expect(roleNames(ctx)).toEqual(["Worker", "Reviewer", "Nightly janitor"]);
    expect(unwrap(ctx.S.roles[0])).toMatchObject({
      name: "Worker",
      starter: true,
      model: "claude-sonnet-4-5",
      limits: { time: 60, cost: 5, rounds: 12 },
    });
    // A save answers the same shape, so the same function applies it: the list it answers wins whole.
    applyRoleList(ctx, { ...list, roles: list.roles.slice(1) });
    expect(roleNames(ctx)).toEqual(["Reviewer", "Nightly janitor"]);
  });

  it("copies what the daemon answered, so one answer can be kept and reused", () => {
    const ctx = contextOf(createTestMarshal());
    const answer = structuredClone(list);
    applyRoleRows(ctx, toRoleRows(answer));
    answer.roles[0]!.name = "Renamed later";
    answer.roles[0]!.spec.skills.push("later");
    expect(ctx.S.roles[0]?.name).toBe("Worker");
    expect(ctx.S.roles[0]?.skills).toEqual(["conventional-commits"]);
  });
});
