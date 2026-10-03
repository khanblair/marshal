import { createSignal } from "solid-js";
import { createStore } from "solid-js/store";
import { type Integration, M } from "~/mock";
import type { EditState } from "./edit-state";
import { createGrantWatcher, openGoogleConsent } from "./google-consent";
import { connectGmail } from "./integration-actions";

/** What the Gmail dialog is given. The optional parts are for a test to replace. */
export interface GmailConnectProps {
  integration: Integration;
  edit: EditState;
  /** Opens Google's consent page. The real one by default. */
  grant?: () => Promise<boolean>;
  /** Reads the connection list again. The store's own call by default. */
  refresh?: () => Promise<void>;
}

/** The fields and the two actions of the Gmail dialog, shared by its tabs. */
export function createGmailConnect(props: GmailConnectProps, onSavedUngranted: () => void) {
  const [draft, setDraft] = createStore({ label: "", projectId: M.S.projects[0]?.id ?? "" });
  const [busy, setBusy] = createSignal(false);
  const connected = () => props.integration.st === "connected";
  const watcher = createGrantWatcher(connected, () =>
    (props.refresh ?? (() => M.refreshIntegrationList()))(),
  );
  return {
    draft,
    set: (key: "label" | "projectId", value: string): void => setDraft(key, value),
    busy,
    connected,
    error: () => props.edit.errorFor(props.integration.id),
    /** Saves the label and the project. Once Google is granted too, the dialog closes. */
    save: (): void => {
      if (!draft.label.trim()) {
        props.edit.fail("Choose the Gmail label Marshal should watch.");
        return;
      }
      if (!draft.projectId) {
        props.edit.fail("Choose the Marshal project a labeled email becomes a card in.");
        return;
      }
      props.edit.clearError();
      setBusy(true);
      void connectGmail({ label: draft.label.trim(), projectId: draft.projectId })
        .then((saved) => {
          if (!saved) return;
          if (connected()) props.edit.close();
          else onSavedUngranted();
        })
        .finally(() => setBusy(false));
    },
    /** Opens Google's page for Gmail's own consent, then asks the row until the grant lands. */
    grant: (): void => {
      props.edit.clearError();
      setBusy(true);
      void (props.grant ?? (() => openGoogleConsent(() => M.authorizeGmail())))()
        .then((opened) => {
          if (opened) watcher.start();
          else props.edit.fail("Marshal has no Google sign-in yet. Connect Google Calendar first.");
        })
        .finally(() => setBusy(false));
    },
  };
}

export type GmailConnectController = ReturnType<typeof createGmailConnect>;
