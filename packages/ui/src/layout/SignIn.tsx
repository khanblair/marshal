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
  /**
   * Called with the code and the name to give this device when the person pairs instead. Without it
   * the screen only takes a token, as it always did.
   */
  onPair?: (code: string, name: string) => void;
  /** Reads the code with the camera. Given only where the device can, it adds a Scan button. */
  onScan?: () => void;
  /** A plain sentence from the caller, shown under the code field when pairing was refused. */
  pairError?: string;
  /** The name the device field starts with, such as "Phone browser". */
  deviceName?: string;
  /** Phone layout. */
  phone?: boolean;
  /**
   * What the screen leads with. `token` (the default) asks for the access token first and offers
   * pairing under it, which suits a computer. `pair` leads with pairing, a scan or a code from the
   * computer, and keeps the token behind "Use an access token instead", which suits a phone: nobody
   * should have to type a token there. Without `onPair` the screen only takes a token either way.
   */
  lead?: "token" | "pair";
}

const ICON_PX = 20;

/**
 * The screen where a person pastes the access token for the daemon. The token stays in the
 * input: it is not put in an attribute, echoed as text, or logged, and it is trimmed before it
 * reaches `onSubmit`. It is a form, so Enter submits, except while the button is disabled.
 */
export function SignIn(props: SignInProps) {
  const [local, others] = splitProps(props, [
    "onSubmit",
    "busy",
    "error",
    "phone",
    "onPair",
    "onScan",
    "pairError",
    "deviceName",
    "lead",
  ]);
  const [token, setToken] = createSignal("");
  const titleId = createUniqueId();
  const errorId = createUniqueId();
  let form: HTMLFormElement | undefined;

  const pairFirst = () => local.lead === "pair" && Boolean(local.onPair);
  // The screen is not a Dialog, so nothing else honors `data-autofocus` here. A pairing-first screen
  // does not take focus, because on a phone that would raise the keyboard before anyone chose.
  onMount(() => {
    if (pairFirst()) return;
    const timer = setTimeout(() => form && focusInitial(form), FOCUS_DELAY_MS);
    onCleanup(() => clearTimeout(timer));
  });

  const [code, setCode] = createSignal("");
  const [name, setName] = createSignal(props.deviceName ?? "");
  const pairErrorId = createUniqueId();
  const canSubmit = () => token().trim() !== "" && !local.busy;
  const canPair = () => code().trim() !== "" && name().trim() !== "" && !local.busy;
  const pair: JSX.EventHandler<HTMLFormElement, SubmitEvent> = (event) => {
    event.preventDefault();
    if (canPair()) local.onPair?.(code().trim(), name().trim());
  };
  const submit: JSX.EventHandler<HTMLFormElement, SubmitEvent> = (event) => {
    event.preventDefault();
    // The disabled button already blocks this, but a script or a held key must not get past it.
    if (canSubmit()) local.onSubmit(token().trim());
  };

  const tokenForm = () => (
    <form
      ref={form}
      aria-label={pairFirst() ? "Sign in with a token" : undefined}
      aria-labelledby={pairFirst() ? undefined : titleId}
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
  );

  const tokenHelp = () => (
    <p class="m-0 text-secondary">
      Paste the access token for this computer. To see it, run{" "}
      <code class="py-px px-1.5 rounded-xs bg-surface-sunken font-mono text-small whitespace-nowrap">
        marshal token --show
      </code>{" "}
      in a terminal on the computer where Marshal runs.
    </p>
  );

  const pairForm = () => (
    <form
      aria-label="Pair with a code"
      aria-busy={local.busy ? "true" : undefined}
      onSubmit={pair}
      class="w-full flex flex-col gap-3.5"
    >
      <Show when={pairFirst() && local.onScan}>
        {(scan) => (
          <Button
            variant="primary"
            size={36}
            disabled={local.busy}
            class="w-full"
            onClick={() => scan()()}
          >
            Scan the QR code
          </Button>
        )}
      </Show>
      <div class="flex flex-col gap-1.5">
        <Field label="Pairing code">
          <Input
            autocomplete="off"
            autocapitalize="characters"
            spellcheck={false}
            mono
            readOnly={local.busy}
            invalid={Boolean(local.pairError)}
            aria-describedby={local.pairError ? pairErrorId : undefined}
            onInput={(event) => setCode(event.currentTarget.value)}
          />
        </Field>
        <Show when={local.pairError}>
          {(message) => (
            <p id={pairErrorId} role="alert" class="m-0 text-small text-status-danger-text">
              {message()}
            </p>
          )}
        </Show>
      </div>
      <Field label="Device name">
        <Input
          autocomplete="off"
          value={name()}
          readOnly={local.busy}
          onInput={(event) => setName(event.currentTarget.value)}
        />
      </Field>
      <Button type="submit" size={36} disabled={!canPair()} class="w-full">
        Pair this device
      </Button>
      <Show when={!pairFirst() && local.onScan}>
        {(scan) => (
          <Button size={36} disabled={local.busy} class="w-full" onClick={() => scan()()}>
            Scan the QR code
          </Button>
        )}
      </Show>
    </form>
  );

  return (
    <ScreenFrame phone={local.phone} {...others}>
      <Show
        when={pairFirst()}
        fallback={
          <>
            <Icon name="key-round" size={ICON_PX} />
            <h1 id={titleId} class="m-0 text-title leading-6 font-semibold">
              Sign in to Marshal
            </h1>
            {tokenHelp()}
            {tokenForm()}
            <Show when={local.onPair}>
              <div class="w-full flex flex-col gap-3.5 text-left border-t border-border pt-3.5">
                <h2 class="m-0 text-body font-semibold">Or pair this device with a code</h2>
                <p class="m-0 text-secondary">
                  On the computer where Marshal runs, open Settings, then Remote control, then Pair
                  a device.
                </p>
                {pairForm()}
              </div>
            </Show>
          </>
        }
      >
        <Icon name="smartphone" size={ICON_PX} />
        <h1 id={titleId} class="m-0 text-title leading-6 font-semibold">
          Connect to your computer
        </h1>
        <p class="m-0 text-secondary">
          Pair this device with a code from your computer. There, open Settings, then Remote
          control, then Pair a device, and scan the code or type it here.
        </p>
        {pairForm()}
        <details class="w-full text-left border-t border-border pt-3.5">
          <summary class="cursor-pointer font-semibold">Use an access token instead</summary>
          <div class="flex flex-col gap-3.5 pt-3.5">
            {tokenHelp()}
            {tokenForm()}
          </div>
        </details>
      </Show>
    </ScreenFrame>
  );
}
