import { Button, Callout, Field, Input, SettingsPanel, TextArea } from "@marshal/ui";
import { Show } from "solid-js";
import type { Role } from "~/mock";
import { RoleLimits, RoleSelects, RoleTags } from "./RoleEditorFields";
import { confirmDeleteRole, confirmResetRole, duplicateRole, saveRole } from "./role-actions";
import { weakModelWarning } from "./role-logic";
import type { RoleDraft } from "./use-role-draft";

/** The form for the picked role: fields, limits, tags, and Save, Duplicate, Reset, and Delete. */
export function RoleEditor(props: { draft: RoleDraft; role: () => Role }) {
  const stored = () => props.draft.selected();
  const warning = () => weakModelWarning(props.role().model, stored()?.name ?? "");
  return (
    <SettingsPanel
      class="flex flex-col gap-3.5 p-4"
      onSubmit={(event) => {
        event.preventDefault();
        saveRole(props.draft);
      }}
    >
      <div class="flex items-center gap-2 flex-wrap">
        <h3 class="m-0 flex-1 text-title leading-6 font-semibold">{props.role().name}</h3>
        <Show when={stored()?.overridden}>
          <span class="text-small text-secondary">Overridden in api-gateway</span>
        </Show>
      </div>
      <Field label="Name">
        <Input
          value={props.role().name}
          onInput={(e) => props.draft.editText("name", e.currentTarget.value)}
        />
      </Field>
      <Field label="Description">
        <Input
          value={props.role().desc}
          onInput={(e) => props.draft.editText("desc", e.currentTarget.value)}
        />
      </Field>
      <Field
        label="Instructions"
        hint="The role's system prompt. Agents also get project memory and board status."
      >
        <TextArea
          rows={4}
          value={props.role().instr}
          onInput={(e) => props.draft.editText("instr", e.currentTarget.value)}
        />
      </Field>
      <RoleSelects draft={props.draft} role={props.role} />
      <Show when={warning()}>
        {(text) => (
          <Callout icon="triangle-alert">
            <span>{text()}</span>
          </Callout>
        )}
      </Show>
      <RoleLimits draft={props.draft} role={props.role} />
      <RoleTags role={props.role} />
      <div class="flex flex-wrap gap-2 pt-1">
        <Button variant="primary" type="submit" disabled={props.draft.unchanged()}>
          Save role
        </Button>
        <Button onClick={() => duplicateRole(props.draft)}>Duplicate</Button>
        <Show when={stored()?.starter}>
          <Button variant="quiet" onClick={() => confirmResetRole(props.draft)}>
            Reset to starter
          </Button>
        </Show>
        <Show when={stored() && !stored()?.starter}>
          <Button variant="quiet" tone="danger" onClick={() => confirmDeleteRole(props.draft)}>
            Delete role
          </Button>
        </Show>
      </div>
    </SettingsPanel>
  );
}
