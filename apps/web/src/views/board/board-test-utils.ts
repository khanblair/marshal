import { cleanup } from "@solidjs/testing-library";
import { afterEach, beforeEach, vi } from "vitest";
import { M } from "~/mock";
import { prototypeCards } from "~/testing/prototype-cards";

const seed = prototypeCards();
const DESKTOP_PX = 1440;
const VIEWPORT_HEIGHT_PX = 900;
export const PHONE_PX = 390;

/** Puts the store back to its seeded state and opens a project's board at a width. */
export function reset(pid = "api", width = DESKTOP_PX): void {
  M.S.cards = structuredClone(seed);
  for (const id of M.S.projects.map((p) => p.id)) {
    M.S.filters[id] = [];
    M.S.query[id] = "";
    M.S.showAllDone[id] = false;
  }
  M.S.laneCollapsed = {};
  M.S.swim = { api: "none", web: "none", mobile: "package" };
  M.S.quickAddAt = null;
  M.S.mobileCol = "working";
  M.S.dragId = null;
  M.S.dropCol = null;
  M.S.openId = null;
  M.S.focusId = null;
  M.S.newCard = null;
  M.S.toasts = [];
  M.nav = null;
  M.setViewport(width, VIEWPORT_HEIGHT_PX);
  M.go("project", pid, "board");
}

export const column = (col: string): HTMLElement => {
  const el = document.querySelector<HTMLElement>(`section[data-col="${col}"]`);
  if (!el) throw new Error(`no ${col} column`);
  return el;
};

/** The keys of the cards drawn under `root`, in document order. */
export const cardIds = (root: ParentNode): string[] =>
  Array.from(root.querySelectorAll<HTMLElement>("[data-card]")).map((el) => el.dataset.card ?? "");

/** Fake timers, a seeded store, and cleanup, for every test of the file. */
export function useBoardTestStore(): void {
  beforeEach(() => {
    vi.useFakeTimers();
    reset();
  });
  afterEach(() => {
    cleanup();
    vi.clearAllTimers();
    vi.useRealTimers();
  });
}
