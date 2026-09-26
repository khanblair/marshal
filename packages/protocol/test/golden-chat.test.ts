import { describe, expect, it } from "vitest";
import type {
  ActivityItem,
  ChatMessage,
  ChatMessageDetail,
  ChatToolDetail,
  EventBatch,
  Page,
  SessionOutputEventData,
  SessionStateChangedEventData,
} from "../src";
import { golden } from "./golden";

// A card's chat and its activity (docs/backend-checklist.md B2.6 and B2.8, N13 and N14). The chat
// sample carries one message of every kind, so the kind a client switches on and the part it reads
// for that kind are both checked against the daemon's own file.
describe("the card chat and activity golden files", () => {
  it("has a page of chat messages, one of every kind", () => {
    const tool: ChatMessage = {
      id: "01M3C107JB041061050R3GG28F",
      kind: "tool",
      seq: 3,
      at: "2026-09-30T12:00:00.000Z",
      text: "",
      tool: {
        id: "call_1",
        title: "Edited internal/upstream/conn.go",
        toolKind: "edit",
        state: "ok",
        hasDetail: true,
      },
      diff: null,
      plan: null,
      approval: null,
      card: null,
    };
    const sample: Page<ChatMessage> = {
      items: [
        {
          id: "01M3C107JB041061050R3GG28A",
          kind: "card",
          seq: 8,
          at: "2026-09-30T12:00:00.000Z",
          text: "Made a card for the failing check.",
          tool: null,
          diff: null,
          plan: null,
          approval: null,
          card: { cards: [{ projectId: "api", number: 43 }] },
        },
        {
          id: "01M3C107JB041061050R3GG28B",
          kind: "approval",
          seq: 7,
          at: "2026-09-30T12:00:00.000Z",
          text: "Asked to run rm -rf build",
          tool: null,
          diff: null,
          plan: null,
          approval: { state: "waiting", command: "rm -rf build", reason: "Clean the build folder" },
          card: null,
        },
        {
          id: "01M3C107JB041061050R3GG28C",
          kind: "plan",
          seq: 6,
          at: "2026-09-30T12:00:00.000Z",
          text: "Plan with 2 steps",
          tool: null,
          diff: null,
          plan: {
            state: "waiting",
            steps: ["Read the callers", "Change the signature"],
            files: ["internal/upstream/conn.go"],
            risks: [],
            checks: ["go test ./..."],
          },
          approval: null,
          card: null,
        },
        {
          id: "01M3C107JB041061050R3GG28D",
          kind: "diff",
          seq: 5,
          at: "2026-09-30T12:00:00.000Z",
          text: "3 files changed",
          tool: null,
          diff: { files: 3, additions: 42, deletions: 7 },
          plan: null,
          approval: null,
          card: null,
        },
        {
          id: "01M3C107JB041061050R3GG28E",
          kind: "system",
          seq: 4,
          at: "2026-09-30T12:00:00.000Z",
          text: "Session resumed after Marshal restarted.",
          tool: null,
          diff: null,
          plan: null,
          approval: null,
          card: null,
        },
        tool,
        {
          id: "01M3C107JB041061050R3GG28G",
          kind: "agent",
          seq: 2,
          at: "2026-09-30T12:00:00.000Z",
          text: "I read the callers before changing the signature.",
          tool: null,
          diff: null,
          plan: null,
          approval: null,
          card: null,
        },
        {
          id: "01M3C107JB041061050R3GG28H",
          kind: "user",
          seq: 1,
          at: "2026-09-30T12:00:00.000Z",
          text: "Upgrade the upstream library.",
          tool: null,
          diff: null,
          plan: null,
          approval: null,
          card: null,
        },
      ],
      nextCursor: "eyJzZXEiOjF9",
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("chat-messages")).toEqual(sample);

    // A tool message keeps its detail out of the page: the block shows the line, and the diffs
    // arrive from the message's own route (see below).
    expect(sample.items[5]?.tool?.hasDetail).toBe(true);
    expect(sample.items[5]?.diff).toBeNull();
  });

  it("has a page of activity items, one of every kind", () => {
    const sample: Page<ActivityItem> = {
      items: [
        {
          id: "01M3C107JB041061050R3GG28A",
          kind: "file",
          seq: 5,
          at: "2026-09-30T12:00:00.000Z",
          text: "Edited internal/upstream/conn.go",
          result: "",
          state: "ok",
        },
        {
          id: "01M3C107JB041061050R3GG28B",
          kind: "command",
          seq: 4,
          at: "2026-09-30T12:00:00.000Z",
          text: "Bash: go build ./...",
          result: "",
          state: "running",
        },
        {
          id: "01M3C107JB041061050R3GG28C",
          kind: "test",
          seq: 3,
          at: "2026-09-30T12:00:00.000Z",
          text: "Bash: go test ./...",
          result: "ok github.com/khanblair/marshal/daemon",
          state: "ok",
        },
        {
          id: "01M3C107JB041061050R3GG28D",
          kind: "tool",
          seq: 2,
          at: "2026-09-30T12:00:00.000Z",
          text: "Searched the repository for conn.go",
          result: "",
          state: "failed",
        },
        {
          id: "01M3C107JB041061050R3GG28E",
          kind: "approval",
          seq: 1,
          at: "2026-09-30T12:00:00.000Z",
          text: "Asked to run go mod tidy",
          result: "",
          state: "waiting",
        },
      ],
      nextCursor: "",
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("activity-items")).toEqual(sample);
    // The end of a list is an empty cursor, never an absent one.
    expect(sample.nextCursor).toBe("");
  });

  it("has one message's tool detail, which is what a chat block opens", () => {
    const tool: ChatToolDetail = {
      id: "call_1",
      title: "Edited internal/upstream/conn.go",
      toolKind: "edit",
      path: "internal/upstream/conn.go",
      command: "",
      content: "Replaced the deprecated Dial with DialContext.",
      diffs: [
        {
          path: "internal/upstream/conn.go",
          oldText: 'conn, err := net.Dial("tcp", addr)\n',
          newText: 'conn, err := dialer.DialContext(ctx, "tcp", addr)\n',
        },
      ],
      truncated: true,
    };
    const sample: ChatMessageDetail = {
      message: {
        id: "01M3C107JB041061050R3GG28F",
        kind: "tool",
        seq: 3,
        at: "2026-09-30T12:00:00.000Z",
        text: "",
        tool: {
          id: "call_1",
          title: "Edited internal/upstream/conn.go",
          toolKind: "edit",
          state: "ok",
          hasDetail: true,
        },
        diff: null,
        plan: null,
        approval: null,
        card: null,
      },
      tool,
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("chat-message-detail")).toEqual(sample);
  });
});

// A project chat's session (docs/backend-checklist.md B2.10) sends the events a card's does, on the
// topic chat:<id>. Each carries `chatId`; `cardId` stays in the payload as an empty string, so its
// type is the same for both kinds of session.
describe("the chat session events golden file", () => {
  it("sends the session events on the chat topic, each with the chat's id", () => {
    const batch = golden("chat-session-events") as EventBatch;
    const chatId = "01M3C107JB041061050R3GG281";
    expect(batch.events.map((ev) => [ev.type, ev.topic])).toEqual([
      ["session.state_changed", `chat:${chatId}`],
      ["session.output", `chat:${chatId}`],
      ["session.tool_call", `chat:${chatId}`],
      ["session.state_changed", `chat:${chatId}`],
    ]);
    const state = batch.events[0]?.data as SessionStateChangedEventData;
    expect(state).toEqual({
      cardId: "",
      chatId,
      sessionId: "01M3C107JF041061050R3GG28B",
      state: "working",
    });
    const output = batch.events[1]?.data as SessionOutputEventData;
    expect(output.chatId).toBe(chatId);
    expect(output.cardId).toBe("");
    expect(output.text).toBe("Nothing is blocked right now.");
  });
});
