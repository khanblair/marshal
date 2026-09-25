import { type Card, M } from "~/mock";

export type PreviewState = "stopped" | "starting" | "running";

const WEB_PORT_BASE = 5100;
const MOBILE_PORT_BASE = 8000;
const PORT_SPREAD = 90;
/** The card whose preview draws a dark page, to show a before and after. */
export const DARK_PREVIEW_CARD_ID = 118;
/** How long the fake dev server takes to start. */
const START_MS = 1800;

const isMobileProject = (card: Card): boolean => card.p === "mobile";
const portOf = (card: Card): number =>
  (isMobileProject(card) ? MOBILE_PORT_BASE : WEB_PORT_BASE) + (card.id % PORT_SPREAD);

export function previewUrl(card: Card): string {
  if (isMobileProject(card)) return `http://localhost:${portOf(card)}/login`;
  return `http://localhost:${portOf(card)}${card.id === DARK_PREVIEW_CARD_ID ? "/settings" : "/reports"}`;
}

export function previewStatus(card: Card, state: PreviewState): string {
  if (state === "running") {
    return `Running from ${card.branch || "main"} on port ${portOf(card)}`;
  }
  if (state === "starting") {
    const command = isMobileProject(card) ? "pnpm --filter apps/ios web" : "pnpm dev";
    return `Starting the dev server with ${command}`;
  }
  return "Stopped. Start the preview to run the app from this card's worktree.";
}

export const previewState = (id: number): PreviewState => M.S.preview?.[id] ?? "stopped";

/** Starts the fake dev server (it is up after a short wait) or stops it. */
export function togglePreview(id: number): void {
  M.S.preview ??= {};
  if (previewState(id) === "stopped") {
    M.S.preview[id] = "starting";
    setTimeout(() => {
      if (M.S.preview) M.S.preview[id] = "running";
    }, START_MS);
    return;
  }
  M.S.preview[id] = "stopped";
  M.toast("Preview stopped");
}
