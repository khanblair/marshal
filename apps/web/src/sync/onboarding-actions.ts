import type { UpdateProgressRequest, Progress as WireProgress } from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import type { Ctx } from "~/mock/context";
import { applyProgress } from "./progress";

/**
 * Section S31a: the first-launch screens and the tour, saved on the daemon so a device resumes where
 * another left off (docs/architecture.md 16.4, checklist B2.13). The screen the person is on, and
 * whether they finished or skipped, are what is saved; the tour's own step is screen state.
 *
 * Every write is fired and not waited for, the way the screens move at once: the daemon's answer
 * comes back as `me.updated` and as the answer itself, and both are applied through `applyProgress`,
 * so a change made on another device is what the person sees next.
 */

function apiOf(ctx: Ctx): ApiClient | null {
  return ctx.env.data?.api ?? null;
}

/**
 * Saves progress and takes back what the daemon answered.
 *
 * `applyStep` is false for the screen the person is on: they may have moved on while the request was
 * in flight, and where they are is the truth until they press Continue again. Everything else the
 * answer carries (a finish, a reset) is applied. A daemon that cannot be reached, or that is not in
 * dev mode, changes nothing on screen; the offline bar says the daemon is away.
 */
async function save(ctx: Ctx, body: UpdateProgressRequest, applyStep = true): Promise<void> {
  const api = apiOf(ctx);
  if (!api) return;
  try {
    const progress: WireProgress = await api.updateProgress(body);
    if (applyStep) applyProgress(ctx, progress);
  } catch {
    // Nothing on screen changes.
  }
}

/** Saves the screen the first-launch screens resume at, so another device opens it there. */
export function setOnboardingStep(ctx: Ctx, step: number): void {
  ctx.S.obStep = step;
  void save(ctx, { onboarding: { step } }, false);
}

/**
 * Ends the first-launch screens, with `done` from Continue on the last screen and `skipped` from
 * Skip, opens the tour as the design does on both, and shows Home.
 */
export function finishOnboarding(ctx: Ctx, status: "done" | "skipped" = "done"): void {
  const { S } = ctx;
  S.onboarding = false;
  S.tour = { step: 0 };
  S.route = { ...S.route, page: "home" };
  S.openId = null;
  void save(ctx, { onboarding: { status } });
}

/** Shows the tour again from the start. Replaying it sets the daemon's own status back to pending. */
export function startTour(ctx: Ctx): void {
  const { S } = ctx;
  S.openId = null;
  S.menu = null;
  S.palette = false;
  S.route = { ...S.route, page: "home" };
  S.mobileTab = "home";
  S.tour = { step: 0 };
  void save(ctx, { tutorial: { status: "pending" } });
}

/** Ends the tour, from its Finish and from its Skip, so no device opens it again by itself. */
export function endTour(ctx: Ctx, status: "done" | "skipped" = "done"): void {
  ctx.S.tour = null;
  void save(ctx, { tutorial: { status } });
}

/** Puts onboarding and the tour back to the start, as on a first launch. Only a dev daemon has it. */
export async function resetFirstLaunch(ctx: Ctx): Promise<void> {
  const { S } = ctx;
  S.openId = null;
  S.obStep = 0;
  S.tour = null;
  S.palette = false;
  S.dialog = null;
  S.menu = null;
  S.route = { ...S.route, page: "home" };
  const api = apiOf(ctx);
  if (!api) return;
  try {
    applyProgress(ctx, await api.resetFirstLaunch());
  } catch {
    // A daemon that is not in dev mode has no such address; the screens stay where they are.
  }
}
