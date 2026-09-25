import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export type TableCellProps = JSX.TdHTMLAttributes<HTMLTableCellElement>;

/**
 * A body cell with the 1 px line under it. Padding, width, wrapping, and alignment come from
 * `class`, because the two tables differ.
 */
export function TableCell(props: TableCellProps) {
  const [local, others] = splitProps(props, ["class", "children"]);
  return (
    <td {...others} class={cx("border-b border-border", local.class)}>
      {local.children}
    </td>
  );
}
