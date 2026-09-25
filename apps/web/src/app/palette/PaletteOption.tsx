import { cx, Icon, Kbd } from "@marshal/ui";
import { For, Show } from "solid-js";
import type { PaletteCommand } from "./palette-results";

const ICON_PX = 16;

export interface PaletteOptionProps {
  item: PaletteCommand;
  /** Position in the result list, used for `data-pi`. */
  index: number;
  /** True for the first row of a group, which gets the group heading above it. */
  showGroup: boolean;
  selected: boolean;
  /** Phones hide the hint and the key hints. */
  phone: boolean;
  /** How the Cmd key is written on this system. */
  modKey: string;
  onHover: () => void;
  onRun: () => void;
}

/** One result: an optional group heading and the option row. */
export function PaletteOption(props: PaletteOptionProps) {
  const kbd = () => (props.item.kbd ?? []).map((k) => (k === "⌘" ? props.modKey : k));
  return (
    <>
      <Show when={props.showGroup}>
        <div class="pt-2 px-2.5 pb-1 text-caption leading-4 font-semibold text-secondary">
          {props.item.group}
        </div>
      </Show>
      <button
        type="button"
        role="option"
        aria-selected={props.selected}
        data-pi={props.index}
        onClick={() => props.onRun()}
        onMouseMove={() => props.onHover()}
        class={cx(
          "w-full flex items-center gap-2.5 min-h-9 px-2.5 border-none rounded-md text-left text-body",
          props.selected ? "bg-surface-selected" : "bg-transparent",
        )}
      >
        <span
          class={cx("inline-flex", !props.item.iconColor && "text-secondary")}
          style={props.item.iconColor ? { color: props.item.iconColor } : undefined}
        >
          <Icon name={props.item.icon} size={ICON_PX} />
        </span>
        <span class="flex-1 min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">
          {props.item.label}
        </span>
        <Show when={props.item.hint && !props.phone}>
          <span class="text-small text-muted whitespace-nowrap">{props.item.hint}</span>
        </Show>
        <Show when={props.item.kbd && !props.phone}>
          <span class="flex gap-0.5">
            <For each={kbd()}>
              {(key) => (
                <Kbd tone="strong" class="min-w-5 text-center leading-4.5!">
                  {key}
                </Kbd>
              )}
            </For>
          </span>
        </Show>
      </button>
    </>
  );
}
