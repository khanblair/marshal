/**
 * Geometry of the Home charts, from HomeView's script in the design. Every
 * number keeps the prototype's operation order and string formatting (bar
 * paths print plain numbers, line paths print one decimal), because the port
 * is compared with the design pixel by pixel.
 */

/** Height of both chart SVGs. */
export const CHART_HEIGHT_PX = 180;
/** Y of the highest value. */
const PLOT_TOP_PX = 8;
/** Y of zero, the base of the bars. */
const PLOT_BOTTOM_PX = 150;
/** Baseline of the date labels under the plot. */
export const X_LABEL_Y_PX = 176;
/** Size of every axis and series label. */
export const AXIS_FONT_PX = 12;
/** Axis labels sit this far below their grid line, so the text centers on it. */
const GRID_LABEL_DROP_PX = 4;

/** Left edge of the bar plot; the y labels end at `BAR_Y_LABEL_X_PX`. */
export const BAR_PLOT_LEFT_PX = 32;
export const BAR_Y_LABEL_X_PX = 26;
/** Share of each day's slot the bar fills. */
const BAR_FILL_RATIO = 0.64;
const MIN_BAR_WIDTH_PX = 2;
const MAX_BAR_RADIUS_PX = 3;
/** The bar axis always reaches at least this many cards. */
const MIN_BAR_AXIS_MAX = 4;

/** Left edge of the line plot; the y labels end at `LINE_Y_LABEL_X_PX`. */
export const LINE_PLOT_LEFT_PX = 40;
export const LINE_Y_LABEL_X_PX = 34;
/** Space right of the line plot for the series and limit labels. */
const LINE_LABEL_SPACE_PX = 96;
/** Gap between the plot's right edge and its labels. */
const LINE_LABEL_GAP_PX = 6;
/** The limit label sits this far above the limit line. */
const LIMIT_LABEL_RISE_PX = 6;
/** Series labels closer than this are pushed apart. */
const SERIES_LABEL_MIN_GAP_PX = 14;
/** The line axis reaches this much past the limit, so the limit line never touches the top. */
const LIMIT_HEADROOM = 1.1;

/** Maps a value to its y coordinate: 0 on the plot bottom, `max` on the plot top. */
export type YScale = (value: number) => number;

export function yScale(max: number): YScale {
  return (value) => PLOT_BOTTOM_PX - (value / max) * (PLOT_BOTTOM_PX - PLOT_TOP_PX);
}

/** A horizontal grid line with its axis label. */
export interface GridLine {
  y: number;
  /** Baseline of the label. */
  labelY: number;
  label: string;
}

function gridLine(y: YScale, value: number, label: string): GridLine {
  return { y: y(value), labelY: y(value) + GRID_LABEL_DROP_PX, label };
}

/** An x axis label at a day index. */
export interface AxisTick {
  index: number;
  label: string;
}

/** A positioned x axis label. */
export interface PlacedTick {
  x: number;
  label: string;
}

export interface BarRect {
  x: number;
  y: number;
  width: number;
  radius: number;
}

/**
 * A bar with rounded top corners standing on the plot bottom, or `""` for a
 * zero bar. The corner radius shrinks for bars shorter than it.
 */
export function roundedBarPath(bar: BarRect): string {
  const { x, y, width } = bar;
  const bottom = PLOT_BOTTOM_PX;
  const height = bottom - y;
  const r = Math.min(bar.radius, height);
  if (height <= 0) return "";
  return `M${x} ${bottom} V${y + r} Q${x} ${y} ${x + r} ${y} H${x + width - r} Q${x + width} ${y} ${x + width} ${y + r} V${bottom} Z`;
}

export interface BarLayout {
  /** One path per value, in order. */
  paths: string[];
  grid: GridLine[];
  ticks: PlacedTick[];
}

/** Bars for `values` across an SVG `width` px wide, with a 0, half, and max grid. */
export function barLayout(
  values: readonly number[],
  width: number,
  ticks: readonly AxisTick[],
): BarLayout {
  const max = Math.max(MIN_BAR_AXIS_MAX, ...values);
  const slot = (width - BAR_PLOT_LEFT_PX) / values.length;
  const barWidth = Math.max(MIN_BAR_WIDTH_PX, slot * BAR_FILL_RATIO);
  const radius = Math.min(MAX_BAR_RADIUS_PX, barWidth / 2);
  const y = yScale(max);
  const paths = values.map((value, i) =>
    roundedBarPath({
      x: BAR_PLOT_LEFT_PX + i * slot + (slot - barWidth) / 2,
      y: y(value),
      width: barWidth,
      radius,
    }),
  );
  const half = Math.round(max / 2);
  return {
    paths,
    grid: [0, half, max].map((v) => gridLine(y, v, String(v))),
    ticks: ticks.map((t) => ({ x: BAR_PLOT_LEFT_PX + t.index * slot + slot / 2, label: t.label })),
  };
}

/** Pushes labels down until no two are closer than `minGap`, keeping their input order. */
export function spreadLabels(ys: readonly number[], minGap: number): number[] {
  const order = ys.map((y, index) => ({ y, index })).sort((a, b) => a.y - b.y);
  for (let k = 1; k < order.length; k++) {
    const prev = order[k - 1];
    const cur = order[k];
    if (prev && cur && cur.y - prev.y < minGap) cur.y = prev.y + minGap;
  }
  const out = [...ys];
  for (const item of order) out[item.index] = item.y;
  return out;
}

export interface LineLayout {
  /** One path per series. */
  paths: string[];
  /** Baseline of each series' name, next to its last point. */
  labelYs: number[];
  grid: GridLine[];
  ticks: PlacedTick[];
  /** Right edge of the plot, where the grid and limit lines end. */
  plotRight: number;
  /** X of the series and limit labels. */
  labelX: number;
  limitY: number;
  limitLabelY: number;
}

export interface LineInput {
  /** Every series has one value per day. */
  series: readonly (readonly number[])[];
  limit: number;
  width: number;
  ticks: readonly AxisTick[];
  /** Grid label text, such as `$12`. */
  format: (value: number) => string;
}

/** Lines for each series with a dashed limit, a 0, half, and max grid, and labels on the right. */
export function lineLayout(input: LineInput): LineLayout {
  const count = input.series[0]?.length ?? 0;
  const plotRight = input.width - LINE_LABEL_SPACE_PX;
  const step = (plotRight - LINE_PLOT_LEFT_PX) / Math.max(1, count - 1);
  const max = Math.max(input.limit * LIMIT_HEADROOM, ...input.series.flat());
  const y = yScale(max);
  const x = (i: number) => LINE_PLOT_LEFT_PX + i * step;
  const path = (values: readonly number[]) =>
    values.map((v, i) => `${i ? "L" : "M"}${x(i).toFixed(1)} ${y(v).toFixed(1)}`).join(" ");
  const lastYs = input.series.map((values) => y(values[count - 1] ?? 0) + GRID_LABEL_DROP_PX);
  return {
    paths: input.series.map(path),
    labelYs: spreadLabels(lastYs, SERIES_LABEL_MIN_GAP_PX),
    grid: [0, max / 2, max].map((v) => gridLine(y, v, input.format(v))),
    ticks: input.ticks.map((t) => ({ x: x(t.index), label: t.label })),
    plotRight,
    labelX: plotRight + LINE_LABEL_GAP_PX,
    limitY: y(input.limit),
    limitLabelY: y(input.limit) - LIMIT_LABEL_RISE_PX,
  };
}

/** How one line is drawn, in the chart and in its legend. */
export interface LineStyle {
  /** Stroke color class. */
  stroke: string;
  width: number;
  dash: string;
  /** Series lines have round ends; the limit line keeps square ones. */
  roundCap: boolean;
}

/** Styles of the series lines, by position: the total first, then each project. */
export const LINE_STYLES: readonly LineStyle[] = [
  { stroke: "stroke-chart-1", width: 2, dash: "0", roundCap: true },
  { stroke: "stroke-chart-2", width: 1.5, dash: "0", roundCap: true },
  { stroke: "stroke-chart-3", width: 1.5, dash: "6 3", roundCap: true },
  { stroke: "stroke-chart-4", width: 2, dash: "1 4", roundCap: true },
];

export const LIMIT_LINE_STYLE: LineStyle = {
  stroke: "stroke-chart-limit",
  width: 1.5,
  dash: "5 4",
  roundCap: false,
};

/** Style of the series at `index`; series past the table reuse its last style. */
export function lineStyle(index: number): LineStyle {
  return LINE_STYLES[Math.min(index, LINE_STYLES.length - 1)] ?? LIMIT_LINE_STYLE;
}
