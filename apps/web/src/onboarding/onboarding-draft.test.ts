import { describe, expect, it } from "vitest";
import { initialDraft, initialsOf, isMonorepoHint, maskKey, repoNameOf } from "./onboarding-draft";

describe("initialsOf", () => {
  it.each([
    ["", "?"],
    ["   ", "?"],
    ["Ada", "A"],
    ["ada okafor", "AO"],
    ["  Ada   Lovelace  ", "AL"],
    ["Ada Augusta King", "AA"],
  ])("turns %j into %j", (name, expected) => {
    expect(initialsOf(name)).toBe(expected);
  });
});

describe("maskKey", () => {
  it("keeps the first six and last four characters", () => {
    expect(maskKey("sk-ant-api03-abcdefgh1234")).toBe("sk-ant…1234");
  });
});

describe("repoNameOf", () => {
  it.each([
    ["~/code/my-repo", "my-repo"],
    ["~/code/my-repo/", "my-repo"],
    ["https://github.com/owner/repo", "repo"],
    ["https://github.com/owner/repo.git", "repo"],
    ["git@github.com:owner/repo.git", "repo"],
    ["repo", "repo"],
    ["///", ""],
    ["", ""],
  ])("takes the name from %j", (value, expected) => {
    expect(repoNameOf(value)).toBe(expected);
  });
});

describe("isMonorepoHint", () => {
  it("matches mono and apps in any case", () => {
    expect(isMonorepoHint("~/code/acme-monorepo")).toBe(true);
    expect(isMonorepoHint("https://github.com/acme/Apps")).toBe(true);
    expect(isMonorepoHint("~/code/api")).toBe(false);
  });
});

describe("initialDraft", () => {
  it("starts empty, on the sample project and the London time zone", () => {
    expect(initialDraft()).toEqual({
      name: "",
      email: "",
      tz: "Europe/London",
      avatarChosen: false,
      keys: { anthropic: "", openai: "", gemini: "" },
      source: "sample",
      path: "",
      url: "",
      chatApps: { telegram: false, discord: false },
    });
  });

  it("returns a fresh object each time", () => {
    expect(initialDraft()).not.toBe(initialDraft());
  });
});
