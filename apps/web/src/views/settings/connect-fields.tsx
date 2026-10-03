import { Button, Field, Input } from "@marshal/ui";
import { type JSX, Show } from "solid-js";

/** A text field of a connection dialog that reads and writes one value the dialog keeps for both tabs. */
export function ValueField(props: {
  label: string;
  hint?: string;
  value: string;
  onValue: (value: string) => void;
  name: string;
  /** A token or secret: hidden as it is typed. */
  secret?: boolean;
  /** An id or an address: shown in the mono face. */
  mono?: boolean;
  placeholder?: string;
  invalid?: boolean;
}) {
  return (
    <Field label={props.label} hint={props.hint}>
      <Input
        mono={props.mono ?? !props.secret}
        type={props.secret ? "password" : "text"}
        name={props.name}
        autocomplete="off"
        spellcheck={false}
        placeholder={props.placeholder}
        value={props.value}
        invalid={props.invalid}
        onInput={(event) => props.onValue(event.currentTarget.value)}
      />
    </Field>
  );
}

/** The sentence under a dialog's fields when saving was refused, in the daemon's own words. */
export function ErrorLine(props: { message: string | null }) {
  return (
    <Show when={props.message}>
      <span role="alert" class="text-small leading-4.5 text-status-danger-text">
        {props.message}
      </span>
    </Show>
  );
}

/** A numbered list of what to do in the other service, before the fields. */
export function Steps(props: { steps: readonly string[] }) {
  return (
    <ol class="m-0 pl-5 flex flex-col gap-0.5 text-small leading-4.5 text-secondary">
      {props.steps.map((step) => (
        <li>{step}</li>
      ))}
    </ol>
  );
}

/** The one Save button each tab of a connection dialog ends with. */
export function SaveRow(props: { busy: boolean; children?: JSX.Element }) {
  return (
    <div class="flex flex-wrap gap-2">
      {props.children}
      <Button variant="primary" type="submit" disabled={props.busy}>
        {props.busy ? "Saving…" : "Save connection"}
      </Button>
    </div>
  );
}
