import { For, type JSX, splitProps } from "solid-js";
import { cx } from "../base/cx";
import { LIMIT_LINE_STYLE, type LineStyle, lineStyle } from "./chart-geometry";

export interface ChartLegendProps extends Omit<JSX.HTMLAttributes<HTMLDivElement>, "children"> {
  /** Series names in `LineChart` order; each gets the line style at its position. */
  names: readonly string[];
  /** Name of the dashed limit line, shown last, such as `Daily limit`. */
  limitName: string;
}

const SWATCH_WIDTH_PX = 22;
const SWATCH_HEIGHT_PX = 8;
const SWATCH_START_PX = 1;
const SWATCH_END_PX = 21;
const SWATCH_MID_PX = 4;

function Swatch(props: { style: LineStyle }) {
  return (
    <svg width={SWATCH_WIDTH_PX} height={SWATCH_HEIGHT_PX} aria-hidden="true">
      <line
        x1={SWATCH_START_PX}
        x2={SWATCH_END_PX}
        y1={SWATCH_MID_PX}
        y2={SWATCH_MID_PX}
        class={props.style.stroke}
        stroke-width={props.style.width}
        stroke-dasharray={props.style.dash}
        stroke-linecap={props.style.roundCap ? "round" : undefined}
      />
    </svg>
  );
}

function Entry(props: { style: LineStyle; children: JSX.Element }) {
  return (
    <span class="inline-flex items-center gap-1.5">
      <Swatch style={props.style} />
      {props.children}
    </span>
  );
}

/** Key of a `LineChart`: a short sample of each line with its name, then the limit line. */
export function ChartLegend(props: ChartLegendProps) {
  const [local, others] = splitProps(props, ["names", "limitName", "class"]);
  return (
    <div
      {...others}
      class={cx(
        "flex flex-wrap gap-y-1 gap-x-3.5 text-caption leading-4 text-secondary",
        local.class,
      )}
    >
      <For each={local.names}>{(name, i) => <Entry style={lineStyle(i())}>{name}</Entry>}</For>
      <Entry style={LIMIT_LINE_STYLE}>{local.limitName}</Entry>
    </div>
  );
}
