import { Button, Checkbox, cx, Icon, IconLabel } from "@marshal/ui";
import { createSignal, createUniqueId, For, Show } from "solid-js";
import type { TrelloCardGroup, TrelloChangeView } from "~/data/mappers/trello";

/** A detail longer than this is cut to a few lines, with a button to read the rest. */
const LONG_DETAIL_CHARS = 160;
const ICON_PX = 14;
const AUTHOR_ICON_PX = 12;

/** What the rows ask of the dialog: the choices it holds and the things it does for a conflict. */
export interface TrelloRowActions {
  isPicked: (id: string) => boolean;
  isOverridden: (id: string) => boolean;
  toggle: (change: TrelloChangeView) => void;
  /** "Apply anyway": ticks the change together with its override, or unticks both. */
  setOverride: (change: TrelloChangeView, on: boolean) => void;
  skip: (change: TrelloChangeView) => void;
  keepTrello: (change: TrelloChangeView) => void;
  busy: () => boolean;
  /** A sentence under a conflict that the dialog has something to say about. */
  note: (id: string) => string;
  /** The card's key in the store, such as "web#118", or empty when the store has no such card. */
  keyOf: (cardId: string) => string;
}

/** The comment or description that would be written, cut to a few lines until the person opens it. */
function Detail(props: { text: string }) {
  const [open, setOpen] = createSignal(false);
  const id = createUniqueId();
  const long = () => props.text.length > LONG_DETAIL_CHARS;
  return (
    <div class="flex flex-col items-start gap-1 pl-7">
      <p
        id={id}
        class={cx(
          "m-0 w-full min-w-0 whitespace-pre-wrap break-words text-small leading-4.5 text-secondary",
          !open() && "line-clamp-3",
        )}
      >
        {props.text}
      </p>
      <Show when={long()}>
        <button
          type="button"
          aria-expanded={open()}
          aria-controls={id}
          class="border-none bg-transparent p-0 text-small text-secondary underline"
          onClick={() => setOpen(!open())}
        >
          {open() ? "Show less" : "Show more"}
        </button>
      </Show>
    </div>
  );
}

/** What Trello changed, and the three ways out of it. */
function ConflictChoices(props: { change: TrelloChangeView; actions: TrelloRowActions }) {
  const noteId = createUniqueId();
  const anyway = () => props.actions.isOverridden(props.change.id);
  return (
    <fieldset class="m-0 ml-7 flex min-w-0 flex-col gap-2 rounded-md border-none bg-status-needs-you-subtle p-3 text-small leading-4.5 text-status-needs-you-text">
      <legend class="sr-only">Trello changed this: {props.change.summary}</legend>
      <span class="flex items-start gap-1.5 font-medium">
        <Icon name="triangle-alert" size={ICON_PX} class="mt-0.5 flex-none" />
        <span>{props.change.conflictNote}</span>
      </span>
      <div class="flex flex-wrap gap-2">
        <Button
          size={28}
          aria-pressed={anyway()}
          disabled={props.actions.busy()}
          onClick={() => props.actions.setOverride(props.change, !anyway())}
        >
          Apply anyway
        </Button>
        <Button
          size={28}
          disabled={props.actions.busy()}
          onClick={() => props.actions.skip(props.change)}
        >
          Skip
        </Button>
        <Button
          size={28}
          aria-describedby={noteId}
          disabled={props.actions.busy()}
          onClick={() => props.actions.keepTrello(props.change)}
        >
          Keep Trello's version
        </Button>
      </div>
      <span id={noteId}>
        Apply anyway writes this change over Trello's. Skip hides it. Keep Trello's version sets
        this card's title, description, and ticks back to what Trello holds.
      </span>
      <Show when={props.actions.note(props.change.id)}>
        {(text) => <span role="status">{text()}</span>}
      </Show>
    </fieldset>
  );
}

/** One pending change: a box, what it does, who made it in Marshal, and what would be written. */
function ChangeRow(props: { change: TrelloChangeView; actions: TrelloRowActions }) {
  const id = createUniqueId();
  return (
    <li class="flex flex-col gap-1.5 py-2.5" data-change={props.change.id}>
      <label for={id} class="flex min-w-0 cursor-pointer items-start gap-2.5">
        <Checkbox
          id={id}
          align="start"
          checked={props.actions.isPicked(props.change.id)}
          disabled={props.actions.busy()}
          onChange={() => props.actions.toggle(props.change)}
        />
        <span class="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2">
          <span class="font-medium">{props.change.summary}</span>
          <Show when={props.change.byLabel}>
            <IconLabel
              icon={props.change.by === "agent" ? "bot" : "user-round"}
              size={AUTHOR_ICON_PX}
              class="text-caption text-secondary"
            >
              {props.change.byLabel}
            </IconLabel>
          </Show>
        </span>
      </label>
      <Show when={props.change.detail}>
        <Detail text={props.change.detail} />
      </Show>
      <Show when={props.change.conflict}>
        <ConflictChoices change={props.change} actions={props.actions} />
      </Show>
    </li>
  );
}

/** The changes of one Marshal card, under its key and title. */
function CardGroup(props: { group: TrelloCardGroup; actions: TrelloRowActions }) {
  const titleId = createUniqueId();
  const key = () => props.actions.keyOf(props.group.cardId);
  return (
    <section aria-labelledby={titleId} class="flex flex-col">
      <h3 id={titleId} class="m-0 flex min-w-0 items-baseline gap-2 text-body-strong">
        <Show when={key()}>
          <span class="flex-none font-mono text-caption font-normal text-secondary">{key()}</span>
        </Show>
        <span class="min-w-0 truncate">{props.group.cardTitle}</span>
      </h3>
      <ul class="m-0 flex list-none flex-col divide-y divide-border p-0">
        <For each={props.group.changes}>
          {(change) => <ChangeRow change={change} actions={props.actions} />}
        </For>
      </ul>
    </section>
  );
}

/** Every pending change, grouped by the card it belongs to. */
export function TrelloChangeList(props: {
  groups: readonly TrelloCardGroup[];
  actions: TrelloRowActions;
}) {
  return (
    <div class="flex min-h-0 flex-col gap-4">
      <For each={props.groups}>
        {(group) => <CardGroup group={group} actions={props.actions} />}
      </For>
    </div>
  );
}
