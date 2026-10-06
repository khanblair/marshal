import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface CheckboxProps extends Omit<JSX.InputHTMLAttributes<HTMLInputElement>, "type"> {
  /** 18 (default) or 16 (column picker). */
  size?: 16 | 18;
  /** Check color: `ink` (default) or `danger` (risk acknowledgements). */
  tone?: "ink" | "danger";
  /** `start` nudges the box down 1 px to line up with the first text line. */
  align?: "center" | "start";
}

const SMALL = 16;

/** The tick is two borders of an empty box, turned, so it takes its color from the theme. */
const TICK =
  "before:block before:rotate-45 before:scale-0 before:-translate-y-px before:transition-transform before:duration-instant checked:before:scale-100";

/**
 * A checkbox drawn from the tokens: a strong border on the surface, filled with ink (or red) and
 * ticked in the on-ink color when checked, so it looks the same on every phone and in both themes. It is
 * still a native input, so events, labels, keyboard, and the browser's own state pass through. Wrap it
 * in a `<label>` with its text, as the design does.
 */
export function Checkbox(props: CheckboxProps) {
  const [local, others] = splitProps(props, ["size", "tone", "align", "class"]);
  const small = () => local.size === SMALL;
  const danger = () => local.tone === "danger";
  return (
    <input
      type="checkbox"
      {...others}
      class={cx(
        "flex-none appearance-none m-0 grid place-content-center rounded-xs border border-border-strong bg-surface",
        "disabled:opacity-50",
        TICK,
        small() ? "size-4 before:w-1 before:h-2" : "size-4.5 before:w-[5px] before:h-[9px]",
        danger()
          ? "checked:border-status-danger-solid checked:bg-status-danger-solid before:border-r-2 before:border-b-2 before:border-white"
          : "checked:border-ink checked:bg-ink before:border-r-2 before:border-b-2 before:border-on-ink",
        local.align === "start" ? "mt-px" : "mt-0",
        local.class,
      )}
    />
  );
}
