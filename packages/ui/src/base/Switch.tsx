import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface SwitchProps
  extends Omit<JSX.ButtonHTMLAttributes<HTMLButtonElement>, "onChange" | "children"> {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  /** Accessible name, such as `Turn off Morning brief`. */
  label: string;
}

/**
 * An on and off switch (`role="switch"`), ink when on. Used for schedules. It keeps its own 36 by
 * 20 px size on every layout (`data-compact`): a phone's minimum control height would stretch the
 * track and leave the knob at its top. Put it in a label, which is the tap area.
 */
export function Switch(props: SwitchProps) {
  const [local, others] = splitProps(props, ["checked", "onCheckedChange", "label", "class"]);
  return (
    <button
      type="button"
      role="switch"
      aria-checked={local.checked}
      aria-label={local.label}
      data-compact="1"
      {...others}
      onClick={() => local.onCheckedChange(!local.checked)}
      class={cx(
        "flex w-9 h-5 p-0.5 rounded-full border-none transition-[background-color] duration-instant",
        local.checked ? "justify-end bg-ink" : "justify-start bg-border-strong",
        local.class,
      )}
    >
      <span class={cx("size-4 rounded-full", local.checked ? "bg-on-ink" : "bg-surface")} />
    </button>
  );
}
