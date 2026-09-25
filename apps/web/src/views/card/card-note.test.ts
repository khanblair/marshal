import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { defaultNote, ensureNote, notePath, noteText, saveNote } from "./card-note";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

describe("card notes", () => {
  beforeEach(() => resetStore());

  it("writes a default note from the title and the area the card works in", () => {
    expect(defaultNote(cardOf("api#41"))).toBe(
      [
        "# Fix token refresh on login",
        "",
        "Goal: fix token refresh on login.",
        "",
        "Decisions",
        "- Keep the change inside internal/proxy",
        "- Add tests before pushing",
        "",
        "Links",
        "[[api-gateway]]  [[lessons/ci-flaky-tests]]",
      ].join("\n"),
    );
  });

  it("names the package when the card has one", () => {
    expect(defaultNote(cardOf("mobile#209"))).toContain("- Keep the change inside apps/android");
  });

  it("shows the default note until one is saved", () => {
    expect(noteText(cardOf("api#41"))).toBe(defaultNote(cardOf("api#41")));
    saveNote(cardOf("api#41"), "Mine");
    expect(noteText(cardOf("api#41"))).toBe("Mine");
    expect(M.S.toasts.at(-1)?.msg).toBe("Note saved");
  });

  it("stores the default note the first time the card is drawn, and keeps a saved one", () => {
    ensureNote(cardOf("api#41"));
    expect(M.S.notes?.["api#41"]).toBe(defaultNote(cardOf("api#41")));
    saveNote(cardOf("api#41"), "Mine");
    ensureNote(cardOf("api#41"));
    expect(M.S.notes?.["api#41"]).toBe("Mine");
  });

  it("places the note in the vault under the card number and a slug of the title", () => {
    expect(notePath(cardOf("api#41"))).toBe("vault/cards/41-fix-token-refresh-on-login.md");
  });
});
