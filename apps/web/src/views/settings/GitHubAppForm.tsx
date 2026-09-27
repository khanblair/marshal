import { Button, Field, Input, TextArea } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import type { Integration } from "~/mock";
import type { EditState } from "./edit-state";
import { fieldValue } from "./form-field";
import { connectGitHub, disconnectConnection, testConnection } from "./integration-actions";

/**
 * The GitHub App's whole setup (B6.1): the App's numeric id, the installation id, the private key,
 * and the webhook secret. They arrive together because no part of them is any use alone, and the
 * daemon refuses to store one without the others.
 *
 * The form's own checks are the design's kind of guard - plainly a number, plainly a key, plainly
 * not empty - so a typo is caught before a keychain write. Everything else is the daemon's: it
 * parses the key, stores it with the secret, tests the connection, and shows its own sentence when
 * it refuses. The form closes only once the daemon has stored the connection.
 */
export function GitHubAppForm(props: { integration: Integration; edit: EditState }) {
  const error = () => props.edit.errorFor(props.integration.id);
  const connected = () => props.integration.st !== "none";
  const [busy, setBusy] = createSignal(false);
  const name = () => props.integration.name;

  const submit = (form: HTMLFormElement): void => {
    const appId = fieldValue(form, "appId").trim();
    const installationId = fieldValue(form, "installationId").trim();
    // The key is a file the person pasted, so it goes up exactly as it is: the daemon parses the
    // PEM, whose own line breaks and trailing newline are part of it. The ids and the secret are
    // single-line values, so whitespace around them is a typo and is trimmed.
    const privateKey = fieldValue(form, "privateKey");
    const webhookSecret = fieldValue(form, "webhookSecret").trim();
    if (!isId(appId) || !isId(installationId)) {
      props.edit.fail(
        "The App id and the installation id are both numbers. Copy each one from the App's page on GitHub.",
      );
      return;
    }
    if (!privateKey.includes("PRIVATE KEY")) {
      props.edit.fail(
        "Paste the App's private key exactly as GitHub generated it, BEGIN and END lines included.",
      );
      return;
    }
    if (!webhookSecret) {
      props.edit.fail("The webhook secret is the one you set on the App. Copy it from there.");
      return;
    }
    setBusy(true);
    void connectGitHub({
      appId: Number(appId),
      installationId: Number(installationId),
      privateKey,
      webhookSecret,
    })
      .then((saved) => {
        if (saved) props.edit.close();
      })
      .finally(() => setBusy(false));
  };

  const runTest = (): void => {
    setBusy(true);
    void testConnection(props.integration.id, name()).finally(() => setBusy(false));
  };

  return (
    <form
      class="flex flex-col gap-2"
      onSubmit={(event) => {
        event.preventDefault();
        submit(event.currentTarget);
      }}
    >
      <Field label="App ID" hint="The numeric id at the top of the App's page on GitHub.">
        <Input mono name="appId" inputmode="numeric" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="Installation ID" hint="The number in the install URL, after /installations/.">
        <Input
          mono
          name="installationId"
          inputmode="numeric"
          autocomplete="off"
          invalid={!!error()}
        />
      </Field>
      <Field label="Private key" hint="Download the key from the App and paste the whole file.">
        <TextArea rows={4} name="privateKey" autocomplete="off" invalid={!!error()} />
      </Field>
      <Field label="Webhook secret" hint="The secret you set for the App's webhook.">
        <Input type="password" name="webhookSecret" autocomplete="off" invalid={!!error()} />
      </Field>
      <Show when={error()}>
        <span class="text-small leading-4.5 text-status-danger-text">{error()}</span>
      </Show>
      <div class="flex flex-wrap gap-2">
        <Button variant="primary" type="submit" disabled={busy()}>
          {busy() ? "Saving…" : "Save connection"}
        </Button>
        <Show when={connected()}>
          <Button disabled={busy()} onClick={runTest}>
            Test connection
          </Button>
          <Button
            variant="destructive"
            disabled={busy()}
            onClick={() => disconnectConnection(props.integration.id, name())}
          >
            Disconnect
          </Button>
        </Show>
        <Button onClick={() => props.edit.close()}>Cancel</Button>
      </div>
    </form>
  );
}

/** True for a plain positive integer, which is what both of GitHub's ids are. */
function isId(text: string): boolean {
  return /^\d+$/.test(text) && Number(text) > 0;
}
