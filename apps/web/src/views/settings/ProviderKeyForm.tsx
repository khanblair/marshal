import { Button, Field, Input } from "@marshal/ui";
import { Show } from "solid-js";
import { M, type Provider } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";

/** A key shorter than this is refused. The Ollama URL is held to it too, as in the design. */
const MIN_KEY_LENGTH = 12;

/**
 * The inline form under a provider row: one key (or server URL) field, Save key, and Cancel.
 *
 * The shortest-value check is the design's, so it is judged here. Everything else - whether the
 * key works, what it is masked as - is the daemon's, and its refusal is shown as its own sentence.
 * The form closes only once the daemon has stored the value.
 */
export function ProviderKeyForm(props: { provider: Provider; edit: EditState }) {
  const error = () => props.edit.errorFor(props.provider.id);
  const submit = (form: HTMLFormElement): void => {
    const value = fieldValue(form, "key").trim();
    if (value.length < MIN_KEY_LENGTH) {
      props.edit.fail(
        `This key is too short. Copy the full key from your ${props.provider.name} dashboard and paste it again.`,
      );
      return;
    }
    void M.saveProviderKey(props.provider.id, value).then((saved) => {
      if (saved) props.edit.close();
    });
  };
  return (
    <form
      class="flex flex-col gap-1.5"
      onSubmit={(event) => {
        event.preventDefault();
        submit(event.currentTarget);
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
