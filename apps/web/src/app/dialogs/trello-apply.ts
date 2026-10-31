import type { ApplyTrelloRequest } from "@marshal/protocol";
import type { TrelloChangeView } from "~/data/mappers/trello";

/*
 * Which pending changes the Apply to Trello dialog will write. A change is picked or not, and a
 * change in conflict is written only when it is picked together with an override: the daemon skips a
 * conflict that is not named in `overrideIds`, so the dialog never offers a picked conflict without
 * one. Choices are kept by change id, so they survive a re-read while the dialog is open.
 */

export interface TrelloSelection {
  /** Every change id the dialog has already seen, so a new one gets its default and an old one keeps its choice. */
  known: ReadonlySet<string>;
  picked: ReadonlySet<string>;
  /** The picked changes that are in conflict and should be written anyway. Always inside `picked`. */
  overridden: ReadonlySet<string>;
}

/** A change starts ticked, unless Trello changed the same thing: that one starts off. */
const startsPicked = (change: TrelloChangeView): boolean => !change.conflict;

export function initialSelection(changes: readonly TrelloChangeView[]): TrelloSelection {
  return reconcileSelection(
    { known: new Set(), picked: new Set(), overridden: new Set() },
    changes,
  );
}

/** Keeps the choices of the changes still listed, drops the rest, and gives a new change its default. */
export function reconcileSelection(
  selection: TrelloSelection,
  changes: readonly TrelloChangeView[],
): TrelloSelection {
  const ids = new Set(changes.map((change) => change.id));
  const picked = new Set<string>();
  for (const change of changes) {
    const wasPicked = selection.known.has(change.id)
      ? selection.picked.has(change.id)
      : startsPicked(change);
    // A change that turned into a conflict since it was picked is not written unless it was overridden.
    const allowed = !change.conflict || selection.overridden.has(change.id);
    if (wasPicked && allowed) picked.add(change.id);
  }
  const overridden = new Set(
    [...selection.overridden].filter((id) => ids.has(id) && picked.has(id)),
  );
  return { known: ids, picked, overridden };
}

/** Ticks or unticks a change. Ticking a conflict also overrides it, and unticking drops both. */
export function togglePicked(
  selection: TrelloSelection,
  change: TrelloChangeView,
): TrelloSelection {
  return setPicked(selection, change, !selection.picked.has(change.id));
}

/** Picks a change (with its override when it is in conflict), or leaves it out. */
export function setPicked(
  selection: TrelloSelection,
  change: TrelloChangeView,
  on: boolean,
): TrelloSelection {
  const picked = new Set(selection.picked);
  const overridden = new Set(selection.overridden);
  if (on) picked.add(change.id);
  else picked.delete(change.id);
  if (on && change.conflict) overridden.add(change.id);
  else overridden.delete(change.id);
  return { ...selection, picked, overridden };
}

/** The body of the apply call: the picked changes in the order they are listed, and their overrides. */
export function applyRequest(
  selection: TrelloSelection,
  changes: readonly TrelloChangeView[],
): ApplyTrelloRequest {
  const chosen = changes.filter((change) => selection.picked.has(change.id));
  return {
    changeIds: chosen.map((change) => change.id),
    overrideIds: chosen.filter((change) => selection.overridden.has(change.id)).map((c) => c.id),
  };
}

/** How many changes the Apply button would write. */
export const pickedCount = (
  selection: TrelloSelection,
  changes: readonly TrelloChangeView[],
): number => changes.filter((change) => selection.picked.has(change.id)).length;
