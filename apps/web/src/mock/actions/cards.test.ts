import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FIXED_TIME, makeTwin, SLOW_TEST_MS, type Twin } from "../testing/twin";
import type { Card } from "../types";

const cardOf = (t: Twin, id: number): Card => {
  const c = t.port.S.cards.find((x) => x.id === id);
  if (!c) throw new Error(`no card ${id}`);
  return c;
};
const lastToast = (t: Twin): string | undefined => t.port.S.toasts.at(-1)?.msg;

describe("card actions", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  describe("moveCard refusals", () => {
    it.each([
      [33, "working", "Done cards are merged. Fork the card to keep working on it."],
      [41, "done", "Cards move to Done by themselves after they merge."],
      [41, "needs", "Cards move to Needs you by themselves when an agent is waiting on you."],
      [
        45,
        "review",
        "In review needs an open pull request. The agent opens one when the work is ready.",
      ],
      [
        46,
        "review",
        "In review needs an open pull request. The agent opens one when the work is ready.",
      ],
      [41, "ready", "Ready to merge needs an approved review and passing checks."],
      [40, "ready", "Checks haven't passed on this card yet, so it can't be ready to merge."],
      [35, "review", "The Integrator is merging this card. Wait for the merge to finish."],
    ] as const)("#%i to %s is refused and snaps back", (id, to, why) => {
      const before = cardOf(t, id).state;
      t.run("moveCard", id, to, true);
      expect(cardOf(t, id).state).toBe(to);
      t.play(500, 100);
      expect(cardOf(t, id).state).toBe(before);
      expect(lastToast(t)).toBe(why);
      expect(t.port.refuse(cardOf(t, id), t.port.colOf(before), to)).toBe(why);
    });

    it("allows moves that no rule refuses", () => {
      expect(t.port.refuse(cardOf(t, 39), "review", "ready")).toBeNull();
    });
  });

  it.each([
    [45, "working", "Card started"],
    [45, "planning", "Card started"],
    [41, "backlog", "Card moved to backlog. Session is asleep."],
    [39, "working", "Sent back to working"],
    [46, "working", "Plan skipped"],
    [41, "review", "Pull request opened"],
    [39, "ready", "Ready to merge"],
  ] as const)("moves #%i to %s like the prototype", (id, to, msg) => {
    t.run("moveCard", id, to, true);
    t.same();
    expect(t.port.S.toasts.map((x) => x.msg)).toContain(msg);
    expect(t.port.S.announce).toContain(`#${id} moved to`);
    t.play(4000);
  });

  it("ignores a move to the same column and unknown cards", () => {
    t.run("moveCard", 41, "working");
    t.run("moveCard", 9999, "working");
    t.same();
    expect(t.port.S.toasts).toHaveLength(0);
  });

  it.each([33, 45, 41])("forks #%i into a new card", (id) => {
    t.run("fork", id);
    t.same();
    const f = t.port.S.cards.at(-1);
    expect(f?.id).toBe(300);
    expect(f?.title).toBe(`${cardOf(t, id).title} (fork)`);
    expect(lastToast(t)).toBe("Card forked");
    t.play(9000, 1000);
  });

  it("deletes a card after the confirm dialog", () => {
    t.run("openCard", 41);
    t.run("deleteCard", 41);
    expect(t.port.S.dialog?.message).toBe(
      "This deletes #41, its session, and its worktree with unmerged work on marshal/41-fix-token-refresh.",
    );
    t.confirmDialog();
    t.same();
    expect(t.port.card(41)).toBeUndefined();
    expect(t.port.S.openId).toBeNull();
    t.run("deleteCard", 45);
    expect(t.port.S.dialog?.message).toBe("This deletes #45 and its notes.");
  });

  it("renames only to a non-empty title", () => {
    t.run("rename", 41, "  New title  ");
    t.run("rename", 42, "   ");
    t.same();
    expect(cardOf(t, 41).title).toBe("New title");
  });

  it.each([
    ["agent", "Codex"],
    ["agent", "Built-in agent"],
    ["model", "deepseek-chat"],
    ["think", "Low"],
    ["role", "Tester"],
    ["perm", "Ask"],
  ] as const)("sets %s to %s", (key, val) => {
    t.run("setSetting", 41, key, val);
    t.run("setSetting", 45, key, val);
    t.same();
  });

  it("brings thinking back when the model supports it again", () => {
    t.run("setSetting", 45, "model", "gpt-5-mini");
    t.same();
    expect(cardOf(t, 45).think).toBe("Medium");
  });

  it("asks before turning on bypass, then turns it off", () => {
    t.run("setSetting", 41, "perm", "Bypass permissions");
    expect(t.port.S.dialog?.ack).toBe(
      "I understand the agent can run any command in the worktree without asking.",
    );
    t.confirmDialog();
    t.same();
    expect(cardOf(t, 41).bypass).toBe(true);
    t.run("turnOffBypass", 41);
    t.same();
    expect(cardOf(t, 41).perm).toBe("Full auto");
  });

  it.each(["backlog", "planning", "working"] as const)("quick-adds a card in %s", (col) => {
    t.run("quickAdd", "api", col, "  Wire up metrics  ");
    t.run("quickAdd", "api", col, "   ");
    t.same();
    const c = t.port.S.cards.at(-1);
    expect(c?.title).toBe("Wire up metrics");
    expect(c?.upd).toBe(FIXED_TIME.getTime());
    t.play(4000, 500);
  });

  it("creates a card from the New card dialog", () => {
    t.run("go", "project", "api");
    t.run("newCard", { template: "Plan first", start: true });
    t.run("createCard");
    t.same();
    expect(t.port.S.newCard?.title).toBe("");
    const draft = {
      title: "Audit logs",
      body: "Add audit logs",
      template: "Plan first",
      start: true,
    };
    t.run("newCard", draft);
    t.run("createCard");
    t.same();
    expect(t.port.S.newCard).toBeNull();
    expect(cardOf(t, 300).perm).toBe("Plan only");
    t.play(4000, 500);
    expect(t.port.dupes("Audit the logs")).toHaveLength(1);
  });
});

describe("sessions", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it.each([
    [41, "Working cards don't sleep. Pause the card first."],
    [43, "This card is waiting on you, so it stays awake."],
    [45, "This card has no awake session."],
    [39, "Card asleep"],
  ] as const)("sleep on #%i says %s", (id, msg) => {
    t.run("sleep", id);
    t.same();
    expect(lastToast(t)).toBe(msg);
  });

  it("pauses, sleeps, wakes, and resumes a working card", () => {
    t.run("pause", 43);
    t.run("pause", 41);
    t.run("sleep", 41);
    t.same();
    expect(cardOf(t, 41).asleep).toBe(true);
    t.run("wake", 41);
    t.play(2000, 200);
    t.run("start", 41);
    t.run("start", 45);
    t.play(4000, 500);
    expect(cardOf(t, 45).state).toBe("working");
  });

  it("pins, keeps awake, and settles the sleep notice", () => {
    t.run("pin", 39);
    t.run("keepAwake", 36);
    t.same();
    expect(t.port.S.notices.find((n) => n.kind === "sleep")).toMatchObject({ cards: [116] });
    t.run("pin", 39);
    t.run("keepAllAwake");
    t.run("sleepAll");
    t.run("dismissNotice", "n2");
    t.same();
    expect(t.port.S.notices.map((n) => n.id)).toEqual(["n3"]);
  });

  it("sleeps every card of the notice at once", () => {
    t.run("sleepAll");
    t.same();
    expect(lastToast(t)).toBe("3 cards asleep");
    expect(cardOf(t, 116).asleep).toBe(true);
  });

  it("drops the notice when its last card is taken off", () => {
    for (const id of [39, 36, 116]) t.run("keepAwake", id);
    t.same();
    expect(t.port.S.notices.some((n) => n.kind === "sleep")).toBe(false);
  });
});
