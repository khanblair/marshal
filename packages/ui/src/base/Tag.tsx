import { type JSX, Show, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";
import { cx } from "./cx";

export interface TagProps extends JSX.HTMLAttributes<HTMLSpanElement> {
  /** Called by the remove button. */
  onRemove: () => void;
  /** Accessible name of the remove button, for example `Remove filter Status Working`. */
  removeLabel: string;
  /**
   * 32 (default): a filter chip with a strong border. 28: an attachment chip
   * with a light border and rounder corners.
   */
  size?: 28 | 32;
  /** Leading icon at 14 px. */
  icon?: IconNameInput;
}

const ICON_PX = 14;
const REMOVE_ICON_PX = 12;
const SMALL = 28;

/** A removable chip on a sunken fill: filter chips and pending attachments. */
export function Tag(props: TagProps) {
  const [local, others] = splitProps(props, [
    "onRemove",
    "removeLabel",
    "size",
    "icon",
    "class",
    "children",
  ]);
  const small = () => local.size === SMALL;
  return (
    <span
      {...others}
      class={cx(
        "inline-flex items-center pl-2 pr-1 bg-surface-sunken text-small border",
        small()
          ? "gap-1.5 h-7 rounded-sm border-border"
          : "flex-none gap-1 h-8 rounded-xs border-border-strong",
        local.class,
      )}
    >
      <Show when={local.icon}>{(name) => <Icon name={name()} size={ICON_PX} />}</Show>
      {local.children}
      <button
        type="button"
        aria-label={local.removeLabel}
        onClick={() => local.onRemove()}
        class={cx(
          "inline-flex items-center justify-center p-0 border-none rounded-xs bg-transparent",
          small() ? "size-5.5" : "size-6 text-secondary hover:bg-surface-hover",
        )}
      >
        <Icon name="x" size={REMOVE_ICON_PX} />
      </button>
    </span>
  );
}
