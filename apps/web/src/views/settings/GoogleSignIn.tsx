import { Button } from "@marshal/ui";
import { Show } from "solid-js";
import { GoogleConnected } from "./GoogleConnected";
import type { GoogleConnectController, GoogleConnectProps } from "./google-connect";

const OPENS_GOOGLE = "Opens Google in a new tab. Use the computer that runs Marshal.";

/**
 * The first tab: sign in with Marshal's own Google client, in one click and with nothing pasted. A
 * build that has no client says so and points to the other tab, and a person who saved a client of
 * their own grants access from here too.
 */
export function GoogleSignIn(props: {
  integration: GoogleConnectProps["integration"];
  google: GoogleConnectController;
}) {
  const google = () => props.google;
  return (
    <div class="flex flex-col gap-3">
      <Show when={google().error()}>
        <span role="alert" class="text-small leading-4.5 text-status-danger-text">
          {google().error()}
        </span>
      </Show>
      <Show when={google().connected()}>
        <p class="m-0 font-semibold">Connected to Google Calendar</p>
        <GoogleConnected integration={props.integration} google={google()} />
      </Show>
      <Show when={!google().connected() && google().oneClick()}>
        <div class="flex flex-col gap-2">
          <span class="text-small leading-4.5 text-secondary">
            Marshal reads your calendar to show your day, brief you, and time its work around your
            meetings. It can only read, never change anything.
          </span>
          <div class="flex flex-wrap items-center gap-2">
            <Button variant="primary" disabled={google().busy()} onClick={google().connect}>
              {google().busy() ? "Opening Google…" : "Connect with Google"}
            </Button>
            <span class="text-small text-secondary">{OPENS_GOOGLE}</span>
          </div>
        </div>
      </Show>
      <Show when={!google().connected() && !google().oneClick() && !google().info.loading}>
        <Show
          when={google().clientSaved()}
          fallback={
            <span class="text-small leading-4.5 text-secondary">
              This build of Marshal has no Google sign-in built in, so you make your own Google
              client once. The other tab shows how.
            </span>
          }
        >
          <div class="flex flex-wrap items-center gap-2">
            <Button variant="primary" disabled={google().busy()} onClick={google().connect}>
              Grant access
            </Button>
            <span class="text-small text-secondary">{OPENS_GOOGLE}</span>
          </div>
        </Show>
      </Show>
    </div>
  );
}
