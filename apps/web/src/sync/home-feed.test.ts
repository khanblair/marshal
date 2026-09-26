import type { FeedEntry, Page } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { createApiClient } from "~/data/api-client";
import { sectionStatus } from "~/data/sections";
import { fakeFetch, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";
import { createFakeDaemon } from "~/testing/fake-daemon";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import {
  applyHomeFeed,
  applyHomeFeedEvent,
  homeActivityPage,
  homeFeedSyncer,
  readActivityPage,
} from "./home-feed";

// Section S20: the Home activity stream (docs/backend-checklist.md B2.3). The syncer loads the
// daemon's newest page and keeps it current from the activity.created events of the home topic.

const page = golden<Page<FeedEntry>>("home-activity");
const created = golden<{ entry: FeedEntry }>("activity-created");

describe("the Home activity section", () => {
  it("is section S20, follows the home topic, and reads the activity events", () => {
    expect(homeFeedSyncer.section).toBe("S20");
    expect(homeFeedSyncer.topics).toEqual(["home"]);
    expect(homeFeedSyncer.onEvent).toBeTypeOf("function");
  });

  it("makes the store's feed the daemon's page, newest first", () => {
    const ctx = contextOf(createTestMarshal());
    applyHomeFeed(ctx, page.items);
    expect(ctx.S.feed.map((item) => item.kind)).toEqual(["merge", "brief", "ci"]);
    expect(ctx.S.feed[0]).toMatchObject({ id: page.items[0]?.id, pid: "web", cardId: "web#110" });
    expect(ctx.S.feed[1]).toMatchObject({ pid: null, job: "s1" });
  });

  it("applies the same page twice to the same list, so a return does not double it", () => {
    const ctx = contextOf(createTestMarshal());
    applyHomeFeed(ctx, page.items);
    applyHomeFeed(ctx, page.items);
    expect(ctx.S.feed).toHaveLength(3);
  });

  it("puts a new row on top, and does not draw it twice when the event arrives twice", () => {
    const ctx = contextOf(createTestMarshal());
    applyHomeFeed(ctx, page.items);
    // An id the page does not carry, which is what a row appended after the load looks like.
    const fresh = "01M3C107JB041061050R3GG28F";
    const event = eventOf(fresh);
    applyHomeFeedEvent(ctx, event);
    applyHomeFeedEvent(ctx, event);
    expect(ctx.S.feed).toHaveLength(4);
    expect(ctx.S.feed[0]?.id).toBe(fresh);
  });

  it("ignores every other event, and a payload that is not a row", () => {
    const ctx = contextOf(createTestMarshal());
    applyHomeFeed(ctx, page.items);
    const before = ctx.S.feed;
    applyHomeFeedEvent(ctx, { type: "card.moved", data: { cardId: "x" } } as never);
    applyHomeFeedEvent(ctx, { type: "activity.created", data: {} } as never);
    applyHomeFeedEvent(ctx, {
      type: "activity.created",
      data: { entry: { kind: "merge" } },
    } as never);
    expect(ctx.S.feed).toBe(before);
  });
});

describe("one page of the view-all list", () => {
  it("asks for the kind, the project, and the cursor, and answers in the feed's shapes", async () => {
    const { fetch, calls } = fakeFetch({
      "GET /v1/home/activity?limit=5&cursor=abc&kind=merge&project=web": () => jsonAnswer(page),
    });
    const client = createApiClient({ getToken: () => null, fetch });
    const got = await readActivityPage(client, {
      limit: 5,
      cursor: "abc",
      kind: "merge",
      project: "web",
    });
    expect(calls[0]?.url).toBe("/v1/home/activity?limit=5&cursor=abc&kind=merge&project=web");
    expect(got.items.map((item) => item.kind)).toEqual(["merge", "brief", "ci"]);
    expect(got.nextCursor).toBe(page.nextCursor);
  });
});

/** One row of the Home activity stream, for a test that seeds a fake daemon with several. */
function feedEntry(id: string, kind: FeedEntry["kind"], projectId: string): FeedEntry {
  return {
    id,
    kind,
    text: `${id} happened`,
    projectId,
    at: "2026-09-30T12:00:00.000Z",
    cardId: "",
    cardKey: "",
    jobId: "",
  };
}

describe("M.loadActivityPage", () => {
  it("pages the daemon's own stream once S20 is switched, narrowed by kind and project", async () => {
    const activity = [
      feedEntry("f5", "merge", "api"),
      feedEntry("f4", "ci", "api"),
      feedEntry("f3", "merge", "web"),
      feedEntry("f2", "merge", "api"),
      feedEntry("f1", "brief", "api"),
    ];
    const d = createFakeDaemon({ activity });
    const M = createTestMarshal({ data: d.data, sections: { ...sectionStatus, S20: "daemon" } });
    await d.connect();
    const ctx = contextOf(M);
    const first = await homeActivityPage(ctx, { kind: "merge", project: "api", limit: 1 });
    expect(first.items.map((item) => item.id)).toEqual(["f5"]);
    expect(first.nextCursor).not.toBe("");
    const second = await homeActivityPage(ctx, {
      kind: "merge",
      project: "api",
      limit: 1,
      cursor: first.nextCursor,
    });
    expect(second.items.map((item) => item.id)).toEqual(["f2"]);
    expect(second.nextCursor).toBe("");
  });

  it("filters the store's own cached feed while S20 is still on the mock, with no cursor to page further", async () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.feed = [
      { id: "f1", kind: "merge", text: "merged", pid: "api", ts: 1 },
      { id: "f2", kind: "ci", text: "failed", pid: "api", ts: 2 },
      { id: "f3", kind: "merge", text: "merged", pid: "web", ts: 3 },
    ];
    const got = await homeActivityPage(ctx, { kind: "merge", project: "api" });
    expect(got.items.map((item) => item.id)).toEqual(["f1"]);
    expect(got.nextCursor).toBe("");
  });
});

/** One activity.created event as the stream delivers it. */
function eventOf(id: string) {
  return {
    seq: 1,
    topic: "home",
    type: "activity.created",
    at: "2026-09-30T12:00:00.000Z",
    data: { entry: { ...created.entry, id } },
  } as never;
}
