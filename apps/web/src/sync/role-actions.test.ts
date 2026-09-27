import type { CreateRoleRequest, RoleList } from "@marshal/protocol";
import { afterEach, describe, expect, it } from "vitest";
import { type Role, toExport, toRole, toRoleSpec } from "~/data/mappers/roles";
import { golden } from "~/data/testing/golden";
import type { Marshal } from "~/mock";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { createSyncedMarshal, createTestMarshal } from "~/testing/test-store";

// The role writes the Roles screen makes once the roles (S27) are the daemon's. The fake daemon
// holds the golden list: Worker and Reviewer, which Marshal ships, and Nightly janitor, which a
// person made. Every one of these answers the daemon's whole list, which is what the store keeps.

const LIST = golden<RoleList>("role-list");

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

async function setup(): Promise<{ M: Marshal; d: FakeDaemon }> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS });
  daemon = d;
  return { M: await createSyncedMarshal(d), d };
}

const names = (M: Marshal): string[] => M.S.roles.map((row) => row.name);
const role = (M: Marshal, name: string): Role => {
  const found = M.S.roles.find((row) => row.name === name);
  if (!found) throw new Error(`the store has no ${name} role`);
  return found;
};
const toasts = (M: Marshal): string[] => M.S.toasts.map((toast) => toast.msg);
const worker = (): Role => toRole(LIST.roles[0] as RoleList["roles"][number]);

describe("saving a role on the daemon", () => {
  it("sends the body alone when the name did not change, and keeps the list it answers", async () => {
    const { M, d } = await setup();
    const edited = { ...worker(), model: "gpt-5-mini" };
    expect(await M.saveRole("Worker", edited)).toBe(true);
    expect(d.bodies("PATCH /v1/roles/Worker")).toEqual([{ spec: toRoleSpec(edited) }]);
    expect(role(M, "Worker").model).toBe("gpt-5-mini");
    expect(names(M)).toEqual(["Worker", "Reviewer", "Nightly janitor"]);
  });

  it("sends the new name as well when the role was renamed, and the rename changes the role itself", async () => {
    const { M, d } = await setup();
    const edited = { ...worker(), name: "Builder" };
    expect(await M.saveRole("Worker", edited)).toBe(true);
    expect(d.bodies("PATCH /v1/roles/Worker")).toEqual([
      { spec: toRoleSpec(edited), name: "Builder" },
    ]);
    expect(names(M)).toEqual(["Builder", "Reviewer", "Nightly janitor"]);
  });

  it("shows the daemon's sentence, and changes nothing, when the new name is taken", async () => {
    const { M } = await setup();
    expect(await M.saveRole("Worker", { ...worker(), name: "Reviewer" })).toBe(false);
    expect(toasts(M)).toEqual(['A role called "Reviewer" already exists. Choose another name.']);
    expect(names(M)).toEqual(["Worker", "Reviewer", "Nightly janitor"]);
  });
});

describe("adding a role on the daemon", () => {
  const janitor = (name: string): CreateRoleRequest => toExport({ ...worker(), name });

  it("creates one from an export document and keeps the list that comes back", async () => {
    const { M, d } = await setup();
    const doc = janitor("Sweeper");
    expect(await M.createRole(doc)).toBe(true);
    expect(d.bodies("POST /v1/roles")).toEqual([doc]);
    expect(names(M)).toEqual(["Worker", "Reviewer", "Nightly janitor", "Sweeper"]);
  });

  it("refuses a name another role has, so an import never overwrites an edit", async () => {
    const { M } = await setup();
    expect(await M.createRole(janitor("Worker"))).toBe(false);
    expect(toasts(M)).toEqual(['A role called "Worker" already exists. Choose another name.']);
    expect(names(M)).toEqual(["Worker", "Reviewer", "Nightly janitor"]);
  });

  it("duplicates the editor's role, unsaved edits and all", async () => {
    const { M, d } = await setup();
    const edited = { ...worker(), model: "unsaved-model" };
    expect(await M.duplicateRole("Worker copy", edited)).toBe(true);
    expect(d.bodies("POST /v1/roles")).toEqual([toExport({ ...edited, name: "Worker copy" })]);
    expect(role(M, "Worker copy").model).toBe("unsaved-model");
  });
});

describe("resetting a role on the daemon", () => {
  it("clears the override of every project, one call each, and leaves the role itself alone", async () => {
    const { M, d } = await setup();
    const before = role(M, "Reviewer").model;
    expect(await M.resetRole("Reviewer")).toBe(true);
    expect(d.routes().filter((route) => route.startsWith("POST /v1/roles/Reviewer/reset"))).toEqual(
      [
        "POST /v1/roles/Reviewer/reset?project=api",
        "POST /v1/roles/Reviewer/reset?project=web",
        "POST /v1/roles/Reviewer/reset?project=mobile",
      ],
    );
    expect(role(M, "Reviewer").model).toBe(before);
  });
});

describe("deleting a role on the daemon", () => {
  it("refuses one of Marshal's own, in the daemon's words, and leaves it there", async () => {
    const { M, d } = await setup();
    expect(await M.deleteRole("Worker")).toBe(false);
    expect(toasts(M)).toEqual([
      "Worker is one of Marshal's own roles, so it cannot be deleted. Reset it instead.",
    ]);
    expect(d.routes()).toContain("DELETE /v1/roles/Worker");
    expect(names(M)).toEqual(["Worker", "Reviewer", "Nightly janitor"]);
  });

  it("removes a role a person made", async () => {
    const { M } = await setup();
    expect(await M.deleteRole("Nightly janitor")).toBe(true);
    expect(names(M)).toEqual(["Worker", "Reviewer"]);
  });
});

describe("importing roles on the daemon", () => {
  it("adds each pasted role, one call each, and says how many landed", async () => {
    const { M, d } = await setup();
    const docs = [toExport({ ...worker(), name: "One" }), toExport({ ...worker(), name: "Two" })];
    expect(await M.importRoles(docs)).toBe(2);
    expect(d.bodies("POST /v1/roles")).toEqual(docs);
    expect(names(M)).toEqual(["Worker", "Reviewer", "Nightly janitor", "One", "Two"]);
  });

  it("stops at the first name that is taken, and says so in the daemon's words", async () => {
    const { M, d } = await setup();
    const docs = [
      toExport({ ...worker(), name: "One" }),
      toExport({ ...worker(), name: "Worker" }),
    ];
    expect(await M.importRoles(docs)).toBe(1);
    // The third was never asked for: importing must never overwrite the role that is there.
    expect(d.bodies("POST /v1/roles")).toEqual(docs.slice(0, 2));
    expect(toasts(M)).toEqual(['A role called "Worker" already exists. Choose another name.']);
  });
});

describe("the role writes with no daemon", () => {
  it("changes nothing and shows nothing, because there is nothing to ask", async () => {
    const M = createTestMarshal();
    expect(await M.saveRole("Worker", worker())).toBe(false);
    expect(await M.createRole(toExport({ ...worker(), name: "One" }))).toBe(false);
    expect(await M.deleteRole("Worker")).toBe(false);
    expect(await M.resetRole("Worker")).toBe(false);
    expect(await M.importRoles([toExport(worker())])).toBe(0);
    expect(toasts(M)).toEqual([]);
  });
});
