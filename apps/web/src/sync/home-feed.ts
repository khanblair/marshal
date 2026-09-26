import {
  EventTypeActivityCreated,
  type Page,
  type Event as WireEvent,
  type FeedEntry as WireFeedEntry,
} from "@marshal/protocol";
import { batch } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import { isDaemon } from "~/data/sections";
import { type Ctx, sectionsOf } from "~/mock/context";
import type { FeedItem } from "~/mock/types";
import { toStoredFeedItem, toStoredFeedItems } from "./home-mapper";
import type { Syncer } from "./syncer";

/** How many of the newest entries Home reads when it loads. */
const FIRST_PAGE = 50;

/**
 * Section S20: the Home activity stream, and the `activity.created` events of the home topic. The
 * stream is the daemon's stored `activity` table, paged newest first, which is the order the feed
 * draws; the view-all page narrows the same list by kind and project with its own calls.
 *
 * Only the newest page is loaded. A person who wants more opens the view-all page, which asks for
 * its own pages with its own filters; nothing here keeps the whole stream in the store.
 */
export const homeFeedSyncer: Syncer<Page<WireFeedEntry>> = {
  section: "S20",
  topics: ["home"],
  async load(api: ApiClient) {
    return api.homeActivity({ limit: FIRST_PAGE });
  },
  apply(ctx, page) {
    applyHomeFeed(ctx, page.items);
  },
  onEvent: applyHomeFeedEvent,
};

/**
 * Makes the store's feed the daemon's newest page, newest first. Applying the same page twice
 * changes nothing to look at, because every row is rebuilt from the same wire entry.
 */
export function applyHomeFeed(ctx: Ctx, entries: readonly WireFeedEntry[]): void {
  ctx.S.feed = toStoredFeedItems(entries);
}

/**
 * One entry the daemon appended. A row that is already there is left alone, so an event that is
 * delivered twice, or a snapshot and an event that describe the same row, cannot draw it twice.
 */
export function applyHomeFeedEvent(ctx: Ctx, event: WireEvent): void {
  if (event.type !== EventTypeActivityCreated) return;
  const entry = readEntry(event.data);
  if (!entry) return;
  if (ctx.S.feed.some((item) => item.id === entry.id)) return;
  batch(() => {
    ctx.S.feed = [toStoredFeedItem(entry), ...ctx.S.feed];
  });
}

/** The row an `activity.created` event carries, or null when the payload is not one. */
function readEntry(data: unknown): WireFeedEntry | null {
  if (!isRecord(data) || !isRecord(data.entry) || typeof data.entry.id !== "string") return null;
  return data.entry as unknown as WireFeedEntry;
}

/** What the view-all page asks for: one kind, one project, and where the last page ended. */
export interface ActivityQuery {
  /** One feed kind, or every kind. */
  kind?: string;
  /** One project's id, or every project. */
  project?: string;
  /** The cursor the previous page ended on. Empty or left out for the newest page. */
  cursor?: string;
  /** How many rows the page holds. The daemon's own default decides when it is left out. */
  limit?: number;
}

/** One page of the view-all list, in the shapes the screens read. */
export interface ActivityFeedPage {
  items: FeedItem[];
  /** The cursor for the page after this one, or empty at the end of the list. */
  nextCursor: string;
}

/**
 * Reads one page of the activity stream for the view-all page, narrowed by kind and project, in the
 * shapes the feed draws. The page is not kept in the store: the view-all page shows one page of its
 * own, and the newest page is what Home shows (`homeFeedSyncer`).
 */
export async function readActivityPage(
  api: ApiClient,
  query: ActivityQuery = {},
): Promise<ActivityFeedPage> {
  const page = await api.homeActivity(query);
  return { items: toStoredFeedItems(page.items), nextCursor: page.nextCursor };
}

/**
 * One page of the view-all list, the way `M.loadActivityPage` answers it: the daemon's own page,
 * kind and project narrowed server-side, once S20 is switched; until then, the mock's single page
 * of `S.feed` narrowed in memory, with no further page to load.
 */
export async function homeActivityPage(
  ctx: Ctx,
  query: ActivityQuery = {},
): Promise<ActivityFeedPage> {
  const api = ctx.env.data?.api;
  if (isDaemon("S20", sectionsOf(ctx.env)) && api) return readActivityPage(api, query);
  const items = ctx.S.feed.filter(
    (item) =>
      (!query.kind || item.kind === query.kind) && (!query.project || item.pid === query.project),
  );
  return { items, nextCursor: "" };
}
