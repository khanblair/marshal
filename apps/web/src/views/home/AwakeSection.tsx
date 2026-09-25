import { Button, Icon, ShowMoreFooter, StatusLabel } from "@marshal/ui";
import { createMemo, For } from "solid-js";
import { type Card, M } from "~/mock";
import { awakeCards } from "./awake";
import { openAgents } from "./home-actions";
import { createShowAll } from "./use-show-all";

function AwakeRow(props: { card: Card }) {
  const view = createMemo(() => M.deco(props.card));
  const sleepLabel = () => `Sleep #${props.card.id}`;
  return (
    <div class="flex items-center gap-2 py-2 border-t border-border">
      <button
        type="button"
        onClick={() => M.openCard(props.card.id)}
        class="flex-1 min-w-0 flex flex-col gap-px border-none bg-transparent p-0 text-left"
      >
        <span class="flex gap-1.5 min-w-0">
          <Icon name={view().icon} size={14} style={{ color: view().iconColor }} />
          <span class="font-semibold overflow-hidden text-ellipsis whitespace-nowrap">
            {view().title}
          </span>
        </span>
        <span class="flex flex-wrap gap-x-2.5 text-caption leading-4 text-secondary">
          <StatusLabel state={view().state} withIcon={false}>
            {view().stateLabel}
          </StatusLabel>
          <span>{view().projectName}</span>
          <span>{view().agent}</span>
        </span>
      </button>
      <Button
        size={28}
        icon="moon"
        class="px-2! gap-1! font-normal!"
        aria-label={sleepLabel()}
        title={sleepLabel()}
        onClick={() => M.sleep(props.card.id)}
      >
        Sleep
      </Button>
    </div>
  );
}

/** Home's "Agents awake": cards with a live session, each with a Sleep button, and the count against the limit. */
export function AwakeSection() {
  const cards = createMemo(awakeCards);
  const rows = createShowAll(cards);
  const limit = () => M.S.limits.global.awake;
  const full = () => cards().length >= limit();
  return (
    <section aria-labelledby="h-awake" class="flex flex-col">
      <div class="flex items-baseline gap-2 mb-2">
        <h2 id="h-awake" class="m-0 flex-1 text-subtitle leading-5.5 font-semibold">
          Agents awake
        </h2>
        <span
          class={`text-small font-semibold ${full() ? "text-status-needs-you-text" : "text-secondary"}`}
        >
          {cards().length} of {limit()}
        </span>
      </div>
      <For each={rows.visible()}>{(card) => <AwakeRow card={card} />}</For>
      <ShowMoreFooter
        total={cards().length}
        expanded={rows.expanded()}
        onToggle={rows.toggle}
        viewAllLabel="Open agents"
        onViewAll={openAgents}
      />
    </section>
  );
}
