import { configure } from "@solidjs/testing-library";
import { M } from "~/mock";

/* No stylesheet loads in jsdom, so nothing is hidden; skipping the visibility check makes role queries on Home fast. */
configure({ defaultHidden: true });

/* Shared by the Home tests: the store is one object per test file, so every test starts from a snapshot. */

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value));

/** The moment the store was seeded (the first notice carries it), so "34 min ago" in the seed stays 34 minutes. */
const SEEDED_AT_MS = M.S.notices[0]?.ts ?? Date.now();

const DEFAULT_RANGE_DAYS = 7;

const DESKTOP_WIDTH_PX = 1440;
export const TABLET_WIDTH_PX = 820;
export const PHONE_WIDTH_PX = 390;
const VIEWPORT_HEIGHT_PX = 900;

/** The parts of the store the Home views read or the tests change. */
export function homeSnapshot() {
  const S = M.S;
  return {
    cards: clone(S.cards),
    chat: clone(S.chat),
    feed: clone(S.feed),
    projects: clone(S.projects),
    schedules: clone(S.schedules),
    calEvents: clone(S.calEvents),
    limits: clone(S.limits),
  };
}

export type HomeSnapshot = ReturnType<typeof homeSnapshot>;

/** Puts the store back as the snapshot has it, on the Home page at `width`, with the clock at the moment of seeding. */
export function resetHome(snapshot: HomeSnapshot, width = DESKTOP_WIDTH_PX): void {
  const S = M.S;
  S.cards = clone(snapshot.cards);
  S.chat = clone(snapshot.chat);
  S.feed = clone(snapshot.feed);
  S.projects = clone(snapshot.projects);
  S.schedules = clone(snapshot.schedules);
  S.calEvents = clone(snapshot.calEvents);
  S.limits = clone(snapshot.limits);
  S.dashRange = DEFAULT_RANGE_DAYS;
  S.openId = null;
  S.toasts = [];
  S.filters = {};
  S.savedView = {};
  S.settingsSection = "profile";
  S.schedEdit = null;
  M.set({ allKind: "activity" });
  M.setViewport(width, VIEWPORT_HEIGHT_PX);
  M.go("home");
  vi.setSystemTime(SEEDED_AT_MS);
}
