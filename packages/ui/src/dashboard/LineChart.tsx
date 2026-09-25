import { createMemo, Index } from "solid-js";
import { ChartGrid, ChartText, ChartXLabels } from "./_ChartParts";
import {
  type AxisTick,
  CHART_HEIGHT_PX,
  LIMIT_LINE_STYLE,
  LINE_PLOT_LEFT_PX,
  LINE_Y_LABEL_X_PX,
  lineLayout,
  lineStyle,
} from "./chart-geometry";

export interface ChartSeries {
  name: string;
  /** One value per day, oldest first. */
  values: readonly number[];
}

export interface LineChartProps {
  /** Width of the SVG in px, measured from the column it sits in. */
  width: number;
  /** Lines in drawing order; each takes the line style at its position. */
  series: readonly ChartSeries[];
  /** Value of the dashed limit line. */
  limit: number;
  /** Text next to the limit line, such as `Limit $25.00`. */
  limitLabel: string;
  /** Date labels under the plot, by day index. */
  ticks: readonly AxisTick[];
  /** Grid label text, such as `$12`. */
  format: (value: number) => string;
  /** Accessible summary of the chart. */
  label: string;
}

/**
 * Daily lines against a dashed limit, with each line's name at its last
 * point, as in Home's Cost per project per day. Pair it with `ChartLegend`.
 */
export function LineChart(props: LineChartProps) {
  const layout = createMemo(() =>
    lineLayout({
      series: props.series.map((s) => s.values),
      limit: props.limit,
      width: props.width,
      ticks: props.ticks,
      format: props.format,
    }),
  );
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
        x1={LINE_PLOT_LEFT_PX}
        x2={layout().plotRight}
        labelX={LINE_Y_LABEL_X_PX}
      />
      <line
        x1={LINE_PLOT_LEFT_PX}
        x2={layout().plotRight}
        y1={layout().limitY}
        y2={layout().limitY}
        class={LIMIT_LINE_STYLE.stroke}
        stroke-width={LIMIT_LINE_STYLE.width}
        stroke-dasharray={LIMIT_LINE_STYLE.dash}
      />
      <ChartText x={layout().labelX} y={layout().limitLabelY} fill="fill-status-danger-text">
        {props.limitLabel}
      </ChartText>
      <Index each={props.series}>
        {(series, i) => (
          <>
            <path
              d={layout().paths[i]}
              fill="none"
              class={lineStyle(i).stroke}
              stroke-width={lineStyle(i).width}
              stroke-dasharray={lineStyle(i).dash}
              stroke-linejoin="round"
              stroke-linecap="round"
            />
            <ChartText x={layout().labelX} y={layout().labelYs[i] ?? 0}>
              {series().name}
            </ChartText>
          </>
        )}
      </Index>
      <ChartXLabels ticks={layout().ticks} />
    </svg>
  );
}
