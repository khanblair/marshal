import { Button, Dialog } from "@marshal/ui";
import { createEffect, createMemo, createSignal, createUniqueId, Show, untrack } from "solid-js";
import {
  applyLabel,
  groupByCard,
  type TrelloApplyReport,
  type TrelloChangeView,
} from "~/data/mappers/trello";
import { M } from "~/mock";
import { DialogHeader } from "./DialogHeader";
import { TrelloChangeList, type TrelloRowActions } from "./TrelloChangeRows";
import { TrelloResult } from "./TrelloResult";
import {
  applyRequest,
  initialSelection,
  pickedCount,
  reconcileSelection,
  setPicked,
  togglePicked,
} from "./trello-apply";

const KEPT_NOTE =
  "Kept Trello's version of this card's title, description, and ticks. This change is not one of those, so it is still pending. Skip it to hide it.";

/** The card's key in the store, such as "web#118". A card the store does not have has none. */
function keyOf(cardId: string): string {
  return M.S.cards.find((card) => card.daemonId === cardId || card.id === cardId)?.id ?? "";
}

/** The board, and when Marshal last read it. Each fact is its own item, never joined with dots. */
function Source(props: { board: string; fetchedAt: number | null }) {
  return (
    <p class="m-0 flex flex-wrap gap-x-3 text-small text-secondary">
      <Show when={props.board}>
        <span>Board: {props.board}</span>
      </Show>
      <span>
        {props.fetchedAt === null ? "Not read yet" : `Last read ${M.rel(props.fetchedAt)}`}
      </span>
    </p>
  );
}

/** The dialog's state: what is picked, whether a call is running, the notes, and the report. */
function createApplyState(pid: string) {
  const changes = (): readonly TrelloChangeView[] => M.S.trello?.[pid]?.pending?.changes ?? [];
  const [selection, setSelection] = createSignal(initialSelection(untrack(changes)));
  createEffect(() => setSelection((prev) => reconcileSelection(prev, changes())));
  const [busy, setBusy] = createSignal(false);
  const [report, setReport] = createSignal<TrelloApplyReport | null>(null);
  const [notes, setNotes] = createSignal<Record<string, string>>({});
  return { changes, selection, setSelection, busy, setBusy, report, setReport, notes, setNotes };
}

type ApplyState = ReturnType<typeof createApplyState>;

/** Runs one call with the dialog's buttons off, then puts focus back inside the dialog. */
async function guarded(state: ApplyState, focus: () => void, run: () => Promise<void>) {
  state.setBusy(true);
  try {
    await run();
  } finally {
    state.setBusy(false);
    focus();
  }
}

/** What the rows can do: choose, and the three ways out of a conflict. */
function rowActions(pid: string, state: ApplyState, focus: () => void): TrelloRowActions {
  return {
    isPicked: (id) => state.selection().picked.has(id),
    isOverridden: (id) => state.selection().overridden.has(id),
    toggle: (change) => state.setSelection((prev) => togglePicked(prev, change)),
    setOverride: (change, on) => state.setSelection((prev) => setPicked(prev, change, on)),
    skip: (change) =>
      void guarded(state, focus, async () => {
        await M.dismissTrelloChanges(pid, [change.id]);
      }),
    keepTrello: (change) =>
      void guarded(state, focus, async () => {
        const done = await M.refreshFromTrello(pid, [change.cardId]);
        if (done && state.changes().some((one) => one.id === change.id)) {
          state.setNotes((prev) => ({ ...prev, [change.id]: KEPT_NOTE }));
        }
      }),
    busy: state.busy,
    note: (id) => state.notes()[id] ?? "",
    keyOf,
  };
}

/** The footer: Cancel and the count, or Close when there is nothing, or Done once it was written. */
function Footer(props: { state: ApplyState; onApply: () => void }) {
  const close = () => M.closeTrelloApply();
  const count = () => pickedCount(props.state.selection(), props.state.changes());
  return (
    <div data-trello-footer class="flex flex-none flex-wrap justify-end gap-2">
      <Show
        when={!props.state.report()}
        fallback={
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        }
      >
        <Show
          when={props.state.changes().length > 0}
          fallback={<Button onClick={close}>Close</Button>}
        >
          <Button onClick={close} disabled={props.state.busy()}>
            Cancel
          </Button>
          <Button
            variant="primary"
            icon={props.state.busy() ? "spinner" : undefined}
            disabled={count() === 0 || props.state.busy()}
            onClick={props.onApply}
          >
            {props.state.busy() ? "Applying" : applyLabel(count())}
          </Button>
        </Show>
      </Show>
    </div>
  );
}

function ApplyForm(props: { pid: string }) {
  const titleId = createUniqueId();
  const descId = createUniqueId();
  const state = createApplyState(props.pid);
  const view = () => M.S.trello?.[props.pid]?.pending ?? null;
  const groups = createMemo(() => groupByCard(state.changes()));
  let region: HTMLElement | undefined;
  // After a change leaves the list its button is gone, so focus goes to the list, or to the footer.
  const focusList = () => {
    const footer = "[data-trello-footer] button:not([disabled])";
    (region?.isConnected ? region : document.querySelector<HTMLElement>(footer))?.focus();
  };
  const actions = rowActions(props.pid, state, focusList);

  async function apply(): Promise<void> {
    const request = applyRequest(state.selection(), state.changes());
    if (request.changeIds.length === 0) return;
    state.setBusy(true);
    try {
      const done = await M.applyTrelloChanges(props.pid, request);
      if (done) state.setReport(done);
    } finally {
      state.setBusy(false);
    }
  }

  return (
    <Dialog
      phone={M.mobile}
      width={560}
      aria-labelledby={titleId}
      aria-describedby={descId}
      onClose={() => M.closeTrelloApply()}
    >
      <DialogHeader id={titleId} title="Apply to Trello" onClose={() => M.closeTrelloApply()} />
      <Source board={view()?.boardName ?? ""} fetchedAt={view()?.fetchedAt ?? null} />
      <Show
        when={state.report()}
        fallback={
          <>
            <p id={descId} class="m-0 text-secondary">
              <Show when={state.changes().length > 0} fallback="Nothing to send to Trello.">
                Marshal never writes to Trello by itself. Pick what to send. Everything is written
                as you, and Trello never shows the agent's name.
              </Show>
            </p>
            <Show when={state.changes().length > 0}>
              <section
                ref={region}
                aria-label="Changes to send to Trello"
                tabindex="0"
                data-autofocus
                class="min-h-0 flex-1 overflow-y-auto"
              >
                <TrelloChangeList groups={groups()} actions={actions} />
              </section>
            </Show>
          </>
        }
      >
        {(report) => (
          <div class="min-h-0 flex-1 overflow-y-auto">
            <TrelloResult report={report()} headingId={descId} />
          </div>
        )}
      </Show>
      <Footer state={state} onApply={() => void apply()} />
    </Dialog>
  );
}

/**
 * The Apply to Trello dialog: the changes Marshal made since it last read the linked board, grouped
 * by card, with a box on each. Nothing reaches Trello until the person presses Apply, and what was
 * written and what was left is shown after. A bottom sheet on phones. Draws nothing while
 * `M.S.trelloApply` is empty.
 */
export function TrelloApplyDialog() {
  return <Show when={M.S.trelloApply}>{(open) => <ApplyForm pid={open().pid} />}</Show>;
}
