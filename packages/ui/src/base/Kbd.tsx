import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export type KbdTone = "default" | "muted" | "strong" | "key" | "current";

export interface KbdProps extends JSX.HTMLAttributes<HTMLElement> {
  /**
   * `default`: light border, secondary text (search box).
   * `muted`: light border, muted text (filter field hint).
   * `strong`: strong border, secondary text (search results, Deny).
   * `key`: strong border on a sunken fill (shortcuts list).
   * `current`: border in the text color, faded (inside filled buttons).
   */
  tone?: KbdTone;
  /** `md` is 12 px text. `sm` is 11 px, for 28 px buttons. Default `md`. */
  size?: "sm" | "md";
}

const TONES: Record<KbdTone, string> = {
  default: "border-border text-secondary",
  muted: "border-border text-muted",
  strong: "border-border-strong text-secondary",
  key: "border-border-strong bg-surface-sunken",
  current: "border-current",
};

/** A keyboard key hint, such as `⌘` `K` or `Esc`. */
export function Kbd(props: KbdProps) {
  const [local, others] = splitProps(props, ["tone", "size", "class"]);
  const tone = () => local.tone ?? "default";
  const small = () => local.size === "sm";
  return (
    <kbd
      {...others}
      class={cx(
        "font-sans rounded-xs border",
        small() ? "text-badge leading-3.5 px-0.75" : "text-caption leading-4 px-1",
        TONES[tone()],
        tone() === "current" && (small() ? "opacity-60" : "opacity-70"),
        local.class,
      )}
    />
  );
}
