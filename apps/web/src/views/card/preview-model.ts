import {
  type Preview,
  type PreviewState,
  PreviewStateRunning,
  PreviewStateStarting,
  PreviewStateStopped,
} from "@marshal/protocol";
import { M } from "~/mock";
import type { CardKey } from "~/mock/card-key";

/*
 * The Preview tab's read of a card's live preview (section S13, docs/backend-checklist.md B6.6).
 * The daemon owns all of it - whether a dev server is running, on which port, what command started
 * it, and which screenshots exist - so the tab decides none of it and only says what the daemon last
 * reported. `sync/preview.ts` holds the calls that start, stop, and shoot.
 */

export type { Preview, PreviewState } from "@marshal/protocol";

/** The card whose preview draws a dark page, to show a before and after. */
export const DARK_PREVIEW_CARD_ID: CardKey = "web#118";

const STOPPED = "Stopped. Start the preview to run the app from this card's worktree.";

/** A card's preview as last read, or undefined when nothing has been read for it yet. */
export const previewOf = (id: CardKey): Preview | undefined => M.S.preview?.[id];

/** Where the card's preview is. A card nothing has been read for yet is stopped, which is what it is. */
export const previewState = (id: CardKey): PreviewState =>
  previewOf(id)?.state ?? PreviewStateStopped;

/** The address the app answers on. Empty until the preview is running: a start has no address worth showing. */
export const previewUrl = (id: CardKey): string => previewOf(id)?.url ?? "";

/**
 * What the tab says under the address: the daemon's own words for the state. A preview that failed
 * to start is stopped with the daemon's sentence for why, so a start that did not work reads as one
 * that did not work rather than as a spinner that never ends.
 */
export function previewStatus(id: CardKey): string {
  const preview = previewOf(id);
  if (!preview) return STOPPED;
  switch (preview.state) {
    case PreviewStateRunning:
      return preview.command
        ? `Running ${preview.command} on port ${preview.port}`
        : `Running on port ${preview.port}`;
    case PreviewStateStarting:
      return preview.command
        ? `Starting the dev server with ${preview.command}`
        : "Starting the dev server";
    default:
      return preview.error || STOPPED;
  }
}
