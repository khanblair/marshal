import {
  EventTypeMeUpdated,
  MeTopic,
  type ProgressStatus,
  type Event as WireEvent,
  type Progress as WireProgress,
} from "@marshal/protocol";
import { batch } from "solid-js";
import { isRecord } from "~/data/guards";
import type { Ctx } from "~/mock/context";
import { toOnboardingState } from "./me-mapper";
import type { Syncer } from "./syncer";

/**
 * Section S31a: how far the person is with the first-launch screens and the tour on Home. It is
 * saved per person, so a new device resumes where another one left off, and it comes with
 * `me.updated`, which carries the whole progress.
 *
 * It fills `S.onboarding`, `S.obStep`, and `S.tour`, which the onboarding screens read.
 */

/**
 * What this store last saw of the daemon's progress, so a change that arrives from another device
 * is told apart from what this device already knows: a tour that finishes elsewhere closes the one
 * here, and a first launch that is put back on another device takes over this screen again.
 */
interface SeenProgress {
  onboarding: boolean;
  tutorial: ProgressStatus;
}

const lastSeen = new WeakMap<Ctx, SeenProgress>();

/** Takes the daemon's progress into the store. The tour's own step is screen state, so it is kept. */
export function applyProgress(ctx: Ctx, wire: WireProgress): void {
  const { S } = ctx;
  const state = toOnboardingState(wire);
  const previous = lastSeen.get(ctx);
  lastSeen.set(ctx, { onboarding: state.onboarding, tutorial: wire.tutorial.status });
  // A tour that is on screen is the person's: only a change that arrived from elsewhere closes it,
  // never an echo of what this device already did, so replaying it from the menu is not cut short
  // by an unrelated `me.updated`.
  const finishedElsewhere = previous?.tutorial === "pending" && wire.tutorial.status !== "pending";
  const resetElsewhere = previous !== undefined && state.onboarding && !previous.onboarding;
  batch(() => {
    if (S.onboarding !== state.onboarding) S.onboarding = state.onboarding;
    if (S.obStep !== state.obStep) S.obStep = state.obStep;
    if (state.tourPending && !S.tour) S.tour = { step: 0 };
    if ((finishedElsewhere || resetElsewhere) && S.tour) S.tour = null;
  });
}

function onProgressEvent(ctx: Ctx, event: WireEvent): void {
  if (event.type !== EventTypeMeUpdated || !isRecord(event.data)) return;
  if (isRecord(event.data.progress))
    applyProgress(ctx, event.data.progress as unknown as WireProgress);
}

export const progressSyncer: Syncer<WireProgress> = {
  section: "S31a",
  topics: [MeTopic],
  load: (api) => api.progress(),
  apply: applyProgress,
  onEvent: onProgressEvent,
};
