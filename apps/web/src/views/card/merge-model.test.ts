import { describe, expect, it } from "vitest";
import { isMergeStop, shortBranch } from "./merge-model";

describe("isMergeStop", () => {
  it("is true for a conflict that waits on a person", () => {
    expect(
      isMergeStop({ state: "needs", reasonKind: "conflict", reason: "api.go conflicts." }),
    ).toBe(true);
  });

  it("is true for a failed CI only when the sentence names the merge", () => {
    const text = "The merge queue's tests did not pass: 2 failed.";
    expect(isMergeStop({ state: "needs", reasonKind: "ci-failed", reason: text })).toBe(true);
    expect(
      isMergeStop({
        state: "needs",
        reasonKind: "ci-failed",
        reason: "CI is still failing on this card's branch after 3 rounds.",
      }),
    ).toBe(false);
  });

  it("is false for every other reason, and for a card that does not wait", () => {
    expect(isMergeStop({ state: "needs", reasonKind: "question", reason: "merge?" })).toBe(false);
    expect(
      isMergeStop({ state: "needs", reasonKind: undefined, reason: "The merge failed." }),
    ).toBe(false);
    expect(isMergeStop({ state: "ready", reasonKind: "conflict", reason: "conflict" })).toBe(false);
  });
});

describe("shortBranch", () => {
  it("drops a full ref's prefix and leaves a short name alone", () => {
    expect(shortBranch("refs/heads/development")).toBe("development");
    expect(shortBranch("marshal/41-fix-token-refresh")).toBe("marshal/41-fix-token-refresh");
  });

  it("answers an empty string for no branch", () => {
    expect(shortBranch(null)).toBe("");
    expect(shortBranch(undefined)).toBe("");
  });
});
