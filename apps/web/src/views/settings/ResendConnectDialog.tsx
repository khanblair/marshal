import { Button, Dialog } from "@marshal/ui";
import { Show } from "solid-js";
import { type Integration, M } from "~/mock";
import { ConnectionStored } from "./ConnectionStored";
import { ErrorLine, SaveRow, ValueField } from "./connect-fields";
import type { EditState } from "./edit-state";
import { createResendConnect, DEFAULT_RESEND_FROM } from "./resend-connect";
import { TestChecks } from "./TestChecks";

/**
 * Connecting email through Resend, as a dialog over Settings. The sender address and the API key are
 * saved; Marshal then sends briefs to the address in the owner's profile. Saving sends nothing: the
 * only send is the Send test email button, which is shown once the connection is saved.
 */
export function ResendConnectDialog(props: { integration: Integration; edit: EditState }) {
  const resend = createResendConnect(props.integration, props.edit);
  return (
    <Dialog
      width={560}
      phone={M.mobile}
      aria-labelledby="resend-title"
      onClose={() => props.edit.close()}
    >
      <h2 id="resend-title" class="m-0 text-title leading-6 font-semibold">
        Connect email
      </h2>
      <form
        class="flex flex-col gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          resend.save();
        }}
      >
        <Show when={resend.connected()}>
          <ConnectionStored integration={props.integration} onDisconnected={() => undefined} />
        </Show>
        <span class="text-small leading-4.5 text-secondary">
          Marshal sends briefs by email through Resend, to the address in your profile. Set your
          address in Onboarding or under Settings, then Profile.
        </span>
        <ValueField
          label="Sender address"
          hint="Resend's own address works on any account, but it only reaches the account owner. To email anyone else, verify a domain on resend.com/domains and send from an address on it."
          name="from"
          placeholder={DEFAULT_RESEND_FROM}
          value={resend.draft.from}
          onValue={(value) => resend.set("from", value)}
          invalid={!!resend.error()}
        />
        <ValueField
          label="API key"
          hint="From resend.com/api-keys. Leave it empty to keep the key already saved."
          name="key"
          secret
          value={resend.draft.key}
          onValue={(value) => resend.set("key", value)}
          invalid={!!resend.error()}
        />
        <ErrorLine message={resend.error()} />
        <SaveRow busy={resend.busy()} />
      </form>
      <Show when={resend.connected()}>
        <div class="flex flex-col gap-2">
          <span class="font-medium">Test email</span>
          <span class="text-small leading-4.5 text-secondary">
            Sends one email to the address in your profile.
          </span>
          <Button class="self-start" disabled={resend.sending()} onClick={() => resend.sendTest()}>
            {resend.sending() ? "Sending…" : "Send test email"}
          </Button>
          <Show when={resend.sent()}>{(result) => <TestChecks test={result()} />}</Show>
        </div>
      </Show>
      <div class="flex justify-end gap-2">
        <Button onClick={() => props.edit.close()}>Close</Button>
      </div>
    </Dialog>
  );
}
