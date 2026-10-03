import { createSignal } from "solid-js";
import { createStore } from "solid-js/store";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { connectDiscord } from "./integration-actions";

/** The fields and the save of the Discord dialog, shared by its tabs. */
export function createDiscordConnect(integration: Integration, edit: EditState) {
  const [draft, setDraft] = createStore({ token: "", channelId: "" });
  const [busy, setBusy] = createSignal(false);
  return {
    draft,
    set: (key: "token" | "channelId", value: string): void => setDraft(key, value),
    busy,
    error: () => edit.errorFor(integration.id),
    connected: () => integration.st !== "none",
    save: (): void => {
      if (!draft.token.trim()) {
        edit.fail("Marshal needs the Discord bot's token to reach it.");
        return;
      }
      if (!draft.channelId.trim()) {
        edit.fail("Enter the channel Marshal should send notices to.");
        return;
      }
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
