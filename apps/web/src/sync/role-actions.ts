import type { CreateRoleRequest, RoleList } from "@marshal/protocol";
import { type Role, toExport, toRoleSpec } from "~/data/mappers/roles";
import type { Ctx } from "~/mock/context";
import { applyRoleList } from "./roles";

/**
 * The writes the Roles screen makes on the daemon (section S27). Every one of them answers with the
 * daemon's whole role list, which is applied to the store as it arrives. The words belong to the
 * screen: a save says "Role saved", an import says how many landed. A refusal is already shown by
 * `optimistic` in the daemon's own words.
 *
 * A role is addressed by its name, which is what a card, a chat target, and the routes call it and
 * which the daemon keeps unique. Nothing here judges a name or a body: the daemon does, and refusing
 * with one plain sentence is the point of there being one authority for it.
 */

/** Runs one role write and applies the whole list it answers. False means the daemon refused it. */
async function write(c: Ctx, key: string, request: () => Promise<RoleList>): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  try {
    const list = await c.optimistic({
      key,
      apply: () => undefined,
      request,
      rollback: () => undefined,
    });
    applyRoleList(c, list);
    return true;
  } catch {
    return false;
  }
}

/** Saves the editor's role: renames it, replaces its body, or both. A field left out is not touched. */
export async function saveRole(c: Ctx, currentName: string, edited: Role): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  const body = {
    spec: toRoleSpec(edited),
    ...(edited.name !== currentName ? { name: edited.name } : {}),
  };
  return write(c, `role:${currentName}`, () => api.updateRole(currentName, body));
}

/** Adds a role from an export document: a fresh one, a duplicate, or an imported role. */
export async function createRole(c: Ctx, doc: CreateRoleRequest): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `role-new:${doc.name}`, () => api.createRole(doc));
}

/** A duplicate is a new role with the body the editor shows, unsaved edits and all. */
export async function duplicateRole(c: Ctx, name: string, role: Role): Promise<boolean> {
  return createRole(c, toExport({ ...role, name }));
}

/**
 * Resets a role: every project that keeps its own version of it is made to use the role itself
 * again. The daemon stores an override per project, so this is one call per project; a project that
 * keeps no override of the role is not an error and has nothing to remove, so the call is the same
 * either way. The role's own body is left exactly as it is, which is what the design's own code
 * comment says a reset does.
 */
export async function resetRole(c: Ctx, name: string): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  const projects = [...c.S.projects];
  if (projects.length === 0) return true;
  let ok = true;
  for (const project of projects) {
    const done = await write(c, `role-reset:${name}:${project.id}`, () =>
      api.resetRole(name, project.id),
    );
    if (!done) ok = false;
  }
  return ok;
}

/** Removes a role a person made. One of Marshal's own is refused by the daemon. */
export async function deleteRole(c: Ctx, name: string): Promise<boolean> {
  const api = c.env.data?.api;
  if (!api) return false;
  return write(c, `role-del:${name}`, () => api.deleteRole(name));
}

/**
 * Imports a pasted export: one role document, or several, one call each. It answers how many landed,
 * so the screen can say so. A refusal stops the run, and is already shown in the daemon's own words,
 * because importing a role whose name is taken must never quietly replace the one that is there.
 */
export async function importRoles(c: Ctx, docs: readonly CreateRoleRequest[]): Promise<number> {
  let added = 0;
  for (const doc of docs) {
    if (!(await createRole(c, doc))) break;
    added += 1;
  }
  return added;
}
