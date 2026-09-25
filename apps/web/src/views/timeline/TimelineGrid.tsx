import { createMemo, For } from "solid-js";
import type { Card } from "~/mock";
import { createBarDrag } from "./bar-drag";
import { DependencyLines } from "./DependencyLines";
import { TimelineHeader } from "./TimelineHeader";
import { TimelineLegend } from "./TimelineLegend";
import { TimelineRow } from "./TimelineRow";
import {
  dependencyLines,
  px,
  spanOf,
  svgHeight,
  TODAY_LINE_X_PX,
  TOTAL_WIDTH_PX,
} from "./timeline-geometry";

export interface TimelineGridProps {
  /** Cards on the timeline, in row order. */
  cards: Card[];
}

/** The Gantt grid: day header, one bar per card, today line, dependency lines, legend. */
export function TimelineGrid(props: TimelineGridProps) {
  const bars = createBarDrag();
  const span = (c: Card) => spanOf(c, bars.drag());
  const lines = createMemo(() => dependencyLines(props.cards, span));
  return (
    <div class="relative min-h-full" style={{ width: px(TOTAL_WIDTH_PX) }}>
      <TimelineHeader />
      <div class="relative">
        <For each={props.cards}>
          {(c) => (
            <TimelineRow
              card={c}
              span={span(c)}
              dragging={bars.drag()?.id === c.id}
              onBarDown={(e) => bars.down(e, c)}
            />
          )}
        </For>
        <div
          aria-hidden="true"
          class="absolute top-0 bottom-0 w-0 border-l border-border-strong z-[3] pointer-events-none"
          style={{ left: px(TODAY_LINE_X_PX) }}
        />
        <DependencyLines lines={lines()} height={svgHeight(props.cards.length)} />
      </div>
      <TimelineLegend />
    </div>
  );
}
