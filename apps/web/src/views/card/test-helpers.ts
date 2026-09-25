import { M } from "~/mock";
import type { CardKey } from "~/mock/card-key";

const DESKTOP_PX = 1440;
const HEIGHT_PX = 900;

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value));

/** The seeded cards and their chats, activity, and checks, taken before any test changes them. */
const seed = {
  cards: clone(M.S.cards),
  chat: clone(M.S.chat),
  act: clone(M.S.act),
  checks: clone(M.S.checks),
};

/** Puts the store back to its seed and closes everything, so each test starts clean. */
export function resetStore(width = DESKTOP_PX): void {
  M.S.cards = clone(seed.cards);
  M.S.chat = clone(seed.chat);
  M.S.act = clone(seed.act);
  M.S.checks = clone(seed.checks);
  delete M.S.notes;
  delete M.S.preview;
  M.S.openId = null;
  M.S.dialog = null;
  M.S.toasts = [];
  M.S.tab = "chat";
  M.S.mode = "chat";
  M.S.switching = false;
  M.S.detailExpanded = false;
  M.setViewport(width, HEIGHT_PX);
}

/** The card as the store holds it, for assertions. */
export function cardOf(id: CardKey) {
  const card = M.card(id);
  if (!card) throw new Error(`no card #${id}`);
  return card;
}
