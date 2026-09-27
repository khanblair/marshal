import {
  type SimulateCIFailureResult,
  type SimulateMode,
  SimulateModeReal,
  SimulateModeSynthetic,
} from "@marshal/protocol";
import { ChangeInFlightError } from "~/data/optimistic";
import type { CardKey } from "~/mock/card-key";
import type { Ctx } from "~/mock/context";
import { confirm, toast } from "~/mock/engine";
import { cardLabelOf, card as cardOf } from "~/mock/selectors";
import type { Card } from "~/mock/types";

/*
 * "Simulate CI failure", the two ways it runs (N28, B6.4, decision D5).
 *
 * The daemon owns both. The synthetic mode injects a failed run through the CI monitor's own path -
 * the same rerun, the same trimmed log, the same loop limits, the same notice - and never touches
 * GitHub, so it is the one every test drives. The real mode pushes a deliberately failing change to
 * the card's own branch, which runs for real and uses Actions minutes, so it is asked about first
 * and is only ever started by a person. Both answer the run they made and are audited; the card and
 * the project follow through the daemon's own `card.updated` and `ci.updated` events.
 *
 * Neither is a plain section switch: the daemon has the routes only in dev mode, so a card the mock
 * made keeps the mock's own story and a card the daemon made has the item only while that daemon
 * runs in dev mode. Two questions decide it, and both the store's action and the card menu's items
 * ask them: `ciOnDaemon` says whether a daemon owns the card at all, and `simulateOnDaemon` whether
 * that daemon is in dev mode.
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

/** The daemon's own id for a card the store knows, which is what the route takes. */
const daemonIdOf = (card: Card | undefined): string => card?.daemonId ?? "";

/** A simulation draws nothing before the daemon answers, so there is nothing to put back if it refuses. */
const nothing = (): void => undefined;

/**
 * True when a daemon owns this card's CI: the store follows one, and the daemon is the one that sent
 * the card (`daemonId`). A store with no daemon is the mock's whatever its cards carry, because the
 * mirror gives every card it applies a `daemonId` - including the test store's prototype cards.
 */
export function ciOnDaemon(ctx: Ctx, card: Card | undefined): boolean {
  return !!ctx.env.data && !!daemonIdOf(card);
}

/**
 * True when the daemon, not the mock, runs "Simulate CI failure" for this card: a daemon owns the
 * card and answered `dev` on its last health check, which is the only mode whose routes exist. A card
 * the mock made is not the daemon's, and a normal daemon has no such route, so the action falls back
 * to the mock's story for the first and does nothing for the second, and the card menu draws neither
 * item for the second.
 */
export function simulateOnDaemon(ctx: Ctx, card: Card | undefined): boolean {
  return ciOnDaemon(ctx, card) && ctx.env.data?.connection.mode() === "dev";
}

/**
 * Asks the daemon for one simulation and hands its answer back, or null when nothing was asked or
 * the daemon said no. The refusal is shown in the daemon's own words, as every other write is, and
 * the change is named after the card, so two runs of one card cannot overlap: the second is refused
 * with "still being saved".
 */
async function ask(
  ctx: Ctx,
  key: CardKey,
  mode: SimulateMode,
): Promise<SimulateCIFailureResult | null> {
  const card = cardOf(ctx, key);
  const api = ctx.env.data?.api;
  const daemonId = daemonIdOf(card);
  if (!api || !card || !daemonId) {
    toast(ctx, NOT_CONNECTED);
    return null;
  }
  try {
    return await ctx.optimistic({
      key: `ci:${key}`,
      apply: nothing,
      request: () => api.simulateCIFailure(daemonId, { mode }),
      rollback: nothing,
    });
  } catch (error) {
    // A refusal is already shown in the daemon's own words by `optimistic`; only the busy answer,
    // which is thrown before the daemon is asked, needs saying here.
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return null;
  }
}

/**
 * Injects a failed run on the card without touching GitHub. The card's badge and the project's CI
 * health follow from the daemon's own events, so nothing is drawn here beyond the sentence that says
 * what was asked for.
 */
export async function simulateSynthetic(ctx: Ctx, key: CardKey): Promise<boolean> {
  const card = cardOf(ctx, key);
  const result = await ask(ctx, key, SimulateModeSynthetic);
  if (!result) return false;
  toast(ctx, `Simulated a CI failure on ${card ? cardLabelOf(ctx, card) : key}`);
  return true;
}

/**
 * Pushes a deliberately failing change to the card's own branch, after asking. The confirmation
 * names the branch and says plainly that GitHub runs it for real and that the marked commit stays
 * until it is deleted, because nothing on the screen would otherwise say so; the daemon is asked
 * only once the person says yes.
 */
export function simulateReal(ctx: Ctx, key: CardKey): boolean {
  const card = cardOf(ctx, key);
  if (!card) return false;
  const branch = card.branch ?? "";
  confirm(ctx, {
    title: "Simulate a real CI failure",
    message: branch
      ? `Marshal pushes a commit with a deliberately failing workflow to ${branch}, so GitHub runs it for real and uses your Actions minutes. The commit stays on the branch until you delete it.`
      : `Marshal pushes a commit with a deliberately failing workflow to ${cardLabelOf(ctx, card)}'s branch, so GitHub runs it for real and uses your Actions minutes. The commit stays on the branch until you delete it.`,
    action: "Push the failing change",
    destructive: true,
    run: () => void pushReal(ctx, key, branch),
  });
  return true;
}

/** The push itself, run from the confirmation above. */
async function pushReal(ctx: Ctx, key: CardKey, branch: string): Promise<boolean> {
  const result = await ask(ctx, key, SimulateModeReal);
  if (!result) return false;
  toast(ctx, `Pushed a failing change to ${result.run.branch || branch}`);
  return true;
}
