import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { GOLDEN_CATALOG } from "~/testing/agents";
import { useCatalog } from "~/testing/test-store";
import { terminalLines } from "./terminal-lines";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const texts = (id: CardKey, keys: string[] = []): string[] =>
  terminalLines(cardOf(id), M.S.chat[id] ?? [], keys).map((line) => line.text);

describe("terminalLines", () => {
  beforeEach(() => resetStore());

  it("starts with the agent, its version, a session id, and the worktree", () => {
    const [first, second, third] = texts("api#41");
    expect(first).toBe("Claude Code 2.0.14   session 4f447a2f");
    expect(second).toBe("Resumed in ~/.marshal/worktrees/api-gateway/41-fix-token-refresh");
    expect(third).toBe(" ");
  });

  it("names only the agent when the catalog does not know it, and not the word undefined", () => {
    cardOf("api#41").agent = "Aider";
    expect(texts("api#41")[0]).toBe("Aider   session 4f447a2f");
  });

  it("names only the agent when it is not installed and has no version", () => {
    const restore = useCatalog(M, GOLDEN_CATALOG);
    cardOf("api#41").agent = "Codex";
    expect(texts("api#41")[0]).toBe("Codex   session 4f447a2f");
    restore();
  });

  it("reads the version from the daemon's catalog", () => {
    const restore = useCatalog(M, GOLDEN_CATALOG);
    expect(texts("api#41")[0]).toBe("Claude Code 2.1.282   session 4f447a2f");
    restore();
  });

  it("uses the project path when the card has no branch", () => {
    expect(texts("api#45")[1]).toBe(`Resumed in ${M.proj("api")?.path}`);
  });

  it("writes the chat as terminal lines", () => {
    const lines = texts("api#41");
    expect(lines.some((line) => line.startsWith("> Users get logged out"))).toBe(true);
    expect(lines).toContain("  Read internal/auth/middleware.go");
    expect(lines).toContain("    212 lines");
  });

  it("writes a waiting approval with a y or n prompt, and a decided one with its state", () => {
    expect(texts("api#44")).toContain("? Allow: go get google.golang.org/grpc@v1.66.0  [y/n]");
    M.approve("api#44");
    expect(texts("api#44")).toContain("? Allow: go get google.golang.org/grpc@v1.66.0  approved");
  });

  it("writes the plan, system notes, and failed tool results", () => {
    expect(texts("api#43")).toContain("? Plan with 5 steps  waiting");
    const stuck = terminalLines(cardOf("web#119"), M.S.chat["web#119"] ?? [], []);
    expect(stuck.find((line) => line.text.startsWith("# The stuck detector"))?.class).toBe(
      "text-muted",
    );
    expect(stuck.find((line) => line.text === "    Failed")?.class).toBe("text-status-danger-text");
  });

  it("ends with the keys pressed on the key bar", () => {
    expect(texts("api#45", ["Escape key", "Ctrl+Tab key"]).slice(-2)).toEqual([
      "[Escape key]",
      "[Ctrl+Tab key]",
    ]);
  });
});
