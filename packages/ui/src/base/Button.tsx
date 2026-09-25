import { type JSX, Show, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";
import { cx } from "./cx";
import { Kbd } from "./Kbd";

export type ButtonVariant = "primary" | "secondary" | "quiet" | "destructive";
/** Height in px, as in the design. */
export type ButtonSize = 28 | 32 | 36;

export interface ButtonProps extends JSX.ButtonHTMLAttributes<HTMLButtonElement> {
  /**
   * `primary`: ink fill. `secondary` (default): strong border on surface.
   * `quiet`: no border or fill, secondary text. `destructive`: bypass red fill.
   */
  variant?: ButtonVariant;
  /** 28 (13 px text), 32 (default), or 36. */
  size?: ButtonSize;
  /** `danger` turns the text red on secondary and quiet buttons. */
  tone?: "default" | "danger";
  /** Leading icon. */
  icon?: IconNameInput;
  /** Icon size in px. Default 14 at size 28, else 16. */
  iconSize?: number;
  /** Trailing keyboard hint, such as `N` or `Enter`. */
  kbd?: string;
  /** Sets `data-compact="1"`, which keeps the button short on touch screens. */
  compact?: boolean;
}

const DEFAULT_SIZE: ButtonSize = 32;
const SMALL_SIZE: ButtonSize = 28;
const SMALL_ICON_PX = 14;
const ICON_PX = 16;

const SIZES: Record<ButtonSize, string> = {
  28: "h-7 px-2.5 text-small",
  32: "h-8 px-3",
  36: "h-9 px-3.5",
};

const VARIANTS: Record<ButtonVariant, string> = {
  primary: "border border-ink bg-ink text-on-ink font-semibold hover:bg-ink-hover",
  secondary: "border border-border-strong bg-surface font-medium hover:bg-surface-hover",
  quiet: "border-none bg-transparent font-medium",
  destructive: "border border-bypass-bg bg-bypass-bg text-white font-semibold",
};

function toneClass(variant: ButtonVariant, danger: boolean): string | false {
  if (variant === "quiet") {
    return danger
      ? "text-status-danger-text hover:bg-status-danger-subtle"
      : "text-secondary hover:bg-surface-hover hover:text-primary";
  }
  return variant === "secondary" && danger && "text-status-danger-text";
}

/** A text button, optionally with a leading icon and a trailing key hint. */
export function Button(props: ButtonProps) {
  const [local, others] = splitProps(props, [
    "variant",
    "size",
    "tone",
    "icon",
    "iconSize",
    "kbd",
    "compact",
    "class",
    "children",
  ]);
  const variant = () => local.variant ?? "secondary";
  const size = () => local.size ?? DEFAULT_SIZE;
  const small = () => size() === SMALL_SIZE;
  const filled = () => variant() === "primary" || variant() === "destructive";
  return (
    <button
      type="button"
      data-compact={local.compact ? "1" : undefined}
      {...others}
      class={cx(
        "inline-flex items-center justify-center gap-1.5 rounded-sm disabled:opacity-50",
        SIZES[size()],
        VARIANTS[variant()],
        toneClass(variant(), local.tone === "danger"),
        local.class,
      )}
    >
      <Show when={local.icon}>
        {(name) => (
          <Icon name={name()} size={local.iconSize ?? (small() ? SMALL_ICON_PX : ICON_PX)} />
        )}
      </Show>
      {local.children}
      <Show when={local.kbd}>
        <Kbd tone={filled() ? "current" : "strong"} size={small() ? "sm" : "md"}>
          {local.kbd}
        </Kbd>
      </Show>
    </button>
  );
}
