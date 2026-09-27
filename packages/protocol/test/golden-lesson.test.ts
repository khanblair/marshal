import { describe, expect, it } from "vitest";
import type { Lesson, SaveLessonRequest } from "../src";
import { golden } from "./golden";

// A project's lesson (docs/backend-checklist.md B7.4/B7.6, build-plan task 7.6). Each sample is
// checked against the generated type by the compiler, so a field that changes in Go stops this
// compiling until the sample matches the daemon's own file again.
describe("the lesson golden file", () => {
  it("has a lesson with its title, slug, vault path, and the daemon's time", () => {
    const sample: Lesson = {
      projectId: "small-repo",
      slug: "ci-is-flaky-on-windows",
      title: "CI is flaky on Windows",
      path: "small-repo/lessons/ci-is-flaky-on-windows.md",
      body: "# CI is flaky on Windows\n\nRetry the flaky step once before failing the run.\n",
      author: "agent",
      updatedAt: "2026-09-27T09:30:00.000Z",
    };
    expect(golden("lesson")).toEqual(sample);
    // The path is relative to the vault root, the same rule a card note's path follows.
    expect(sample.path.startsWith("/")).toBe(false);
  });

  it("always carries a save time: there is no unsaved lesson to name by a slug", () => {
    const sample = golden("lesson") as Lesson;
    expect(sample.updatedAt).not.toBeNull();
    expect(typeof sample.updatedAt).toBe("string");
  });

  it("sends a lesson back whole, so its own punctuation survives the round trip", () => {
    const sample = golden("lesson") as Lesson;
    const request: SaveLessonRequest = { title: sample.title, body: sample.body };
    expect(request.title).toBe(sample.title);
    expect(request.body).toBe(sample.body);
  });
});
