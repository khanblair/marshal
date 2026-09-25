import { type JSX, Show, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import type { IconNameInput } from "../icons/icon-names";
import { type ToneKey, toneSubtleBg, toneText } from "../icons/status";
import { cx } from "./cx";

/**
 * A status tone (subtle fill with that tone's text), or:
 * `selected`: selected fill, secondary text (column and tab counts).
 * `outline`: light border, secondary text (package, Starter).
 * `bypass`: bypass red with white text.
 */
export type BadgeTone = ToneKey | "selected" | "outline" | "bypass";
/** Height in px, as in the design. */
export type BadgeSize = 18 | 20 | 22 | 24;

export interface BadgeProps extends JSX.HTMLAttributes<HTMLSpanElement> {
  /** Default `neutral` (sunken fill, secondary text). */
  tone?: BadgeTone;
  /** 18, 20 (default), 22, or 24. */
  size?: BadgeSize;
  /** Leading icon at 12 px, in the text color. For a colored state icon, pass a `StatusIcon` child. */
  icon?: IconNameInput;
}

const SIZES: Record<BadgeSize, string> = {
  18: "h-4.5 px-1.5",
  20: "h-5 px-1.5",
  22: "h-5.5 px-2",
  24: "h-6 px-2",
};

const EXTRA_TONES: Record<"selected" | "outline" | "bypass", string> = {
  selected: "bg-surface-selected text-secondary",
  outline: "border border-border text-secondary",
  bypass: "bg-bypass-bg text-bypass-text",
};

const DEFAULT_SIZE: BadgeSize = 20;
const ICON_PX = 12;

function toneClass(tone: BadgeTone): string {
  if (tone === "selected" || tone === "outline" || tone === "bypass") return EXTRA_TONES[tone];
  return `${toneSubtleBg[tone]} ${toneText[tone]}`;
}

/**
 * A small label or count with square corners: the card state pill, provider
 * key state, bypass and package labels, and column counts.
 */
export function Badge(props: BadgeProps) {
  const [local, others] = splitProps(props, ["tone", "size", "icon", "class", "children"]);
  return (
    <span
      {...others}
      class={cx(
        "inline-flex items-center justify-center gap-1 rounded-xs text-caption font-semibold",
        SIZES[local.size ?? DEFAULT_SIZE],
        toneClass(local.tone ?? "neutral"),
        local.class,
      )}
    >
      <Show when={local.icon}>{(name) => <Icon name={name()} size={ICON_PX} />}</Show>
      {local.children}
    </span>
  );
}
