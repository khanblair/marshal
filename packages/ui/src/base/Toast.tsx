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
 * The result of an action, on a raised surface like a menu or a dialog, so it follows the theme: a
 * message, an optional action, and a dismiss button. Stack toasts in a `ToastRegion`. (The design draws
 * it on ink, which turns black on a light page; the owner asked for it to follow the theme.)
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
        "pointer-events-auto flex items-center gap-3 max-w-[440px] min-h-10 py-1.5 pr-1.5 pl-3.5 rounded-md border border-border bg-surface-raised text-primary shadow-e2 text-body",
        local.class,
      )}
    >
      <span class="flex-1">{local.children}</span>
      <Show when={local.actionLabel}>
        <button
          type="button"
          onClick={() => local.onAction?.()}
          class="h-7 phone:h-(--control-h) px-2.5 rounded-sm border border-border-strong bg-surface text-primary font-semibold hover:bg-surface-hover"
        >
          {local.actionLabel}
        </button>
      </Show>
      <button
        type="button"
        aria-label={local.dismissLabel ?? "Dismiss"}
        onClick={() => local.onDismiss()}
        class="size-7 phone:size-(--control-h) inline-flex items-center justify-center p-0 border-none rounded-sm bg-transparent text-secondary hover:bg-surface-hover"
      >
        <Icon name="x" size={DISMISS_ICON_PX} />
      </button>
    </div>
  );
}
