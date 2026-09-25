import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface TableRowProps extends JSX.HTMLAttributes<HTMLTableRowElement> {
  /** The card the keyboard focus is on: selected fill. Otherwise the row is transparent. */
  highlight?: boolean;
}

/**
 * A body row that opens something: focusable, with a pointer cursor and a hover fill. The fill is
 * a class, not an inline style, so hover still shows on the highlighted row.
 */
export function TableRow(props: TableRowProps) {
  const [local, others] = splitProps(props, ["highlight", "class", "children"]);
  return (
    <tr
      tabindex="0"
      {...others}
      class={cx(
        "cursor-pointer hover:bg-surface-hover",
        local.highlight ? "bg-surface-selected" : "bg-transparent",
        local.class,
      )}
    >
      {local.children}
    </tr>
  );
}
