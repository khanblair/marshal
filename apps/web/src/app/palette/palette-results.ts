import type { Marshal } from "~/mock";
import { cleanQuery, type PaletteHits } from "~/sync/search";

/** One row of the palette: the store's command shape. */
export type PaletteCommand = ReturnType<Marshal["commands"]>[number];

/** The palette lists at most this many rows. */
const MAX_RESULTS = 60;
/** Space kept above or below a row that keyboard selection scrolls into view. */
const SCROLL_MARGIN_PX = 8;

const NEEDS_GROUP = "Cards that need you";

/** Every command that contains all the words of the query, in the store's order. */
function matching(all: PaletteCommand[], query: string): PaletteCommand[] {
  const words = query.split(/\s+/).filter(Boolean);
  return all.filter((x) => {
    const text = `${x.label} ${x.hint || ""} ${x.group}`.toLowerCase();
    return words.every((w) => text.includes(w));
  });
}

/** With no query: every command except cards, then the cards that need you, regrouped. */
function withNeedsGroup(M: Marshal, all: PaletteCommand[]): PaletteCommand[] {
  const needs = M.needs().map((c) => all.find((x) => x.group === "Cards" && x.card === c.id));
  const regrouped = needs.filter((x) => x !== undefined).map((x) => ({ ...x, group: NEEDS_GROUP }));
  return all.filter((x) => x.group !== "Cards").concat(regrouped);
}

/**
 * With the daemon's answer to the query: the actions that match, then the daemon's projects, cards,
 * and chats in place of the store's own project and card rows, then the settings that match. The
 * daemon's rows are not filtered again: the daemon has matched them (a card's `api#41` is in its
 * key, which its row does not show).
 */
function withHits(all: PaletteCommand[], q: string, hits: PaletteHits): PaletteCommand[] {
  const own = matching(
    all.filter((x) => x.group !== "Projects" && x.group !== "Cards"),
    q,
  );
  const actions = own.filter((x) => x.group === "Actions");
  const settings = own.filter((x) => x.group !== "Actions");
  return [...actions, ...hits.projects, ...hits.cards, ...hits.chats, ...settings];
}

/**
 * What the palette shows for a query: filtered, grouped, and capped at 60 rows. `hits` is the
 * daemon's answer, used only when it answers this very query; while the answer is on its way, or
 * when there is none (the daemon is away, or the request failed), the palette searches what the
 * store holds, as it does with no daemon at all.
 */
export function paletteResults(
  M: Marshal,
  query: string,
  hits: PaletteHits | null = null,
): PaletteCommand[] {
  const all = M.commands();
  const q = query.trim().toLowerCase();
  const answered = q !== "" && hits !== null && hits.query === cleanQuery(query);
  if (answered) return withHits(all, q, hits).slice(0, MAX_RESULTS);
  const found = q ? matching(all, q) : withNeedsGroup(M, matching(all, q));
  return found.slice(0, MAX_RESULTS);
}

/** The selection after an arrow key, kept inside the list. */
export function stepSelection(
  current: number,
  key: "ArrowDown" | "ArrowUp",
  length: number,
): number {
  const next = current + (key === "ArrowDown" ? 1 : -1);
  return Math.max(0, Math.min(length - 1, next));
}

/**
 * Scrolls the option at `index` into view inside the listbox. It measures from the panel (the
 * listbox is not positioned), as the design does, so the offsets include the search row.
 */
export function scrollOptionIntoView(list: HTMLElement, index: number): void {
  const el = list.querySelector<HTMLElement>(`[data-pi="${index}"]`);
  const parent = el?.parentElement;
  if (!el || !parent) return;
  if (el.offsetTop < parent.scrollTop) {
    parent.scrollTop = el.offsetTop - SCROLL_MARGIN_PX;
  } else if (el.offsetTop + el.offsetHeight > parent.scrollTop + parent.clientHeight) {
    parent.scrollTop = el.offsetTop + el.offsetHeight - parent.clientHeight + SCROLL_MARGIN_PX;
  }
}
