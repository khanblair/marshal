import {
  EventTypePreviewStateChanged,
  type Preview,
  type PreviewShotKind,
  type Event as WireEvent,
} from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import { ChangeInFlightError } from "~/data/optimistic";
import { isDaemon } from "~/data/sections";
import type { CardKey } from "~/mock/card-key";
import { type Ctx, sectionsOf } from "~/mock/context";
import { toast } from "~/mock/engine";

/*
 * A card's live preview (section S13, docs/backend-checklist.md B6.6 and B6.7, docs/architecture.md
 * 11.2, N10): one dev server per card, on its own port, with the before and after screenshots taken
 * of it. The daemon owns all of it - whether a server is running, on which port, and which shots
 * exist - so the store only ever shows what the daemon last said, either in a snapshot or in a
 * `preview.state_changed` event.
 *
 * Reading the tab starts nothing. Only `start` runs the project's dev command, and only `stop`
 * stops it, which is what keeps looking at a card from starting a server on the machine.
 */

const NOT_CONNECTED = "Marshal is not connected to its daemon.";

/** True while a card's live preview is the daemon's (section S13). */
export function previewOnDaemon(ctx: Ctx): boolean {
  return isDaemon("S13", sectionsOf(ctx.env));
}

/** A card's preview as last read, or undefined when nothing has been read for it yet. */
export function previewOf(ctx: Ctx, key: CardKey): Preview | undefined {
  return ctx.S.preview?.[key];
}

/**
 * Writes a card's preview into the store, whole, so a snapshot and an event are applied the same way
 * and their two answers can never disagree. Exported so the read that happens when a card opens
 * (`card-session.ts`) writes it the same way.
 */
export function applyCardPreview(ctx: Ctx, key: CardKey, preview: Preview): void {
  ctx.S.preview ??= {};
  ctx.S.preview[key] = preview;
}

/**
 * Runs one preview change: start, stop, or take a screenshot. The daemon owns the state, so nothing
 * is drawn before it answers; a refusal carries the daemon's own sentence and is shown by the
 * caller's `optimistic`. False means nothing was asked, or the daemon said no.
 *
 * The change is named after the card so two changes to one card cannot overlap, and after the shot
 * kind for a screenshot so the before and after halves can be asked for at once.
 */
async function runPreviewChange(
  ctx: Ctx,
  key: CardKey,
  name: string,
  request: (api: ApiClient, daemonId: string) => Promise<void>,
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
      key: name,
      // The preview is the daemon's to report, and nothing about it is on the screen until it does.
      apply: () => undefined,
      request: () => request(api, daemonId),
      rollback: () => undefined,
    });
    return true;
  } catch (error) {
    if (error instanceof ChangeInFlightError) toast(ctx, error.message);
    return false;
  }
}

/** Runs the project's dev command for the card in the card's own worktree, on a port picked for it. */
export function startCardPreview(ctx: Ctx, key: CardKey): Promise<boolean> {
  return runPreviewChange(ctx, key, `preview:${key}`, async (api, daemonId) => {
    // The route answers the preview as it is now, wrapped with the daemon's own time (`PreviewSnapshot`),
    // so the tab holds the same `Preview` a read or an event writes.
    applyCardPreview(ctx, key, (await api.startPreview(daemonId)).preview);
  });
}

/** Stops the card's dev server. Stopping a preview that is not running is not an error. */
export function stopCardPreview(ctx: Ctx, key: CardKey): Promise<boolean> {
  return runPreviewChange(ctx, key, `preview:${key}`, async (api, daemonId) => {
    applyCardPreview(ctx, key, (await api.stopPreview(daemonId)).preview);
  });
}

/**
 * Takes one half of the running preview's before and after pair. The daemon's own sentence is shown
 * either way - what was captured, or why the check was skipped - so a screenshot Marshal could not
 * take never reads as one it did.
 */
export function takeCardPreviewShot(
  ctx: Ctx,
  key: CardKey,
  kind: PreviewShotKind,
): Promise<boolean> {
  return runPreviewChange(ctx, key, `previewShot:${key}:${kind}`, async (api, daemonId) => {
    const result = await api.takePreviewShot(daemonId, { kind });
    applyCardPreview(ctx, key, result.preview);
    toast(ctx, result.notice);
  });
}

/**
 * The bytes of a screenshot, fetched with the token the way an avatar is, so a screen can show an
 * object URL. It answers nothing when there is no daemon or the daemon refuses: a screenshot that
 * could not be fetched is a missing image, not a failed preview.
 */
export async function readPreviewShotImage(ctx: Ctx, shotUrl: string): Promise<Blob | null> {
  const api = ctx.env.data?.api;
  if (!api) return null;
  try {
    return await api.previewShotImage(shotUrl);
  } catch {
    return null;
  }
}

/** The `preview.state_changed` event that a card's own topic carries, applied to the card's row. */
export function applyPreviewEvent(ctx: Ctx, event: WireEvent): void {
  if (event.type !== EventTypePreviewStateChanged) return;
  if (!isRecord(event.data) || !isRecord(event.data.preview)) return;
  const preview = event.data.preview as unknown as Preview;
  const card = ctx.S.cards.find((one) => one.daemonId === preview.cardId);
  if (card) applyCardPreview(ctx, card.id, preview);
}
