import type { RoleList } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { type RoleRows, toRoleRows } from "~/data/mappers/roles";
import type { Ctx } from "~/mock/context";
import type { Syncer } from "./syncer";

/**
 * Section S27: the role templates. The daemon holds them, so the store mirrors what it says and
 * every screen edits them through it.
 *
 * The list is global: the roles themselves do not change with a project, only whether each reads as
 * overridden, and the Roles screen looks at no project. So the snapshot is asked for without one and
 * `overridden` is false for every role until a screen that is scoped to a project is built.
 *
 * There are no role events: the routes answer the whole list, so a load, a save, a rename, a delete,
 * a reset, and an import all arrive through `applyRoleList`, and the snapshot this syncer loads is
 * the same shape. It publishes no topic, so it is loaded once when the app comes online and after
 * every re-sync.
 */
export const rolesSyncer: Syncer<RoleRows> = {
  section: "S27",
  topics: [],
  async load(api: ApiClient) {
    return toRoleRows(await api.listRoles());
  },
  apply(ctx, rows) {
    applyRoleRows(ctx, rows);
  },
};

/** The daemon's list answer as the store holds it. Every roles route answers this shape. */
export function applyRoleList(ctx: Ctx, list: RoleList): void {
  applyRoleRows(ctx, toRoleRows(list));
}

/**
 * Replaces the store's roles with the daemon's. It is one write, so a screen redraws once. The
 * picked name is left alone: the role editor falls back to the first role when the picked one is
 * gone, so a delete needs no bookkeeping here.
 */
export function applyRoleRows(ctx: Ctx, rows: RoleRows): void {
  ctx.S.roles = rows.roles;
}

/** Every role's name, in the order the store holds them: what the role pickers list. */
export function roleNames(ctx: Ctx): readonly string[] {
  return ctx.S.roles.map((role) => role.name);
}
