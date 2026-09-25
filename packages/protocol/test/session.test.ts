import { describe, expect, it } from "vitest";
import type {
  AgentToolCall,
  EventBatch,
  SessionOutputEventData,
  SessionStateChangedEventData,
  SessionToolCallEventData,
} from "../src";
import { golden } from "./golden";

// Each sample is checked against the generated type by the compiler. If a field of a session
// event payload changes in Go, its golden file changes, and this stops compiling until the
// sample matches again, so the two sides cannot drift apart unnoticed.
describe("session event payloads from the daemon", () => {
  it("has a message chunk", () => {
    const sample: SessionOutputEventData = {
      cardId: "01M3C107JB041061050R3GG28A",
      kind: "message",
      text: "I read the code and found the health check route.",
    };
    expect(golden("session-output")).toEqual(sample);
  });

  it("has a plan update", () => {
    const sample: SessionOutputEventData = {
      cardId: "01M3C107JB041061050R3GG28A",
      kind: "plan",
      plan: [
        { text: "Read the router", status: "completed" },
        { text: "Add the health check route", status: "in_progress" },
        { text: "Write a test", status: "pending" },
      ],
    };
    expect(golden("session-output-plan")).toEqual(sample);
  });

  it("has a tool call starting", () => {
    const toolCall: AgentToolCall = {
      id: "call-1",
      title: "Read main.go",
      toolKind: "read",
      status: "in_progress",
      path: "/home/ada/code/api/main.go",
    };
    const sample: SessionToolCallEventData = {
      cardId: "01M3C107JB041061050R3GG28A",
      kind: "tool_call",
      toolCall,
    };
    expect(golden("session-tool-call")).toEqual(sample);
  });

  it("has a tool call update, with only the fields that changed", () => {
    const sample: SessionToolCallEventData = {
      cardId: "01M3C107JB041061050R3GG28A",
      kind: "tool_call_update",
      toolCall: {
        id: "call-1",
        status: "completed",
        content: "package main\n",
        diffs: [{ path: "main.go", newText: "package main\n" }],
      },
    };
    expect(golden("session-tool-call-update")).toEqual(sample);
  });

  it("has a session state change", () => {
    const sample: SessionStateChangedEventData = {
      cardId: "01M3C107JB041061050R3GG28A",
      sessionId: "01M3C107JF041061050R3GG28B",
      state: "awake",
    };
    expect(golden("session-state-changed")).toEqual(sample);
  });

  it("has a session state change with a reason", () => {
    const sample: SessionStateChangedEventData = {
      cardId: "01M3C107JB041061050R3GG28A",
      sessionId: "01M3C107JF041061050R3GG28B",
      state: "stopped",
      reason: "Marshal could not pick this session back up. Start a fresh session from the card.",
    };
    expect(golden("session-state-changed-reason")).toEqual(sample);
  });

  it("has a batch of the session events a card's chat listens for", () => {
    const batch = golden("session-events") as EventBatch;
    expect(batch.events.map((e) => e.type)).toEqual([
      "session.state_changed",
      "session.output",
      "session.tool_call",
      "session.tool_call",
      "session.state_changed",
    ]);
    expect(batch.events.every((e) => e.topic === "card:01M3C107JB041061050R3GG28A")).toBe(true);
  });
});
