import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import {
  createTestContext,
  MOCK_CARDS_AND_HISTORY,
  MOCK_PERSON_SECTIONS,
} from "~/testing/test-store";
import type { Env } from "../context";
import { createIds } from "../ids";
import { createProtoShape } from "../testing/proto-shape";
import { loadPrototype, snapshot } from "../testing/prototype";
import { buildSeed } from ".";

const FIXED = new Date("2026-09-24T10:00:00");

const env = (): Env => ({
  hash: "#nosim",
  storage: window.localStorage,
  viewport: { w: window.innerWidth, h: window.innerHeight },
  applyTheme: () => {},
  // This suite proves the seed the reservoir draws from, so the Home feed stays on the mock too,
  // whatever the register says.
  sections: { ...MOCK_CARDS_AND_HISTORY, ...MOCK_PERSON_SECTIONS, S17: "mock", S20: "mock" },
});

describe("seed", () => {
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED });
    window.localStorage.clear();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it("matches the prototype's initial state value for value", () => {
    const proto = loadPrototype("#nosim");
    // The store starts with no projects: they come in through the mirror, like the daemon's.
    const ctx = createTestContext(env());
    const expected = snapshot(proto.S) as Record<string, unknown>;
    // The store names a card by project and number; the prototype numbers cards across projects.
    const actual = createProtoShape({ S: ctx.S }, proto).shape(snapshot(ctx.S)) as Record<
      string,
      unknown
    >;
    // `ready` is set by index.ts after boot; `resolvedTheme` by the DOM theme hook.
    for (const key of ["ready", "resolvedTheme"]) {
      delete expected[key];
      delete actual[key];
    }
    expect(Object.keys(expected).length).toBeGreaterThan(60);
    expect(expected.cards).toHaveLength(29);
    expect(JSON.stringify(expected.chats)).toContain('"id":"ch');
    expect(actual).toEqual(expected);
  });

  it("uses unique ids for cards, messages, chats, activity, and feed", () => {
    const seed = buildSeed(createIds(), FIXED.getTime());
    const cardIds = seed.cards.map((c) => c.id);
    expect(new Set(cardIds).size).toBe(cardIds.length);
    const msgIds = [
      ...Object.values(seed.chat).flat(),
      ...Object.values(seed.chats)
        .flat()
        .flatMap((ch) => ch.msgs),
    ].map((m) => m.id);
    const chatIds = Object.values(seed.chats)
      .flat()
      .map((ch) => ch.id);
    const actIds = Object.values(seed.act)
      .flat()
      .map((a) => a.id);
    const feedIds = seed.feed.map((f) => f.id);
    // The Upgrade grpc-go chat reuses three starter messages, as in the prototype.
    const all = [...new Set(msgIds), ...chatIds, ...actIds, ...feedIds];
    expect(new Set(all).size).toBe(all.length);
  });

  it("resolves every card dependency, member, and project", () => {
    const seed = buildSeed(createIds(), FIXED.getTime());
    const ids = new Set(seed.cards.map((c) => c.id));
    const people = new Set(seed.people.map((p) => p.id));
    const projects = new Set(PROTOTYPE_PROJECTS.map((p) => p.id));
    expect(seed.cards.flatMap((c) => c.deps).every((id) => ids.has(id))).toBe(true);
    expect(seed.cards.flatMap((c) => c.members).every((id) => people.has(id))).toBe(true);
    expect(seed.cards.every((c) => projects.has(c.p))).toBe(true);
  });

  it("resolves every card that chats, the feed, and notices point to", () => {
    const seed = buildSeed(createIds(), FIXED.getTime());
    const ids = new Set(seed.cards.map((c) => c.id));
    const chatRefs = Object.values(seed.chats)
      .flat()
      .flatMap((ch) => ch.msgs)
      .flatMap((m) => {
        if (m.k === "card") return [m.cardId];
        if (m.k === "links") return m.cards;
        return m.k === "approval" && m.cardId ? [m.cardId] : [];
      });
    const feedRefs = seed.feed.flatMap((f) => (f.cardId ? [f.cardId] : []));
    const noticeRefs = seed.notices.flatMap((n) => {
      if (n.kind === "sleep") return n.cards;
      return n.cardId ? [n.cardId] : [];
    });
    expect(chatRefs).toEqual([
      "api#43",
      "api#44",
      "api#44",
      "api#42",
      "api#33",
      "web#119",
      "web#121",
      "mobile#213",
      "mobile#207",
    ]);
    expect(feedRefs).toHaveLength(6);
    expect(noticeRefs).toEqual(["api#39", "api#36", "web#116", "mobile#213"]);
    for (const id of [...chatRefs, ...feedRefs, ...noticeRefs]) expect(ids.has(id)).toBe(true);
  });

  it("gives every card a chat, an activity log, and checks", () => {
    const seed = buildSeed(createIds(), FIXED.getTime());
    for (const c of seed.cards) {
      expect(seed.chat[c.id]).toBeDefined();
      expect(seed.act[c.id]).toBeDefined();
      expect(seed.checks[c.id]?.length).toBeGreaterThanOrEqual(3);
    }
  });

  it("builds independent copies, so two stores never share arrays", () => {
    const a = buildSeed(createIds(), FIXED.getTime());
    const b = buildSeed(createIds(), FIXED.getTime());
    expect(a.cards[0]?.labels).not.toBe(b.cards[0]?.labels);
    expect(a.cards[0]?.members).not.toBe(b.cards[0]?.members);
    expect(a.cards[0]?.comments[0]?.att).not.toBe(b.cards[0]?.comments[0]?.att);
    expect(a.schedules[0]?.days).not.toBe(b.schedules[0]?.days);
    expect(a.people).not.toBe(b.people);
  });
});
