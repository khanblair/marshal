import { describe, expect, it } from "vitest";
import { stepAt, TOUR_STEPS } from "./tour-steps";

describe("TOUR_STEPS", () => {
  it("has nine steps, each with a target, a title, and a body", () => {
    expect(TOUR_STEPS).toHaveLength(9);
    for (const step of TOUR_STEPS) {
      expect(step.targets.length).toBeGreaterThan(0);
      expect(step.title).not.toBe("");
      expect(step.body).not.toBe("");
    }
  });

  it("names a phone target after the desktop one where the two differ", () => {
    expect(stepAt(3).targets).toEqual(["projects", "projects-phone"]);
    expect(stepAt(4).targets).toEqual(["new-project", "more-phone"]);
    expect(stepAt(5).targets).toEqual(["views", "views-phone"]);
    expect(stepAt(6).targets).toEqual(["search", "more-phone"]);
  });
});

describe("stepAt", () => {
  it("returns the step at an index", () => {
    expect(stepAt(0).title).toBe("Summary and charts");
    expect(stepAt(8).title).toBe("Your profile");
  });

  it("returns the first step for an index out of range", () => {
    expect(stepAt(9)).toBe(stepAt(0));
    expect(stepAt(-1)).toBe(stepAt(0));
  });
});
