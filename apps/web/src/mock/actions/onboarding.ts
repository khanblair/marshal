import { writeKey } from "~/data/storage";
import type { Ctx } from "../context";
import { ONBOARDED_KEY } from "../storage";

/** Whether the first-launch screens ended by being finished or by being skipped. */
export type OnboardingOutcome = "done" | "skipped";

/**
 * The mock's own first-launch screens. They are used while S31a is still on the mock (a test that
 * pins it, and the offline app); on the daemon the same actions are `sync/onboarding-actions.ts`.
 */
export function setOnboardingStep(ctx: Ctx, step: number): void {
  ctx.S.obStep = step;
}

/**
 * Ends onboarding on the last screen, from Continue (`done`) or from Skip (`skipped`), and opens the
 * tour, as the design does on both.
 */
export function finishOnboarding(ctx: Ctx, status: OnboardingOutcome = "done"): void {
  void status;
  const { S } = ctx;
  S.onboarding = false;
  S.tour = { step: 0 };
  writeKey(ctx.env.storage, ONBOARDED_KEY, "1");
  S.route = { ...S.route, page: "home" };
  S.openId = null;
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

/** Ends the tour, from its Finish and from its Skip. */
export function endTour(ctx: Ctx, status: OnboardingOutcome = "done"): void {
  void status;
  ctx.S.tour = null;
}
