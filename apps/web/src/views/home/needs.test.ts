import { describe, expect, it } from "vitest";
import { countLabel, nothingNeedsYouText, reasonButton, reasonIcon } from "./needs";

describe("reasonIcon", () => {
  it.each([
    ["Plan ready for review", "list-checks"],
    ["Approval needed: run go get", "terminal"],
    ["CI failed on the branch", "circle-x"],
    ["Merge conflict: queue.ts also changed", "git-merge"],
    ["Stuck: same type error 3 times", "repeat"],
    ["Waiting for a reply", "st-needs"],
    ["", "st-needs"],
  ])("maps %j to %s", (reason, icon) => {
    expect(reasonIcon(reason)).toBe(icon);
  });

  it("tests CI before conflict, so a CI reason that mentions a conflict shows the CI icon", () => {
    expect(reasonIcon("CI failed with a merge conflict")).toBe("circle-x");
  });
});

describe("reasonButton", () => {
  it.each([
    ["Plan ready for review", "Review plan"],
    ["Approval needed: run go get", "Review command"],
    ["Merge conflict: queue.ts", "Resolve conflict"],
    ["CI failed on the branch", "See failure"],
    ["Stuck: same type error", "Reply"],
    ["", "Reply"],
  ])("maps %j to %s", (reason, label) => {
    expect(reasonButton(reason)).toBe(label);
  });

  it("tests conflict before CI, the opposite order to the icon", () => {
    expect(reasonButton("CI failed with a merge conflict")).toBe("Resolve conflict");
  });

  it("matches conflict in any case and Plan only at the start", () => {
    expect(reasonButton("Resolve the CONFLICT")).toBe("Resolve conflict");
    expect(reasonButton("Not a Plan")).toBe("Reply");
  });
});

describe("countLabel", () => {
  it("uses the singular only for one", () => {
    expect(countLabel(1)).toBe("1 card");
    expect(countLabel(2)).toBe("2 cards");
    expect(countLabel(0)).toBe("0 cards");
  });
});

describe("nothingNeedsYouText", () => {
  it("counts the working agents", () => {
    expect(nothingNeedsYouText(0)).toBe("Nothing needs you right now. 0 agents are working.");
    expect(nothingNeedsYouText(1)).toBe("Nothing needs you right now. 1 agent is working.");
    expect(nothingNeedsYouText(5)).toBe("Nothing needs you right now. 5 agents are working.");
  });
});
