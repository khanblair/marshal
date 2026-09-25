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

/**
 * A native checkbox in the ink accent color. Native events pass through.
 * Wrap it in a `<label>` with its text, as the design does.
 */
export function Checkbox(props: CheckboxProps) {
  const [local, others] = splitProps(props, ["size", "tone", "align", "class"]);
  return (
    <input
      type="checkbox"
      {...others}
      class={cx(
        "flex-none mx-0 mb-0",
        local.size === SMALL ? "size-4" : "size-4.5",
        local.align === "start" ? "mt-px" : "mt-0",
        local.tone === "danger" ? "accent-status-danger-solid" : "accent-ink",
        local.class,
      )}
    />
  );
}
