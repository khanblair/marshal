import { describe, expect, it } from "vitest";
import {
  cardKey,
  cardLabel,
  cardLabelIn,
  cardNumber,
  nextCardNumber,
  parseCardKey,
} from "./card-key";

/* These cases are the ones in daemon/internal/protocol/ids_test.go, so both sides agree. */

describe("cardKey", () => {
  it.each([
    ["api", 1, "api#1"],
    ["web-dashboard", 12, "web-dashboard#12"],
    ["app-2-go", 4096, "app-2-go#4096"],
  ])("writes %s and %i as %s", (projectId, number, want) => {
    expect(cardKey(projectId, number)).toBe(want);
  });
});

describe("parseCardKey", () => {
  it.each([
    ["api#1", "api", 1],
    ["web-dashboard#12", "web-dashboard", 12],
    ["app-2-go#4096", "app-2-go", 4096],
    ["ab#2147483647", "ab", 2_147_483_647],
    ["project-42#7", "project-42", 7],
  ])("reads %s", (key, projectId, number) => {
    expect(parseCardKey(key)).toEqual({ projectId, number });
  });

  it("round-trips what cardKey writes", () => {
    for (const [projectId, number] of [
      ["api", 1],
      ["web-dashboard", 12],
      ["app-2-go", 4096],
    ] as const) {
      expect(parseCardKey(cardKey(projectId, number))).toEqual({ projectId, number });
    }
  });

  it.each([
    "",
    "api",
    "api#",
    "#12",
    "api#0",
    "api#012",
    "api#-1",
    "api#+1",
    "api#1.5",
    "api#x",
    "API#1",
    "1api#1",
    "api#1#2",
    "api #1",
    "api#2147483648",
    "api#99999999999999999999",
    // A project id is 2 to 24 characters, like the daemon's rule.
    "a#1",
    `${"a".repeat(25)}#1`,
    "my_project#1",
    "-api#1",
  ])("refuses %j", (key) => {
    expect(parseCardKey(key)).toBeNull();
  });

  it("accepts a project id of the longest length", () => {
    expect(parseCardKey(`${"a".repeat(24)}#3`)).toEqual({ projectId: "a".repeat(24), number: 3 });
  });
});

describe("cardNumber", () => {
  it("is the number of the key, and 0 when the text is not a key", () => {
    expect(cardNumber("api#41")).toBe(41);
    expect(cardNumber("web-dashboard#12")).toBe(12);
    expect(cardNumber("41")).toBe(0);
    expect(cardNumber("")).toBe(0);
  });
});

describe("labels", () => {
  it("shows the number alone, and with the project name for lists that mix projects", () => {
    expect(cardLabel({ n: 41 })).toBe("#41");
    expect(cardLabelIn({ n: 41 }, "api-gateway")).toBe("api-gateway #41");
  });
});

describe("nextCardNumber", () => {
  const cards = [
    { p: "api", n: 3 },
    { p: "api", n: 41 },
    { p: "web", n: 120 },
  ];

  it("is one more than the highest number in that project, and 1 for a project with none", () => {
    expect(nextCardNumber(cards, "api")).toBe(42);
    expect(nextCardNumber(cards, "web")).toBe(121);
    expect(nextCardNumber(cards, "mobile")).toBe(1);
    expect(nextCardNumber([], "api")).toBe(1);
  });
});
