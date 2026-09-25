import { type JSX, Show, splitProps } from "solid-js";
import { Icon } from "../icons/Icon";
import { cx } from "./cx";

export interface ToastProps extends JSX.HTMLAttributes<HTMLDivElement> {
  /** Label of the optional action button, such as `Undo` or `Open`. */
  actionLabel?: string;
  onAction?: () => void;
  onDismiss: () => void;
  /** Accessible name of the dismiss button. Default `Dismiss`. */
  dismissLabel?: string;
}

const DISMISS_ICON_PX = 14;

/**
 * The result of an action, on ink: a message, an optional action, and a
 * dismiss button. Stack toasts in a `ToastRegion`.
 */
export function Toast(props: ToastProps) {
  const [local, others] = splitProps(props, [
    "actionLabel",
    "onAction",
    "onDismiss",
    "dismissLabel",
    "class",
    "children",
  ]);
  return (
    <div
      role="status"
      {...others}
      class={cx(
        "pointer-events-auto flex items-center gap-3 max-w-[440px] min-h-10 py-1.5 pr-1.5 pl-3.5 rounded-md bg-ink text-on-ink shadow-e1 text-body",
        local.class,
      )}
    >
      <span class="flex-1">{local.children}</span>
      <Show when={local.actionLabel}>
        <button
          type="button"
          onClick={() => local.onAction?.()}
          class="h-7 px-2.5 rounded-sm border border-current bg-transparent text-inherit font-semibold"
        >
          {local.actionLabel}
        </button>
      </Show>
      <button
        type="button"
        aria-label={local.dismissLabel ?? "Dismiss"}
        onClick={() => local.onDismiss()}
        class="size-7 inline-flex items-center justify-center p-0 border-none rounded-sm bg-transparent text-inherit"
      >
        <Icon name="x" size={DISMISS_ICON_PX} />
      </button>
    </div>
  );
}
