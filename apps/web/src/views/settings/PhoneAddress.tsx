import { Button, SettingsPanel } from "@marshal/ui";
import { Show } from "solid-js";
import type { NextStep, PhoneAccess } from "~/platform/phone-access";
import { copyText } from "./copy-text";

const SUBHEADING = "mt-2 mb-0 text-subtitle leading-5.5 font-semibold";

/** A command to run on this computer, with a Copy button. Marshal never runs it: it changes Tailscale. */
function Command(props: { text: string }) {
  return (
    <div class="flex flex-wrap items-start gap-2">
      <code class="flex-1 min-w-0 font-mono text-small py-2 px-2.5 rounded-sm bg-surface-sunken break-all">
        {props.text}
      </code>
      <Button icon="copy" onClick={() => copyText(props.text, "Command copied")}>
        Copy
      </Button>
    </div>
  );
}

/** What is missing, said with the real names, and the command that fixes it when there is one. */
function Step(props: { step: NextStep }) {
  return (
    <SettingsPanel class="flex flex-col gap-2 py-3.5 px-4">
      <span class="font-semibold">{props.step.title}</span>
      <span class="text-small leading-4.5 text-secondary">{props.step.detail}</span>
      <Show when={props.step.command}>{(command) => <Command text={command()} />}</Show>
      <Show when={props.step.link}>
        {(link) => (
          <a
            href={link().href}
            target="_blank"
            rel="noreferrer"
            class="self-start text-small underline"
          >
            {link().label}
          </a>
        )}
      </Show>
      <Show when={props.step.alternative}>
        <span class="text-small leading-4.5 text-secondary">{props.step.alternative}</span>
      </Show>
    </SettingsPanel>
  );
}

/**
 * The address a phone types, when there is one, how Marshal knows it works, and what is missing when
 * it does not. Every sentence comes from what the daemon and Tailscale reported, never from a guess.
 */
export function PhoneAddress(props: { access: PhoneAccess }) {
  return (
    <>
      <h3 class={SUBHEADING}>Phone address</h3>
      <Show when={props.access.address}>
        {(address) => (
          <>
            <div class="flex flex-wrap items-center gap-2">
              <code class="font-mono text-title leading-6 font-semibold py-1 px-2.5 rounded-sm bg-surface-sunken break-all">
                {address()}
              </code>
              <Button icon="copy" onClick={() => copyText(address(), "Address copied")}>
                Copy
              </Button>
            </div>
            <p class="m-0 text-small leading-4.5 text-secondary">
              Type this in the Marshal app on your phone, or scan the code below.
            </p>
          </>
        )}
      </Show>
      <p class="m-0 text-small leading-4.5 text-secondary">{props.access.checked}</p>
      <Show when={props.access.next}>{(step) => <Step step={step()} />}</Show>
    </>
  );
}
