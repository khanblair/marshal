import { type Accessor, createSignal } from "solid-js";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { connectTelegram, detectTelegramChat } from "./integration-actions";

/** What both tabs of the Telegram dialog read and do, so they show the one connection. */
export interface TelegramConnectController {
  token: Accessor<string>;
  setToken: (value: string) => void;
  chat: Accessor<string>;
  setChat: (value: string) => void;
  busy: Accessor<boolean>;
  error: () => string | null;
  /** The sentence after Find my chat: what was found, or what to do next. */
  found: Accessor<string>;
  /** A bot is saved, so there is a connection to test and to disconnect. */
  connected: () => boolean;
  /** Saves the token and the chat, and closes the dialog once the daemon has tested them. */
  save: () => void;
  /** Looks for the chat that last wrote to the bot, and fills it in. Nothing is saved. */
  find: () => void;
}

/** The fields and the two actions of the Telegram dialog, shared by its tabs. */
export function createTelegramConnect(
  integration: Integration,
  edit: EditState,
): TelegramConnectController {
  const [token, setToken] = createSignal("");
  const [chat, setChat] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const [found, setFound] = createSignal("");

  const save = (): void => {
    if (!token().trim()) {
      edit.fail("Marshal needs the Telegram bot's token to reach it.");
      return;
    }
    if (!chat().trim()) {
      edit.fail("Enter the chat Marshal should send notices to.");
      return;
    }
    edit.clearError();
    setBusy(true);
    void connectTelegram({ token: token().trim(), chatId: chat().trim() })
      .then((saved) => {
        if (saved) edit.close();
      })
      .finally(() => setBusy(false));
  };

  const find = (): void => {
    if (!token().trim()) {
      edit.fail("Paste the bot's token first, then Marshal can look for your chat.");
      return;
    }
    edit.clearError();
    setBusy(true);
    setFound("");
    void detectTelegramChat(token().trim())
      .then((answer) => {
        if ("error" in answer) {
          edit.fail(answer.error);
          return;
        }
        if (!answer.found) {
          setFound(answer.message);
          return;
        }
        setChat(answer.chatId);
        setFound(
          `Found ${answer.name || "a chat"} (${answer.kind}). Save the connection to use it.`,
        );
      })
      .finally(() => setBusy(false));
  };

  return {
    token,
    setToken,
    chat,
    setChat,
    busy,
    error: () => edit.errorFor(integration.id),
    found,
    connected: () => integration.st !== "none",
    save,
    find,
  };
}
