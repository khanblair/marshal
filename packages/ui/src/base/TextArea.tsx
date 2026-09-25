import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface TextAreaProps extends JSX.TextareaHTMLAttributes<HTMLTextAreaElement> {
  /** Red border and `aria-invalid`. */
  invalid?: boolean;
  /** Monospace 13 px text on 20 px lines with 10 px padding (card notes). */
  mono?: boolean;
}

/**
 * A multi-line field with a strong border that resizes vertically. Native
 * events pass through. For the borderless chat input, use `Composer`.
 */
export function TextArea(props: TextAreaProps) {
  const [local, others] = splitProps(props, ["invalid", "mono", "class"]);
  return (
    <textarea
      aria-invalid={local.invalid ? "true" : undefined}
      {...others}
      class={cx(
        "rounded-sm border bg-surface resize-y",
        local.invalid ? "border-status-danger-solid" : "border-border-strong",
        local.mono ? "p-2.5 font-mono text-small leading-5" : "py-2 px-2.5",
        local.class,
      )}
    />
  );
}
