import { Button, Icon } from "@marshal/ui";
import { createResource, createSignal, For, onMount, Show } from "solid-js";
import { M } from "~/mock";
import { platform } from "~/platform";
import { pairingQrText } from "~/platform/pairing";
import { DiscordForm } from "~/views/settings/DiscordForm";
import { createEditState } from "~/views/settings/edit-state";
import { NtfyForm } from "~/views/settings/NtfyForm";
import { PairingQr } from "~/views/settings/PairingQr";
import { TelegramForm } from "~/views/settings/TelegramForm";
import { StatusCheck } from "./StatusCheck";
import { StepIntro } from "./StepIntro";

const MS_PER_MINUTE = 60_000;

const APPS = [
  {
    id: "telegram",
    name: "Telegram",
    icon: "send",
    desc: "Alerts and approval buttons in a Telegram chat",
  },
  {
    id: "discord",
    name: "Discord",
    icon: "message-circle",
    desc: "Alerts and approval buttons in a Discord channel",
  },
  {
    id: "ntfy",
    name: "ntfy",
    icon: "bell",
    desc: "Phone alerts from the free ntfy app, even when Marshal is closed",
  },
] as const;

/** How long a pairing code has left, in whole minutes, for "Expires in 5 minutes". */
function minutesLeft(expiresAt: string): number {
  return Math.max(0, Math.ceil((Date.parse(expiresAt) - Date.now()) / MS_PER_MINUTE));
}

/** The pairing half: a code the daemon just made, and a QR code when a phone can reach this computer. */
function PairAPhone() {
  const [node] = createResource(async () => await M.tailnetStatus());
  onMount(() => void M.createPairingCode());
  const code = () => M.pairingCode();
  const qr = () => pairingQrText(node(), code()?.code);
  return (
    <div class="flex flex-wrap gap-4 items-center py-3.5 border-t border-b border-border">
      <div class="flex-[1_1_220px] flex flex-col gap-1">
        <span class="font-semibold">Pair a phone over Tailscale</span>
        <span class="text-small leading-4.5 text-secondary">
          <Show
            when={node()?.dnsName}
            fallback="Turn Tailscale on for this computer to reach it from a phone. Start Marshal with --tailnet."
          >
            {(name) => `Open ${name()} on your phone, or scan the code, and enter the code.`}
          </Show>
        </span>
        <Button onClick={() => void M.createPairingCode()}>New code</Button>
      </div>
      <Show when={code()}>
        {(live) => (
          <div class="flex flex-col items-center gap-0.5">
            {/* biome-ignore lint/a11y/useAriaPropsSupportedByRole: the design names the code so a screen reader does not spell it out */}
            <code
              aria-label="Pairing code"
              class="font-mono text-display leading-8 font-semibold px-3 py-1 rounded-sm bg-surface-sunken"
            >
              {live().code}
            </code>
            <span class="text-caption text-muted">
              Expires in {minutesLeft(live().expiresAt)} minutes
            </span>
          </div>
        )}
      </Show>
      <Show when={qr()}>{(payload) => <PairingQr text={payload()} label="Pairing QR code" />}</Show>
    </div>
  );
}

/** One alert channel: Connect opens the real form, and a saved connection says so. */
function ChatApp(props: { app: (typeof APPS)[number]; edit: ReturnType<typeof createEditState> }) {
  const row = () => M.S.integrations.find((one) => one.id === props.app.id);
  const open = () => props.edit.id() === props.app.id;
  return (
    <div class="flex flex-col gap-2">
      <div class="flex items-center gap-2.5">
        <Icon name={props.app.icon} size={18} />
        <span class="flex flex-1 flex-col">
          <span class="font-semibold">{props.app.name}</span>
          <span class="text-small text-secondary">{props.app.desc}</span>
        </span>
        <Show
          when={row()?.st === "connected"}
          fallback={
            <Button class="hover:bg-surface!" onClick={() => props.edit.toggle(props.app.id)}>
              {open() ? "Cancel" : `Connect ${props.app.name}`}
            </Button>
          }
        >
          <StatusCheck>Connected</StatusCheck>
        </Show>
      </div>
      <Show when={open() && row()}>
        {(integration) => (
          <>
            <Show when={props.app.id === "telegram"}>
              <TelegramForm integration={integration()} edit={props.edit} />
            </Show>
            <Show when={props.app.id === "discord"}>
              <DiscordForm integration={integration()} edit={props.edit} />
            </Show>
            <Show when={props.app.id === "ntfy"}>
              <NtfyForm integration={integration()} edit={props.edit} />
            </Show>
          </>
        )}
      </Show>
    </div>
  );
}

/**
 * Screen 5 once the daemon owns it (section S31b, B9.1 to B9.3): a real pairing code and QR code, and
 * the real Telegram, Discord, and ntfy connections. On the phone app there is nothing to pair - the
 * phone in hand already is - so it says so and goes straight to where alerts should go.
 */
export function ControlStepLive() {
  const edit = createEditState();
  const [isPhoneApp] = createSignal(platform().kind === "mobile");
  return (
    <>
      <StepIntro>
        Get alerts and approve plans and commands from your phone. All optional.
      </StepIntro>
      <Show
        when={!isPhoneApp()}
        fallback={
          <div class="flex items-center gap-2.5 py-3.5 border-t border-b border-border">
            <StatusCheck>This phone is paired</StatusCheck>
            <span class="text-small text-secondary">It controls your computer over Tailscale.</span>
          </div>
        }
      >
        <PairAPhone />
      </Show>
      <For each={APPS}>{(app) => <ChatApp app={app} edit={edit} />}</For>
    </>
  );
}
