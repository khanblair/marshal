import { describe, expect, it } from "vitest";
import { CHECK_LOOKS, percentDone } from "./checks-model";

describe("percentDone", () => {
  it("rounds to a whole percent and is zero for an empty list", () => {
    expect(percentDone(3, 4)).toBe(75);
    expect(percentDone(1, 3)).toBe(33);
    expect(percentDone(0, 0)).toBe(0);
  });
});

describe("CHECK_LOOKS", () => {
  it("has a look for every check state", () => {
    expect(Object.keys(CHECK_LOOKS).sort()).toEqual(["failed", "passed", "pending", "running"]);
    expect(CHECK_LOOKS.failed.label).toBe("Failed");
  });
});
