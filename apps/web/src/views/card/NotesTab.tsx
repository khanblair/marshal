import { Button, Icon, TextArea } from "@marshal/ui";
import { Show } from "solid-js";
import type { Card } from "~/mock";
import { notePath, noteText, saveNote } from "./card-note";
import type { Panel } from "./panel-state";

export interface NotesTabProps {
  card: Card;
  panel: Panel;
}

/** The Notes tab: the card's note in the Obsidian vault, with an editor. */
export function NotesTab(props: NotesTabProps) {
  const text = () => noteText(props.card);
  const editing = () => props.panel.state.noteEdit;
  return (
    <div class="flex-1 min-h-0 overflow-auto py-3 px-4 flex flex-col gap-2.5">
      <div class="flex items-center gap-2">
        <Icon name="book-open" size={14} />
        <span class="flex-1 min-w-0 font-mono text-caption text-secondary overflow-hidden text-ellipsis whitespace-nowrap">
          {notePath(props.card)}
        </span>
        <Show when={!editing()}>
          <Button
            size={28}
            class="text-small"
            onClick={() => props.panel.set({ noteEdit: true, noteDraft: text() })}
          >
            Edit note
          </Button>
        </Show>
      </div>
      <Show
        when={editing()}
        fallback={<div class="max-w-[72ch] whitespace-pre-wrap text-pretty">{text()}</div>}
      >
        <TextArea
          mono
          rows={12}
          aria-label="Card note"
          value={props.panel.state.noteDraft}
          onInput={(e) => props.panel.set({ noteDraft: e.currentTarget.value })}
        />
        <span class="text-small text-secondary">
          Saved to the Obsidian vault. Agents read this note at the start of each turn.
        </span>
        <div class="flex gap-2">
          <Button
            variant="primary"
            disabled={props.panel.state.noteDraft === text()}
            onClick={() => {
              saveNote(props.card, props.panel.state.noteDraft);
              props.panel.set({ noteEdit: false });
            }}
          >
            Save note
          </Button>
          <Button onClick={() => props.panel.set({ noteEdit: false })}>Cancel</Button>
        </div>
      </Show>
    </div>
  );
}
