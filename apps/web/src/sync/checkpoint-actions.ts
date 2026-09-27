import type { ApiClient } from "~/data/api-client";
import { toCheckpointRows } from "~/data/mappers/checkpoints";
import { ChangeInFlightError } from "~/data/optimistic";
import { isDaemon } from "~/data/sections";
import type { CardKey } from "~/mock/card-key";
import { type Ctx, sectionsOf } from "~/mock/context";
import { toast } from "~/mock/engine";
import { applyCard } from "./cards";

/*
 * A card's restore points and the one write a person makes with them (B5.3, section S10).
 *
 * A checkpoint is Marshal's own commit, made before a turn, so the screens never make one: the
 * activity tab lists the daemon's and puts the card back to one. Restoring is refused while the
 * card's agent is running a turn, and the daemon's own sentence is what the person reads.
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

/** True while a card's activity, and so its restore points, is the daemon's. */
export function checkpointsOnDaemon(ctx: Ctx): boolean {
  return isDaemon("S10", sectionsOf(ctx.env));
}

/**
 * Reads a card's restore points into the store, newest first. Twice over is fine: the list is small
 * and a read that arrives after the card moved on is dropped.
 */
export async function readCardCheckpoints(
  ctx: Ctx,
  api: ApiClient,
  key: CardKey,
  daemonId: string,
): Promise<void> {
  const list = await api.checkpoints(daemonId);
  // The card may have been closed, or replaced by another with the same key, while the daemon
  // answered. Writing then would put one card's restore points under another's row.
  if (!ctx.S.cards.some((one) => one.id === key && one.daemonId === daemonId)) return;
  ctx.S.checkpoints[key] = toCheckpointRows(list);
}

/**
 * Puts a card's worktree and branch back to one of its restore points. The card the daemon answers
 * is drawn, and the restore points are read again: a restore does not change them, but a card that
 * was mid-turn when the list was read may have gained one since.
 *
 * False means nothing was asked, or the daemon said no; either way the reason has already been
 * shown. The change is named after the card, so two restores of one card cannot overlap.
 */
export async function restoreCheckpoint(
  ctx: Ctx,
  key: CardKey,
  checkpointId: string,
): Promise<boolean> {
  const card = ctx.S.cards.find((one) => one.id === key);
  const daemonId = card?.daemonId ?? "";
  if (!card || !daemonId) return false;
  const api = ctx.env.data?.api;
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return false;
  }
  try {
    await ctx.optimistic({
      key: `checkpoint:${key}`,
      // A restore draws nothing before the daemon answers: the card's state is the daemon's to
      // report, and the worktree is not on the screen.
      apply: () => undefined,
      request: async () => {
        applyCard(ctx, await api.restoreCheckpoint(daemonId, checkpointId));
        await readCardCheckpoints(ctx, api, key, daemonId);
      },
      rollback: () => undefined,
    });
    return true;
  } catch (error) {
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return false;
  }
}
