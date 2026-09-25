import { type JSX, Show, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";
import { cx } from "./cx";

export interface EmptyStateProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** Icon at 20 px above the message. */
  icon?: IconNameInput;
  /** A button under the message. */
  action?: JSX.Element;
  /** Classes for the message paragraph, such as `max-w-[40ch]`. */
  messageClass?: string;
}

const ICON_PX = 20;

/**
 * Nothing to show yet: an icon, a message (children), and an action, centered
 * in the free space. Used for an empty board, no chat open, and no preview.
 */
export function EmptyState(props: EmptyStateProps) {
  const [local, others] = splitProps(props, [
    "icon",
    "action",
    "messageClass",
    "class",
    "children",
  ]);
  return (
    <div
      {...others}
      class={cx(
        "flex-1 flex flex-col items-center justify-center gap-3 p-6 text-center",
        local.class,
      )}
    >
      <Show when={local.icon}>{(name) => <Icon name={name()} size={ICON_PX} />}</Show>
      <p class={cx("m-0 text-secondary", local.messageClass)}>{local.children}</p>
      {local.action}
    </div>
  );
}
