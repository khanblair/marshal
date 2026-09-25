import { type JSX, Show, splitProps } from "solid-js";
import { cx } from "./cx";

export interface FieldProps extends Omit<JSX.LabelHTMLAttributes<HTMLLabelElement>, "title"> {
  /** The label above the control. */
  label: JSX.Element;
  /** Help text under the control. Hidden while there is an error. */
  hint?: JSX.Element;
  /** Error text under the control, in red. */
  error?: JSX.Element;
  /** Small secondary label with a 4 px gap, as the card session settings. */
  compact?: boolean;
}

/**
 * A `<label>` that stacks a label, the control (children), and a hint or an
 * error. Clicking the label focuses the control.
 */
export function Field(props: FieldProps) {
  const [local, others] = splitProps(props, [
    "label",
    "hint",
    "error",
    "compact",
    "class",
    "children",
  ]);
  return (
    // biome-ignore lint/a11y/noLabelWithoutControl: the control is passed as children and sits inside the label
    <label
      {...others}
      class={cx("flex flex-col", local.compact ? "gap-1 min-w-0" : "gap-1.5", local.class)}
    >
      <span
        class={local.compact ? "text-caption leading-4 text-secondary font-medium" : "font-medium"}
      >
        {local.label}
      </span>
      {local.children}
      <Show
        when={local.error}
        fallback={
          <Show when={local.hint}>
            <span class="text-small leading-4.5 text-secondary">{local.hint}</span>
          </Show>
        }
      >
        <span class="text-small text-status-danger-text">{local.error}</span>
      </Show>
    </label>
  );
}
