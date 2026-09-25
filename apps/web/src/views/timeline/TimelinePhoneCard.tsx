import { StatusLabel } from "@marshal/ui";
import { createMemo, For, Show } from "solid-js";
import { type Card, M } from "~/mock";
import { isLateFor, spanLabel } from "./timeline-model";

export interface TimelinePhoneCardProps {
  card: Card;
}

/** A button for a card this card waits for; red while that card is not done in time. */
function WaitsForChip(props: { card: Card; waiting: Card }) {
  const late = () => isLateFor(props.card, props.waiting);
  return (
    <button
      type="button"
      onClick={() => M.openCard(props.card.id)}
      class="min-h-8 py-0 px-2 rounded-sm border bg-surface text-small font-semibold"
      style={{
        "border-color": late() ? M.tone("danger", "solid") : "var(--color-border-strong)",
        color: late() ? M.tone("danger", "text") : "var(--color-text-primary)",
      }}
    >
      {`#${props.card.id} ${M.STATUS[props.card.state].label}`}
    </button>
  );
}

/** One card in the phone list: title, state, end day, and what it waits for. */
export function TimelinePhoneCard(props: TimelinePhoneCardProps) {
  const d = createMemo(() => M.deco(props.card));
  const waitsFor = createMemo(() =>
    props.card.deps.map((id) => M.card(id)).filter((a): a is Card => a !== undefined),
  );
  return (
    <div
      class="flex flex-col gap-1 pt-2.5 pr-0 pb-2.5 pl-2.5 border-b border-l-[3px] border-border"
      style={{ "border-left-color": d().edge }}
    >
      <button
        type="button"
        onClick={() => M.openCard(props.card.id)}
        class="flex gap-2 items-baseline border-none bg-transparent p-0 text-left"
      >
        <span class="font-semibold" style={{ color: d().titleColor }}>
          {props.card.title}
        </span>
        <span class="text-small text-muted">{d().num}</span>
      </button>
      <span class="flex flex-wrap gap-x-3 gap-y-0.5 text-small text-secondary">
        <StatusLabel state={props.card.state}>{d().stateLabel}</StatusLabel>
        <span>{spanLabel(props.card, M.T0, M.D)}</span>
      </span>
      <Show when={props.card.deps.length > 0}>
        <span class="flex flex-wrap items-center gap-x-2 gap-y-1 text-small text-secondary">
          <span>Waits for</span>
          <For each={waitsFor()}>{(a) => <WaitsForChip card={a} waiting={props.card} />}</For>
        </span>
      </Show>
    </div>
  );
}
