import { Button, Input } from "@marshal/ui";
import { Index, onMount, Show } from "solid-js";
import { type Card, M } from "~/mock";
import { ChecklistSection } from "./ChecklistSection";
import { ChecksList } from "./ChecksList";
import type { Panel } from "./panel-state";

export interface ChecklistsTabProps {
  card: Card;
  panel: Panel;
}

function NewChecklistForm(props: ChecklistsTabProps) {
  let input: HTMLInputElement | undefined;
  onMount(() => setTimeout(() => input?.focus(), 0));
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        M.addChecklist(props.card.id, input?.value);
        props.panel.set({ newCl: false });
      }}
      class="flex flex-wrap gap-1.5"
    >
      <Input
        ref={input}
        name="title"
        placeholder="Checklist name, for example Definition of done"
        aria-label="Checklist name"
        class="flex-[1_1_220px]"
      />
      <Button variant="primary" type="submit">
        Add checklist
      </Button>
      <Button class="text-small" onClick={() => props.panel.set({ newCl: false })}>
        Cancel
      </Button>
    </form>
  );
}

/** The Checklists tab: the card's checklists, a way to add one, and the acceptance checks. */
export function ChecklistsTab(props: ChecklistsTabProps) {
  return (
    <div class="flex-1 min-h-0 overflow-auto py-3 px-4 flex flex-col gap-2.5">
      <Index each={props.card.checklists}>
        {(list) => <ChecklistSection card={props.card} list={list()} panel={props.panel} />}
      </Index>
      <Show
        when={props.panel.state.newCl}
        fallback={
          <Button
            icon="plus"
            iconSize={14}
            class="self-start text-small"
            onClick={() => props.panel.set({ newCl: true })}
          >
            Add checklist
          </Button>
        }
      >
        <NewChecklistForm {...props} />
      </Show>
      <ChecksList cardId={props.card.id} />
    </div>
  );
}
