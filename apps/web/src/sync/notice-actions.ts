import {
  type NoticeActionRequest,
  NoticeActionKeepAll,
  NoticeActionKeepAwake,
  NoticeActionSleepAll,
} from "@marshal/protocol";
import type { SleepNotice } from "~/data/mappers/notices";
import type { CardKey } from "~/mock/card-key";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import type { Card } from "~/mock/types";

/*
 * The four notice calls a person makes while S23 is on the daemon: keep one card awake, keep all the
 * cards of a notice awake, sleep them all now, and dismiss a notice. The daemon owns the notices, so
 * none of these draws the change first: each asks, and the daemon's own `notice.created` /
 * `notice.dismissed` brings the list the screens then show. A refusal - sleeping a card that is
 * working and not paused, for instance - is shown in the daemon's own words by `ctx.optimistic`, and
 * nothing is drawn at all.
 *
 * A row's own "Sleep now" and "Pin" are not here: they are the card's hold controls (S7c) and live
 * in `sync/card-hold.ts`. The mock's own versions of the four above stay in `mock/actions/sessions.ts`
 * for the sections that are still mock.
 */

/** The one sleep group that is standing, if any, which is what the two whole-notice calls act on. */
const sleepNotice = (ctx: Ctx): SleepNotice | undefined =>
  ctx.S.notices.find((one): one is SleepNotice => one.kind === "sleep");

/** The sleep group that names a card, which is the one a per-card call belongs to. */
const noticeNaming = (ctx: Ctx, key: CardKey): SleepNotice | undefined =>
  ctx.S.notices.find((one): one is SleepNotice => one.kind === "sleep" && one.cards.includes(key));

/** The daemon's own id for a card the store knows, which is what a notice call carries. */
const daemonIdOf = (ctx: Ctx, key: CardKey): string | null => {
  const card: Card | undefined = ctx.S.cards.find((one) => one.id === key);
  return card?.daemonId ?? null;
};

/** One notice call. It answers how many cards it changed, or null when the daemon said no. */
async function call(ctx: Ctx, notice: string, body: NoticeActionRequest): Promise<number | null> {
  const api = ctx.env.data?.api;
  if (!api) return null;
  try {
    const result = await ctx.optimistic({
      key: `notice:${notice}:${body.action}:${body.cardId ?? "all"}`,
      apply: () => undefined,
      request: () => api.noticeAction(notice, body),
      rollback: () => undefined,
    });
    return result.cards;
  } catch {
    return null;
  }
}

/** The sentence "Keep awake" says, with the keep-awake length the settings hold. */
const keptAwakeWords = (n: number): string =>
  `Kept awake for ${n} more ${n === 1 ? "minute" : "minutes"}`;

/**
 * Keeps one idle card awake: it leaves its group, and the idle timer is held off it for the
 * keep-awake setting's length. Nothing happens for a card no notice names, which is a card that was
 * already kept awake, pinned, or put to sleep.
 */
export async function keepAwake(ctx: Ctx, id: CardKey): Promise<boolean> {
  const notice = noticeNaming(ctx, id);
  const cardId = notice ? daemonIdOf(ctx, id) : null;
  if (!notice || !cardId) return false;
  const changed = await call(ctx, notice.id, { action: NoticeActionKeepAwake, cardId });
  if (changed === null) return false;
  toast(ctx, keptAwakeWords(ctx.S.sleep.keepAwake));
  return true;
}

/** Keeps every card a notice names awake, and says how many were held. */
export async function keepAllAwake(ctx: Ctx): Promise<boolean> {
  const notice = sleepNotice(ctx);
  if (!notice) return false;
  const changed = await call(ctx, notice.id, { action: NoticeActionKeepAll });
  if (changed === null) return false;
  toast(ctx, `Kept ${changed} cards awake`);
  return true;
}

/** Puts every card a notice names to sleep now, and says how many went. */
export async function sleepAll(ctx: Ctx): Promise<boolean> {
  const notice = sleepNotice(ctx);
  if (!notice) return false;
  const changed = await call(ctx, notice.id, { action: NoticeActionSleepAll });
  if (changed === null) return false;
  toast(ctx, `${changed} cards asleep`);
  return true;
}

/** Takes one notice off the panel. A notice that is already gone is not an error. */
export async function dismissNotice(ctx: Ctx, id: string): Promise<boolean> {
  const api = ctx.env.data?.api;
  const shown = ctx.S.notices;
  if (!api) return false;
  const kept = shown.filter((one) => one.id !== id);
  try {
    await ctx.optimistic({
      key: `notice:${id}:dismiss`,
      apply: () => {
        ctx.S.notices = kept;
      },
      request: () => api.dismissNotice(id),
      rollback: () => {
        ctx.S.notices = shown;
      },
    });
    return true;
  } catch {
    return false;
  }
}
