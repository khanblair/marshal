import { Button, Field, Input } from "@marshal/ui";
import { batch, Show } from "solid-js";
import { M, type Provider } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";

/** A key shorter than this is refused. The Ollama URL is held to it too, as in the design. */
const MIN_KEY_LENGTH = 12;
const MASK_HEAD_LENGTH = 6;
const MASK_TAIL_LENGTH = 4;

function saveKey(provider: Provider, form: HTMLFormElement, edit: EditState): void {
  const value = fieldValue(form, "key").trim();
  if (value.length < MIN_KEY_LENGTH) {
    edit.fail(
      `This key is too short. Copy the full key from your ${provider.name} dashboard and paste it again.`,
    );
    return;
  }
  batch(() => {
    provider.st = "saved";
    provider.masked = provider.local
      ? value
      : `${value.slice(0, MASK_HEAD_LENGTH)}…${value.slice(-MASK_TAIL_LENGTH)}`;
    edit.close();
    M.toast("Key saved");
  });
}

/** The inline form under a provider row: one key (or server URL) field, Save key, and Cancel. */
export function ProviderKeyForm(props: { provider: Provider; edit: EditState }) {
  const error = () => props.edit.errorFor(props.provider.id);
  return (
    <form
      class="flex flex-col gap-1.5"
      onSubmit={(event) => {
        event.preventDefault();
        saveKey(props.provider, event.currentTarget, props.edit);
      }}
    >
      <Field label={props.provider.local ? "Server URL" : `${props.provider.name} API key`}>
        <Input
          mono
          name="key"
          type={props.provider.local ? "text" : "password"}
          autocomplete="off"
          placeholder={props.provider.local ? "http://localhost:11434" : "Paste the key"}
          invalid={!!error()}
        />
      </Field>
      <Show when={error()}>
        <span class="text-small leading-4.5 text-status-danger-text">{error()}</span>
      </Show>
      <div class="flex gap-2">
        <Button variant="primary" type="submit">
          Save key
        </Button>
        <Button onClick={props.edit.close}>Cancel</Button>
      </div>
    </form>
  );
}
