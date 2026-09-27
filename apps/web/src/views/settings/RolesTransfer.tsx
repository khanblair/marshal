import { Button, Dialog, Field, TextArea } from "@marshal/ui";
import { createMemo, createSignal, Show } from "solid-js";
import { toExports } from "~/data/mappers/roles";
import { M } from "~/mock";
import { importRoleExport } from "./role-actions";

/** Which of the two transfer dialogs the Roles section has open. Null means neither. */
export type TransferMode = "export" | "import";

/** The indentation the export is written with: two spaces, the same as the wire's golden files. */
const JSON_INDENT = 2;

/** How tall the JSON box is, in text lines. */
const BOX_ROWS = 14;

/**
 * Export: every role as one JSON document, ready to be copied or saved. It is the same shape an
 * import reads, so a role moves between machines without a second format to keep in step; the id and
 * the two flags are left out because they are the daemon's to set.
 */
export function RoleExportDialog(props: { onClose: () => void }) {
  const text = createMemo(() => JSON.stringify(toExports(M.S.roles), null, JSON_INDENT));
  const count = createMemo(() => M.S.roles.length);
  const copy = (): void => {
    // A browser without the clipboard API can still select the text and copy it by hand.
    void navigator.clipboard?.writeText(text()).then(
      () => M.toast("Roles copied"),
      () => M.toast("Copy the text in the box instead."),
    );
  };
  return (
    <Dialog
      width={560}
      phone={M.mobile}
      aria-labelledby="roles-export-title"
      onClose={props.onClose}
    >
      <h2 id="roles-export-title" class="m-0 text-title leading-6 font-semibold">
        Export roles
      </h2>
      <p class="m-0 text-secondary">
        <Show when={count() === 1} fallback={`Every role, as JSON: ${count()} of them.`}>
          The only role, as JSON.
        </Show>{" "}
        Paste it back into Import on this or another machine.
      </p>
      <Field label="Roles">
        <TextArea
          mono
          readOnly
          rows={BOX_ROWS}
          data-autofocus
          value={text()}
          onFocus={(event) => event.currentTarget.select()}
        />
      </Field>
      <div class="flex justify-end flex-wrap gap-2 mt-1">
        <Button icon="copy" onClick={copy}>
          Copy
        </Button>
        <Button variant="primary" class="hover:bg-ink!" onClick={props.onClose}>
          Close
        </Button>
      </div>
    </Dialog>
  );
}

/**
 * Import: paste what Export wrote - one role or several - and each is added. A name that is already
 * taken is refused rather than replacing the role that is there, so a mistake never costs an edit.
 */
export function RoleImportDialog(props: { onClose: () => void }) {
  const [text, setText] = createSignal("");
  const [error, setError] = createSignal("");
  const [busy, setBusy] = createSignal(false);
  const run = async (): Promise<void> => {
    const pasted = text().trim();
    if (!pasted) {
      setError("Paste what Export gave you first.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const added = await importRoleExport(pasted);
      // Zero added means the daemon refused the first one; its own sentence is already shown, and
      // the dialog stays open so the pasted text can be fixed.
      if (added === 0) return;
      M.toast(added === 1 ? "Imported 1 role" : `Imported ${added} roles`);
      props.onClose();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "That export could not be read.");
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      width={560}
      phone={M.mobile}
      aria-labelledby="roles-import-title"
      onClose={props.onClose}
    >
      <h2 id="roles-import-title" class="m-0 text-title leading-6 font-semibold">
        Import roles
      </h2>
      <p class="m-0 text-secondary">
        Paste one role or several, as Export writes them. A role whose name is taken is left alone.
      </p>
      <Field label="Roles" error={error() || undefined}>
        <TextArea
          mono
          rows={BOX_ROWS}
          data-autofocus
          invalid={!!error()}
          placeholder='{"name": "Nightly janitor", "spec": { … }}'
          value={text()}
          onInput={(event) => {
            setText(event.currentTarget.value);
            if (error()) setError("");
          }}
        />
      </Field>
      <div class="flex justify-end flex-wrap gap-2 mt-1">
        <Button onClick={props.onClose}>Cancel</Button>
        <Button
          variant="primary"
          class="hover:bg-ink!"
          disabled={busy()}
          onClick={() => void run()}
        >
          Import
        </Button>
      </div>
    </Dialog>
  );
}
