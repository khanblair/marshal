import { createMemo, Index } from "solid-js";
import { ChartGrid, ChartXLabels } from "./_ChartParts";
import {
  type AxisTick,
  BAR_PLOT_LEFT_PX,
  BAR_Y_LABEL_X_PX,
  barLayout,
  CHART_HEIGHT_PX,
} from "./chart-geometry";

export interface BarChartProps {
  /** Width of the SVG in px, measured from the column it sits in. */
  width: number;
  /** One value per day, oldest first. */
  values: readonly number[];
  /** Hover text of each bar, such as `Sep 24: 3 cards`. */
  tips: readonly string[];
  /** Date labels under the plot, by day index. */
  ticks: readonly AxisTick[];
  /** Accessible summary of the chart. */
  label: string;
}

/**
 * Daily bars with rounded tops over a three line grid, as in Home's Cards
 * finished per day. The SVG is 180 px high and draws outside its box where
 * labels overflow.
 */
export function BarChart(props: BarChartProps) {
  const layout = createMemo(() => barLayout(props.values, props.width, props.ticks));
  return (
    <svg
      role="img"
      aria-label={props.label}
      width={props.width}
      height={CHART_HEIGHT_PX}
      class="block overflow-visible"
    >
      <ChartGrid
        lines={layout().grid}
        x1={BAR_PLOT_LEFT_PX}
        x2={props.width}
        labelX={BAR_Y_LABEL_X_PX}
      />
      <Index each={layout().paths}>
        {(d, i) => (
          <path d={d()} class="fill-chart-1">
            <title>{props.tips[i]}</title>
          </path>
        )}
      </Index>
      <ChartXLabels ticks={layout().ticks} />
    </svg>
  );
}
