import { Index } from "solid-js";
import { CARD_COLUMN_PX, type DepLine, px, TRACK_WIDTH_PX } from "./timeline-geometry";

export interface DependencyLinesProps {
  lines: DepLine[];
  /** Height of the drawing in px. */
  height: number;
}

const OK_COLOR = "var(--color-text-secondary)";
const BAD_COLOR = "var(--color-status-danger-solid)";
const ARROW_PATH = "M0 0 L8 4 L0 8 z";

function Arrow(props: { id: string; fill: string }) {
  return (
    <marker
      id={props.id}
      viewBox="0 0 8 8"
      refX="7"
      refY="4"
      markerWidth="7"
      markerHeight="7"
      orient="auto"
    >
      <path d={ARROW_PATH} fill={props.fill} />
    </marker>
  );
}

/** Arrows from each blocking bar to the bars that wait for it; broken ones are red and dashed. */
export function DependencyLines(props: DependencyLinesProps) {
  return (
    <svg
      aria-hidden="true"
      width={TRACK_WIDTH_PX}
      height={props.height}
      class="absolute top-0 pointer-events-none z-[4] overflow-visible"
      style={{ left: px(CARD_COLUMN_PX) }}
    >
      <defs>
        <Arrow id="tl-arrow" fill={OK_COLOR} />
        <Arrow id="tl-arrow-bad" fill={BAD_COLOR} />
      </defs>
      <Index each={props.lines}>
        {(line) => (
          <path
            d={line().d}
            fill="none"
            stroke={line().bad ? BAD_COLOR : OK_COLOR}
            stroke-width="1.5"
            stroke-dasharray={line().bad ? "4 3" : "0"}
            marker-end={line().bad ? "url(#tl-arrow-bad)" : "url(#tl-arrow)"}
          />
        )}
      </Index>
    </svg>
  );
}
