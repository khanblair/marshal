import { type JSX, Show, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import { cx } from "./cx";

export type SortDirection = "ascending" | "descending" | "none";

export interface SortHeaderProps
  extends Omit<JSX.ThHTMLAttributes<HTMLTableCellElement>, "children"> {
  label: string;
  /** Current sort of this column. Default `none`. */
  sort?: SortDirection;
  /** Makes the header a sort button. Without it the header is plain text. */
  onSort?: () => void;
  /** Right-align (numbers such as Cost). */
  alignEnd?: boolean;
  /** Row height in px: 34 (list) or 36 (default, agents). */
  size?: 34 | 36;
}

const ARROW_PX = 12;
const SHORT = 34;

/** A sticky table header cell on the sunken fill, optionally a sort button with an arrow. */
export function SortHeader(props: SortHeaderProps) {
  const [local, others] = splitProps(props, [
    "label",
    "sort",
    "onSort",
    "alignEnd",
    "size",
    "class",
  ]);
  const height = () => (local.size === SHORT ? "h-8.5" : "h-9");
  const active = () => local.sort === "ascending" || local.sort === "descending";
  return (
    <th
      scope="col"
      aria-sort={local.sort ?? "none"}
      {...others}
      class={cx(
        "sticky top-0 z-sticky p-0 border-b border-border bg-surface-sunken font-semibold whitespace-nowrap",
        height(),
        local.alignEnd ? "text-right" : "text-left",
        local.class,
      )}
    >
      <Show
        when={local.onSort}
        fallback={<span class="block px-3 text-secondary">{local.label}</span>}
      >
        <button
          type="button"
          onClick={() => local.onSort?.()}
          class={cx(
            "w-full flex items-center gap-1 px-3 border-none bg-transparent font-semibold text-secondary text-left hover:text-primary",
            height(),
            local.alignEnd ? "justify-end" : "justify-start",
          )}
        >
          {local.label}
          <Show when={active()}>
            <Icon name={local.sort === "ascending" ? "arrow-up" : "arrow-down"} size={ARROW_PX} />
          </Show>
        </button>
      </Show>
    </th>
  );
}
