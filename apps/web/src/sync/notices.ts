import {
  type Event as WireEvent,
  EventTypeNoticeCreated,
  EventTypeNoticeDismissed,
  type NoticeList,
} from "@marshal/protocol";
import type { ApiClient } from "~/data/api-client";
import { isRecord } from "~/data/guards";
import { toNotices } from "~/data/mappers/notices";
import type { CardKey } from "~/mock/card-key";
import type { Ctx } from "~/mock/context";
import type { Card } from "~/mock/types";
import type { Syncer } from "./syncer";

/**
 * Section S23: the bell and the notices panel. The daemon holds which notices are standing, answers
 * the whole list, and publishes that same whole list whenever it changes (architecture.md 11.2), so a
 * load, a `notice.created`, and a `notice.dismissed` are all the same one write to the store.
 *
 * The notices themselves are runtime state of the session manager, not a table, so there is nothing
 * to read before the app comes online: `startSync` loads this with every other switched section.
 */
export const noticesSyncer: Syncer<NoticeList> = {
  section: "S23",
  topics: ["home"],
  load: (api: ApiClient) => api.listNotices(),
  apply(ctx, list) {
    applyNoticeList(ctx, list);
  },
  onEvent(ctx: Ctx, event: WireEvent) {
    if (event.type !== EventTypeNoticeCreated && event.type !== EventTypeNoticeDismissed) return;
    if (!isRecord(event.data) || !Array.isArray(event.data.notices)) return;
    applyNoticeList(ctx, { notices: event.data.notices as NoticeList["notices"] });
  },
};

/**
 * The daemon's whole list as the store holds it. Every notice change arrives in this shape, so a
 * snapshot and an event are applied by the same call and applying both is harmless.
 */
export function applyNoticeList(ctx: Ctx, list: NoticeList): void {
  ctx.S.notices = toNotices(list, cardKeysOf(ctx.S.cards));
}

/**
 * The store's own card keys, by the daemon's opaque card ids. A notice names its cards the way the
 * daemon does, and every screen works in keys: `parseCardKey` on an opaque id gives nothing, so a
 * notice left in opaque ids would be hidden the moment a project changed (`sync/reservoir.ts`).
 */
function cardKeysOf(cards: readonly Card[]): (daemonId: string) => CardKey | null {
  const keys = new Map<string, CardKey>();
  for (const card of cards) if (card.daemonId) keys.set(card.daemonId, card.id);
  return (daemonId) => keys.get(daemonId) ?? null;
}
