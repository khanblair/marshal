import { type JSX, splitProps } from "solid-js";
import { cx } from "../base/cx";

export interface ChartFigureProps extends Omit<JSX.HTMLAttributes<HTMLElement>, "title"> {
  /** Bold heading of the chart, such as `Cards finished per day`. */
  title: string;
  /** One line under the heading that says what the chart shows. */
  summary: string;
}

/**
 * A chart with its caption: the title in bold, a secondary summary line, then
 * the children (a `BarChart` or a `LineChart` with its `ChartLegend`).
 */
export function ChartFigure(props: ChartFigureProps) {
  const [local, others] = splitProps(props, ["title", "summary", "class", "children"]);
  return (
    <figure {...others} class={cx("m-0 flex flex-col gap-1.5 min-w-0", local.class)}>
      <figcaption class="flex flex-col">
        <span class="font-semibold">{local.title}</span>
        <span class="text-small text-secondary">{local.summary}</span>
      </figcaption>
      {local.children}
    </figure>
  );
}
