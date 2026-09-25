import { Badge, Button, cx, SettingsPanel, SettingsSection } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M, type Role } from "~/mock";
import { RoleEditor } from "./RoleEditor";
import { createRole } from "./role-actions";
import type { RoleDraft } from "./use-role-draft";

/** From this width on, the role list sits beside the editor. */
const SIDE_BY_SIDE_MIN_WIDTH_PX = 900;

function RoleOption(props: { role: Role; selected: boolean; onSelect: () => void }) {
  return (
    <button
      type="button"
      role="option"
      aria-selected={props.selected}
      onClick={props.onSelect}
      class={cx(
        "flex flex-col gap-0.5 py-2.5 px-3 border-0 border-b border-border text-left hover:bg-surface-hover",
        props.selected ? "bg-surface-selected" : "bg-transparent",
      )}
    >
      <span class="flex items-center gap-1.5 font-semibold">
        {props.role.name}
        <Show when={props.role.starter}>
          <Badge tone="outline" size={18} class="font-medium! leading-4">
            Starter
          </Badge>
        </Show>
      </span>
      <span class="flex flex-wrap gap-x-2.5 text-caption leading-4 text-secondary">
        <span>{props.role.model}</span>
        <span>{props.role.overridden ? "Overridden in 1 project" : props.role.perm}</span>
      </span>
    </button>
  );
}

/** Roles: the list of roles, and the editor for the picked one. */
export function RolesSection(props: { draft: RoleDraft }) {
  return (
    <SettingsSection
      title="Roles"
      description="Roles decide how an agent behaves. Marshal ships starter templates, and you can edit every field. Projects can override a role."
      actions={
        <Button icon="plus" onClick={() => createRole(props.draft)}>
          New role
        </Button>
      }
    >
      <div
        class={cx(
          "grid gap-4 items-start",
          M.S.vw < SIDE_BY_SIDE_MIN_WIDTH_PX
            ? "grid-cols-[1fr]"
            : "grid-cols-[240px_minmax(0,1fr)]",
        )}
      >
        <SettingsPanel list role="listbox" aria-label="Roles">
          <For each={M.S.roles}>
            {(role) => (
              <RoleOption
                role={role}
                selected={role.name === props.draft.selected()?.name}
                onSelect={() => props.draft.select(role.name)}
              />
            )}
          </For>
        </SettingsPanel>
        <Show when={props.draft.draft()}>
          {(role) => <RoleEditor draft={props.draft} role={role} />}
        </Show>
      </div>
    </SettingsSection>
  );
}
