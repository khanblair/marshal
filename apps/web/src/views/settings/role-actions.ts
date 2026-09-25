import { batch } from "solid-js";
import { M, type Role } from "~/mock";
import { cloneJson } from "./json";
import type { RoleDraft } from "./use-role-draft";

const DEFAULT_ROLE_NAME = "Worker";
const NEW_ROLE_NAME = "New role";

/** Adds a copy of `source` as a custom role and selects it. */
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
  batch(() => {
    addRole(draft, edited, `${edited.name} copy`);
    M.toast("Role duplicated");
  });
}

/** A new role starts as a copy of Worker (or, when Worker was renamed, of the first role). */
export function createRole(draft: RoleDraft): void {
  const source = M.S.roles.find((role) => role.name === DEFAULT_ROLE_NAME) ?? M.S.roles[0];
  if (!source) return;
  batch(() => addRole(draft, source, NEW_ROLE_NAME));
}

/** The design only clears the "overridden" flag here; it does not restore the starter text. */
export function confirmResetRole(draft: RoleDraft): void {
  const role = draft.selected();
  if (!role) return;
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
