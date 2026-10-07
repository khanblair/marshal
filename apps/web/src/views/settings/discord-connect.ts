import { createSignal } from "solid-js";
import { createStore } from "solid-js/store";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { connectDiscord } from "./integration-actions";

type Field = "token" | "channelId";

/**
 * The fields and the save of the Discord dialog, shared by its tabs. A field that is empty opens
 * the tab it is on (`show`), so the refusal is read beside the field it is about.
 */
export function createDiscordConnect(
  integration: Integration,
  edit: EditState,
  show: (tab: "bot" | "channel") => void,
) {
  const [draft, setDraft] = createStore({ token: "", channelId: integration.target ?? "" });
  const [busy, setBusy] = createSignal(false);
  const [empty, setEmpty] = createSignal<Field | null>(null);
  const refuse = (field: Field, message: string): void => {
    setEmpty(field);
    show(field === "token" ? "bot" : "channel");
    edit.fail(message);
  };
  return {
    draft,
    set: (key: Field, value: string): void => setDraft(key, value),
    busy,
    error: () => edit.errorFor(integration.id),
    /** Whether a field is the one the refusal is about: any field, for a refusal the daemon made. */
    invalid: (field: Field): boolean =>
      !!edit.errorFor(integration.id) && (!empty() || empty() === field),
    connected: () => integration.st !== "none",
    save: (): void => {
      // A token left empty keeps the one already saved, so only a new connection needs one.
      if (!draft.token.trim() && integration.st === "none") {
        refuse("token", "Marshal needs the Discord bot's token to reach it.");
        return;
      }
      if (!draft.channelId.trim()) {
        refuse("channelId", "Enter the channel Marshal should send notices to.");
        return;
      }
      setEmpty(null);
      edit.clearError();
      setBusy(true);
      void connectDiscord({ token: draft.token.trim(), channelId: draft.channelId.trim() })
        .then((saved) => {
          if (saved) edit.close();
        })
        .finally(() => setBusy(false));
    },
  };
}

export type DiscordConnectController = ReturnType<typeof createDiscordConnect>;
