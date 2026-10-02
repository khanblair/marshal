import { Badge, Button, Icon } from "@marshal/ui";
import { createSignal, createUniqueId, For, Show } from "solid-js";
import type { DeliveredCard } from "~/data/mappers/integration";
import { type Card, M } from "~/mock";
import { undoMerge } from "~/sync/integration-flow";

/** One delivered card: when it landed, what the Integrator resolved and said, and Undo while it can. */
function DeliveredRow(props: { item: DeliveredCard; card: Card | undefined }) {
  const [busy, setBusy] = createSignal(false);
  async function undo(): Promise<void> {
    setBusy(true);
    try {
      await undoMerge(props.item.cardId);
    } finally {
      setBusy(false);
    }
  }
  return (
    <article
      data-delivered={props.item.key}
      aria-label={`${props.item.key} ${props.item.title}`}
      class="flex flex-wrap items-start gap-x-4 gap-y-2 py-3 border-t border-border"
    >
      <div class="flex-[1_1_320px] min-w-0 flex flex-col gap-1">
        <div class="flex flex-wrap items-baseline gap-x-2">
          <Show when={props.card} fallback={<span class="font-semibold">{props.item.title}</span>}>
            {(card) => (
              <button
                type="button"
                onClick={() => M.openCard(card().id)}
                class="border-none bg-transparent p-0 text-left font-semibold"
              >
                {props.item.title}
              </button>
            )}
          </Show>
          <span class="text-caption leading-5 text-muted">{props.item.key}</span>
          <time class="text-caption leading-5 text-secondary" title={M.full(props.item.at)}>
            {M.rel(props.item.at)}
          </time>
          <Show when={props.item.resolvedLabel}>
            <Badge tone="review">{props.item.resolvedLabel}</Badge>
          </Show>
        </div>
        <Show when={props.item.summary}>
          <p class="m-0 text-small leading-4.5 text-secondary">{props.item.summary}</p>
        </Show>
      </div>
      <Show when={props.item.canUndo}>
        <Button size={28} icon="history" disabled={busy()} onClick={() => void undo()}>
          Undo
        </Button>
      </Show>
    </article>
  );
}

/** What the Integrator has delivered, newest first. */
export function DeliveredList(props: {
  items: readonly DeliveredCard[];
  cardOf: (daemonId: string) => Card | undefined;
}) {
  const id = createUniqueId();
  return (
    <section aria-labelledby={id} class="flex flex-col">
      <div class="flex items-center gap-2 h-7 mb-1">
        <Icon name="st-done" size={16} />
        <h3 id={id} class="m-0 text-body leading-5 font-semibold">
          Delivered
        </h3>
        <Badge tone="selected" class="min-w-5 leading-5" aria-label={`${props.items.length} cards`}>
          {props.items.length}
        </Badge>
      </div>
      <For each={props.items}>
        {(item) => <DeliveredRow item={item} card={props.cardOf(item.cardId)} />}
      </For>
    </section>
  );
}
