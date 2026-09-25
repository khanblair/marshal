import { Button, Checkbox, Icon, IconButton, Input, ProgressBar } from "@marshal/ui";
import { Index, onMount, Show } from "solid-js";
import { type Card, type Checklist, M } from "~/mock";
import { percentDone } from "./checks-model";
import type { Panel } from "./panel-state";

export interface ChecklistSectionProps {
  card: Card;
  list: Checklist;
  panel: Panel;
}

const COMPLETE_PERCENT = 100;

/** Who checked an item, and when: "Ada completed this 5 min ago". */
function completedBy(card: Card, by: string, doneAt: number | undefined): string {
  const who = by === "agent" ? card.agent : (M.person(by)?.name ?? "You");
  return `${who} completed this ${M.rel(doneAt || Date.now()).toLowerCase()}`;
}

function ItemRow(props: { card: Card; list: Checklist; item: Checklist["items"][number] }) {
  return (
    <li class="flex items-start gap-2.5 py-1.5 px-1 rounded-sm hover:bg-surface-hover">
      <Checkbox
        align="start"
        checked={props.item.done}
        onChange={() => M.toggleItem(props.card.id, props.list.id, props.item.id)}
        aria-label={props.item.text}
      />
      <span class="flex-1 min-w-0 flex flex-col gap-px">
        <span class={props.item.done ? "line-through text-secondary" : "no-underline text-primary"}>
          {props.item.text}
        </span>
        <Show when={props.item.done && props.item.by}>
          {(by) => (
            <span class="inline-flex items-center gap-1 text-caption leading-4 text-secondary">
              <Icon name={by() === "agent" ? "bot" : "user-round"} size={12} />
              {completedBy(props.card, by(), props.item.doneAt)}
            </span>
          )}
        </Show>
      </span>
      <IconButton
        label={`Remove ${props.item.text}`}
        icon="x"
        iconSize={14}
        tone="muted"
        class="hover:text-muted!"
        onClick={() => M.removeItem(props.card.id, props.list.id, props.item.id)}
      />
    </li>
  );
}

function AddItemForm(props: ChecklistSectionProps) {
  let input: HTMLInputElement | undefined;
  onMount(() => setTimeout(() => input?.focus(), 0));
  const close = () => props.panel.set({ addingIn: null });
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        M.addItem(props.card.id, props.list.id, input?.value ?? "");
        if (input) {
          input.value = "";
          input.focus();
        }
      }}
      class="flex flex-wrap gap-1.5"
    >
      <Input
        ref={input}
        name="item"
        placeholder="Add an item"
        aria-label="New checklist item"
        class="flex-[1_1_200px]"
        onKeyDown={(e) => {
          if (e.key !== "Escape") return;
          e.preventDefault();
          e.stopPropagation();
          close();
        }}
      />
      <Button variant="primary" type="submit">
        Add item
      </Button>
      <Button class="text-small" onClick={close}>
        Cancel
      </Button>
    </form>
  );
}

/** One checklist: title, progress, its items, and the Add an item form. */
export function ChecklistSection(props: ChecklistSectionProps) {
  const done = () => props.list.items.filter((item) => item.done).length;
  const percent = () => percentDone(done(), props.list.items.length);
  const shown = () =>
    props.list.hideDone ? props.list.items.filter((item) => !item.done) : props.list.items;
  const adding = () => props.panel.state.addingIn === props.list.id;
  return (
    <section class="flex flex-col gap-1.5 pb-3 border-b border-border">
      <div class="flex flex-wrap items-center gap-2">
        <Icon name="square-check" size={16} />
        <h3 class="m-0 flex-1 text-subtitle leading-5.5 font-semibold">{props.list.title}</h3>
        <Show when={done() > 0}>
          <Button class="text-small" onClick={() => M.toggleHideDone(props.card.id, props.list.id)}>
            {props.list.hideDone ? "Show checked items" : "Hide checked items"}
          </Button>
        </Show>
        <Button class="text-small" onClick={() => M.deleteChecklist(props.card.id, props.list.id)}>
          Delete
        </Button>
      </div>
      <div class="flex items-center gap-2">
        <span class="w-9 text-caption text-secondary tabular-nums">{percent()}%</span>
        <ProgressBar
          value={percent()}
          label={`${props.list.title} progress`}
          tone={percent() === COMPLETE_PERCENT ? "working" : "neutral"}
          class="flex-1"
        />
      </div>
      <Show when={props.list.hideDone && done() > 0}>
        <span class="text-small text-secondary">
          {done()}
          {done() === 1 ? " checked item hidden" : " checked items hidden"}
        </span>
      </Show>
      <ul class="m-0 p-0 list-none flex flex-col">
        <Index each={shown()}>
          {(item) => <ItemRow card={props.card} list={props.list} item={item()} />}
        </Index>
      </ul>
      <Show
        when={adding()}
        fallback={
          <Button
            class="self-start text-small"
            onClick={() => props.panel.set({ addingIn: props.list.id })}
          >
            Add an item
          </Button>
        }
      >
        <AddItemForm {...props} />
      </Show>
    </section>
  );
}
