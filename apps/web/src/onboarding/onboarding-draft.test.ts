import { describe, expect, it } from "vitest";
import { initialDraft, initialsOf, repoNameOf } from "./onboarding-draft";

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
