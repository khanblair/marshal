import { writeKey } from "~/data/storage";
import type { Ctx } from "../context";
import { ONBOARDED_KEY } from "../storage";

export function finishOnboarding(ctx: Ctx): void {
  const { S } = ctx;
  S.onboarding = false;
  writeKey(ctx.env.storage, ONBOARDED_KEY, "1");
  S.route = { ...S.route, page: "home" };
  S.openId = null;
  S.tour = { step: 0 };
}

/** Shows onboarding again, as on a first launch. */
export function resetFirstLaunch(ctx: Ctx): void {
  const { S } = ctx;
  writeKey(ctx.env.storage, ONBOARDED_KEY, null);
  S.onboarding = true;
  S.obStep = 0;
  S.tour = null;
  S.openId = null;
  S.palette = false;
  S.dialog = null;
  S.menu = null;
  S.route = { ...S.route, page: "home" };
}

export function startTour(ctx: Ctx): void {
  const { S } = ctx;
  S.openId = null;
  S.menu = null;
  S.palette = false;
  S.route = { ...S.route, page: "home" };
  S.mobileTab = "home";
  S.tour = { step: 0 };
}
