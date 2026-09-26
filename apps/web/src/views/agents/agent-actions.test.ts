import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type Card, M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { prototypeCards } from "~/testing/prototype-cards";
import { actionsFor, openChatAction, stopSession } from "./agent-actions";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seed = prototypeCards();

const live = (id: CardKey): Card => {
  const c = M.card(id);
  if (!c) throw new Error(`no card ${id}`);
  return c;
};
const labels = (id: CardKey): string[] => actionsFor(live(id)).map((a) => a.label);

beforeEach(() => {
  vi.useFakeTimers();
  M.S.cards = structuredClone(seed);
  M.S.dialog = null;
  M.S.toasts = [];
});
afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe("actionsFor", () => {
  it("offers Open, Sleep, Pin, and Stop on a live card", () => {
    expect(labels("api#41")).toEqual(["Open", "Sleep", "Pin", "Stop"]);
    expect(actionsFor(live("api#41")).map((a) => a.aria)).toEqual([
      "Open #41",
      "Sleep #41",
      "Pin #41",
      "Stop session on #41",
    ]);
  });

  it("offers Wake instead of Sleep and no Stop on a sleeping card", () => {
    expect(labels("web#115")).toEqual(["Open", "Wake", "Pin"]);
  });

  it("offers Unpin, with the pin-off icon, on a pinned card", () => {
    const pin = actionsFor(live("mobile#207")).find((a) => a.label === "Unpin");
    expect(pin?.icon).toBe("pin-off");
    expect(pin?.aria).toBe("Unpin #207");
  });

  it("offers only Open on a done card", () => {
    expect(labels("api#33")).toEqual(["Open"]);
  });

  it("runs the store action of each button", () => {
    const run = (id: CardKey, label: string) =>
      actionsFor(live(id))
        .find((a) => a.label === label)
        ?.run();
    run("api#43", "Open");
    expect(M.S.openId).toBe("api#43");
    run("api#43", "Pin");
    expect(live("api#43").pinned).toBe(true);
    run("web#115", "Wake");
    expect(live("web#115").waking).toBe(true);
    vi.advanceTimersByTime(2000);
    expect(live("web#115").asleep).toBe(false);
    run("api#39", "Sleep");
    expect(live("api#39").asleep).toBe(true);
  });
});

describe("openChatAction", () => {
  it("switches the project to its chat view", () => {
    M.go("project", "api", "agents");
    openChatAction().run();
    expect(M.S.route.view).toBe("chat");
  });
});

describe("stopSession", () => {
  it("asks first, then puts the card to sleep, paused if it was working", () => {
    const card = live("api#41");
    stopSession(card);
    expect(M.S.dialog).toMatchObject({
      title: "Stop session",
      action: "Stop session",
      message:
        "This stops the agent process for #41. The session is kept, and you can resume it later.",
    });
    expect(card.asleep).toBe(false);
    M.S.dialog?.run();
    expect(card.asleep).toBe(true);
    expect(card.paused).toBe(true);
    expect(card.doing).toBe("");
    expect(M.S.toasts.at(-1)?.msg).toBe("Session stopped");
  });

  it("does not mark a card that was not working as paused", () => {
    const card = live("api#39");
    stopSession(card);
    M.S.dialog?.run();
    expect(card.asleep).toBe(true);
    expect(card.paused).toBe(false);
  });
});
