import { type JSX, splitProps } from "solid-js";
import { cx } from "./cx";

export interface ToastRegionProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** Phone layout: sit above the bottom navigation. */
  phone?: boolean;
}

/**
 * The live region that stacks `Toast`s at the bottom right, newest last.
 * Clicks pass through it except on the toasts.
 */
export function ToastRegion(props: ToastRegionProps) {
  const [local, others] = splitProps(props, ["phone", "class"]);
  return (
    <div
      aria-live="polite"
      {...others}
      class={cx(
        "absolute right-4 left-4 z-toast flex flex-col gap-2 items-end pointer-events-none",
        local.phone ? "bottom-[calc(72px+env(safe-area-inset-bottom))]" : "bottom-4",
        local.class,
      )}
    />
  );
}
