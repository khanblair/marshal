import { Button, Checkbox, Dialog } from "@marshal/ui";
import { Show } from "solid-js";
import { type Dialog as DialogModel, M } from "~/mock";

const closeDialog = (): void => M.closeDialog();

/** Runs the confirmed action, unless the acknowledgement it asks for is still unchecked. */
function confirmDialog(dialog: DialogModel | null): void {
  if (!dialog || (dialog.ack && !dialog.acked)) return;
  M.set({ dialog: null });
  dialog.run();
}

/**
 * The confirmation dialog behind `M.confirm(...)`. Cancel has focus when it opens. A dialog with
 * an acknowledgement keeps its button disabled until the box is checked. Renders nothing while
 * `M.S.dialog` is empty.
 */
export function ConfirmDialog() {
  return (
    <Show when={M.S.dialog}>
      {(dialog) => {
        const blocked = () => !!dialog().ack && !dialog().acked;
        return (
          <Dialog
            role="alertdialog"
            phone={M.mobile}
            width={460}
            aria-labelledby="dlg-title"
            onClose={closeDialog}
          >
            <h2 id="dlg-title" class="m-0 text-title leading-6 font-semibold">
              {dialog().title}
            </h2>
            <p class="m-0 text-secondary">{dialog().message}</p>
            <Show when={dialog().ack}>
              {/* biome-ignore lint/a11y/noLabelWithoutControl: the checkbox component renders the input inside this label */}
              <label class="flex gap-2.5 items-start p-3 rounded-md border border-border-strong bg-status-danger-subtle text-status-danger-text font-medium cursor-pointer">
                <Checkbox
                  tone="danger"
                  align="start"
                  checked={dialog().acked}
                  onChange={() => {
                    dialog().acked = !dialog().acked;
                  }}
                />
                <span>{dialog().ack}</span>
              </label>
            </Show>
            <div class="flex justify-end flex-wrap gap-2 mt-1">
              <Button data-autofocus onClick={closeDialog}>
                Cancel
              </Button>
              <Button
                variant={dialog().destructive ? "destructive" : "primary"}
                class={dialog().destructive ? undefined : "hover:bg-ink!"}
                disabled={blocked()}
                onClick={() => confirmDialog(M.S.dialog)}
              >
                {dialog().action}
              </Button>
            </div>
          </Dialog>
        );
      }}
    </Show>
  );
}
