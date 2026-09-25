import { type JSX, Show, splitProps } from "solid-js";
import { cx } from "../base/cx";
import { Scrim } from "./Scrim";

export interface SheetProps extends Omit<JSX.HTMLAttributes<HTMLDivElement>, "title"> {
  /** Heading under the grab handle. Also the dialog's accessible name. */
  title?: string;
  /** Called on a scrim click. Without it, no scrim is drawn. */
  onClose?: () => void;
  /** Draw the grab handle. Default true when there is a title. */
  grabber?: boolean;
}

/**
 * The phone bottom sheet: rounded top corners, raised surface, a grab handle,
 * and a heading, over a dimmed scrim. Used for the project picker and the More
 * menu on phones. Fill it with `MenuItem size={48} kind="plain"` rows.
 */
export function Sheet(props: SheetProps) {
  const [local, others] = splitProps(props, ["title", "onClose", "grabber", "class", "children"]);
  const grabber = () => local.grabber ?? !!local.title;
  return (
    <>
      <Show when={local.onClose}>
        <Scrim tone="sheet" fixed class="z-[290]" onClick={() => local.onClose?.()} />
      </Show>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={local.title}
        {...others}
        class={cx(
          "fixed left-0 right-0 bottom-0 z-sheet max-h-[85%] overflow-auto pt-2 px-3 pb-[calc(16px+env(safe-area-inset-bottom))] rounded-t-xl bg-surface-raised shadow-e2",
          local.class,
        )}
      >
        <Show when={grabber()}>
          <div class="w-9 h-1 mt-1 mx-auto mb-2.5 rounded-full bg-border-strong" />
        </Show>
        <Show when={local.title}>
          <h2 class="mt-0 mx-1 mb-2 text-subtitle leading-5.5 font-semibold">{local.title}</h2>
        </Show>
        {local.children}
      </div>
    </>
  );
}
