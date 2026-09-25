import { For, type JSX, Show, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";
import { cx } from "./cx";

export interface SegmentOption<T extends string> {
  value: T;
  label: string;
  icon?: IconNameInput;
  title?: string;
  disabled?: boolean;
}

/** Segment height in px, as in the design. */
export type SegmentSize = 24 | 26 | 30 | 40;

export interface SegmentedControlProps<T extends string>
  extends Omit<JSX.HTMLAttributes<HTMLDivElement>, "onChange" | "role"> {
  options: readonly SegmentOption<T>[];
  value: T;
  onValueChange: (value: T) => void;
  /** Accessible name of the group. */
  label: string;
  /** `radio` (default): radiogroup of radios. `tabs`: tablist of tabs. */
  kind?: "radio" | "tabs";
  /** 24, 26 (default), 30, or 40 (touch screens). */
  size?: SegmentSize;
  /** Segments share the width and center their content. */
  fill?: boolean;
  /** Sets `data-compact="1"` on each segment. */
  compact?: boolean;
  /** Text color of unselected segments: `secondary` (default) or `primary`. */
  unselectedTone?: "secondary" | "primary";
  /** Extra classes for every segment, for example `px-3!`. */
  segmentClass?: string;
}

const SIZES: Record<SegmentSize, string> = { 24: "h-6", 26: "h-6.5", 30: "h-7.5", 40: "h-10" };
const DEFAULT_SIZE: SegmentSize = 26;
const ICON_PX = 14;
const NEXT_KEYS = new Set(["ArrowRight", "ArrowDown"]);
const PREV_KEYS = new Set(["ArrowLeft", "ArrowUp"]);

/** Index of the next enabled option in a direction, wrapping around. */
function step(options: readonly { disabled?: boolean }[], from: number, direction: 1 | -1): number {
  for (let i = 1; i <= options.length; i++) {
    const at = (from + direction * i + options.length) % options.length;
    if (!options[at]?.disabled) return at;
  }
  return from;
}

function arrowDirection(key: string): 1 | -1 | 0 {
  if (NEXT_KEYS.has(key)) return 1;
  return PREV_KEYS.has(key) ? -1 : 0;
}

function stateClass(selected: boolean, unselectedTone: "secondary" | "primary" | undefined) {
  if (selected) return "border-border-strong bg-surface text-primary";
  const text = unselectedTone === "primary" ? "text-primary" : "text-secondary";
  return `border-transparent bg-transparent hover:text-primary ${text}`;
}

type SegmentLook = Pick<
  SegmentedControlProps<string>,
  "size" | "fill" | "compact" | "unselectedTone" | "segmentClass"
>;

interface SegmentProps {
  option: SegmentOption<string>;
  selected: boolean;
  tabs: boolean;
  look: SegmentLook;
  ref: (el: HTMLButtonElement) => void;
  onSelect: () => void;
  onKeyDown: (event: KeyboardEvent) => void;
}

function Segment(props: SegmentProps) {
  return (
    // biome-ignore lint/a11y/useAriaPropsSupportedByRole: the role is always tab or radio, which take aria-selected and aria-checked
    <button
      ref={props.ref}
      type="button"
      role={props.tabs ? "tab" : "radio"}
      aria-selected={props.tabs ? props.selected : undefined}
      aria-checked={props.tabs ? undefined : props.selected}
      data-compact={props.look.compact ? "1" : undefined}
      title={props.option.title}
      disabled={props.option.disabled}
      onClick={() => props.onSelect()}
      onKeyDown={(event) => props.onKeyDown(event)}
      class={cx(
        "inline-flex items-center gap-1.5 px-2.5 rounded-sm border text-small font-medium",
        SIZES[props.look.size ?? DEFAULT_SIZE],
        props.look.fill ? "flex-1 justify-center" : "flex-none",
        stateClass(props.selected, props.look.unselectedTone),
        props.look.segmentClass,
      )}
    >
      <Show when={props.option.icon}>{(name) => <Icon name={name()} size={ICON_PX} />}</Show>
      <span>{props.option.label}</span>
    </button>
  );
}

/**
 * A sunken track of options with the selected one raised. Used for the view
 * tabs, chart range, calendar layout, chat or terminal, theme, and the new
 * project source. Arrow keys move the selection; every segment stays in the
 * tab order, as in the design.
 */
export function SegmentedControl<T extends string>(props: SegmentedControlProps<T>) {
  const [local, others] = splitProps(props, [
    "options",
    "value",
    "onValueChange",
    "label",
    "kind",
    "size",
    "fill",
    "compact",
    "unselectedTone",
    "segmentClass",
    "class",
  ]);
  const buttons: HTMLButtonElement[] = [];
  const move = (event: KeyboardEvent, index: number) => {
    const direction = arrowDirection(event.key);
    if (!direction) return;
    event.preventDefault();
    const next = step(local.options, index, direction);
    const option = local.options[next];
    if (option) local.onValueChange(option.value);
    buttons[next]?.focus();
  };
  return (
    // biome-ignore lint/a11y/useAriaPropsSupportedByRole: the role is always tablist or radiogroup, which take a label
    <div
      role={local.kind === "tabs" ? "tablist" : "radiogroup"}
      aria-label={local.label}
      {...others}
      class={cx(
        "flex gap-0.5 p-0.5 rounded-md bg-surface-sunken border border-border",
        local.class,
      )}
    >
      <For each={local.options}>
        {(option, index) => (
          <Segment
            option={option}
            selected={option.value === local.value}
            tabs={local.kind === "tabs"}
            look={local}
            ref={(el) => {
              buttons[index()] = el;
            }}
            onSelect={() => local.onValueChange(option.value)}
            onKeyDown={(event) => move(event, index())}
          />
        )}
      </For>
    </div>
  );
}
