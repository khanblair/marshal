import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type CardKey, cardNumber } from "../card-key";
import { FIXED_TIME, makeTwin, SLOW_TEST_MS, type Twin } from "../testing/twin";
import type { Card } from "../types";

const cardOf = (t: Twin, id: CardKey): Card => {
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
      ["api#33", "working", "Done cards are merged. Fork the card to keep working on it."],
      ["api#41", "done", "Cards move to Done by themselves after they merge."],
      ["api#41", "needs", "Cards move to Needs you by themselves when an agent is waiting on you."],
      [
        "api#45",
        "review",
        "In review needs an open pull request. The agent opens one when the work is ready.",
      ],
      [
        "api#46",
        "review",
        "In review needs an open pull request. The agent opens one when the work is ready.",
      ],
      ["api#41", "ready", "Ready to merge needs an approved review and passing checks."],
      ["api#40", "ready", "Checks haven't passed on this card yet, so it can't be ready to merge."],
      ["api#35", "review", "The Integrator is merging this card. Wait for the merge to finish."],
    ] as const)("%s to %s is refused and snaps back", (id, to, why) => {
      const before = cardOf(t, id).state;
      t.run("moveCard", id, to, true);
      expect(cardOf(t, id).state).toBe(to);
      t.play(500, 100);
      expect(cardOf(t, id).state).toBe(before);
      expect(lastToast(t)).toBe(why);
      expect(t.port.refuse(cardOf(t, id), t.port.colOf(before), to)).toBe(why);
    });

    it("allows moves that no rule refuses", () => {
      expect(t.port.refuse(cardOf(t, "api#39"), "review", "ready")).toBeNull();
    });
  });

  it.each([
    ["api#45", "working", "Card started"],
    ["api#45", "planning", "Card started"],
    ["api#41", "backlog", "Card moved to backlog. Session is asleep."],
    ["api#39", "working", "Sent back to working"],
    ["api#46", "working", "Plan skipped"],
    ["api#41", "review", "Pull request opened"],
    ["api#39", "ready", "Ready to merge"],
  ] as const)("moves %s to %s like the prototype", (id, to, msg) => {
    t.run("moveCard", id, to, true);
    t.same();
    expect(t.port.S.toasts.map((x) => x.msg)).toContain(msg);
    expect(t.port.S.announce).toContain(`#${cardNumber(id)} moved to`);
    t.play(4000);
  });

  it("ignores a move to the same column and unknown cards", () => {
    t.run("moveCard", "api#41", "working");
    t.run("moveCard", "api#9999", "working");
    t.same();
    expect(t.port.S.toasts).toHaveLength(0);
  });

  it.each(["api#33", "api#45", "api#41"])("forks %s into a new card", (id) => {
    t.run("fork", id);
    t.same();
    const f = t.port.S.cards.at(-1);
    expect(f?.id).toBe("api#47");
    expect(f?.title).toBe(`${cardOf(t, id).title} (fork)`);
    expect(lastToast(t)).toBe("Card forked");
    t.play(9000, 1000);
  });

  it("deletes a card after the confirm dialog", () => {
    t.run("openCard", "api#41");
    t.run("deleteCard", "api#41");
    expect(t.port.S.dialog?.message).toBe(
      "This deletes #41, its session, and its worktree with unmerged work on marshal/41-fix-token-refresh.",
    );
    t.confirmDialog();
    t.same();
    expect(t.port.card("api#41")).toBeUndefined();
    expect(t.port.S.openId).toBeNull();
    t.run("deleteCard", "api#45");
    expect(t.port.S.dialog?.message).toBe("This deletes #45 and its notes.");
  });

  it("renames only to a non-empty title", () => {
    t.run("rename", "api#41", "  New title  ");
    t.run("rename", "api#42", "   ");
    t.same();
    expect(cardOf(t, "api#41").title).toBe("New title");
  });

  it.each([
    ["agent", "Codex"],
    ["agent", "Built-in agent"],
    ["model", "deepseek-chat"],
    ["think", "Low"],
    ["role", "Tester"],
    ["perm", "Ask"],
  ] as const)("sets %s to %s", (key, val) => {
    t.run("setSetting", "api#41", key, val);
    t.run("setSetting", "api#45", key, val);
    t.same();
  });

  it("brings thinking back when the model supports it again", () => {
    t.run("setSetting", "api#45", "model", "gpt-5-mini");
    t.same();
    expect(cardOf(t, "api#45").think).toBe("Medium");
  });

  it("asks before turning on bypass, then turns it off", () => {
    t.run("setSetting", "api#41", "perm", "Bypass permissions");
    expect(t.port.S.dialog?.ack).toBe(
      "I understand the agent can run any command in the worktree without asking.",
    );
    t.confirmDialog();
    t.same();
    expect(cardOf(t, "api#41").bypass).toBe(true);
    t.run("turnOffBypass", "api#41");
    t.same();
    expect(cardOf(t, "api#41").perm).toBe("Full auto");
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
    expect(cardOf(t, "api#47").perm).toBe("Plan only");
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
    ["api#41", "Working cards don't sleep. Pause the card first."],
    ["api#43", "This card is waiting on you, so it stays awake."],
    ["api#45", "This card has no awake session."],
    ["api#39", "Card asleep"],
  ] as const)("sleep on %s says %s", (id, msg) => {
    t.run("sleep", id);
    t.same();
    expect(lastToast(t)).toBe(msg);
  });

  it("pauses, sleeps, wakes, and resumes a working card", () => {
    t.run("pause", "api#43");
    t.run("pause", "api#41");
    t.run("sleep", "api#41");
    t.same();
    expect(cardOf(t, "api#41").asleep).toBe(true);
    t.run("wake", "api#41");
    t.play(2000, 200);
    t.run("start", "api#41");
    t.run("start", "api#45");
    t.play(4000, 500);
    expect(cardOf(t, "api#45").state).toBe("working");
  });

  it("pins, keeps awake, and settles the sleep notice", () => {
    t.run("pin", "api#39");
    t.run("keepAwake", "api#36");
    t.same();
    expect(t.port.S.notices.find((n) => n.kind === "sleep")).toMatchObject({ cards: ["web#116"] });
    t.run("pin", "api#39");
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
    expect(cardOf(t, "web#116").asleep).toBe(true);
  });

  it("drops the notice when its last card is taken off", () => {
    for (const id of ["api#39", "api#36", "web#116"]) t.run("keepAwake", id);
    t.same();
    expect(t.port.S.notices.some((n) => n.kind === "sleep")).toBe(false);
  });
});
