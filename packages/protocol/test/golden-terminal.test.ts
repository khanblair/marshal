import { describe, expect, it } from "vitest";
import type {
  Card,
  CardView,
  TerminalInput,
  TerminalOutputEventData,
  TerminalRefusal,
  TerminalResize,
  TerminalScreen,
  TerminalSnapshotRequest,
  Event as WireEvent,
} from "../src";
import { golden } from "./golden";

// The terminal channel (section S9, docs/architecture.md 4.2, 4.3, 11.2): the chat/terminal view
// switch, the client's `terminal.input`, `terminal.resize`, and `terminal.snapshot` messages, the
// daemon's `terminal.screen` and `terminal.refused` frames, and the live-only `session.terminal_output`
// event. `Event as WireEvent` is renamed on import for the same reason as in golden.test.ts.
describe("the terminal golden files", () => {
  it("has a card open in the terminal view, with its session awake", () => {
    const sample: Card = {
      id: "01M3C107JB041061050R3GG28A",
      projectId: "web-dashboard",
      number: 12,
      key: "web-dashboard#12",
      title: "Add a health check endpoint",
      body: "Serve GET /health with the version.",
      state: "working",
      agent: "claude",
      model: "claude-sonnet-4-5",
      thinking: "high",
      permissionMode: "auto-edits",
      role: "",
      labels: [],
      package: "",
      plannedStart: null,
      plannedEnd: null,
      due: null,
      actualStart: null,
      actualEnd: null,
      pullRequest: null,
      ci: null,
      contextUsed: 0,
      needsReason: null,
      doingNow: "",
      paused: false,
      pinned: false,
      session: "awake",
      viewMode: "terminal",
      branch: "",
      createdAt: "2026-09-25T10:15:30.123Z",
      updatedAt: "2026-09-25T10:16:30.123Z",
    };
    expect(golden("card-terminal")).toEqual(sample);
  });

  it("has the answer to POST /v1/cards/{id}/view", () => {
    const sample: CardView = {
      cardId: "01M3C107JB041061050R3GG28A",
      mode: "terminal",
      session: "awake",
      serverTime: "2026-09-25T11:15:30.123Z",
    };
    expect(golden("card-view")).toEqual(sample);
  });

  it("has what a person typed, as text or as a named key", () => {
    const typed: TerminalInput = {
      type: "terminal.input",
      cardId: "01M3C107JB041061050R3GG28A",
      data: "ls -la\r",
    };
    expect(golden("terminal-input")).toEqual(typed);
    const key: TerminalInput = {
      type: "terminal.input",
      cardId: "01M3C107JB041061050R3GG28A",
      key: "ctrl-c",
    };
    expect(golden("terminal-input-key")).toEqual(key);
    // Exactly one of data and key is ever sent; both are optional so each sample can leave the
    // other out, and the compiler still checks each shape against the real field names.
    expect(typed.key).toBeUndefined();
    expect(key.data).toBeUndefined();
  });

  it("has the view's size, in character cells", () => {
    const sample: TerminalResize = {
      type: "terminal.resize",
      cardId: "01M3C107JB041061050R3GG28A",
      cols: 96,
      rows: 28,
    };
    expect(golden("terminal-resize")).toEqual(sample);
  });

  it("has a request for the recent screen, by card id alone", () => {
    const sample: TerminalSnapshotRequest = {
      type: "terminal.snapshot",
      cardId: "01M3C107JB041061050R3GG28A",
    };
    expect(golden("terminal-snapshot-request")).toEqual(sample);
  });

  it("has the recent screen the daemon answers with, and an empty one for a fresh terminal", () => {
    const sample: TerminalScreen = {
      type: "terminal.screen",
      cardId: "01M3C107JB041061050R3GG28A",
      cols: 120,
      rows: 32,
      throughSeq: 412,
      data: "G1syShtbSHJlYWR5DQo=",
    };
    expect(golden("terminal-screen")).toEqual(sample);
    const empty: TerminalScreen = {
      type: "terminal.screen",
      cardId: "01M3C107JB041061050R3GG28A",
      cols: 120,
      rows: 32,
      throughSeq: 0,
      data: "",
    };
    expect(golden("terminal-screen-empty")).toEqual(empty);
    // ThroughSeq orders the screen against the live output events that follow it: a fresh
    // terminal (nothing printed yet) reads 0, never null and never missing.
    expect(empty.throughSeq).toBe(0);
  });

  it("refuses a message the terminal cannot answer right now, with the daemon's own sentence", () => {
    const sample: TerminalRefusal = {
      type: "terminal.refused",
      cardId: "01M3C107JB041061050R3GG28A",
      error: {
        code: "refused",
        message: "This card has no terminal running. Switch it to terminal view first.",
        details: { cardId: "01M3C107JB041061050R3GG28A", reason: "terminal_not_active" },
      },
    };
    expect(golden("terminal-refused")).toEqual(sample);
  });

  it("has one piece of terminal output, as base64, never replayed", () => {
    const sample: TerminalOutputEventData = {
      cardId: "01M3C107JB041061050R3GG28A",
      data: "ZWNobzogaGkNCg==",
    };
    expect(golden("session-terminal-output")).toEqual(sample);
  });

  it("has a batch of ordinary events, one of them a terminal_output, since it is a normal event on the wire even though it is never replayed", () => {
    const sample = golden("terminal-events") as { events: WireEvent[] };
    expect(sample.events).toHaveLength(2);
    expect(sample.events[0]?.type).toBe("session.state_changed");
    expect(sample.events[1]?.type).toBe("session.terminal_output");
    const output = sample.events[1]?.data as TerminalOutputEventData;
    expect(output.cardId).toBe("01M3C107JB041061050R3GG28A");
    expect(output.data).toBe("cmVhZHkNCg==");
  });
});
