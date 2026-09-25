import { describe, expect, it } from "vitest";
import { type CardKey, cardKey } from "~/mock/card-key";
import {
  barTip,
  breakMessage,
  groupByStart,
  groupLabel,
  isLateFor,
  planMove,
  shortDay,
  spanLabel,
  timelineCards,
} from "./timeline-model";

const DAY_MS = 86_400_000;
const TODAY = new Date(2026, 8, 24).getTime();

interface P {
  id: CardKey;
  n: number;
  title: string;
  s: number | null;
  e: number | null;
  deps: CardKey[];
}
/** A card of the api project: `card(5, ...)` is `api#5`, and `deps` are numbers in that project. */
const card = (n: number, s: number | null, e: number | null, deps: number[] = []): P => ({
  id: cardKey("api", n),
  n,
  title: `Card ${n}`,
  s,
  e,
  deps: deps.map((d) => cardKey("api", d)),
});

describe("barTip", () => {
  it("names the card and its state", () => {
    expect(barTip(card(5, 0, 1), "Working", [])).toBe("#5 Card 5. Working");
  });

  it("adds what it depends on and what it blocks", () => {
    const tip = barTip(card(5, 0, 1, [3, 4]), "Backlog", [card(8, 2, 3), card(9, 2, 3)]);
    expect(tip).toBe("#5 Card 5. Backlog. Depends on #3, #4. Blocks #8, #9");
  });
});

describe("timelineCards", () => {
  it("drops cards without planned dates and sorts by start, then id", () => {
    const list = [card(3, 2, 3), card(1, null, null), card(2, 2, 4), card(4, -1, 0)];
    expect(timelineCards(list).map((c) => c.n)).toEqual([4, 2, 3]);
  });
});

describe("planMove", () => {
  const a = card(1, 0, 3);
  const b = card(2, 5, 6, [1]);
  const all = [a, b];
  const find = (id: CardKey) => all.find((c) => c.id === id);

  it("shifts both dates", () => {
    expect(planMove(b, 2, all, find)).toMatchObject({ s: 7, e: 8, broken: [] });
  });

  it("reports moving a card to start before the card it waits for ends", () => {
    const plan = planMove(b, -3, all, find);
    expect(plan.broken).toEqual(["#2 would start before #1 finishes"]);
  });

  it("reports moving a card to end after a card that waits for it starts", () => {
    const plan = planMove(a, 3, all, find);
    expect(plan.broken).toEqual(["#2 would start before #1 finishes"]);
  });

  it("does not report a dependency that was already broken", () => {
    const early = card(2, 2, 3, [1]);
    const list = [a, early];
    expect(planMove(early, -1, list, (id) => list.find((c) => c.id === id)).broken).toEqual([]);
  });

  it("ignores a dependency that is not on the timeline", () => {
    const lone = card(7, 1, 2, [99]);
    expect(planMove(lone, -5, [lone], () => undefined).broken).toEqual([]);
  });
});

describe("breakMessage", () => {
  it("joins the sentences and says the dependent card still waits", () => {
    expect(breakMessage(["A would start early", "B would start early"])).toBe(
      "A would start early. B would start early. The dependent card still waits for the merge before it starts.",
    );
  });
});

describe("groupByStart", () => {
  it("groups by start day, earliest first, keeping card order", () => {
    const groups = groupByStart([card(1, 3, 4), card(2, -1, 0), card(3, 3, 5), card(4, 0, 0)]);
    expect(groups.map((g) => [g.offset, g.cards.map((c) => c.n)])).toEqual([
      [-1, [2]],
      [0, [4]],
      [3, [1, 3]],
    ]);
  });
});

describe("day labels", () => {
  const short = (offset: number) =>
    new Date(TODAY + offset * DAY_MS).toLocaleDateString(undefined, {
      weekday: "short",
      month: "short",
      day: "numeric",
    });

  it("formats a short day", () => {
    expect(shortDay(TODAY, DAY_MS, 2)).toBe(short(2));
  });

  it("prefixes today's group", () => {
    expect(groupLabel(TODAY, DAY_MS, 0)).toBe(`Today, ${short(0)}`);
    expect(groupLabel(TODAY, DAY_MS, 1)).toBe(short(1));
  });

  it("says One day, or until the end day", () => {
    expect(spanLabel(card(1, 2, 2), TODAY, DAY_MS)).toBe("One day");
    expect(spanLabel(card(1, 2, 4), TODAY, DAY_MS)).toBe(`Until ${short(4)}`);
  });
});

describe("isLateFor", () => {
  it("is late when the card it waits for ends on or after the day this one starts", () => {
    expect(isLateFor(card(1, 0, 3), card(2, 3, 4))).toBe(true);
    expect(isLateFor(card(1, 0, 2), card(2, 3, 4))).toBe(false);
  });
});
