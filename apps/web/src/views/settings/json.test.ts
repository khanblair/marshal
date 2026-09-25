import { describe, expect, it } from "vitest";
import { cloneJson, sameJson } from "./json";

describe("cloneJson", () => {
  it("copies deeply, so an edit to the copy leaves the original alone", () => {
    const original = { name: "Worker", limits: { time: 60 }, skills: ["a"] };
    const copy = cloneJson(original);
    copy.limits.time = 90;
    copy.skills.push("b");
    expect(original).toEqual({ name: "Worker", limits: { time: 60 }, skills: ["a"] });
    expect(copy.limits.time).toBe(90);
  });
});

describe("sameJson", () => {
  it("is true for equal values and false for any difference", () => {
    expect(sameJson({ a: 1, b: [2] }, { a: 1, b: [2] })).toBe(true);
    expect(sameJson({ a: 1 }, { a: 2 })).toBe(false);
    expect(sameJson({ a: 1 }, { a: 1, b: 0 })).toBe(false);
  });

  it("treats a NaN number as different from a number, as JSON turns it into null", () => {
    expect(sameJson({ a: 1 }, { a: Number.NaN })).toBe(false);
  });
});
