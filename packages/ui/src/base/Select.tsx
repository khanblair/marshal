import { For, type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface SelectOption {
  value: string;
  /** Shown text. Defaults to the value. */
  label?: string;
  disabled?: boolean;
}

export interface SelectProps extends Omit<JSX.SelectHTMLAttributes<HTMLSelectElement>, "value"> {
  /** Options as objects, or plain strings used as both value and label. */
  options: readonly (SelectOption | string)[];
  /** The selected value. Marks the matching option `selected`. */
  value?: string;
  /** Red border, red bold text: a risky choice such as bypass permissions. */
  danger?: boolean;
}

const asOption = (option: SelectOption | string): SelectOption =>
  typeof option === "string" ? { value: option } : option;

/**
 * A native 32 px select with a strong border. Native events pass through, so
 * `onChange={(e) => ... e.currentTarget.value}` works as in the design.
 */
export function Select(props: SelectProps) {
  const [local, others] = splitProps(props, ["options", "value", "danger", "class"]);
  return (
    <select
      {...others}
      class={cx(
        "h-8 px-2 rounded-sm border bg-surface",
        local.danger
          ? "border-status-danger-solid text-status-danger-text font-semibold"
          : "border-border-strong text-primary",
        local.class,
      )}
    >
      <For each={local.options.map(asOption)}>
        {(option) => (
          <option
            value={option.value}
            selected={option.value === local.value}
            disabled={option.disabled}
          >
            {option.label ?? option.value}
          </option>
        )}
      </For>
    </select>
  );
}
