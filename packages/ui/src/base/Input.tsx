import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface InputProps extends JSX.InputHTMLAttributes<HTMLInputElement> {
  /** Red border and `aria-invalid`. */
  invalid?: boolean;
  /** Monospace 13 px text, for paths, URLs, branches, keys, and commands. */
  mono?: boolean;
}

/**
 * A 32 px text field with a strong border. Native events pass through, so
 * `onInput={(e) => ... e.currentTarget.value}` works as in the design. For
 * other heights or padding add classes such as `h-9!` or `px-2!`.
 */
export function Input(props: InputProps) {
  const [local, others] = splitProps(props, ["invalid", "mono", "class"]);
  return (
    <input
      aria-invalid={local.invalid ? "true" : undefined}
      {...others}
      class={cx(
        "h-8 px-2.5 rounded-sm border bg-surface",
        local.invalid ? "border-status-danger-solid" : "border-border-strong",
        local.mono && "font-mono text-small",
        local.class,
      )}
    />
  );
}
