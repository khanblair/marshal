import { IconLabel, PhoneList, PhoneRowMeta, PhoneRowTitle, StatusLabel } from "@marshal/ui";
import { createMemo, For, Show } from "solid-js";
import { type Card, M } from "~/mock";

function ListPhoneRow(props: { card: Card }) {
  const d = createMemo(() => M.deco(props.card));
  return (
    <li>
      <button
        type="button"
        data-card={props.card.id}
        onClick={() => M.openCard(props.card.id)}
        style={{ "border-left-color": d().edge }}
        class="w-full flex flex-col gap-1 py-3 pr-0 pl-2.5 border-0 border-b border-l-3 border-b-border bg-transparent text-left"
      >
        <PhoneRowTitle title={props.card.title} num={d().num} titleColor={d().titleColor} />
        <PhoneRowMeta class="text-secondary">
          <StatusLabel state={props.card.state}>{d().stateLabel}</StatusLabel>
          <span>{props.card.role}</span>
          <span>{props.card.agent}</span>
          <Show when={d().hasCi}>
            <IconLabel icon={d().ciIcon} style={{ color: d().ciColor }}>
              {d().ciLabel}
            </IconLabel>
          </Show>
          <Show when={d().hasCost}>
            <span>{d().cost}</span>
          </Show>
          <span>{d().upd}</span>
        </PhoneRowMeta>
      </button>
    </li>
  );
}

/** The cards on a phone: a stacked row each, with an edge in the state's color. */
export function ListPhoneList(props: { cards: readonly Card[] }) {
  return (
    <PhoneList>
      <For each={props.cards}>{(card) => <ListPhoneRow card={card} />}</For>
    </PhoneList>
  );
}
