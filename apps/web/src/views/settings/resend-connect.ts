import { createSignal } from "solid-js";
import { createStore } from "solid-js/store";
import type { ProviderTest } from "~/data/mappers/providers";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { connectResend, sendResendTest } from "./integration-actions";

/** Resend's own sender. It works on any account, but only reaches the account owner until a domain is verified. */
export const DEFAULT_RESEND_FROM = "Marshal <onboarding@resend.dev>";

/** What the Resend dialog fills in: the address mail comes from, and the API key. */
export interface ResendDraft {
  from: string;
  key: string;
}

/** The fields, the save, and the test email of the Resend dialog. */
export function createResendConnect(integration: Integration, edit: EditState) {
  const [draft, setDraft] = createStore<ResendDraft>({
    from: integration.target ?? DEFAULT_RESEND_FROM,
    key: "",
  });
  const [busy, setBusy] = createSignal(false);
  const [sending, setSending] = createSignal(false);
  const [sent, setSent] = createSignal<ProviderTest | null>(null);
  return {
    draft,
    set: <K extends keyof ResendDraft>(key: K, value: ResendDraft[K]): void => setDraft(key, value),
    busy,
    sending,
    sent,
    error: () => edit.errorFor(integration.id),
    connected: () => integration.st !== "none",
    /** Saves the sender and the key. A key left empty keeps the one already saved. */
    save: (): void => {
      // An empty sender means Resend's own, so only a sender that was typed has to be an address.
      const from = draft.from.trim() || DEFAULT_RESEND_FROM;
      if (!from.includes("@")) {
        edit.fail(
          "Enter the address Marshal should send email from, such as marshal@your-domain.com.",
        );
        return;
      }
      if (!draft.key.trim() && integration.st === "none") {
        edit.fail("Paste your Resend API key.");
        return;
      }
      edit.clearError();
      setBusy(true);
      void connectResend({ from, key: draft.key.trim() })
        .then((saved) => {
          if (saved) {
            setDraft("key", "");
            edit.close();
          }
        })
        .finally(() => setBusy(false));
    },
    /** Sends one test email to the owner's profile address. It is the only action that sends mail. */
    sendTest: (): void => {
      setSending(true);
      setSent(null);
      void sendResendTest()
        .then((result) => setSent(result))
        .finally(() => setSending(false));
    },
  };
}

export type ResendConnectController = ReturnType<typeof createResendConnect>;
