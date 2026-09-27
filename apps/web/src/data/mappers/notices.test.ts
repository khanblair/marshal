import type { NoticeList, Notice as WireNotice } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "../testing/golden";
import { toNotice, toNotices } from "./notices";

/* The list the real daemon answers with, written by its own Go tests, so the two sides cannot drift. */
const list = golden<NoticeList>("notice-list");

/** The store's own card keys, as the daemon's opaque ids: the two the golden names, and nothing else. */
const keyOf = (daemonId: string): string | null =>
  daemonId.endsWith("C3") ? "web#1" : daemonId.endsWith("C4") ? "web#2" : null;

describe("toNotice", () => {
  it("maps the golden sleep group, translating its cards into the store's keys", () => {
    const wire = list.notices[0] as WireNotice;
    expect(toNotice(wire, keyOf)).toEqual({
      id: "sleep:web-dashboard",
      kind: "sleep",
      cards: ["web#1", "web#2"],
      deadline: Date.parse("2026-09-27T09:32:00.000Z"),
      ts: Date.parse("2026-09-27T09:30:00.000Z"),
    });
  });

  it("leaves out a card the store cannot place, and drops a group left with none", () => {
    const wire = list.notices[0] as WireNotice;
    expect(toNotice(wire, (id) => (id.endsWith("C3") ? "web#1" : null))).toMatchObject({
      cards: ["web#1"],
    });
    expect(toNotice(wire, () => null)).toBeNull();
  });

  it("stands the notice's own time in for a deadline the daemon did not send", () => {
    const wire = { ...(list.notices[0] as WireNotice), deadline: undefined };
    expect(toNotice(wire, keyOf)).toMatchObject({
      deadline: Date.parse("2026-09-27T09:30:00.000Z"),
    });
  });

  it("keeps an informational notice with its own two sentences and project", () => {
    const wire: WireNotice = {
      id: "cost:global",
      kind: "cost",
      text: "You are near today's cost limit",
      sub: "Raise it or pause the cards",
      projectId: "web-dashboard",
      createdAt: "2026-09-27T09:30:00.000Z",
    };
    expect(toNotice(wire, keyOf)).toEqual({
      id: "cost:global",
      kind: "cost",
      pid: "web-dashboard",
      text: "You are near today's cost limit",
      sub: "Raise it or pause the cards",
      ts: Date.parse("2026-09-27T09:30:00.000Z"),
    });
  });

  it("draws nothing for a kind this build does not know", () => {
    const wire = { ...(list.notices[0] as WireNotice), kind: "mention" } as unknown as WireNotice;
    expect(toNotice(wire, keyOf)).toBeNull();
  });
});

describe("toNotices", () => {
  it("maps the whole golden list", () => {
    expect(toNotices(list, keyOf)).toHaveLength(1);
    expect(toNotices(list, () => null)).toEqual([]);
  });

  it("answers an empty list for an empty one, never null", () => {
    expect(toNotices({ notices: [] }, keyOf)).toEqual([]);
  });
});
