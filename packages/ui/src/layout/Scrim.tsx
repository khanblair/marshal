import { type JSX, splitProps } from "solid-js";
import { cx } from "../base/cx";

/**
 * `side` (30 %): sidebar overlay and tablet card panel. `sheet` (40 %): phone
 * sheets and the search palette. `dialog` (45 %): dialogs. `clear`: catches
 * clicks outside a popover without dimming.
 */
export type ScrimTone = "side" | "sheet" | "dialog" | "clear";

export interface ScrimProps extends Omit<JSX.HTMLAttributes<HTMLDivElement>, "children"> {
  /** Default `dialog`. */
  tone?: ScrimTone;
  /** Cover the viewport (`fixed`) instead of the positioned parent (`absolute`). */
  fixed?: boolean;
}

const TONES: Record<ScrimTone, string> = {
  side: "bg-scrim-side",
  sheet: "bg-scrim-sheet",
  dialog: "bg-scrim-dialog",
  clear: "bg-transparent",
};

/**
 * A full-cover layer behind an overlay. Set its layer with a class, for
 * example `z-scrim` or `z-[290]`, and close the overlay in `onClick`.
 */
export function Scrim(props: ScrimProps) {
  const [local, others] = splitProps(props, ["tone", "fixed", "class"]);
  return (
    <div
      aria-hidden="true"
      {...others}
      class={cx(
        local.fixed ? "fixed" : "absolute",
        "inset-0",
        TONES[local.tone ?? "dialog"],
        local.class,
      )}
    />
  );
}
