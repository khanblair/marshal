import { type JSX, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";
import { cx } from "./cx";

/** Square size in px, as in the design. */
export type IconButtonSize = 24 | 28 | 32 | 36 | 44;
export type IconButtonVariant = "ghost" | "outline" | "primary";

export interface IconButtonProps
  extends Omit<JSX.ButtonHTMLAttributes<HTMLButtonElement>, "children"> {
  /** Accessible name. Sets `aria-label`. */
  label: string;
  icon: IconNameInput;
  /** 24, 28 (default), 32, 36, or 44. */
  size?: IconButtonSize;
  /** Icon size in px. Default 16, or 12 at size 24. */
  iconSize?: number;
  /**
   * `ghost` (default): no border or fill until hover. `outline`: strong border
   * on surface. `primary`: ink fill, as the send button.
   */
  variant?: IconButtonVariant;
  /**
   * Text color of a ghost button. `secondary` (default), `muted` (row menus,
   * which hover to the selected fill), `default` (inherits), or `danger` (delete and remove).
   */
  tone?: "secondary" | "muted" | "default" | "danger";
  /** Sets `data-compact="1"`, which keeps the button small on touch screens. */
  compact?: boolean;
}

const DEFAULT_SIZE: IconButtonSize = 28;
const SMALL_SIZE: IconButtonSize = 24;
const SMALL_ICON_PX = 12;
const ICON_PX = 16;

/** The same radii as `Button`: 7 px up to 32, 10 px at 36 and above and on touch screens. */
const SIZES: Record<IconButtonSize, string> = {
  24: "size-6 rounded-xs phone:size-(--control-h)",
  28: "size-7 rounded-md phone:size-(--control-h)",
  32: "size-8 rounded-md phone:size-(--control-h)",
  36: "size-9 rounded-lg phone:size-(--control-h)",
  44: "size-11 rounded-lg phone:size-(--control-h)",
};

const TOUCH_RADIUS = "[[data-touch='1']_&]:rounded-lg";

const GHOST_TONES = {
  secondary: "text-secondary hover:bg-surface-hover",
  muted: "text-muted hover:bg-surface-selected hover:text-primary",
  default: "hover:bg-surface-hover",
  danger: "text-status-danger-text hover:bg-status-danger-subtle",
} as const;

function variantClass(variant: IconButtonVariant, tone: keyof typeof GHOST_TONES): string {
  if (variant === "outline") {
    const base = "border border-border-strong bg-surface disabled:opacity-50";
    // A danger outline button keeps its border and surface, and turns its icon red.
    return tone === "danger"
      ? `${base} text-status-danger-text hover:bg-status-danger-subtle`
      : `${base} hover:bg-surface-hover`;
  }
  if (variant === "primary") return "border border-ink bg-ink text-on-ink disabled:opacity-40";
  return `border-none bg-transparent disabled:opacity-50 ${GHOST_TONES[tone]}`;
}

/** A square button with only an icon. `label` is required for screen readers. */
export function IconButton(props: IconButtonProps) {
  const [local, others] = splitProps(props, [
    "label",
    "icon",
    "size",
    "iconSize",
    "variant",
    "tone",
    "compact",
    "class",
  ]);
  const size = () => local.size ?? DEFAULT_SIZE;
  return (
    <button
      type="button"
      aria-label={local.label}
      data-compact={local.compact ? "1" : undefined}
      {...others}
      class={cx(
        "inline-flex flex-none items-center justify-center p-0",
        SIZES[size()],
        size() !== SMALL_SIZE && TOUCH_RADIUS,
        variantClass(local.variant ?? "ghost", local.tone ?? "secondary"),
        local.class,
      )}
    >
      <Icon
        name={local.icon}
        size={local.iconSize ?? (size() === SMALL_SIZE ? SMALL_ICON_PX : ICON_PX)}
      />
    </button>
  );
}
