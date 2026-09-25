import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FIXED_TIME, makeTwin, SLOW_TEST_MS, type Twin } from "./testing/twin";
import type { Card, Status } from "./types";

describe("view models match the prototype", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
    vi.advanceTimersByTime(5000);
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  const protoDeco = (id: number): unknown => {
    const c = t.proto.call("card", id);
    return t.proto.call("deco", c);
  };

  it("deco for every card", () => {
    t.run("openCard", 41);
    t.run("set", { focusId: 118, dragId: 209 });
    for (const c of t.port.S.cards) {
      expect(JSON.parse(JSON.stringify(t.port.deco(c)))).toEqual(
        JSON.parse(JSON.stringify(protoDeco(c.id))),
      );
    }
  });

  it("decoMsgs for every card chat and project chat", () => {
    for (const c of t.port.S.cards) {
      const port = t.port.decoMsgs(t.port.S.chat[c.id] ?? [], c.id);
      const proto = t.proto.call(
        "decoMsgs",
        (t.proto.S as { chat: Record<number, unknown> }).chat[c.id],
        c.id,
      );
      expect(JSON.parse(JSON.stringify(port))).toEqual(JSON.parse(JSON.stringify(proto)));
    }
    for (const pid of ["api", "web", "mobile"]) {
      for (const [i, ch] of (t.port.S.chats[pid] ?? []).entries()) {
        const protoChat = (t.proto.S as { chats: Record<string, { msgs: unknown }[]> }).chats[
          pid
        ]?.[i];
        const port = JSON.parse(JSON.stringify(t.port.decoMsgs(ch.msgs, null)));
        expect(port).toEqual(
          JSON.parse(JSON.stringify(t.proto.call("decoMsgs", protoChat?.msgs, null))),
        );
      }
    }
  });

  it.each([["needs"], ["awake"], ["working"], ["costs"], ["commands"]])(
    "%s() for all projects and each project",
    (name) => {
      for (const pid of [undefined, "api", "web", "mobile"]) {
        const r = t.ask(name, pid);
        expect(r.port).toEqual(r.proto);
      }
    },
  );

  it("commands with an open card and a waiting approval", () => {
    t.run("go", "project", "api");
    t.run("openCard", 44);
    const r = t.ask("commands");
    expect(r.port).toEqual(r.proto);
    expect(t.port.commands().map((c) => c.label)).toContain("Approve on #44");
  });

  it("other queries", () => {
    const pairs: [string, ...unknown[]][] = [
      ["cardsOf", "web"],
      ["filtered", "api"],
      ["person", "godana"],
      ["pendingApproval", 43],
      ["chatById", "api", t.port.S.chats.api?.[0]?.id],
      ["dupes", "Fix the token refresh flow"],
      ["dupes", "token"],
      ["diffFor", t.port.card(41)],
      ["diffFor", t.port.card(118)],
      ["diffFor", t.port.card(45)],
      ["filesFor", t.port.card(209)],
      ["costTone", 9, 10],
      ["costTone", 10, 10],
      ["costTone", 2, 10],
      ["rel", FIXED_TIME.getTime() - 3 * 60_000],
      ["proj", "mobile"],
    ];
    for (const [name, ...args] of pairs) {
      const r = t.ask(name, ...args);
      expect(r.port, name).toEqual(r.proto);
    }
    for (const col of t.port.COLUMNS) {
      const port = t.port.colCards(t.port.S.cards, col).map((c) => c.id);
      const proto = (
        t.proto.call("colCards", (t.proto.S as { cards: unknown }).cards, col) as Card[]
      ).map((c) => c.id);
      expect(port).toEqual(proto);
    }
  });
});

describe("deco details", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  const d = (id: number) => {
    const c = t.port.card(id);
    if (!c) throw new Error(`no card ${id}`);
    return t.port.deco(c);
  };

  it.each([
    [45, "backlog", "var(--color-border)", "var(--color-text-muted)"],
    [46, "planning", "var(--color-status-planning-solid)", "var(--color-status-planning-solid)"],
    [41, "working", "var(--color-status-working-solid)", "var(--color-status-working-solid)"],
    [43, "needs", "var(--color-status-needs-you-solid)", "var(--color-status-needs-you-solid)"],
    [39, "review", "var(--color-status-review-solid)", "var(--color-status-review-solid)"],
    [36, "ready", "var(--color-status-ready-solid)", "var(--color-status-ready-solid)"],
    [35, "merging", "var(--color-status-ready-solid)", "var(--color-status-ready-solid)"],
    [33, "done", "var(--color-status-done-solid)", "var(--color-status-done-solid)"],
  ] as [number, Status, string, string][])(
    "#%i in %s gets its edge and icon colors",
    (id, state, edge, icon) => {
      expect(d(id)).toMatchObject({ state, edge, iconColor: icon });
    },
  );

  it("dims done and sleeping titles", () => {
    expect(d(33).titleColor).toBe("var(--color-text-secondary)");
    expect(d(115).titleColor).toBe("var(--color-text-secondary)");
    expect(d(41).titleColor).toBe("var(--color-text-primary)");
  });

  it("builds footer flags, counts, and avatars", () => {
    expect(d(41)).toMatchObject({
      hasFooter: true,
      clLabel: "4/7",
      clTip: "4 of 7 checklist items done",
      commentsTip: "3 comments",
      attachTip: "2 attachments",
      hasAttach: true,
      ciTip: "CI running",
    });
    expect(d(41).avatars.map((a) => a.initials || a.title)).toEqual([
      "AO",
      "BA",
      "Claude Code agent",
    ]);
    expect(d(45).avatars).toEqual([]);
    expect(d(45)).toMatchObject({
      hasFooter: false,
      hasModel: false,
      hasThink: false,
      hasAvatars: false,
    });
    expect(d(119).commentsTip).toBe("1 comment");
  });

  it("writes an aria label with the reason, sleep, pin, and bypass", () => {
    expect(d(43).aria).toBe("#43 Add rate limiting per API key. Needs you: Plan ready for review");
    expect(d(208).aria).toBe("#208 Typed errors in api-client. Ready to merge. Asleep");
    expect(d(207).aria).toBe("#207 Shared Button component variants. In review. Pinned");
    expect(d(209).aria).toBe("#209 Biometric login on Android. Working. Bypass permissions on");
  });

  it("shows what a live card is doing unless it sleeps or is paused", () => {
    expect(d(41).showDoing).toBe(true);
    expect(d(117).showDoing).toBe(false);
    t.port.pause(41);
    expect(d(41)).toMatchObject({ showDoing: false, paused: true });
  });

  it("marks selection, focus, and dragging", () => {
    t.port.openCard(41);
    expect(d(41)).toMatchObject({ selected: true, ring: "0 0 0 2px var(--color-ink)" });
    t.port.set({ openId: null, focusId: 41, dragId: 41 });
    expect(d(41)).toMatchObject({
      focused: true,
      ring: "0 0 0 2px var(--color-border-strong)",
      opacity: "0.4",
    });
  });

  it("opens the card on click, unless a drag just ended", () => {
    d(118).open();
    expect(t.port.S.openId).toBe(118);
    d(41).key2(new KeyboardEvent("keydown", { key: "Enter" }));
    expect(t.port.S.openId).toBe(41);
    d(118).key2(new KeyboardEvent("keydown", { key: "a" }));
    expect(t.port.S.openId).toBe(41);
  });

  it("labels the session for waking cards", () => {
    t.port.wake(115);
    expect(d(115)).toMatchObject({ sleepLabel: "Waking", awakeLabel: "Waking" });
    expect(d(33).awakeLabel).toBe("No session");
    expect(d(41).awakeLabel).toBe("Awake");
  });
});
