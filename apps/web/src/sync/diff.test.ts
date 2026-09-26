import { describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import { cardId, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { loadCardDiff, loadFileHunks } from "./diff";

// Section S11: a card's diff (docs/backend-checklist.md B2.9). It is switched, and these tests still
// say so in their own table (with the cards on the daemon too, which a real card needs), so they
// keep testing the daemon half on the day the register changes and the mock half whenever they pin it.
const CARD_AND_DIFF_ON_DAEMON = {
  ...sectionStatus,
  S5a: "daemon" as const,
  S11: "daemon" as const,
};

const api41 = () =>
  wireCard({ projectId: "api", number: 41, title: "Fix token refresh on login", state: "working" });

describe("loadCardDiff and loadFileHunks", () => {
  it("reads the daemon's own changed files and, per file, its hunks, once S11 is switched", async () => {
    const changed = {
      path: "internal/auth/refresh.go",
      oldPath: "",
      status: "modified" as const,
      additions: 12,
      deletions: 3,
      binary: false,
      large: false,
    };
    const hunk = {
      header: "@@ -1,1 +1,1 @@",
      lines: [{ kind: "added" as const, oldLine: 0, newLine: 1, text: "package auth" }],
    };
    const d = createFakeDaemon({
      projects: PROTOTYPE_PROJECTS.slice(0, 1),
      cards: [api41()],
      diffs: { [cardId(41)]: { files: [changed], hunks: { [changed.path]: [hunk] } } },
    });
    const M = createTestMarshal({ data: d.data, sections: CARD_AND_DIFF_ON_DAEMON });
    await d.connect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    const ctx = contextOf(M);
    const card = M.card("api#41");
    if (!card) throw new Error("card api#41 was not synced");

    const files = await loadCardDiff(ctx, card);
    expect(files).toEqual([
      { path: "internal/auth/refresh.go", add: 12, del: 3, large: false, hunks: [] },
    ]);

    const hunks = await loadFileHunks(ctx, card, "internal/auth/refresh.go");
    expect(hunks).toEqual([{ h: "@@ -1,1 +1,1 @@", lines: [["+", 1, "package auth"]] }]);
  });

  it("reads a removed line's old line number, not the new one", async () => {
    const changed = {
      path: "a.go",
      oldPath: "",
      status: "modified" as const,
      additions: 0,
      deletions: 1,
      binary: false,
      large: false,
    };
    const hunk = {
      header: "@@ -5,1 +5,0 @@",
      lines: [{ kind: "removed" as const, oldLine: 5, newLine: 0, text: "old line" }],
    };
    const d = createFakeDaemon({
      projects: PROTOTYPE_PROJECTS.slice(0, 1),
      cards: [api41()],
      diffs: { [cardId(41)]: { files: [changed], hunks: { "a.go": [hunk] } } },
    });
    const M = createTestMarshal({ data: d.data, sections: CARD_AND_DIFF_ON_DAEMON });
    await d.connect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    const ctx = contextOf(M);
    const card = M.card("api#41");
    if (!card) throw new Error("card api#41 was not synced");

    const hunks = await loadFileHunks(ctx, card, "a.go");
    expect(hunks).toEqual([{ h: "@@ -5,1 +5,0 @@", lines: [["-", 5, "old line"]] }]);
  });

  it("falls back to the mock's own diffFor when there is no daemon to ask", async () => {
    const M = createTestMarshal({ sections: CARD_AND_DIFF_ON_DAEMON });
    const ctx = contextOf(M);
    const card = M.card("api#41");
    if (!card) throw new Error("card api#41 was not in the mock's own seed");
    const files = await loadCardDiff(ctx, card);
    expect(files.length).toBeGreaterThan(0);
    expect(files[0]?.hunks.length).toBeGreaterThan(0);
  });

  it("answers the mock's large file with a sample of its lines, since it has none of its own", async () => {
    const M = createTestMarshal({ sections: CARD_AND_DIFF_ON_DAEMON });
    const ctx = contextOf(M);
    const card = M.card("api#41");
    if (!card) throw new Error("card api#41 was not in the mock's own seed");
    const large = (await loadCardDiff(ctx, card)).find((file) => file.large);
    if (!large) throw new Error("the mock's diff for api#41 has no large file");
    expect(large.hunks).toEqual([]);
    const hunks = await loadFileHunks(ctx, card, large.path);
    expect(hunks[0]?.h).toBe("@@ -0,0 +1,1240 @@");
    expect(hunks[0]?.lines).toHaveLength(3);
  });

  it("does not ask the daemon while S11 is pinned to the mock, and gives the mock's hunks up front", async () => {
    const d = createFakeDaemon({
      projects: PROTOTYPE_PROJECTS.slice(0, 1),
      cards: [api41()],
      diffs: { [cardId(41)]: { files: [] } },
    });
    const M = createTestMarshal({
      data: d.data,
      sections: { ...CARD_AND_DIFF_ON_DAEMON, S11: "mock" },
    });
    await d.connect();
    await vi.waitFor(() => expect(M.S.ready).toBe(true));
    const ctx = contextOf(M);
    const card = M.card("api#41");
    if (!card) throw new Error("card api#41 was not synced");

    const files = await loadCardDiff(ctx, card);
    expect(files.length).toBeGreaterThan(0);
    expect(files[0]?.hunks.length).toBeGreaterThan(0);
    expect(d.routes().filter((route) => route.includes("/diff"))).toEqual([]);
  });
});
