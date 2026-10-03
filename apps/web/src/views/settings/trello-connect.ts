import { createSignal } from "solid-js";
import { createStore } from "solid-js/store";
import { type Integration, M } from "~/mock";
import type { EditState } from "./edit-state";
import { connectTrello } from "./integration-actions";

/** What the two tabs of the Trello dialog fill in between them. */
export interface TrelloDraft {
  apiKey: string;
  token: string;
  projectId: string;
  boardId: string;
  newCardListId: string;
  webhookSecret: string;
  callbackUrl: string;
}

/** The fields and the save of the Trello dialog, shared by its tabs. */
export function createTrelloConnect(integration: Integration, edit: EditState) {
  const [draft, setDraft] = createStore<TrelloDraft>({
    apiKey: "",
    token: "",
    projectId: M.S.projects[0]?.id ?? "",
    boardId: "",
    newCardListId: "",
    webhookSecret: "",
    callbackUrl: "",
  });
  const [busy, setBusy] = createSignal(false);

  const refusal = (): string | null => {
    if (!draft.apiKey.trim() || !draft.token.trim()) {
      return "Marshal needs both a Trello API key and a token.";
    }
    if (!draft.projectId) return "Choose the Marshal project this Trello board is linked to.";
    if (!draft.boardId.trim()) return "Enter the id of the Trello board Marshal should watch.";
    if (!!draft.webhookSecret.trim() !== !!draft.callbackUrl.trim()) {
      return "Enter both the webhook secret and the callback URL, or leave both empty.";
    }
    return null;
  };

  return {
    draft,
    set: <K extends keyof TrelloDraft>(key: K, value: TrelloDraft[K]): void => setDraft(key, value),
    busy,
    error: () => edit.errorFor(integration.id),
    connected: () => integration.st !== "none",
    save: (): void => {
      const problem = refusal();
      if (problem) {
        edit.fail(problem);
        return;
      }
      edit.clearError();
      setBusy(true);
      void connectTrello({
        apiKey: draft.apiKey.trim(),
        token: draft.token.trim(),
        projectId: draft.projectId,
        boardId: draft.boardId.trim(),
        newCardListId: draft.newCardListId.trim(),
        webhookSecret: draft.webhookSecret.trim(),
        callbackUrl: draft.callbackUrl.trim(),
      })
        .then((saved) => {
          if (saved) edit.close();
        })
        .finally(() => setBusy(false));
    },
  };
}

export type TrelloConnectController = ReturnType<typeof createTrelloConnect>;
