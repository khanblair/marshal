import type { GoogleClientInfo } from "@marshal/protocol";
import { createResource, createSignal, type Resource } from "solid-js";
import type { Integration } from "~/mock";
import { M } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import { createGrantWatcher, openGoogleConsent } from "./google-consent";
import { authorizeGoogleCalendar, connectGoogleCalendar } from "./integration-actions";

/** What the Google Calendar dialog is given. The optional parts are for a test to replace. */
export interface GoogleConnectProps {
  integration: Integration;
  edit: EditState;
  /** Which Google client this build has. The daemon's answer by default. */
  info?: () => Promise<GoogleClientInfo | null>;
  /** Opens Google's consent page. The real one by default. */
  grant?: () => Promise<boolean>;
  /** Reads the connection list again. The store's own call by default. */
  refresh?: () => Promise<void>;
}

/** What both tabs of the Google Calendar dialog read and do, so they show the one connection. */
export interface GoogleConnectController {
  info: Resource<GoogleClientInfo | null>;
  /** Marshal's own client is built in and the person has not saved one of their own. */
  oneClick: () => boolean;
  /** A client is saved, the person's own or a row left by an earlier grant. */
  clientSaved: () => boolean;
  connected: () => boolean;
  busy: () => boolean;
  error: () => string | null;
  /** Opens Google's page, then asks the row how it went until the grant lands. */
  connect: () => void;
  /** Saves the pasted client, then opens Google's page. */
  saveOwnAndGrant: (form: HTMLFormElement) => void;
}

/** The state and the two actions of the Google Calendar dialog, shared by its tabs. */
export function createGoogleConnect(props: GoogleConnectProps): GoogleConnectController {
  const [info] = createResource(() => (props.info ?? (() => M.googleClientInfo()))());
  const [busy, setBusy] = createSignal(false);
  const connected = () => props.integration.st === "connected";
  const watcher = createGrantWatcher(connected, () =>
    (props.refresh ?? (() => M.refreshIntegrationList()))(),
  );

  const grant = async (): Promise<boolean> => {
    const opened = await (
      props.grant ?? (() => openGoogleConsent(() => authorizeGoogleCalendar()))
    )();
    if (opened) watcher.start();
    else props.edit.fail("Marshal could not start the Google sign-in. Try again.");
    return opened;
  };

  return {
    info,
    oneClick: () => info()?.bundled === true && info()?.own !== true,
    clientSaved: () => info()?.own === true || props.integration.st !== "none",
    connected,
    busy,
    error: () => props.edit.errorFor(props.integration.id),
    connect: () => {
      props.edit.clearError();
      setBusy(true);
      void grant().finally(() => setBusy(false));
    },
    saveOwnAndGrant: (form) => {
      const clientId = fieldValue(form, "clientId").trim();
      const clientSecret = fieldValue(form, "clientSecret").trim();
      if (!clientId || !clientSecret) {
        props.edit.fail("Marshal needs both a Google OAuth client id and secret.");
        return;
      }
      props.edit.clearError();
      setBusy(true);
      void connectGoogleCalendar({ clientId, clientSecret })
        .then((saved) => (saved ? grant() : false))
        .finally(() => setBusy(false));
    },
  };
}
