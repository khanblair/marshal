import { type JSX, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import { Button } from "./Button";
import { cx } from "./cx";

export interface NoResultsProps extends JSX.HTMLAttributes<HTMLDivElement> {
  onClear: () => void;
  /** Default `Clear search`. */
  clearLabel?: string;
}

const ICON_PX = 16;

/**
 * The bordered notice when a search or filters hide every card, with a clear
 * button. The message is the children, such as `No cards match "auth".`
 */
export function NoResults(props: NoResultsProps) {
  const [local, others] = splitProps(props, ["onClear", "clearLabel", "class", "children"]);
  return (
    <div
      {...others}
      class={cx("flex items-center gap-3 py-2.5 px-3 rounded-md border border-border", local.class)}
    >
      <Icon name="search-x" size={ICON_PX} />
      <span class="flex-1">{local.children}</span>
      <Button size={28} onClick={() => local.onClear()}>
        {local.clearLabel ?? "Clear search"}
      </Button>
    </div>
  );
}
