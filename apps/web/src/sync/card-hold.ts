import type { ApiClient } from "~/data/api-client";
import { ChangeInFlightError } from "~/data/optimistic";
import type { CardKey } from "~/mock/card-key";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import { card as cardOf } from "~/mock/selectors";
import type { Card } from "~/mock/types";
import { applyCard } from "./cards";

/*
 * The write path of the session hold (section S7c, docs/backend-checklist.md B2.15): pause, sleep,
 * wake, pin, and unpin. Each is the daemon-backed version of one the card panel and the Home awake
 * list call on the mock today, and keeps that one's arguments. Releasing a pause has no button of
 * its own: `POST /v1/cards/{id}/start` already does it (`Manager.Start` checks `card.Paused`
 * first), so the existing daemon-backed `start` (section S5a, sync/card-actions.ts) already covers
 * it once a card is the daemon's.
 *
 * A pause, a pin, and an unpin answer with the card as it now is, so the store is updated the same
 * way a `card.updated` event would draw it. Sleep and wake answer with nothing: what changed is the
 * session, and the card that carries it (`Card.session`, see data/mappers/card.ts) arrives as
 * `card.updated` on the project topic, so a sleep or a wake reaches every screen, the Home awake
 * list included, whether or not the card is open. The open card also hears it a moment sooner as
 * `session.state_changed` (see card-session.ts). Nothing here draws it from the answer, so a card
 * the daemon did not put to sleep is never drawn asleep.
 *
 * Every refusal is the daemon's own sentence from docs/architecture.md 5.1 ("Only working cards can
 * be paused.", and the rest), shown exactly as `ctx.optimistic` already shows any refusal: nothing
 * here repeats or guesses at the rules.
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

function apiOf(ctx: Ctx): ApiClient | null {
  return ctx.env.data?.api ?? null;
}

const daemonIdOf = (card: Card): string => card.daemonId ?? "";

/** Asks the daemon for one change and hands its answer back, or null when nothing was asked or the
 * daemon refused it (shown as a toast, the daemon's own sentence). */
async function ask<T>(
  ctx: Ctx,
  key: string,
  run: (api: ApiClient) => Promise<T>,
): Promise<T | null> {
  const api = apiOf(ctx);
  if (!api) {
    toast(ctx, NOT_CONNECTED);
    return null;
  }
  try {
    return await ctx.optimistic({
      key,
      apply: () => undefined,
      request: () => run(api),
      rollback: () => undefined,
    });
  } catch (error) {
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return null;
  }
}

/** Holds the card and puts the daemon's answer in the store. It says nothing: the caller does. */
async function holdCard(ctx: Ctx, id: CardKey): Promise<boolean> {
  const card = cardOf(ctx, id);
  if (!card) return false;
  const paused = await ask(ctx, `hold:${id}`, (api) => api.pauseCard(daemonIdOf(card)));
  if (!paused) return false;
  applyCard(ctx, paused);
  return true;
}

/** Puts the card's session to sleep. It says nothing: the caller does. */
async function sleepCard(ctx: Ctx, id: CardKey): Promise<boolean> {
  const card = cardOf(ctx, id);
  if (!card) return false;
  return (await ask(ctx, `hold:${id}`, (api) => api.sleepCard(daemonIdOf(card)))) !== null;
}

export async function pause(ctx: Ctx, id: CardKey): Promise<boolean> {
  if (!(await holdCard(ctx, id))) return false;
  toast(ctx, "Card paused");
  return true;
}

export async function sleep(ctx: Ctx, id: CardKey): Promise<boolean> {
  if (!(await sleepCard(ctx, id))) return false;
  toast(ctx, "Card asleep");
  return true;
}

export async function wake(ctx: Ctx, id: CardKey): Promise<boolean> {
  const card = cardOf(ctx, id);
  if (!card) return false;
  const ok = await ask(ctx, `hold:${id}`, (api) => api.wakeCard(daemonIdOf(card)));
  if (ok === null) return false;
  toast(ctx, "Session resumed");
  return true;
}

/**
 * The Agents view's Stop, which the mock drew as a pause and a sleep together (a working card is
 * held, then its session ends and is kept). It asks the daemon for the same two things in the same
 * order, and stops at the first refusal with the daemon's own sentence: a card that is waiting on
 * the person stays awake, and a card with a message being held is told to resume first. Nothing is
 * drawn until the daemon has answered, so a refused stop leaves the card as it was.
 */
export async function stopSession(ctx: Ctx, id: CardKey): Promise<boolean> {
  const card = cardOf(ctx, id);
  if (!card) return false;
  if (card.state === "working" && !card.paused && !(await holdCard(ctx, id))) return false;
  if (!(await sleepCard(ctx, id))) return false;
  toast(ctx, "Session stopped");
  return true;
}

/** Pins an unpinned card, or unpins a pinned one, the same toggle the mock's own button does. */
export async function pin(ctx: Ctx, id: CardKey): Promise<boolean> {
  const card = cardOf(ctx, id);
  if (!card) return false;
  const call = card.pinned
    ? (api: ApiClient) => api.unpinCard(daemonIdOf(card))
    : (api: ApiClient) => api.pinCard(daemonIdOf(card));
  const changed = await ask(ctx, `hold:${id}`, call);
  if (!changed) return false;
  applyCard(ctx, changed);
  toast(ctx, changed.pinned ? "Card pinned. It won't sleep." : "Card unpinned");
  return true;
}

/*
 * The three actions of the idle-card notice (keep awake, keep all awake, sleep all) belong to the
 * idle timer, which is Phase 5 (B5.6): the daemon has none of them. While the hold controls are the
 * daemon's, the notice's own buttons therefore change nothing and say so, and point at the controls
 * that do work (Pin and Sleep on the card). They must never mark a daemon card asleep on the screen
 * alone, or promise the person a keep-awake time nobody keeps. The only way a daemon card gets into
 * such a notice is the mock's seeded one, which names the prototype's card keys, and a daemon that
 * holds the prototype fixture has cards under the same keys.
 */
const KEEP_AWAKE_UNAVAILABLE =
  "Keeping cards awake for a while is not available yet. Pin a card to keep it awake.";
const SLEEP_ALL_UNAVAILABLE =
  "Putting all these cards to sleep at once is not available yet. Use Sleep on each card.";

export function keepAwake(ctx: Ctx, _id: CardKey): void {
  toast(ctx, KEEP_AWAKE_UNAVAILABLE);
}

export function keepAllAwake(ctx: Ctx): void {
  toast(ctx, KEEP_AWAKE_UNAVAILABLE);
}

export function sleepAll(ctx: Ctx): void {
  toast(ctx, SLEEP_ALL_UNAVAILABLE);
}
