import { type JSX, splitProps } from "solid-js";
import { Button } from "../base/Button";
import { cx } from "../base/cx";

export interface JumpToLatestProps
  extends Omit<JSX.ButtonHTMLAttributes<HTMLButtonElement>, "children"> {
  /** Distance from the bottom of the message list in px: 10 (default, card panel) or 12 (chats). */
  offset?: 10 | 12;
  /** Default `Jump to latest`. */
  label?: string;
}

const CHATS_OFFSET = 12;

/**
 * The floating pill that scrolls a chat back to the newest message. Place it
 * right after the scrolling list; it sits in a zero-height row and floats up.
 */
export function JumpToLatest(props: JumpToLatestProps) {
  const [local, others] = splitProps(props, ["offset", "label", "class"]);
  return (
    <div class="relative h-0">
      <Button
        {...others}
        icon="arrow-down"
        iconSize={14}
        class={cx(
          "absolute left-1/2 -translate-x-1/2 rounded-full! bg-surface-raised! shadow-e1",
          local.offset === CHATS_OFFSET ? "bottom-3" : "bottom-2.5",
          local.class,
        )}
      >
        {local.label ?? "Jump to latest"}
      </Button>
    </div>
  );
}
