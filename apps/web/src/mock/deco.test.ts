import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CardKey } from "./card-key";
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

  const protoDeco = (key: CardKey): unknown => {
    const c = t.proto.call("card", t.protoId(key));
    return t.proto.call("deco", c);
  };

  it("deco for every card", () => {
    t.run("openCard", "api#41");
    t.run("set", { focusId: "web#118", dragId: "mobile#209" });
    for (const c of t.port.S.cards) {
      expect(t.shape(JSON.parse(JSON.stringify(t.port.deco(c))))).toEqual(
        JSON.parse(JSON.stringify(protoDeco(c.id))),
      );
    }
  });

  it("decoMsgs for every card chat and project chat", () => {
    for (const c of t.port.S.cards) {
      const port = t.port.decoMsgs(t.port.S.chat[c.id] ?? [], c.id);
      const proto = t.proto.call(
        "decoMsgs",
        (t.proto.S as { chat: Record<number, unknown> }).chat[t.protoId(c.id) ?? 0],
        t.protoId(c.id),
      );
      expect(t.shape(JSON.parse(JSON.stringify(port)))).toEqual(JSON.parse(JSON.stringify(proto)));
    }
    for (const pid of ["api", "web", "mobile"]) {
      for (const [i, ch] of (t.port.S.chats[pid] ?? []).entries()) {
        const protoChat = (t.proto.S as { chats: Record<string, { msgs: unknown }[]> }).chats[
          pid
        ]?.[i];
        const port = t.shape(JSON.parse(JSON.stringify(t.port.decoMsgs(ch.msgs, null))));
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
    t.run("openCard", "api#44");
    const r = t.ask("commands");
    expect(r.port).toEqual(r.proto);
    expect(t.port.commands().map((c) => c.label)).toContain("Approve on #44");
  });

  it("other queries", () => {
    const pairs: [string, ...unknown[]][] = [
      ["cardsOf", "web"],
      ["filtered", "api"],
      ["person", "godana"],
      ["pendingApproval", "api#43"],
      ["chatById", "api", t.port.S.chats.api?.[0]?.id],
      ["dupes", "Fix the token refresh flow"],
      ["dupes", "token"],
      ["diffFor", t.port.card("api#41")],
      ["diffFor", t.port.card("web#118")],
      ["diffFor", t.port.card("api#45")],
      ["filesFor", t.port.card("mobile#209")],
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
      const port = t.port.colCards(t.port.S.cards, col).map((c) => t.protoId(c.id));
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

  const d = (id: CardKey) => {
    const c = t.port.card(id);
    if (!c) throw new Error(`no card ${id}`);
    return t.port.deco(c);
  };

  it.each([
    ["api#45", "backlog", "var(--color-border)", "var(--color-text-muted)"],
    [
      "api#46",
      "planning",
      "var(--color-status-planning-solid)",
      "var(--color-status-planning-solid)",
    ],
    ["api#41", "working", "var(--color-status-working-solid)", "var(--color-status-working-solid)"],
    [
      "api#43",
      "needs",
      "var(--color-status-needs-you-solid)",
      "var(--color-status-needs-you-solid)",
    ],
    ["api#39", "review", "var(--color-status-review-solid)", "var(--color-status-review-solid)"],
    ["api#36", "ready", "var(--color-status-ready-solid)", "var(--color-status-ready-solid)"],
    ["api#35", "merging", "var(--color-status-ready-solid)", "var(--color-status-ready-solid)"],
    ["api#33", "done", "var(--color-status-done-solid)", "var(--color-status-done-solid)"],
  ] as [CardKey, Status, string, string][])(
    "%s in %s gets its edge and icon colors",
    (id, state, edge, icon) => {
      expect(d(id)).toMatchObject({ state, edge, iconColor: icon });
    },
  );

  it("dims done and sleeping titles", () => {
    expect(d("api#33").titleColor).toBe("var(--color-text-secondary)");
    expect(d("web#115").titleColor).toBe("var(--color-text-secondary)");
    expect(d("api#41").titleColor).toBe("var(--color-text-primary)");
  });

  it("builds footer flags, counts, and avatars", () => {
    expect(d("api#41")).toMatchObject({
      hasFooter: true,
      clLabel: "4/7",
      clTip: "4 of 7 checklist items done",
      commentsTip: "3 comments",
      attachTip: "2 attachments",
      hasAttach: true,
      ciTip: "CI running",
    });
    expect(d("api#41").avatars.map((a) => a.initials || a.title)).toEqual([
      "AO",
      "BA",
      "Claude Code agent",
    ]);
    expect(d("api#45").avatars).toEqual([]);
    expect(d("api#45")).toMatchObject({
      hasFooter: false,
      hasModel: false,
      hasThink: false,
      hasAvatars: false,
    });
    expect(d("web#119").commentsTip).toBe("1 comment");
  });

  it("writes an aria label with the reason, sleep, pin, and bypass", () => {
    expect(d("api#43").aria).toBe(
      "#43 Add rate limiting per API key. Needs you: Plan ready for review",
    );
    expect(d("mobile#208").aria).toBe("#208 Typed errors in api-client. Ready to merge. Asleep");
    expect(d("mobile#207").aria).toBe("#207 Shared Button component variants. In review. Pinned");
    expect(d("mobile#209").aria).toBe(
      "#209 Biometric login on Android. Working. Bypass permissions on",
    );
  });

  it("shows what a live card is doing unless it sleeps or is paused", () => {
    expect(d("api#41").showDoing).toBe(true);
    expect(d("web#117").showDoing).toBe(false);
    t.port.pause("api#41");
    expect(d("api#41")).toMatchObject({ showDoing: false, paused: true });
  });

  it("marks selection, focus, and dragging", () => {
    t.port.openCard("api#41");
    expect(d("api#41")).toMatchObject({ selected: true, ring: "0 0 0 2px var(--color-ink)" });
    t.port.set({ openId: null, focusId: "api#41", dragId: "api#41" });
    expect(d("api#41")).toMatchObject({
      focused: true,
      ring: "0 0 0 2px var(--color-border-strong)",
      opacity: "0.4",
    });
  });

  it("opens the card on click, unless a drag just ended", () => {
    d("web#118").open();
    expect(t.port.S.openId).toBe("web#118");
    d("api#41").key2(new KeyboardEvent("keydown", { key: "Enter" }));
    expect(t.port.S.openId).toBe("api#41");
    d("web#118").key2(new KeyboardEvent("keydown", { key: "a" }));
    expect(t.port.S.openId).toBe("api#41");
  });

  it("labels the session for waking cards", () => {
    t.port.wake("web#115");
    expect(d("web#115")).toMatchObject({ sleepLabel: "Waking", awakeLabel: "Waking" });
    expect(d("api#33").awakeLabel).toBe("No session");
    expect(d("api#41").awakeLabel).toBe("Awake");
  });
});
