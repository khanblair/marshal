import { Badge, Button, Icon } from "@marshal/ui";
import { createSignal, createUniqueId, For, type JSX, Show } from "solid-js";
import type { MergeQueueCard } from "~/data/mappers/integration";
import { type Card, M } from "~/mock";
import { retryMerge } from "~/sync/integration-flow";

const CARD = "flex flex-col gap-1.5 p-3 bg-surface border border-border border-l-3 rounded-md";

/** The card's name: a button that opens the card when the store has it, and plain text when not. */
function CardName(props: { cardKey: string; title: string; card: Card | undefined }) {
  return (
    <div class="flex items-start gap-2">
      <Show
        when={props.card}
        fallback={<span class="flex-1 min-w-0 font-semibold line-clamp-2">{props.title}</span>}
      >
        {(card) => (
          <button
            type="button"
            onClick={() => M.openCard(card().id)}
            class="flex-1 min-w-0 border-none bg-transparent p-0 text-left font-semibold line-clamp-2"
          >
            {props.title}
          </button>
        )}
      </Show>
      <span class="flex-none text-caption leading-5 text-muted">{props.cardKey}</span>
    </div>
  );
}

/** One card on its way in: its name, its place in line, and the merge's own note when it has one. */
function QueueCard(props: { item: MergeQueueCard; card: Card | undefined }) {
  const view = () => (props.card ? M.deco(props.card) : undefined);
  return (
    <article
      data-card={props.item.key}
      aria-label={`${props.item.key} ${props.item.title}`}
      class={CARD}
      style={{ "border-left-color": view()?.edge ?? "var(--color-border)" }}
    >
      <CardName cardKey={props.item.key} title={props.item.title} card={props.card} />
      <Show when={props.item.phase === "queued"}>
        <span class="text-small leading-4.5 text-secondary">
          Place {props.item.position} in line
        </span>
      </Show>
      <Show when={view()?.mergeNote}>
        <span class="text-small leading-4.5 text-secondary">{view()?.mergeNote}</span>
      </Show>
    </article>
  );
}

/** One card whose merge stopped: why, and the one thing to press about it. */
function StoppedCard(props: { card: Card }) {
  const [busy, setBusy] = createSignal(false);
  async function retry(): Promise<void> {
    setBusy(true);
    try {
      await retryMerge(props.card.id);
    } finally {
      setBusy(false);
    }
  }
  return (
    <article
      data-card={props.card.id}
      aria-label={`${props.card.id} ${props.card.title}`}
      class={CARD}
      style={{ "border-left-color": M.deco(props.card).edge }}
    >
      <CardName cardKey={props.card.id} title={props.card.title} card={props.card} />
      <Show when={props.card.reason}>
        <p class="m-0 text-small leading-4.5 text-status-needs-you-text font-medium">
          {props.card.reason}
        </p>
      </Show>
      <Button
        size={28}
        icon="repeat"
        class="self-start"
        disabled={busy()}
        onClick={() => void retry()}
      >
        Retry
      </Button>
    </article>
  );
}

/** A named group of cards with how many it holds. */
function Lane(props: { title: string; icon: string; count: number; children: JSX.Element }) {
  const id = createUniqueId();
  return (
    <section aria-labelledby={id} class="flex flex-col gap-2">
      <div class="flex items-center gap-2 h-7">
        <Icon name={props.icon} size={16} />
        <h3 id={id} class="m-0 text-body leading-5 font-semibold">
          {props.title}
        </h3>
        <Badge tone="selected" class="min-w-5 leading-5" aria-label={`${props.count} cards`}>
          {props.count}
        </Badge>
      </div>
      <div class="grid gap-2 @lg:grid-cols-2">{props.children}</div>
    </section>
  );
}

/** A lane of cards that are in the merge queue. */
export function QueueLane(props: {
  title: string;
  icon: string;
  items: readonly MergeQueueCard[];
  cardOf: (daemonId: string) => Card | undefined;
}) {
  return (
    <Lane title={props.title} icon={props.icon} count={props.items.length}>
      <For each={props.items}>
        {(item) => <QueueCard item={item} card={props.cardOf(item.cardId)} />}
      </For>
    </Lane>
  );
}

/** The cards that stopped and need the owner, each with a Retry. */
export function NeedsYouLane(props: { cards: readonly Card[] }) {
  return (
    <Lane title="Needs you" icon="st-needs" count={props.cards.length}>
      <For each={props.cards}>{(card) => <StoppedCard card={card} />}</For>
    </Lane>
  );
}
