import type { NoticeList, Notice as WireNotice } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import type { ApiClient } from "~/data/api-client";
import { golden } from "~/data/testing/golden";
import { boardOf, wireCard } from "~/testing/fake-cards";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { applyCardSnapshot } from "./cards";
import { applyNoticeList, noticesSyncer } from "./notices";

// Section S23: the bell and the notices panel. The daemon holds the notices and answers the whole
// list, and it publishes that same whole list whenever it changes, so a load, a notice.created, and
// a notice.dismissed are all the same one write to the store.

const list = golden<NoticeList>("notice-list");
const C3 = "01JD7Q4M2X8K9V0P5T3RB6NHC3";
const C4 = "01JD7Q4M2X8K9V0P5T3RB6NHC4";

/** A store whose cards include the two the golden notice names, so the notice can be placed. */
function storeWithNoticeCards() {
  const ctx = contextOf(createTestMarshal());
  applyCardSnapshot(ctx, [
    boardOf("web", [
      wireCard({
        id: C3,
        projectId: "web",
        number: 1,
        title: "Refresh the token",
        state: "ready",
        session: "awake",
      }),
      wireCard({
        id: C4,
        projectId: "web",
        number: 2,
        title: "CSV export",
        state: "ready",
        session: "awake",
      }),
    ]),
  ]);
  return ctx;
}

const noticeEvent = (type: string, data: unknown) =>
  ({ seq: 1, topic: "home", type, at: "2026-09-27T09:30:00.000Z", data }) as never;

describe("the notices section", () => {
  it("is section S23, follows the home topic, and loads the daemon's whole list", async () => {
    expect(noticesSyncer.section).toBe("S23");
    expect(noticesSyncer.topics).toEqual(["home"]);
    const asked: string[] = [];
    const api = {
      listNotices: async () => {
        asked.push("listNotices");
        return list;
      },
    } as unknown as ApiClient;
    expect(await noticesSyncer.load(api, contextOf(createTestMarshal()))).toEqual(list);
    expect(asked).toEqual(["listNotices"]);
  });

  it("puts the daemon's list in the store, with the opaque ids translated to card keys", () => {
    const ctx = storeWithNoticeCards();
    applyNoticeList(ctx, list);
    expect(ctx.S.notices).toEqual([
      {
        id: "sleep:web-dashboard",
        kind: "sleep",
        cards: ["web#1", "web#2"],
        deadline: Date.parse("2026-09-27T09:32:00.000Z"),
        ts: Date.parse("2026-09-27T09:30:00.000Z"),
      },
    ]);
  });

  it("replaces the list it had, rather than adding to it", () => {
    const ctx = storeWithNoticeCards();
    applyNoticeList(ctx, list);
    applyNoticeList(ctx, { notices: [] });
    expect(ctx.S.notices).toEqual([]);
  });

  it("applies a notice.created and a notice.dismissed the same way, and ignores anything else", () => {
    const ctx = storeWithNoticeCards();
    noticesSyncer.onEvent?.(ctx, noticeEvent("notice.created", { notices: list.notices }));
    expect(ctx.S.notices).toHaveLength(1);
    const empty = { notices: [] };
    noticesSyncer.onEvent?.(ctx, noticeEvent("notice.dismissed", empty));
    expect(ctx.S.notices).toEqual([]);
    noticesSyncer.onEvent?.(ctx, noticeEvent("card.updated", { card: {} }));
    expect(ctx.S.notices).toEqual([]);
  });

  it("ignores an event whose payload is not a list of notices", () => {
    const ctx = storeWithNoticeCards();
    applyNoticeList(ctx, list);
    noticesSyncer.onEvent?.(ctx, noticeEvent("notice.created", { notices: "nope" }));
    expect(ctx.S.notices).toHaveLength(1);
    noticesSyncer.onEvent?.(ctx, noticeEvent("notice.created", null));
    expect(ctx.S.notices).toHaveLength(1);
  });

  it("does not draw a sleep group whose cards the store does not know", () => {
    const ctx = contextOf(createTestMarshal());
    applyNoticeList(ctx, { notices: [list.notices[0] as WireNotice] });
    expect(ctx.S.notices).toEqual([]);
  });
});
