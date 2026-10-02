import { Button, Icon } from "@marshal/ui";
import { Show } from "solid-js";
import { M } from "~/mock";

const NOT_CONNECTED = "Google Calendar is not connected, so your Google events are not shown.";
const PHONE_ADVICE = " Connect it from the computer that runs Marshal.";

function openIntegrations(): void {
  M.set({ settingsSection: "integrations" });
  M.go("settings");
}

/**
 * Says what is wrong with Google's events, in the calendar and in Home's "Coming up today": Google
 * Calendar is not connected, access was refused, or Google could not be reached. It draws nothing
 * while all is well, and nothing before the daemon has answered, so it never claims a Google it has
 * not asked about.
 */
export function GoogleNotice(props: { class?: string }) {
  const google = () => M.S.calGoogle;
  const problem = () => google().known && (!google().connected || google().error !== "");
  const text = () => {
    const { connected, error } = google();
    if (error) return error;
    return connected ? "" : NOT_CONNECTED + (M.mobile ? PHONE_ADVICE : "");
  };
  return (
    <Show when={problem()}>
      <div
        role="status"
        class={`flex items-center gap-2.5 py-2 px-3 border border-border rounded-md bg-surface-sunken text-small leading-4.5 ${props.class ?? ""}`}
      >
        <Icon
          name={google().error ? "triangle-alert" : "calendar"}
          size={14}
          class="flex-none text-secondary"
        />
        <span class="flex-1 min-w-0">{text()}</span>
        <Show when={!M.mobile}>
          <Button size={28} class="text-small flex-none" onClick={openIntegrations}>
            {google().connected ? "Open settings" : "Connect"}
          </Button>
        </Show>
      </div>
    </Show>
  );
}
