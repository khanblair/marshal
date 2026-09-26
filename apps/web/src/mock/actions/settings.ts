import type { CardKey } from "../card-key";
import type { Ctx } from "../context";
import { addAct, later, toast } from "../engine";
import { card, cardLabelOf } from "../selectors";
import { ciFailure } from "../sim/ci-failure";
import type { Theme } from "../types";

const DEFAULT_CI_CARD_ID: CardKey = "api#40";

export function simulateCiFailure(ctx: Ctx, id?: CardKey): void {
  const target = id || DEFAULT_CI_CARD_ID;
  ciFailure(ctx, target, true);
  const c = card(ctx, target);
  toast(ctx, `Simulating a CI failure on ${c ? cardLabelOf(ctx, c) : target}`);
}

export function setTheme(ctx: Ctx, theme: Theme): void {
  ctx.S.theme = theme;
  ctx.env.applyTheme(ctx.S);
}

/** Saves the Profile form. It is the mock's own until the profile is the daemon's (S2a), so it cannot be refused. */
export function saveProfile(
  ctx: Ctx,
  fields: { name: string; email: string; tz: string },
): boolean {
  Object.assign(ctx.S.profile, {
    name: fields.name.trim(),
    email: fields.email.trim(),
    tz: fields.tz,
  });
  toast(ctx, "Profile saved");
  return true;
}

/** The Upload image button of the Profile screen. The mock has no picker: it says what the button is for. */
export function chooseAvatar(ctx: Ctx): void {
  toast(ctx, "Choose an image to use as your avatar");
}

/** Checks finish one after another; the first fails while the card's CI is failing. */
const FIRST_CHECK_MS = 1500;
const NEXT_CHECK_MS = 900;

export function runChecks(ctx: Ctx, id: CardKey): void {
  const ks = ctx.S.checks[id];
  if (!ks) return;
  for (const k of ks) if (k.cmd) k.st = "running";
  ks.forEach((k, i) => {
    if (!k.cmd) return;
    later(FIRST_CHECK_MS + i * NEXT_CHECK_MS, () => {
      k.st = card(ctx, id)?.ci === "failed" && i === 0 ? "failed" : "passed";
      const passed = k.st === "passed";
      addAct(ctx, id, {
        kind: "test",
        text: `Check: ${k.name}`,
        result: passed ? "Passed" : "Failed",
        st: passed ? "ok" : "fail",
      });
    });
  });
}
