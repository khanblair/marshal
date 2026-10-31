import { createEffect, createSignal, For, type JSX, on, Show, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import { cx } from "./cx";
import { Menu } from "./Menu";
import { MenuItem } from "./MenuItem";

export interface SelectOption {
  value: string;
  /** Shown text. Defaults to the value. */
  label?: string;
  disabled?: boolean;
}

/** What `onChange` receives: the chosen value, both as `value` and as `currentTarget.value`. */
export interface SelectChange {
  value: string;
  currentTarget: { value: string };
}

export interface SelectProps
  extends Omit<JSX.ButtonHTMLAttributes<HTMLButtonElement>, "value" | "onChange" | "options"> {
  /** Options as objects, or plain strings used as both value and label. */
  options: readonly (SelectOption | string)[];
  /** The selected value. Changing it moves the selection. */
  value?: string;
  /** Red border, red bold text: a risky choice such as bypass permissions. */
  danger?: boolean;
  /** The field name, so a form submits the chosen value. */
  name?: string;
  /** Called when the person picks a different option. */
  onChange?: (event: SelectChange) => void;
}

const asOption = (option: SelectOption | string): SelectOption =>
  typeof option === "string" ? { value: option } : option;

/**
 * A dropdown drawn by Marshal, not the browser's: a 32 px trigger with a
 * chevron, and a popover list of options with the current one checked. The
 * list uses the Menu and MenuItem pieces, so arrow keys, Home, End, Escape and
 * an outside press work as they do in every other menu. A hidden field carries
 * `name` for forms. `onChange` gets the value as `currentTarget.value`, as a
 * native select's does.
 */
export function Select(props: SelectProps) {
  const [local, others] = splitProps(props, [
    "options",
    "value",
    "danger",
    "name",
    "onChange",
    "class",
    "disabled",
  ]);
  const options = () => local.options.map(asOption);
  const [current, setCurrent] = createSignal(local.value ?? options()[0]?.value ?? "");
  createEffect(
    on(
      () => local.value,
      (value) => value !== undefined && setCurrent(value),
    ),
  );
  const [open, setOpen] = createSignal(false);
  let trigger: HTMLButtonElement | undefined;

  const label = () => options().find((option) => option.value === current())?.label ?? current();

  const pick = (option: SelectOption): void => {
    setOpen(false);
    trigger?.focus();
    if (option.value === current()) return;
    setCurrent(option.value);
    local.onChange?.({ value: option.value, currentTarget: { value: option.value } });
  };

  return (
    <span class="relative inline-flex min-w-0">
      <button
        {...others}
        ref={trigger}
        type="button"
        role="combobox"
        aria-haspopup="menu"
        aria-expanded={open()}
        disabled={local.disabled}
        onClick={() => setOpen(!open())}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" && !open()) {
            event.preventDefault();
            setOpen(true);
          }
        }}
        class={cx(
          "h-8 min-w-0 inline-flex items-center gap-2 px-2 rounded-sm border bg-surface text-left phone:h-(--control-h)",
          "disabled:opacity-50 disabled:cursor-not-allowed",
          local.danger
            ? "border-status-danger-solid text-status-danger-text font-semibold"
            : "border-border-strong text-primary",
          local.class,
        )}
      >
        <span class="min-w-0 truncate">{label()}</span>
        <Icon name="chevron-down" size={14} class="flex-none text-secondary" />
      </button>
      <input type="hidden" name={local.name} value={current()} />
      <Show when={open()}>
        <Menu
          class="absolute left-0 top-[calc(100%+2px)] min-w-full w-max max-h-72 overflow-auto z-[170]"
          trigger={() => trigger}
          onClose={() => {
            setOpen(false);
            trigger?.focus();
          }}
        >
          <For each={options()}>
            {(option) => (
              <MenuItem
                data-value={option.value}
                kind="radio"
                size={32}
                checked={option.value === current()}
                disabled={option.disabled}
                onClick={() => pick(option)}
              >
                {option.label ?? option.value}
              </MenuItem>
            )}
          </For>
        </Menu>
      </Show>
    </span>
  );
}
