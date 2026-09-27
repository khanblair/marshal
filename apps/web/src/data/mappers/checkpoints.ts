import type { CheckpointList, Checkpoint as WireCheckpoint } from "@marshal/protocol";
import { toMillis } from "./time";

/*
 * A card's restore points (section S10, docs/backend-checklist B5.3): the wire's `Checkpoint`, with
 * its time already in milliseconds, which is the shape the store keeps.
 *
 * There is no checkpoint to edit and none to make from the screens: a restore point is Marshal's own
 * commit, made before a turn, and the one thing a person does with one is put the card back to it.
 * So this maps a read and nothing else, and the restore route answers the card, not a checkpoint.
 */

/** One restore point as the store holds it. */
export interface CheckpointRow {
  id: string;
  cardId: string;
  /** The commit the restore point names, full length. A screen shows the short form of it. */
  sha: string;
  /** What the restore point was made before ("before turn 3"), or empty when there is nothing to say. */
  label: string;
  /** When Marshal made it, in ms on this device's clock. */
  at: number;
}

/** One wire checkpoint as the store holds it. */
export function toCheckpointRow(cp: WireCheckpoint): CheckpointRow {
  return { id: cp.id, cardId: cp.cardId, sha: cp.sha, label: cp.label, at: toMillis(cp.createdAt) };
}

/** Every restore point of a card, newest first, in the order the daemon sent them. */
export function toCheckpointRows(list: CheckpointList): CheckpointRow[] {
  return list.checkpoints.map(toCheckpointRow);
}
