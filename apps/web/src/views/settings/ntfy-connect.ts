import { createSignal } from "solid-js";
import { createStore } from "solid-js/store";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { connectNtfy } from "./integration-actions";

const TOPIC_LENGTH = 12;
/** Letters and digits: the base a number is written in to give both. */
const ALPHANUMERIC = 36;

/** A topic nobody else will guess: "marshal-" and twelve random letters and digits. */
export function randomTopic(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(TOPIC_LENGTH));
  return `marshal-${Array.from(bytes, (byte) => (byte % ALPHANUMERIC).toString(ALPHANUMERIC)).join("")}`;
}

/** Which of the two tabs is saving: ntfy's public server, or a server of the person's own. */
export type NtfyWay = "public" | "own";

/** The fields and the save of the ntfy dialog, shared by its tabs. */
export function createNtfyConnect(integration: Integration, edit: EditState) {
  const [draft, setDraft] = createStore({ topic: "", server: "", token: "" });
  const [busy, setBusy] = createSignal(false);
  return {
    draft,
    set: (key: "topic" | "server" | "token", value: string): void => setDraft(key, value),
    busy,
    error: () => edit.errorFor(integration.id),
    connected: () => integration.st !== "none",
    /** Saving from the first tab uses ntfy's own server and no token, whatever the other tab holds. */
    save: (way: NtfyWay): void => {
      if (!draft.topic.trim()) {
        edit.fail("Enter the ntfy topic Marshal should send alerts to.");
        return;
      }
      if (way === "own" && !draft.server.trim()) {
        edit.fail("Enter your server's address, or use the ntfy.sh tab.");
        return;
      }
      edit.clearError();
      setBusy(true);
      void connectNtfy({
        topic: draft.topic.trim(),
        server: way === "own" ? draft.server.trim() : "",
        token: way === "own" ? draft.token.trim() : "",
      })
        .then((saved) => {
          if (saved) edit.close();
        })
        .finally(() => setBusy(false));
    },
  };
}

export type NtfyConnectController = ReturnType<typeof createNtfyConnect>;
