import { batch, createEffect, on } from "solid-js";
import { addDays, startOfDay, zoneName } from "~/data/zone";
import type { Ctx } from "~/mock/context";

/** Extra time after midnight, so the timer never wakes a moment early and finds it still the old day. */
const SLACK_MS = 1000;

/**
 * Keeps `ctx.today` at midnight of the day it is now in the chosen time zone: it moves at the next
 * midnight, again every midnight after, and whenever the zone changes. It returns how to stop. Run it
 * inside a reactive root. `onRoll` runs after today moved, so what was worked out from the old day
 * (card dates) can be read again.
 */
export function followToday(ctx: Ctx, onRoll?: () => void): () => void {
  let timer: ReturnType<typeof setTimeout> | undefined;

  const roll = (): void => {
    const today = startOfDay(Date.now());
    if (today === ctx.today) return;
    const before = ctx.today;
    batch(() => {
      ctx.today = today;
      // A calendar that was showing today keeps showing it.
      if (ctx.S.calCursor === before) ctx.S.calCursor = today;
    });
    onRoll?.();
  };

  const arm = (): void => {
    clearTimeout(timer);
    const now = Date.now();
    const next = addDays(startOfDay(now), 1);
    timer = setTimeout(
      () => {
        roll();
        arm();
      },
      next - now + SLACK_MS,
    );
  };

  arm();
  createEffect(
    on(
      zoneName,
      () => {
        roll();
        arm();
      },
      { defer: true },
    ),
  );
  return () => clearTimeout(timer);
}
