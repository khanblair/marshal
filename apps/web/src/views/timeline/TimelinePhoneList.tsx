import { DayHeading } from "@marshal/ui";
import { createMemo, For } from "solid-js";
import { type Card, M } from "~/mock";
import { TimelinePhoneCard } from "./TimelinePhoneCard";
import { groupByStart, groupLabel } from "./timeline-model";

export interface TimelinePhoneListProps {
  /** Cards on the timeline, sorted by start day. */
  cards: Card[];
}

/** Phone layout: the cards grouped under their planned start day. */
export function TimelinePhoneList(props: TimelinePhoneListProps) {
  const groups = createMemo(() => groupByStart(props.cards));
  return (
    <div class="pt-2 px-4 pb-8">
      <p class="mt-2 mx-0 mb-3 text-small text-secondary">
        Cards by planned start day. Open a card to change its dates.
      </p>
      <For each={groups()}>
        {(group) => (
          <section class="flex flex-col mb-3">
            <DayHeading today={group.offset === 0} class="py-2">
              {groupLabel(M.T0, M.D, group.offset)}
            </DayHeading>
            <For each={group.cards}>{(c) => <TimelinePhoneCard card={c} />}</For>
          </section>
        )}
      </For>
    </div>
  );
}
