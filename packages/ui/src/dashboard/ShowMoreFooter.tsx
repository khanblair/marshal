import { type JSX, Show, splitProps } from "solid-js";
import { Button } from "../base/Button";
import { cx } from "../base/cx";

export interface ShowMoreFooterProps extends Omit<JSX.HTMLAttributes<HTMLDivElement>, "children"> {
  /** How many items the full list has. */
  total: number;
  /** Rows shown while collapsed. Default 5. The toggle shows only above this. */
  limit?: number;
  /** Whether the list shows every item. */
  expanded: boolean;
  onToggle: () => void;
  /** Label of the link to the full page, such as `View all activity`. */
  viewAllLabel: string;
  onViewAll: () => void;
}

const DEFAULT_LIMIT = 5;

/**
 * The buttons under a Home section: `Show all N` or `Show less`, then a quiet
 * link to the full page.
 */
export function ShowMoreFooter(props: ShowMoreFooterProps) {
  const [local, others] = splitProps(props, [
    "total",
    "limit",
    "expanded",
    "onToggle",
    "viewAllLabel",
    "onViewAll",
    "class",
  ]);
  return (
    <div {...others} class={cx("flex flex-wrap gap-2 mt-2", local.class)}>
      <Show when={local.total > (local.limit ?? DEFAULT_LIMIT)}>
        <Button aria-expanded={local.expanded} class="text-small" onClick={() => local.onToggle()}>
          {local.expanded ? "Show less" : `Show all ${local.total}`}
        </Button>
      </Show>
      <Button variant="quiet" class="text-small" onClick={() => local.onViewAll()}>
        {local.viewAllLabel}
      </Button>
    </div>
  );
}
