import type { GoogleClientInfo, GoogleFileKind } from "@marshal/protocol";
import { createResource, createSignal } from "solid-js";
import { type Integration, M } from "~/mock";
import { platform } from "~/platform";
import type { GoogleFilesAnswer } from "~/sync/google-files-actions";
import type { EditState } from "./edit-state";
import { createGrantWatcher, openGoogleConsent } from "./google-consent";
import type { GoogleFileSpec } from "./google-files-spec";

/** What the Google file dialog is given. The optional parts are for a test to replace. */
export interface GoogleFilesProps {
  spec: GoogleFileSpec;
  integration: Integration;
  edit: EditState;
  /** Which Google client this build has. The daemon's answer by default. */
  info?: () => Promise<GoogleClientInfo | null>;
  /** Opens Google's consent page. The real one by default. */
  grant?: () => Promise<boolean>;
  /** Reads the connection list again. The store's own call by default. */
  refresh?: () => Promise<void>;
  /** Reads the files Marshal made. The store's own call by default. */
  load?: (kind?: GoogleFileKind) => Promise<GoogleFilesAnswer>;
  /** Saves the Drive folder. The store's own call by default. */
  saveFolder?: (folder: string) => Promise<{ saved: true } | { error: string }>;
  /** True on a phone, where Google cannot be connected. This device's own answer by default. */
  phone?: boolean;
}

/** The state and the one action of a Google file dialog, shared by its tabs. */
export function createGoogleFilesConnect(props: GoogleFilesProps) {
  const [info] = createResource(() => (props.info ?? (() => M.googleClientInfo()))());
  const [busy, setBusy] = createSignal(false);
  const connected = () => props.integration.st === "connected";
  const watcher = createGrantWatcher(connected, () =>
    (props.refresh ?? (() => M.refreshIntegrationList()))(),
  );
  const consent = () => openGoogleConsent(() => M.authorizeGoogleService(props.integration.id));
  return {
    /** Something is stored for it: connected, or needing a reconnect. */
    stored: () => props.integration.st !== "none",
    connected,
    /** The daemon says this build has no Google client of its own and none was saved. */
    noClient: (): boolean => {
      const answer = info();
      return !!answer && !answer.bundled && !answer.own;
    },
    phone: (): boolean => props.phone ?? (platform().kind === "mobile" || M.mobile),
    busy,
    error: () => props.edit.errorFor(props.integration.id),
    /** Opens Google's page, then asks the row how it went until the grant lands. */
    connect: (): void => {
      props.edit.clearError();
      setBusy(true);
      void (props.grant ?? consent)()
        .then((opened) => {
          if (opened) watcher.start();
          else props.edit.fail("Marshal could not start the Google sign-in. Try again.");
        })
        .finally(() => setBusy(false));
    },
  };
}

export type GoogleFilesController = ReturnType<typeof createGoogleFilesConnect>;
