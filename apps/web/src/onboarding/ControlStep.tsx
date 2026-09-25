import { Button, Icon } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M } from "~/mock";
import { CHAT_APPS, type ChatAppId, PAIRING_CODE } from "./onboarding-data";
import { StatusCheck } from "./StatusCheck";
import { StepIntro } from "./StepIntro";
import type { StepProps } from "./StepProps";

/** Screen 5: pair a phone, and connect Telegram or Discord for approvals. */
export function ControlStep(props: StepProps) {
  const connect = (id: ChatAppId, name: string) => {
    props.setDraft("chatApps", id, true);
    M.toast(`${name} connected`);
  };
  return (
    <>
      <StepIntro>Approve plans and commands from your phone. Both are optional.</StepIntro>
      <div class="flex flex-wrap gap-4 items-center py-3.5 border-t border-b border-border">
        <div class="flex-[1_1_220px] flex flex-col gap-1">
          <span class="font-semibold">Pair a phone over Tailscale</span>
          <span class="text-small leading-4.5 text-secondary">
            Open marshal-laptop.tail3f2a.ts.net on your phone and enter this code.
          </span>
        </div>
        <div class="flex flex-col items-center gap-0.5">
          {/* biome-ignore lint/a11y/useAriaPropsSupportedByRole: the design names the code so a screen reader does not spell it out */}
          <code
            aria-label="Pairing code"
            class="font-mono text-display leading-8 font-semibold px-3 py-1 rounded-sm bg-surface-sunken"
          >
            {PAIRING_CODE}
          </code>
          <span class="text-caption text-muted">Expires in 10 minutes</span>
        </div>
      </div>
      <For each={CHAT_APPS}>
        {(app) => (
          <div class="flex items-center gap-2.5">
            <Icon name={app.icon} size={18} />
            <span class="flex flex-1 flex-col">
              <span class="font-semibold">{app.name}</span>
              <span class="text-small text-secondary">{app.desc}</span>
            </span>
            <Show
              when={props.draft.chatApps[app.id]}
              fallback={
                <Button class="hover:bg-surface!" onClick={() => connect(app.id, app.name)}>
                  {app.button}
                </Button>
              }
            >
              <StatusCheck>Connected</StatusCheck>
            </Show>
          </div>
        )}
      </For>
    </>
  );
}
