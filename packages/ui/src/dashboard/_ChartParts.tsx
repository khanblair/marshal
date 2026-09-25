import { Index, type JSX } from "solid-js";
import { AXIS_FONT_PX, type GridLine, type PlacedTick, X_LABEL_Y_PX } from "./chart-geometry";

/* Private SVG pieces shared by BarChart and LineChart. */

interface ChartTextProps {
  x: number;
  y: number;
  anchor?: "end" | "middle";
  /** Fill color class. Default: the axis color. */
  fill?: string;
  children: JSX.Element;
}

/** A 12 px label in the sans font. The size is an attribute, so the phone text rule leaves it alone. */
export function ChartText(props: ChartTextProps) {
  return (
    <text
      x={props.x}
      y={props.y}
      text-anchor={props.anchor}
      font-size={String(AXIS_FONT_PX)}
      class={`font-sans ${props.fill ?? "fill-chart-axis"}`}
    >
      {props.children}
    </text>
  );
}

interface ChartGridProps {
  lines: readonly GridLine[];
  x1: number;
  x2: number;
  /** Right edge of the value labels. */
  labelX: number;
}

/** Horizontal grid lines, each followed by its value label, in the design's paint order. */
export function ChartGrid(props: ChartGridProps) {
  return (
    <Index each={props.lines}>
      {(line) => (
        <>
          <line
            x1={props.x1}
            x2={props.x2}
            y1={line().y}
            y2={line().y}
            stroke-width="1"
            class="stroke-chart-grid"
          />
          <ChartText x={props.labelX} y={line().labelY} anchor="end">
            {line().label}
          </ChartText>
        </>
      )}
    </Index>
  );
}

/** Date labels centered under the plot. */
export function ChartXLabels(props: { ticks: readonly PlacedTick[] }) {
  return (
    <Index each={props.ticks}>
      {(tick) => (
        <ChartText x={tick().x} y={X_LABEL_Y_PX} anchor="middle">
          {tick().label}
        </ChartText>
      )}
    </Index>
  );
}
