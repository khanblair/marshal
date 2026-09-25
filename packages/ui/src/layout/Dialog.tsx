import { type JSX, onCleanup, onMount, splitProps } from "solid-js";
import { Dynamic } from "solid-js/web";
import { cx } from "../base/cx";
import { FOCUS_DELAY_MS, focusInitial, trapTab } from "./focus-trap";
import { Scrim } from "./Scrim";

/** Panel width in px on tablet and desktop. */
export type DialogWidth = 460 | 520 | 560;

export interface DialogProps extends Omit<JSX.HTMLAttributes<HTMLElement>, "role" | "onSubmit"> {
  /** 460 (default, confirm), 520 (remove project), or 560 (forms). */
  width?: DialogWidth;
  /** Phone layout: a bottom sheet instead of a centered panel. */
  phone?: boolean;
  /** Default `dialog`. Use `alertdialog` for confirmations. */
  role?: "dialog" | "alertdialog";
  /**
   * Called on a scrim click and on Escape inside the dialog. The dialog stops
   * that Escape, so the app's own Escape handler does not close a second layer.
   */
  onClose?: () => void;
  /** Makes the panel a `<form>` with this submit handler. */
  onSubmit?: JSX.EventHandler<HTMLFormElement, SubmitEvent>;
}

const WIDTHS: Record<DialogWidth, string> = {
  460: "w-[min(460px,calc(100%-24px))]",
  520: "w-[min(520px,calc(100%-24px))]",
  560: "w-[min(560px,calc(100%-24px))]",
};
const DEFAULT_WIDTH: DialogWidth = 460;

/**
 * A modal over a dimmed scrim: centered with 14 px corners on tablet and
 * desktop, a bottom sheet on phones. Tab stays inside. On open it focuses the
 * first element marked `data-autofocus`; on close it gives focus back.
 */
export function Dialog(props: DialogProps) {
  const [local, others] = splitProps(props, [
    "width",
    "phone",
    "role",
    "onClose",
    "onSubmit",
    "onKeyDown",
    "ref",
    "class",
  ]);
  let panel: HTMLElement | undefined;
  const opener = document.activeElement as HTMLElement | null;

  onMount(() => {
    const timer = setTimeout(() => panel && focusInitial(panel), FOCUS_DELAY_MS);
    onCleanup(() => clearTimeout(timer));
  });
  onCleanup(() => {
    if (opener?.isConnected) opener.focus();
  });

  const onKeyDown: JSX.EventHandler<HTMLElement, KeyboardEvent> = (event) => {
    if (typeof local.onKeyDown === "function") local.onKeyDown(event);
    if (panel) trapTab(event, panel);
    if (event.key === "Escape" && local.onClose) {
      event.preventDefault();
      event.stopPropagation();
      local.onClose();
    }
  };

  return (
    <>
      <Scrim tone="dialog" fixed class="z-scrim" onClick={() => local.onClose?.()} />
      <Dynamic
        component={(local.onSubmit ? "form" : "div") as "div"}
        ref={(el: HTMLDivElement) => {
          panel = el;
          if (typeof local.ref === "function") local.ref(el);
        }}
        role={local.role ?? "dialog"}
        aria-modal="true"
        onSubmit={local.onSubmit as JSX.EventHandler<HTMLElement, SubmitEvent> | undefined}
        {...others}
        onKeyDown={onKeyDown}
        class={cx(
          "fixed z-dialog overflow-auto flex flex-col gap-3.5 border border-border bg-surface-raised shadow-e2",
          local.phone
            ? "left-0 right-0 bottom-0 max-h-[92%] rounded-t-xl pt-5 px-4 pb-[calc(20px+env(safe-area-inset-bottom))]"
            : cx(
                "left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 max-h-[90%] rounded-xl p-5",
                WIDTHS[local.width ?? DEFAULT_WIDTH],
              ),
          local.class,
        )}
      />
    </>
  );
}
