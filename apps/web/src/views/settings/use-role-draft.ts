import { createMemo, createSignal } from "solid-js";
import { M, type Role } from "~/mock";
import { cloneJson, sameJson } from "./json";

export type RoleTextKey =
  | "name"
  | "desc"
  | "instr"
  | "agent"
  | "model"
  | "backup"
  | "think"
  | "perm";
export type RoleLimitKey = keyof Role["limits"];

interface Edited {
  /** The role the edit was made on. Selecting another role hides it. */
  forName: string;
  role: Role;
}

/** The role editor's unsaved edits, kept while the section is switched away. */
export function createRoleDraft() {
  const [edited, setEdited] = createSignal<Edited | null>(null);
  /** The stored role picked in the list; the first one when the picked name is gone. */
  const selected = createMemo(
    () => M.S.roles.find((role) => role.name === M.S.roleSel) ?? M.S.roles[0],
  );
  const draft = createMemo<Role | undefined>(() => {
    const role = selected();
    if (!role) return undefined;
    const current = edited();
    return current && current.forName === role.name ? current.role : cloneJson(role);
  });
  const store = (role: Role) => {
    const forName = selected()?.name;
    if (forName !== undefined) setEdited({ forName, role });
  };
  return {
    selected,
    draft,
    unchanged: () => {
      const role = selected();
      const current = draft();
      return !role || !current || sameJson(current, role);
    },
    clear: () => setEdited(null),
    select(name: string): void {
      M.set({ roleSel: name });
      setEdited(null);
    },
    editText(key: RoleTextKey, value: string): void {
      const current = draft();
      if (!current) return;
      const next = cloneJson(current);
      next[key] = value;
      // A new agent brings its own models, so the model resets to the agent's first one.
      if (key === "agent") next.model = M.AGENTS[value]?.models[0] ?? next.model;
      store(next);
    },
    editLimit(key: RoleLimitKey, value: string): void {
      const current = draft();
      if (!current) return;
      const next = cloneJson(current);
      next.limits[key] = +value;
      store(next);
    },
  };
}

export type RoleDraft = ReturnType<typeof createRoleDraft>;
