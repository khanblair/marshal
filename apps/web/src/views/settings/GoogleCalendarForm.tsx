import type { GoogleClientInfo } from "@marshal/protocol";
import { Button, Field, Input } from "@marshal/ui";
import { createResource, createSignal, onCleanup, Show } from "solid-js";
import type { Integration } from "~/mock";
import { M } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import { GoogleCalendarPicker } from "./GoogleCalendarPicker";
import { openGoogleConsent } from "./google-consent";
import {
  authorizeGoogleCalendar,
  connectGoogleCalendar,
  disconnectConnection,
  testConnection,
} from "./integration-actions";

/** How often the row is asked whether Google has been granted, while the person is on Google's page. */
const WAIT_MS = 2000;
/** How long it keeps asking. A person who has not finished by then reads the row again by hand. */
const WAIT_LIMIT_MS = 120_000;
const CONSOLE_URL = "https://console.cloud.google.com/apis/credentials";

const STEPS = [
  "In the Google Cloud console, make a project and turn on the Google Calendar API.",
  "Under OAuth consent screen, choose External, and add your own Google address as a test user.",
  "Under Credentials, make an OAuth client ID of the type Desktop app.",
  "Paste its client id and secret below.",
];

export interface GoogleCalendarFormProps {
  integration: Integration;
  edit: EditState;
  /** Which Google client this build has. The daemon's answer by default; a test gives another. */
  info?: () => Promise<GoogleClientInfo | null>;
  /** Opens Google's consent page. The real one by default; a test gives another. */
  grant?: () => Promise<boolean>;
  /** Reads the connection list again. The store's own call by default; a test gives another. */
  refresh?: () => Promise<void>;
}

/**
 * Google Calendar's connection (B8.3). With Marshal's own Google client built in, it is one button,
 * Connect with Google, and nothing is pasted. Without one, or when the person wants their own, they
 * save a client of their own and then grant access. Granting happens in Google's own tab, so while
 * the person is there the row asks the daemon how it went, and updates when the grant lands.
 */
export function GoogleCalendarForm(props: GoogleCalendarFormProps) {
  const error = () => props.edit.errorFor(props.integration.id);
  const connected = () => props.integration.st === "connected";
  const [info] = createResource(() => (props.info ?? (() => M.googleClientInfo()))());
  const oneClick = () => info()?.bundled === true && info()?.own !== true;
  const clientSaved = () => info()?.own === true || props.integration.st !== "none";
  const [busy, setBusy] = createSignal(false);
  const name = () => props.integration.name;

  let waiting: ReturnType<typeof setInterval> | undefined;
  const stopWaiting = (): void => {
    if (waiting) clearInterval(waiting);
    waiting = undefined;
  };
  onCleanup(stopWaiting);
  const waitForGrant = (): void => {
    stopWaiting();
    const started = Date.now();
    waiting = setInterval(() => {
      void (props.refresh ?? (() => M.refreshIntegrationList()))();
      if (connected() || Date.now() - started > WAIT_LIMIT_MS) stopWaiting();
    }, WAIT_MS);
  };

  const grant = async (): Promise<boolean> => {
    const opened = await (
      props.grant ?? (() => openGoogleConsent(() => authorizeGoogleCalendar()))
    )();
    if (opened) waitForGrant();
    else props.edit.fail("Marshal could not start the Google sign-in. Try again.");
    return opened;
  };

  const connect = (): void => {
    setBusy(true);
    void grant().finally(() => setBusy(false));
  };

  const saveOwnAndGrant = (form: HTMLFormElement): void => {
    const clientId = fieldValue(form, "clientId").trim();
    const clientSecret = fieldValue(form, "clientSecret").trim();
    if (!clientId || !clientSecret) {
      props.edit.fail("Marshal needs both a Google OAuth client id and secret.");
      return;
    }
    setBusy(true);
    void connectGoogleCalendar({ clientId, clientSecret })
      .then((saved) => (saved ? grant() : false))
      .finally(() => setBusy(false));
  };

  const runTest = (): void => {
    setBusy(true);
    void testConnection(props.integration.id, name()).finally(() => setBusy(false));
  };

  const ownClientForm = () => (
    <form
      class="flex flex-col gap-2"
      onSubmit={(event) => {
        event.preventDefault();
        saveOwnAndGrant(event.currentTarget);
      }}
    >
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
        <Input mono name="clientId" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="OAuth client secret">
        <Input type="password" name="clientSecret" autocomplete="off" invalid={!!error()} />
      </Field>
      <div class="flex flex-wrap gap-2">
        <Button variant="primary" type="submit" disabled={busy()}>
          {busy() ? "Working…" : "Save and grant access"}
        </Button>
      </div>
    </form>
  );

  return (
    <div class="flex flex-col gap-3">
      <Show when={error()}>
        <span class="text-small leading-4.5 text-status-danger-text">{error()}</span>
      </Show>
      <Show when={oneClick() && !connected()}>
        <div class="flex flex-col gap-2">
          <span class="text-small leading-4.5 text-secondary">
            Marshal reads your calendar to show your day, brief you, and time its work around your
            meetings. It can only read, never change anything.
          </span>
          <div class="flex flex-wrap items-center gap-2">
            <Button variant="primary" disabled={busy()} onClick={connect}>
              {busy() ? "Opening Google…" : "Connect with Google"}
            </Button>
            <span class="text-small text-secondary">
              Opens Google in a new tab. Use the computer that runs Marshal.
            </span>
          </div>
        </div>
      </Show>
      <Show when={connected()}>
        <div class="flex flex-wrap gap-2">
          <Button disabled={busy()} onClick={connect}>
            Reconnect Google Calendar
          </Button>
          <Button disabled={busy()} onClick={runTest}>
            Test connection
          </Button>
          <Button
            variant="destructive"
            disabled={busy()}
            onClick={() => disconnectConnection(props.integration.id, name())}
          >
            Disconnect
          </Button>
        </div>
      </Show>
      <Show when={!oneClick() && clientSaved() && !connected()}>
        <div class="flex flex-wrap items-center gap-2">
          <Button variant="primary" disabled={busy()} onClick={connect}>
            Grant access
          </Button>
          <span class="text-small text-secondary">
            Opens Google in a new tab. Use the computer that runs Marshal.
          </span>
        </div>
      </Show>
      <Show
        when={oneClick()}
        fallback={
          <Show when={!info.loading}>
            <Show when={info()?.bundled !== true && !clientSaved()}>
              <span class="text-small leading-4.5 text-secondary">
                This build of Marshal has no Google sign-in built in, so you make your own Google
                client once.
              </span>
            </Show>
            {ownClientForm()}
          </Show>
        }
      >
        <details class="text-small">
          <summary class="cursor-pointer text-secondary">Use my own Google client instead</summary>
          <div class="pt-2">{ownClientForm()}</div>
        </details>
      </Show>
      <GoogleCalendarPicker connected={connected()} />
      <Button class="self-start" onClick={() => props.edit.close()}>
        Close
      </Button>
    </div>
  );
}
