import {
  BarChart,
  ChartFigure,
  ChartLegend,
  LineChart,
  SegmentedControl,
  type SegmentOption,
} from "@marshal/ui";
import { createMemo } from "solid-js";
import { M } from "~/mock";
import {
  barSummary,
  barTips,
  type ChartDays,
  costSeries,
  dateTicks,
  dollarLabel,
  finishedPerDay,
} from "./chart-data";
import { mergedTodayCount } from "./home-actions";
import { chartColumns, rangeControlSize } from "./home-layout";
import { useChartWidth } from "./use-chart-width";

/** Charts never get narrower than this, however small the column. */
const MIN_CHART_WIDTH_PX = 240;
/** The cost chart draws this many projects, the first in the store. */
const MAX_COST_PROJECTS = 3;

const RANGE_OPTIONS: readonly SegmentOption<string>[] = ["7", "30", "90"].map((days) => ({
  value: days,
  label: `${days} days`,
}));

/** The "How it's going" heading with its range control, and the two charts under it. */
export function HomeCharts() {
  const chart = useChartWidth();
  const width = () => Math.max(MIN_CHART_WIDTH_PX, chart.width());
  const days = createMemo<ChartDays>(() => ({
    today: M.T0,
    dayMs: M.D,
    range: M.S.dashRange,
  }));
  const finished = createMemo(() => finishedPerDay(days(), mergedTodayCount()));
  const series = createMemo(() =>
    costSeries(
      days(),
      M.S.projects
        .slice(0, MAX_COST_PROJECTS)
        .map((p) => ({ id: p.id, name: p.name, todayCost: M.costs(p.id).today })),
    ),
  );
  const cost = () => M.costs();
  const ticks = createMemo(() => dateTicks(days()));
  const tips = createMemo(() => barTips(days(), finished()));
  const barLabel = () => barSummary(finished(), days().range);
  const lineLabel = () =>
    `All projects today ${M.money(cost().today)} of the ${M.money(cost().day)} daily limit`;
  return (
    <>
      <div class="flex flex-wrap items-center gap-3">
        <h2 class="m-0 flex-1 text-subtitle leading-5.5 font-semibold">How it's going</h2>
        <SegmentedControl
          label="Chart range"
          compact
          size={rangeControlSize()}
          options={RANGE_OPTIONS}
          value={String(M.S.dashRange)}
          onValueChange={(value) => M.set({ dashRange: Number(value) })}
        />
      </div>
      <div ref={chart.ref} class={`grid gap-6 ${chartColumns() ? "grid-cols-2" : "grid-cols-1"}`}>
        <ChartFigure title="Cards finished per day" summary={barLabel()}>
          <BarChart
            width={width()}
            values={finished()}
            tips={tips()}
            ticks={ticks()}
            label={barLabel()}
          />
        </ChartFigure>
        <ChartFigure title="Cost per project per day" summary={lineLabel()}>
          <LineChart
            width={width()}
            series={series()}
            limit={cost().day}
            limitLabel={`Limit ${M.money(cost().day)}`}
            ticks={ticks()}
            format={dollarLabel}
            label={lineLabel()}
          />
          <ChartLegend names={series().map((s) => s.name)} limitName="Daily limit" />
        </ChartFigure>
      </div>
    </>
  );
}
