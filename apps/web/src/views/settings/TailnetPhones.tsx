import type { TailnetPhone, TailnetStatus } from "@marshal/protocol";
import { Icon, ItemText } from "@marshal/ui";
import { For, Show } from "solid-js";
import { M } from "~/mock";

const SUBHEADING = "mt-2 mb-0 text-subtitle leading-5.5 font-semibold";

/** What Tailscale says about one phone: online, or when it was last seen. */
function stateOf(phone: TailnetPhone): string {
  if (phone.online) return "Online on your tailnet";
  if (!phone.lastSeen) return "Offline";
  return `Offline, last seen ${M.rel(Date.parse(phone.lastSeen)).toLowerCase()}`;
}

function PhoneRow(props: { phone: TailnetPhone }) {
  return (
    <div class="flex flex-wrap items-center gap-x-3 gap-y-2 py-2.5 border-b border-border">
      <Icon name="smartphone" size={18} />
      <ItemText
        basis={160}
        title={props.phone.name}
        description={
          <span class={props.phone.online ? "text-status-working-text" : undefined}>
            {stateOf(props.phone)}
          </span>
        }
      />
    </div>
  );
}

/**
 * The phones and tablets that are signed in to the tailnet, and whether Tailscale says they are
 * online. A phone that is offline on Tailscale cannot reach this computer whatever Marshal is set up
 * as, which is the first thing to look at when the app cannot connect.
 */
export function TailnetPhones(props: { status: TailnetStatus | null | undefined }) {
  const phones = () => props.status?.host.phones ?? [];
  // Only a computer that is on a tailnet can say which phones are on it.
  const onTailnet = () => props.status?.host.state === "running";
  return (
    <Show when={onTailnet()}>
      <h3 class={SUBHEADING}>Phones on your tailnet</h3>
      <div class="flex flex-col border-t border-border">
        <Show
          when={phones().length > 0}
          fallback={
            <p class="my-2.5 text-secondary">
              No phone or tablet is signed in to this tailnet yet. Install Tailscale on your phone
              and sign in as {props.status?.host.account || "the same account"}.
            </p>
          }
        >
          <For each={phones()}>{(phone) => <PhoneRow phone={phone} />}</For>
        </Show>
      </div>
    </Show>
  );
}
