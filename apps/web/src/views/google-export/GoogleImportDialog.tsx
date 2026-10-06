import type { GoogleLinkContent } from "@marshal/protocol";
import { Badge, Button, Callout, Dialog, Field, Input } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import { Portal } from "solid-js/web";
import { Markdown } from "~/features/markdown/Markdown";
import { type Card, M } from "~/mock";
import { noteText, saveNote } from "~/views/card/card-note";
import type { Panel } from "~/views/card/panel-state";
import { ErrorLine } from "~/views/settings/connect-fields";
import { connectionProblem } from "./export-run";
import { KIND_NAMES, linkService, noteWithImport } from "./google-import";
import { noteState } from "./note-state";

export interface GoogleImportDialogProps {
  card: Card;
  panel: Panel;
}

const TRUNCATED = "This file is long. Only the start was read.";

/** What the dialog knows: the link, the sentence the daemon refused it with, and what it read. */
function createImport(props: GoogleImportDialogProps) {
  const [link, setLink] = createSignal("");
  const [reading, setReading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [content, setContent] = createSignal<GoogleLinkContent | null>(null);

  const change = (value: string): void => {
    setLink(value);
    setError(null);
    setContent(null);
  };

  async function read(): Promise<void> {
    const url = link().trim();
    if (reading() || !url) return;
    const service = linkService(url);
    const problem = service ? connectionProblem(service) : null;
    if (problem) {
      M.toast(problem);
      return;
    }
    setReading(true);
    setError(null);
    setContent(null);
    const answer = await M.readGoogleLink({ url });
    setReading(false);
    // The field changed while the file was read, so the answer is for a link no longer there.
    if (link().trim() !== url) return;
    if ("error" in answer) setError(answer.error);
    else setContent(answer.content);
  }

  /** While the note is still being read, adding would save over a note this page has not seen. */
  const noteReady = (): boolean => noteState(props.card) !== "loading";

  function addToNote(): void {
    const found = content();
    if (!found || !noteReady()) return;
    const { state } = props.panel;
    const base = state.noteEdit ? state.noteDraft : noteText(props.card);
    saveNote(props.card, noteWithImport(base, found));
    props.panel.set({ noteEdit: false, importOpen: false });
  }

  return { link, change, reading, error, content, read, noteReady, addToNote };
}

/** What was read: its name and kind, a notice when it was cut short, and the markdown to be added. */
function Preview(props: { content: GoogleLinkContent }) {
  return (
    <div class="flex flex-col gap-2">
      <div class="flex items-center gap-2">
        <span class="min-w-0 flex-1 truncate font-semibold">{props.content.title}</span>
        <Badge size={22}>Google {KIND_NAMES[props.content.kind]}</Badge>
      </div>
      <Show when={props.content.truncated}>
        <Callout class="items-center">{TRUNCATED}</Callout>
      </Show>
      <section
        aria-label="What was read"
        tabindex="0"
        class="max-h-64 overflow-auto rounded-sm border border-border bg-surface-sunken p-3"
      >
        <Markdown text={props.content.markdown} />
      </section>
    </div>
  );
}

function ImportForm(props: GoogleImportDialogProps) {
  const form = createImport(props);
  const close = (): void => props.panel.set({ importOpen: false });
  return (
    <Dialog
      width={560}
      phone={M.mobile}
      aria-labelledby="gi-title"
      onClose={close}
      onSubmit={(event) => {
        event.preventDefault();
        void form.read();
      }}
    >
      <h2 id="gi-title" class="m-0 text-title leading-6 font-semibold">
        Import from Google link
      </h2>
      <div class="flex items-end gap-2">
        <Field label="Google link" class="flex-1 min-w-0">
          <Input
            data-autofocus
            mono
            autocomplete="off"
            spellcheck={false}
            placeholder="https://docs.google.com/document/d/…"
            value={form.link()}
            invalid={!!form.error()}
            onInput={(event) => form.change(event.currentTarget.value)}
          />
        </Field>
        <Button type="submit" disabled={form.reading() || !form.link().trim()}>
          {form.reading() ? "Reading…" : "Read"}
        </Button>
      </div>
      <ErrorLine message={form.error()} />
      <Show when={form.content()}>{(content) => <Preview content={content()} />}</Show>
      <div class="flex justify-end gap-2">
        <Button onClick={close}>Cancel</Button>
        <Button
          variant="primary"
          disabled={!form.content() || !form.noteReady()}
          onClick={form.addToNote}
        >
          Add to card note
        </Button>
      </div>
    </Dialog>
  );
}

/**
 * Reads a Google Doc, Sheet or Slides presentation from a pasted link, shows what it found, and adds
 * it to the end of the card's note. It is drawn above everything else, wherever the card panel sits.
 */
export function GoogleImportDialog(props: GoogleImportDialogProps) {
  return (
    <Show when={props.panel.state.importOpen}>
      <Portal>
        <ImportForm card={props.card} panel={props.panel} />
      </Portal>
    </Show>
  );
}
