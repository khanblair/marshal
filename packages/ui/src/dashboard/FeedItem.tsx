import { type JSX, Show, splitProps } from "solid-js";
import { cx } from "../base/cx";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";

export interface FeedItemProps extends Omit<JSX.LiHTMLAttributes<HTMLLIElement>, "onClick"> {
  icon: IconNameInput;
  /** Icon color from data, as a CSS color such as `var(--color-status-ready-solid)`. */
  iconColor?: string;
  /** Project name under the text. */
  project?: string;
  /** Relative time, such as `22 min ago`. */
  when: string;
  /** Full date shown on hover over the time. */
  whenTitle?: string;
  onClick?: () => void;
}

const ICON_PX = 16;

/**
 * One row of the activity feed on Home and in Recent activity: a colored
 * icon, the text (children) with its project, and the time. Put rows in an
 * `<ol class="m-0 p-0 list-none">`.
 */
export function FeedItem(props: FeedItemProps) {
  const [local, others] = splitProps(props, [
    "icon",
    "iconColor",
    "project",
    "when",
    "whenTitle",
    "onClick",
    "class",
    "children",
  ]);
  return (
    <li {...others} class={cx("border-t border-border", local.class)}>
      <button
        type="button"
        onClick={() => local.onClick?.()}
        class="w-full flex items-start gap-2.5 py-2.5 px-0 border-none bg-transparent text-left hover:bg-surface-hover"
      >
        <Icon name={local.icon} size={ICON_PX} class="mt-0.5" style={{ color: local.iconColor }} />
        <span class="flex-1 min-w-0 flex flex-col gap-px">
          <span class="wrap-anywhere">{local.children}</span>
          <Show when={local.project}>
            <span class="text-caption leading-4 text-secondary">{local.project}</span>
          </Show>
        </span>
        <span title={local.whenTitle} class="flex-none text-caption leading-5 text-muted">
          {local.when}
        </span>
      </button>
    </li>
  );
}
