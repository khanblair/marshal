import { unwrap } from "solid-js/store";
import { parseCardKey } from "~/mock/card-key";
import type { Ctx } from "~/mock/context";
import type { Card, Chat, FeedItem, Notice } from "~/mock/types";

/**
 * The mock records that belong to a project the daemon does not have. Cards, chats, notices, and
 * feed items are still mock (sections S5a, S17, S20, and S23), but they must never show for a
 * project that does not exist (backend-checklist.md 2.3, rules 1 and 2). They wait here, out of
 * `M.S`, and come back when their project arrives.
 */
export interface Reservoir {
  cards: Card[];
  chats: Record<string, Chat[]>;
  notices: Notice[];
  feed: FeedItem[];
}

const projectOfKey = (key: string | undefined): string | null =>
  key ? (parseCardKey(key)?.projectId ?? null) : null;

/** The projects a notice is about. None means it is about no project, so it always shows. */
function noticeProjects(notice: Notice): string[] {
  if (notice.kind === "sleep") {
    return notice.cards.flatMap((key) => projectOfKey(key) ?? []);
  }
  const pid = notice.pid ?? projectOfKey(notice.cardId);
  return pid ? [pid] : [];
}

const noticeShows = (present: ReadonlySet<string>, notice: Notice): boolean => {
  const projects = noticeProjects(notice);
  // A sleep notice with cards in several projects stays while any of them exists; a card that is not
  // there is ignored wherever a notice lists cards.
  return notice.kind === "sleep"
    ? projects.some((pid) => present.has(pid))
    : projects.every((pid) => present.has(pid));
};

const feedShows = (present: ReadonlySet<string>, item: FeedItem): boolean => {
  const pid = item.pid ?? projectOfKey(item.cardId);
  return pid === null || present.has(pid);
};

/** Splits a list in two by a test, keeping the order of each part. */
function split<T>(list: readonly T[], keep: (item: T) => boolean): [T[], T[]] {
  const kept: T[] = [];
  const rest: T[] = [];
  for (const item of list) (keep(item) ? kept : rest).push(item);
  return [kept, rest];
}

/** Moves the records that should be shown out of the reservoir, and the ones that should not out of `S`. */
function exchange<T>(shown: readonly T[], hidden: T[], keep: (item: T) => boolean): T[] | null {
  const [stay, leaving] = split(shown, keep);
  const [arriving, staying] = split(hidden, keep);
  if (!leaving.length && !arriving.length) return null;
  hidden.splice(0, hidden.length, ...staying, ...leaving.map((item) => unwrap(item)));
  return [...stay, ...arriving];
}

/**
 * Shows exactly the mock records whose project is in `S.projects`, and hides the rest. It runs after
 * every change to the projects and changes nothing when nothing needs to move, so running it twice
 * is harmless and a view that reads a list is not redrawn for no reason.
 */
export function reconcileMock(ctx: Ctx): void {
  const { S, hidden } = ctx;
  const present = new Set(S.projects.map((project) => project.id));
  const cards = exchange(S.cards, hidden.cards, (card) => present.has(card.p));
  if (cards) S.cards = cards;
  const notices = exchange(S.notices, hidden.notices, (notice) => noticeShows(present, notice));
  if (notices) S.notices = notices;
  const feed = exchange(S.feed, hidden.feed, (item) => feedShows(present, item));
  if (feed) S.feed = feed;
  for (const pid of Object.keys(S.chats)) {
    const list = S.chats[pid];
    if (present.has(pid) || !list) continue;
    hidden.chats[pid] = unwrap(list);
    delete S.chats[pid];
  }
  for (const pid of Object.keys(hidden.chats)) {
    const list = hidden.chats[pid];
    if (!present.has(pid) || !list) continue;
    S.chats[pid] = list;
    delete hidden.chats[pid];
  }
}

/** A notice after a project is gone: a sleep notice keeps the cards of other projects, and any other notice about it goes. */
function afterRemoval(notices: Notice[], pid: string): Notice[] {
  for (const notice of notices) {
    if (notice.kind === "sleep") {
      notice.cards = notice.cards.filter((key) => projectOfKey(key) !== pid);
    }
  }
  return notices.filter((notice) =>
    notice.kind === "sleep" ? notice.cards.length > 0 : !noticeProjects(notice).includes(pid),
  );
}

/** Forgets every mock record of a project for good, shown or hidden. The project is being removed. */
export function dropMock(ctx: Ctx, pid: string): void {
  const { S, hidden } = ctx;
  const openCard = S.openId ? S.cards.find((card) => card.id === S.openId) : undefined;
  if (openCard?.p === pid) S.openId = null;
  S.cards = S.cards.filter((card) => card.p !== pid);
  hidden.cards = hidden.cards.filter((card) => card.p !== pid);
  delete S.chats[pid];
  delete hidden.chats[pid];
  S.notices = afterRemoval(S.notices, pid);
  hidden.notices = afterRemoval(hidden.notices, pid);
}
