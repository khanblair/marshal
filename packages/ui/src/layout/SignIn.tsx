import {
  createSignal,
  createUniqueId,
  type JSX,
  onCleanup,
  onMount,
  Show,
  splitProps,
} from "solid-js";
import { Button } from "../base/Button";
import { Field } from "../base/Field";
import { Input } from "../base/Input";
import { Icon } from "../icons/Icon";
import { ScreenFrame } from "./_ScreenFrame";
import { FOCUS_DELAY_MS, focusInitial } from "./focus-trap";

export interface SignInProps extends Omit<JSX.HTMLAttributes<HTMLDivElement>, "onSubmit"> {
  /** Called with the trimmed token when the person signs in. It is never called with an empty one. */
  onSubmit: (token: string) => void;
  /** The token is being checked: the button is disabled and shows a spinner. */
  busy?: boolean;
  /** A plain sentence from the caller, shown under the field when the token was refused. */
  error?: string;
  /** Phone layout. */
  phone?: boolean;
}

const ICON_PX = 20;

/**
 * The screen where a person pastes the access token for the daemon. The token stays in the
 * input: it is not put in an attribute, echoed as text, or logged, and it is trimmed before it
 * reaches `onSubmit`. It is a form, so Enter submits, except while the button is disabled.
 */
export function SignIn(props: SignInProps) {
  const [local, others] = splitProps(props, ["onSubmit", "busy", "error", "phone"]);
  const [token, setToken] = createSignal("");
  const titleId = createUniqueId();
  const errorId = createUniqueId();
  let form: HTMLFormElement | undefined;

  // The screen is not a Dialog, so nothing else honors `data-autofocus` here.
  onMount(() => {
    const timer = setTimeout(() => form && focusInitial(form), FOCUS_DELAY_MS);
    onCleanup(() => clearTimeout(timer));
  });

  const canSubmit = () => token().trim() !== "" && !local.busy;
  const submit: JSX.EventHandler<HTMLFormElement, SubmitEvent> = (event) => {
    event.preventDefault();
    // The disabled button already blocks this, but a script or a held key must not get past it.
    if (canSubmit()) local.onSubmit(token().trim());
  };

  return (
    <ScreenFrame phone={local.phone} {...others}>
      <Icon name="key-round" size={ICON_PX} />
      <h1 id={titleId} class="m-0 text-title leading-6 font-semibold">
        Sign in to Marshal
      </h1>
      <p class="m-0 text-secondary">
        Paste the access token for this computer. To see it, run{" "}
        <code class="py-px px-1.5 rounded-xs bg-surface-sunken font-mono text-small whitespace-nowrap">
          marshal token --show
        </code>{" "}
        in a terminal on the computer where Marshal runs.
      </p>
      <form
        ref={form}
        aria-labelledby={titleId}
        aria-busy={local.busy ? "true" : undefined}
        onSubmit={submit}
        class="w-full flex flex-col gap-3.5 text-left"
      >
        <div class="flex flex-col gap-1.5">
          <Field label="Access token">
            <Input
              type="password"
              autocomplete="off"
              spellcheck={false}
              data-autofocus
              readOnly={local.busy}
              invalid={Boolean(local.error)}
              aria-describedby={local.error ? errorId : undefined}
              onInput={(event) => setToken(event.currentTarget.value)}
            />
          </Field>
          <Show when={local.error}>
            {(message) => (
              <p id={errorId} role="alert" class="m-0 text-small text-status-danger-text">
                {message()}
              </p>
            )}
          </Show>
        </div>
        <Button
          type="submit"
          variant="primary"
          size={36}
          disabled={!canSubmit()}
          icon={local.busy ? "spinner" : undefined}
          class="w-full"
        >
          Sign in
        </Button>
      </form>
    </ScreenFrame>
  );
}
