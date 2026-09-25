/**
 * The app's global keyboard handler, a `window` keydown listener exactly as in the
 * prototype: Cmd or Ctrl with K and 1 to 6, Escape closing one layer at a time, the
 * single-letter card shortcuts, Enter to open the focused card, and arrow keys to move
 * focus between cards.
 */
import { onCleanup, onMount } from "solid-js";
import { M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { openPalette } from "./shell-actions";

/** Wait before focusing a card, so its view has rendered the new focus. */
const FOCUS_DELAY_MS = 20;

const FIELD_TAGS = /INPUT|TEXTAREA|SELECT/;
const VIEW_DIGIT = /^[1-6]$/;

function isInField(el: Element | null): boolean {
  if (!el) return false;
  return FIELD_TAGS.test(el.tagName) || (el as HTMLElement).isContentEditable;
}

/** Closes the top layer, or does nothing when none is open. */
function closeTopLayer(inField: boolean, active: Element | null): void {
  const S = M.S;
  if (S.palette) M.set({ palette: false });
  else if (S.dialog) M.closeDialog();
  else if (S.newCard) M.set({ newCard: null });
  else if (S.newProject) M.set({ newProject: null });
  else if (S.removeProject) M.set({ removeProject: null });
  else if (S.menu) M.set({ menu: null });
  else if (S.noticesOpen) M.set({ noticesOpen: false });
  else if (S.sideOpen) M.set({ sideOpen: false });
  else if (inField) (active as HTMLElement).blur();
  else if (S.openId) M.closeCard();
}

/** Cmd or Ctrl shortcuts. Returns true when the key was one of them. */
function modShortcut(e: KeyboardEvent): boolean {
  const S = M.S;
  if (e.key.toLowerCase() === "k") {
    e.preventDefault();
    if (S.palette) M.set({ palette: false });
    else openPalette();
    return true;
  }
  if (!VIEW_DIGIT.test(e.key)) return false;
  e.preventDefault();
  const view = M.VIEWS[Number(e.key) - 1]?.key;
  if (!view) return true;
  if (S.route.page !== "project") M.go("project", S.route.pid, view);
  else M.setView(view);
  return true;
}

/** A dialog, the palette, or a draft is open, so single keys belong to it. */
function layerOpen(): boolean {
  const S = M.S;
  return !!(S.palette || S.dialog || S.newCard || S.newProject || S.removeProject);
}

function newCardKey(e: KeyboardEvent): void {
  e.preventDefault();
  if (M.S.route.page !== "project") M.go("project", M.S.route.pid, "board");
  M.newCard();
}

function focusSearch(e: KeyboardEvent): void {
  const el = document.querySelector<HTMLElement>("[data-search]");
  if (!el) return;
  e.preventDefault();
  el.focus();
}

function approveKey(e: KeyboardEvent, id: CardKey | null): void {
  if (!id || !M.pendingApproval(id)) return;
  e.preventDefault();
  M.approve(id);
}

function sleepKey(id: CardKey | null): void {
  const c = id ? M.card(id) : undefined;
  if (!(id && c)) return;
  if (c.asleep) M.wake(id);
  else M.sleep(id);
}

function pinKey(id: CardKey | null): void {
  if (id && M.card(id)) M.pin(id);
}

function enterKey(e: KeyboardEvent, active: Element | null): void {
  const focusId = M.S.focusId;
  if (!focusId || !(active === document.body || !active)) return;
  e.preventDefault();
  M.openCard(focusId);
}

/** Single keys without a modifier, outside fields. */
function plainKey(e: KeyboardEvent, active: Element | null): void {
  const k = e.key;
  const id = M.S.openId || M.S.focusId;
  const kl = k.length === 1 ? k.toLowerCase() : k;
  if (kl === "n") newCardKey(e);
  else if (k === "/") focusSearch(e);
  else if (kl === "a") approveKey(e, id);
  else if (kl === "s") sleepKey(id);
  else if (kl === "p") pinKey(id);
  else if (k === "Enter") enterKey(e, active);
  else if (k.startsWith("Arrow") && !active?.closest?.("[data-no-nav]")) navigate(k, e);
}

function onGlobalKey(e: KeyboardEvent): void {
  const S = M.S;
  if (S.onboarding || S.tour) return;
  const mod = e.metaKey || e.ctrlKey;
  const active = document.activeElement;
  const inField = isInField(active);
  if (mod && modShortcut(e)) return;
  if (e.key === "Escape") {
    closeTopLayer(inField, active);
    return;
  }
  if (layerOpen() || inField || mod || e.altKey) return;
  plainKey(e, active);
}

/** Next card in a board-like grid of columns. */
function gridTarget(grid: CardKey[][], focusId: CardKey | null, key: string): CardKey | undefined {
  let ci = -1;
  let ri = -1;
  grid.forEach((col, i) => {
    const j = focusId == null ? -1 : col.indexOf(focusId);
    if (j >= 0) {
      ci = i;
      ri = j;
    }
  });
  if (ci < 0) return grid.find((c) => c.length)?.[0];
  const column = grid[ci] ?? [];
  if (key === "ArrowDown") return column[Math.min(ri + 1, column.length - 1)];
  if (key === "ArrowUp") return column[Math.max(ri - 1, 0)];
  const step = key === "ArrowRight" ? 1 : -1;
  let c = ci + step;
  while (c >= 0 && c < grid.length && !grid[c]?.length) c += step;
  const target = grid[c];
  return target ? target[Math.min(ri, target.length - 1)] : undefined;
}

/** Next card in a list of rows; only up and down move. */
function rowTarget(rows: CardKey[], focusId: CardKey | null, key: string): CardKey | undefined {
  if (key !== "ArrowDown" && key !== "ArrowUp") return undefined;
  const i = focusId == null ? -1 : rows.indexOf(focusId);
  const next =
    i < 0 ? 0 : Math.max(0, Math.min(rows.length - 1, i + (key === "ArrowDown" ? 1 : -1)));
  return rows[next];
}

/** Moves card focus inside the navigation model the current view published on `M.nav`. */
function navigate(key: string, e: KeyboardEvent): void {
  const model = M.nav;
  if (!model) return;
  const focusId = M.S.focusId;
  let next: CardKey | undefined;
  if (model.grid) next = gridTarget(model.grid, focusId, key);
  else if (model.rows) next = rowTarget(model.rows, focusId, key);
  if (next == null) return;
  e.preventDefault();
  M.set({ focusId: next });
  const id = next;
  setTimeout(() => {
    // The page, or a test environment, may be gone by the time the timer fires.
    if (typeof document === "undefined") return;
    document.querySelector<HTMLElement>(`[data-card="${id}"]`)?.focus();
  }, FOCUS_DELAY_MS);
}

/** Installs the global keyboard handler for the component's lifetime. */
export function useGlobalKeys(): void {
  onMount(() => window.addEventListener("keydown", onGlobalKey));
  onCleanup(() => window.removeEventListener("keydown", onGlobalKey));
}
