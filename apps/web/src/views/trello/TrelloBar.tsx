import { Badge, Button, IconButton, IconLabel } from "@marshal/ui";
import { createSignal, Show } from "solid-js";
import { M } from "~/mock";
import { refreshWords } from "~/sync/trello-flow";

/** The strip above a linked project's board: the board, and the two things to press for Trello. */
function Actions(props: { pid: string; pending: number }) {
  const [busy, setBusy] = createSignal(false);
  async function refresh(): Promise<void> {
    setBusy(true);
    try {
      const done = await M.refreshFromTrello(props.pid);
      if (done) M.toast(refreshWords(done));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div class="flex flex-none items-center gap-2">
      <Show
        when={M.mobile}
        fallback={
          <Button
            icon={busy() ? "spinner" : "repeat"}
            disabled={busy()}
            onClick={() => void refresh()}
          >
            Refresh from Trello
          </Button>
        }
      >
        <IconButton
          label="Refresh from Trello"
          title="Refresh from Trello"
          icon={busy() ? "spinner" : "repeat"}
          variant="outline"
          disabled={busy()}
          onClick={() => void refresh()}
        />
      </Show>
      <Button icon="trello" onClick={() => M.openTrelloApply(props.pid)}>
        Apply to Trello
        <Show when={props.pending > 0}>
          <Badge tone="selected" size={18} aria-label={`${props.pending} pending`}>
            {props.pending}
          </Badge>
        </Show>
      </Button>
    </div>
  );
}

/**
 * The Trello strip of a project's board. It draws only for a project linked to a Trello board, so a
 * project with none looks as it always did. Nothing here writes to Trello: Apply opens the dialog
 * where the person picks what to send.
 */
export function TrelloBar(props: { pid: string }) {
  const slot = () => M.S.trello?.[props.pid];
  const pending = () => (slot()?.pending?.linked ? slot()?.pending : null);
  return (
    <Show when={pending()}>
      {(view) => (
        <div
          data-trello-bar
          class="flex flex-none flex-wrap items-center gap-x-3 gap-y-2 border-b border-border bg-surface px-4 py-2"
        >
          <IconLabel icon="trello" class="min-w-0 flex-1 text-small text-secondary">
            <span class="truncate">{view().boardName}</span>
          </IconLabel>
          <Show when={view().fetchedAt !== null}>
            <span class="text-caption text-secondary">
              Last read {M.rel(view().fetchedAt ?? 0)}
            </span>
          </Show>
          <Actions pid={props.pid} pending={view().changes.length} />
          <Show when={slot()?.error}>
            {(message) => (
              <div
                role="status"
                class="flex basis-full items-center gap-2 text-small text-secondary"
              >
                <span class="min-w-0 flex-1">{message()}</span>
                <Button size={28} onClick={() => void M.reloadTrello(props.pid)}>
                  Try again
                </Button>
              </div>
            )}
          </Show>
        </div>
      )}
    </Show>
  );
}
