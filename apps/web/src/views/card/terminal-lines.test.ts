import { beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { terminalLines } from "./terminal-lines";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const texts = (id: number, keys: string[] = []): string[] =>
  terminalLines(cardOf(id), M.S.chat[id] ?? [], keys).map((line) => line.text);

describe("terminalLines", () => {
  beforeEach(() => resetStore());

  it("starts with the agent, its version, a session id, and the worktree", () => {
    const [first, second, third] = texts(41);
    expect(first).toBe("Claude Code 2.0.14   session 4f447a2f");
    expect(second).toBe("Resumed in ~/.marshal/worktrees/api-gateway/41-fix-token-refresh");
    expect(third).toBe(" ");
  });

  it("uses the project path when the card has no branch", () => {
    expect(texts(45)[1]).toBe(`Resumed in ${M.proj("api")?.path}`);
  });

  it("writes the chat as terminal lines", () => {
    const lines = texts(41);
    expect(lines.some((line) => line.startsWith("> Users get logged out"))).toBe(true);
    expect(lines).toContain("  Read internal/auth/middleware.go");
    expect(lines).toContain("    212 lines");
  });

  it("writes a waiting approval with a y or n prompt, and a decided one with its state", () => {
    expect(texts(44)).toContain("? Allow: go get google.golang.org/grpc@v1.66.0  [y/n]");
    M.approve(44);
    expect(texts(44)).toContain("? Allow: go get google.golang.org/grpc@v1.66.0  approved");
  });

  it("writes the plan, system notes, and failed tool results", () => {
    expect(texts(43)).toContain("? Plan with 5 steps  waiting");
    const stuck = terminalLines(cardOf(119), M.S.chat[119] ?? [], []);
    expect(stuck.find((line) => line.text.startsWith("# The stuck detector"))?.class).toBe("text-muted");
    expect(stuck.find((line) => line.text === "    Failed")?.class).toBe("text-status-danger-text");
  });

  it("ends with the keys pressed on the key bar", () => {
    expect(texts(45, ["Escape key", "Ctrl+Tab key"]).slice(-2)).toEqual([
      "[Escape key]",
      "[Ctrl+Tab key]",
    ]);
  });
});
