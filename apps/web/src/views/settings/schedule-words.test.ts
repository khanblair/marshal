import { describe, expect, it } from "vitest";
import {
  briefAction,
  clockWords,
  normalDays,
  orderedSections,
  readInterval,
  toggled,
  whenForDays,
  whenForInterval,
} from "./schedule-words";

const catalog = {
  sections: [
    { id: "calendar", label: "Calendar", hint: "" },
    { id: "needs-you", label: "Needs you", hint: "" },
    { id: "stale", label: "Stale cards", hint: "" },
  ],
  channels: [
    { id: "telegram", label: "Telegram" },
    { id: "discord", label: "Discord" },
  ],
};

describe("the sentence a time is written as", () => {
  it.each([
    [[1, 2, 3, 4, 5], "08:00", "Every weekday at 8:00"],
    [[0, 6], "10:30", "Every weekend at 10:30"],
    [[], "06:00", "Every day at 6:00"],
    [[0, 1, 2, 3, 4, 5, 6], "06:00", "Every day at 6:00"],
    [[1], "09:00", "Every Monday at 9:00"],
    [[0], "18:00", "Every Sunday at 18:00"],
    [[5, 1, 3], "08:15", "Every Mon, Wed and Fri at 8:15"],
    [[1, 5], "13:00", "Every Mon and Fri at 13:00"],
  ])("days %j at %s read %s", (days, time, words) => {
    expect(whenForDays(days, time)).toBe(words);
  });

  it("keeps the days in the order of a week, with Sunday last, and every day as none", () => {
    expect(normalDays([0, 6, 1])).toEqual([1, 6, 0]);
    expect(normalDays([7, 1])).toEqual([1, 0]);
    expect(normalDays([0, 1, 2, 3, 4, 5, 6])).toEqual([]);
  });

  it("writes a clock the way people say it", () => {
    expect(clockWords("08:05")).toBe("8:05");
    expect(clockWords("13:00")).toBe("13:00");
    expect(clockWords("soon")).toBe("soon");
  });

  it("reads an interval back from its words", () => {
    expect(readInterval(whenForInterval(4, "hours"))).toEqual({ every: 4, unit: "hours" });
    expect(readInterval("Every 30 minutes")).toEqual({ every: 30, unit: "minutes" });
    expect(readInterval("Every weekday at 9:00")).toBeNull();
  });
});

describe("what a brief says about itself", () => {
  it("names its parts and where it goes", () => {
    expect(briefAction(["calendar", "needs-you"], ["telegram", "discord"], catalog)).toBe(
      "Calendar, Needs you, sent to Telegram and Discord",
    );
    expect(briefAction(["stale"], [], catalog)).toBe("Stale cards, kept in History");
  });

  it("lists the parts it has first, in its own order, then the rest", () => {
    expect(orderedSections(["stale", "calendar"], catalog.sections).map((s) => s.id)).toEqual([
      "stale",
      "calendar",
      "needs-you",
    ]);
  });

  it("adds a part at the end and takes it out when it is there", () => {
    expect(toggled(["a", "b"], "c")).toEqual(["a", "b", "c"]);
    expect(toggled(["a", "b"], "a")).toEqual(["b"]);
  });
});
