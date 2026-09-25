import { Toast, ToastRegion } from "@marshal/ui";
import { For } from "solid-js";
import { M, type Toast as ToastModel } from "~/mock";

/** Running a toast's action also dismisses it, as the design does. */
function runAction(toast: ToastModel): void {
  toast.dismiss();
  toast.action?.run();
}

/** The toast stack, and the polite live region that announces state changes to screen readers. */
export function Toasts() {
  return (
    <>
      <ToastRegion phone={M.mobile}>
        <For each={M.S.toasts}>
          {(toast) => (
            <Toast
              actionLabel={toast.action?.label}
              onAction={() => runAction(toast)}
              onDismiss={toast.dismiss}
            >
              {toast.msg}
            </Toast>
          )}
        </For>
      </ToastRegion>
      <div aria-live="polite" class="absolute w-px h-px overflow-hidden [clip:rect(0_0_0_0)]">
        {M.S.announce}
      </div>
    </>
  );
}
