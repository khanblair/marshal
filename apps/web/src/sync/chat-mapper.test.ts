import type { ActivityItem, ChatMessage } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { toStoredActivity, toStoredActivityList, toStoredMessages } from "./chat-mapper";

/** A message as the daemon sends one: newest first in a page, and one kind at a time. */
const message = (fields: Partial<ChatMessage> & Pick<ChatMessage, "id" | "kind">): ChatMessage => ({
  seq: 1,
  at: "2026-09-26T12:00:00.000Z",
  text: "",
  tool: null,
  diff: null,
  plan: null,
  approval: null,
  card: null,
  ...fields,
});

const NOW = Date.parse("2026-09-26T12:00:10.000Z");

describe("a page of a card's chat", () => {
  it("is kept oldest first, which is the order the screen draws it in", () => {
    const page = [
      message({ id: "c", kind: "agent", text: "third", seq: 3 }),
      message({ id: "b", kind: "agent", text: "second", seq: 2 }),
      message({ id: "a", kind: "user", text: "first", seq: 1 }),
    ];
    expect(toStoredMessages(page).map((one) => one.id)).toEqual(["a", "b", "c"]);
  });

  it("draws a person's words and the agent's own as the two message kinds", () => {
    expect(toStoredMessages([message({ id: "a", kind: "user", text: "Ship it" })])[0]).toEqual({
      id: "a",
      k: "user",
      text: "Ship it",
    });
    expect(toStoredMessages([message({ id: "b", kind: "agent", text: "Working" })])[0]).toEqual({
      id: "b",
      k: "agent",
      text: "Working",
    });
  });

  it("draws a tool call from its one line, its state, and the icon of its kind", () => {
    const tool = {
      id: "call-1",
      title: "Read README.md",
      toolKind: "read",
      state: "running" as const,
      hasDetail: true,
    };
    expect(toStoredMessages([message({ id: "t", kind: "tool", tool })])[0]).toEqual({
      id: "t",
      k: "tool",
      call: "call-1",
      icon: "book-open",
      action: "Read README.md",
      result: "",
      st: "running",
      detail: "",
      open: false,
    });
  });

  it("says a failed tool call failed, in the store's own word", () => {
    const stored = toStoredMessages([
      message({
        id: "t",
        kind: "tool",
        tool: {
          id: "c",
          title: "Run tests",
          toolKind: "execute",
          state: "failed",
          hasDetail: false,
        },
      }),
    ]);
    expect(stored[0]).toMatchObject({ st: "fail", icon: "square-terminal" });
  });

  it("gives a tool kind the screens do not know the generic icon rather than none", () => {
    const stored = toStoredMessages([
      message({
        id: "t",
        kind: "tool",
        tool: { id: "c", title: "Fetch", toolKind: "fetch", state: "ok", hasDetail: false },
      }),
    ]);
    expect(stored[0]).toMatchObject({ icon: "terminal" });
  });

  it("draws a system note and a diff summary as they are", () => {
    const page = [
      message({ id: "d", kind: "diff", diff: { files: 3, additions: 12, deletions: 4 } }),
      message({ id: "s", kind: "system", text: "Session started" }),
    ];
    expect(toStoredMessages(page)).toEqual([
      { id: "s", k: "system", text: "Session started" },
      { id: "d", k: "diff", files: 3, add: 12, del: 4 },
    ]);
  });

  it("falls back to a system note when a kind arrives without its own part", () => {
    const stored = toStoredMessages([message({ id: "t", kind: "tool", text: "a tool call" })]);
    expect(stored[0]).toEqual({ id: "t", k: "system", text: "a tool call" });
  });
});

describe("a page of a card's activity", () => {
  const item = (fields: Partial<ActivityItem>): ActivityItem => ({
    id: "a1",
    kind: "command",
    seq: 1,
    at: "2026-09-26T12:00:00.000Z",
    text: "pnpm test",
    result: "Passed",
    state: "ok",
    ...fields,
  });

  it("is kept newest first, as the daemon sends it and the screen draws it", () => {
    const stored = toStoredActivity(item({}), NOW);
    expect(stored).toMatchObject({
      kind: "command",
      text: "pnpm test",
      result: "Passed",
      st: "ok",
    });
  });

  it("says failed in the store's own word, and keeps the moment it happened", () => {
    const stored = toStoredActivity(item({ state: "failed" }), NOW);
    expect(stored.st).toBe("fail");
    expect(stored.ts).toBe(Date.parse("2026-09-26T12:00:00.000Z"));
  });

  it("maps a whole page in the order the daemon sent it, newest first", () => {
    const stored = toStoredActivityList(
      [item({ id: "new", seq: 2 }), item({ id: "old", seq: 1, state: "running" })],
      NOW,
    );
    expect(stored.map((one) => one.id)).toEqual(["new", "old"]);
  });

  it("marks a row fresh only while it is recent, so the pulse means something", () => {
    expect(
      toStoredActivity(item({}), Date.parse("2026-09-26T12:00:10.000Z")).fresh,
    ).toBeUndefined();
    expect(toStoredActivity(item({}), Date.parse("2026-09-26T12:00:01.000Z")).fresh).toBe(true);
  });
});
