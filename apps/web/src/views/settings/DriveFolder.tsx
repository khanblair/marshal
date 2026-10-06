import { Button } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import { M } from "~/mock";
import { ErrorLine, ValueField } from "./connect-fields";

const DEFAULT_FOLDER = "Marshal";

/**
 * Google Drive's one setting: the folder Marshal puts the files it makes in, for all four services.
 * It can be saved before Google is connected, and a refusal shows the daemon's own sentence.
 */
export function DriveFolder(props: {
  /** The folder the daemon says is saved, once the files have been read. */
  saved?: string;
  /** Saves the folder. The store's own call by default; a test gives another. */
  save?: (folder: string) => Promise<{ saved: true } | { error: string }>;
}) {
  const [draft, setDraft] = createSignal<string | null>(null);
  const [busy, setBusy] = createSignal(false);
  const [problem, setProblem] = createSignal<string | null>(null);
  const [done, setDone] = createSignal(false);
  const folder = () => draft() ?? props.saved ?? DEFAULT_FOLDER;
  const submit = (event: SubmitEvent): void => {
    event.preventDefault();
    setBusy(true);
    setProblem(null);
    setDone(false);
    void (props.save ?? ((name) => M.saveGoogleDrive({ folder: name })))(folder().trim())
      .then((answer) => {
        if ("error" in answer) setProblem(answer.error);
        else setDone(true);
      })
      .finally(() => setBusy(false));
  };
  return (
    <form class="flex flex-col gap-2" onSubmit={submit}>
      <ValueField
        label="Folder"
        hint="Marshal saves every file it makes in this folder in your Drive. It makes the folder the first time."
        name="folder"
        mono={false}
        value={folder()}
        onValue={(value) => {
          setDraft(value);
          setDone(false);
        }}
        invalid={!!problem()}
      />
      <ErrorLine message={problem()} />
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" disabled={busy()}>
          {busy() ? "Saving…" : "Save folder"}
        </Button>
        <Show when={done()}>
          <span role="status" class="text-small text-secondary">
            Folder saved.
          </span>
        </Show>
      </div>
    </form>
  );
}
