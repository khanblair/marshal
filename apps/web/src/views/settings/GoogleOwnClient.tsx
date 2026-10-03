import { Button, Field, Input } from "@marshal/ui";
import { Show } from "solid-js";
import { GoogleConnected } from "./GoogleConnected";
import type { GoogleConnectController, GoogleConnectProps } from "./google-connect";

const CONSOLE_URL = "https://console.cloud.google.com/apis/credentials";

const STEPS = [
  "In the Google Cloud console, make a project and turn on the Google Calendar API.",
  "Under OAuth consent screen, choose External, and add your own Google address as a test user.",
  "Under Credentials, make an OAuth client ID of the type Desktop app.",
  "Paste its client id and secret below.",
];

/**
 * The second tab: a Google client of the person's own, made in their own Google Cloud project. A
 * client saved here is the one Marshal uses, in place of its own. Saving and granting access is one
 * press.
 */
export function GoogleOwnClient(props: {
  integration: GoogleConnectProps["integration"];
  google: GoogleConnectController;
}) {
  const google = () => props.google;
  return (
    <form
      class="flex flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        google().saveOwnAndGrant(event.currentTarget);
      }}
    >
      <Show when={google().connected()}>
        <GoogleConnected integration={props.integration} google={google()} />
      </Show>
      <ol class="m-0 pl-5 flex flex-col gap-0.5 text-small leading-4.5 text-secondary">
        {STEPS.map((step) => (
          <li>{step}</li>
        ))}
      </ol>
      <a
        href={CONSOLE_URL}
        target="_blank"
        rel="noopener noreferrer"
        class="self-start text-small underline"
      >
        Open the Google Cloud console
      </a>
      <Field label="OAuth client id">
        <Input mono name="clientId" autocomplete="off" invalid={!!google().error()} />
      </Field>
      <Field label="OAuth client secret">
        <Input
          type="password"
          name="clientSecret"
          autocomplete="off"
          invalid={!!google().error()}
        />
      </Field>
      <Show when={google().error()}>
        <span role="alert" class="text-small leading-4.5 text-status-danger-text">
          {google().error()}
        </span>
      </Show>
      <div class="flex flex-wrap gap-2">
        <Button variant="primary" type="submit" disabled={google().busy()}>
          {google().busy() ? "Working…" : "Save and grant access"}
        </Button>
      </div>
      <Show when={google().connected()}>
        <span class="text-small leading-4.5 text-secondary">
          Saving a different client replaces the one in use, and you grant access again.
        </span>
      </Show>
    </form>
  );
}
