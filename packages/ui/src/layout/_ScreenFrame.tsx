import { type JSX, splitProps } from "solid-js";
import { cx } from "../base/cx";

/* Private: the centered full-screen shell of ConnectionLost and SignIn. */

interface ScreenFrameProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** Phone layout: 16 px sides and room for the home bar. */
  phone?: boolean;
}

/**
 * Fills its container and centers a column of content at most 460 px wide, the same width as
 * a confirm dialog. The content is centered with auto margins and not `justify-center`, so a
 * window shorter than the content scrolls to the top of it and does not cut it off.
 */
export function ScreenFrame(props: ScreenFrameProps) {
  const [local, others] = splitProps(props, ["phone", "class", "children"]);
  return (
    <div
      {...others}
      class={cx(
        "flex-1 flex flex-col w-full h-full min-h-0 overflow-auto",
        local.phone ? "pt-4 px-4 pb-[calc(16px+env(safe-area-inset-bottom))]" : "p-6",
        local.class,
      )}
    >
      <div class="m-auto flex flex-col items-center gap-3.5 w-full max-w-[460px] text-center">
        {local.children}
      </div>
    </div>
  );
}
