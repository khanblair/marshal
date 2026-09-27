import { batch } from "solid-js";
import { type Role, readRoleExports, toExport } from "~/data/mappers/roles";
import { M } from "~/mock";
import { cloneJson } from "./json";
import type { RoleDraft } from "./use-role-draft";

const DEFAULT_ROLE_NAME = "Worker";
const NEW_ROLE_NAME = "New role";

/**
 * A name the store is not already using: the wanted one, or the wanted one and a number. The daemon
 * keeps a role's name unique, so a second "New role" has to be called something else.
 */
function freeName(wanted: string, roles: readonly Role[]): string {
  const used = new Set(roles.map((role) => role.name));
  if (!used.has(wanted)) return wanted;
  let n = 2;
  while (used.has(`${wanted} ${n}`)) n += 1;
  return `${wanted} ${n}`;
}

/** Adds a copy of `source` as a custom role and selects it. The mock's own path. */
function addRole(draft: RoleDraft, source: Role, name: string): void {
  const copy = cloneJson(source);
  copy.name = name;
  copy.starter = false;
  copy.overridden = false;
  M.S.roles.push(copy);
  M.S.roleSel = name;
  draft.clear();
}

export function saveRole(draft: RoleDraft): void {
  const role = draft.selected();
  const edited = draft.draft();
  if (!role || !edited || draft.unchanged()) return;
  if (M.rolesOnDaemon()) {
    void M.saveRole(role.name, edited).then((saved) => {
      if (!saved) return;
      // The daemon answers the whole list, so the renamed role is already in the store; the editor
      // follows it by its new name rather than falling back to the first role.
      batch(() => {
        M.S.roleSel = edited.name;
        draft.clear();
        M.toast("Role saved");
      });
    });
    return;
  }
  batch(() => {
    Object.assign(role, cloneJson(edited));
    M.S.roleSel = role.name;
    draft.clear();
    M.toast("Role saved");
  });
}

/** Copies the role as it is in the form, unsaved edits included. */
export function duplicateRole(draft: RoleDraft): void {
  const edited = draft.draft();
  if (!edited) return;
  const name = freeName(`${edited.name} copy`, M.S.roles);
  if (M.rolesOnDaemon()) {
    void M.duplicateRole(name, edited).then((added) => {
      if (!added) return;
      batch(() => {
        M.S.roleSel = name;
        M.toast("Role duplicated");
      });
    });
    return;
  }
  batch(() => {
    addRole(draft, edited, `${edited.name} copy`);
    M.toast("Role duplicated");
  });
}

/** A new role starts as a copy of Worker (or, when Worker was renamed, of the first role). */
export function createRole(draft: RoleDraft): void {
  const source = M.S.roles.find((role) => role.name === DEFAULT_ROLE_NAME) ?? M.S.roles[0];
  if (!source) return;
  if (M.rolesOnDaemon()) {
    const name = freeName(NEW_ROLE_NAME, M.S.roles);
    void M.createRole(toExport({ ...source, name })).then((added) => {
      if (added) M.set({ roleSel: name });
    });
    return;
  }
  batch(() => addRole(draft, source, NEW_ROLE_NAME));
}

/**
 * Reset clears the projects' own versions of a role and nothing else. It does not restore the
 * starter text, because the role's own body is not the thing being reset - this is what the design's
 * own code comment says, and its confirm text is the only thing that suggests otherwise.
 */
export function confirmResetRole(draft: RoleDraft): void {
  const role = draft.selected();
  if (!role) return;
  if (M.rolesOnDaemon()) {
    M.confirm({
      title: "Reset to starter",
      message: `This replaces your edits to ${role.name} with the starter template.`,
      action: "Reset to starter",
      run: () => {
        void M.resetRole(role.name).then((reset) => {
          if (reset) {
            draft.clear();
            M.toast("Role reset");
          }
        });
      },
    });
    return;
  }
  M.confirm({
    title: "Reset to starter",
    message: `This replaces your edits to ${role.name} with the starter template.`,
    action: "Reset to starter",
    run: () =>
      batch(() => {
        role.overridden = false;
        draft.clear();
        M.toast("Role reset");
      }),
  });
}

export function confirmDeleteRole(draft: RoleDraft): void {
  const role = draft.selected();
  if (!role) return;
  if (M.rolesOnDaemon()) {
    M.confirm({
      title: "Delete role",
      message: `This deletes the ${role.name} role. Cards using it switch to ${DEFAULT_ROLE_NAME}.`,
      action: "Delete role",
      destructive: true,
      run: () => {
        void M.deleteRole(role.name).then((deleted) => {
          if (!deleted) return;
          batch(() => {
            M.S.roleSel = DEFAULT_ROLE_NAME;
            M.toast("Role deleted");
          });
        });
      },
    });
    return;
  }
  M.confirm({
    title: "Delete role",
    message: `This deletes the ${role.name} role. Cards using it switch to ${DEFAULT_ROLE_NAME}.`,
    action: "Delete role",
    destructive: true,
    run: () =>
      batch(() => {
        M.S.roles = M.S.roles.filter((r) => r !== role);
        M.S.roleSel = DEFAULT_ROLE_NAME;
        M.toast("Role deleted");
      }),
  });
}

/** Imports a pasted export and answers how many roles it added. Bad text throws a plain sentence. */
export function importRoleExport(text: string): Promise<number> {
  return M.importRoles(readRoleExports(text));
}
