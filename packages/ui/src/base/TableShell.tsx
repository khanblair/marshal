import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface TableShellProps
  extends Omit<JSX.HTMLAttributes<HTMLTableElement>, "style" | "children"> {
  /** The header cells (`SortHeader`s), placed in the one header row. */
  header: JSX.Element;
  /** Minimum width in px. A narrower parent scrolls sideways. */
  minWidth: number;
  /** The body rows (`TableRow`s). */
  children?: JSX.Element;
}

/**
 * The table of the Agents and List views: full width, 13 px text on an 18 px line, cells with
 * separate borders so sticky headers keep their bottom line.
 */
export function TableShell(props: TableShellProps) {
  const [local, others] = splitProps(props, ["header", "minWidth", "class", "children"]);
  return (
    <table
      {...others}
      class={cx("w-full border-separate border-spacing-0 text-small leading-4.5", local.class)}
      style={{ "min-width": `${local.minWidth}px` }}
    >
      <thead>
        <tr>{local.header}</tr>
      </thead>
      <tbody>{local.children}</tbody>
    </table>
  );
}
