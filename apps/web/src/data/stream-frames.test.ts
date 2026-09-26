import { describe, expect, it } from "vitest";
import {
  buildTerminalInput,
  buildTerminalResize,
  buildTerminalSnapshot,
  parseFrame,
} from "./stream-frames";
import { golden } from "./testing/golden";

const CARD_ID = "01M3C107JB041061050R3GG28A";

describe("the terminal channel's frame readers (docs/architecture.md 11.2)", () => {
  it("reads a terminal.screen frame, matching the daemon's own golden file", () => {
    const frame = parseFrame(JSON.stringify(golden("terminal-screen")));
    expect(frame).toEqual({
      kind: "terminal.screen",
      frame: {
        type: "terminal.screen",
        cardId: CARD_ID,
        cols: 120,
        rows: 32,
        throughSeq: 412,
        data: "G1syShtbSHJlYWR5DQo=",
      },
    });
  });

  it("reads an empty terminal.screen (a fresh terminal), data and throughSeq checked by type and not truthiness", () => {
    const frame = parseFrame(JSON.stringify(golden("terminal-screen-empty")));
    expect(frame).toEqual({
      kind: "terminal.screen",
      frame: {
        type: "terminal.screen",
        cardId: CARD_ID,
        cols: 120,
        rows: 32,
        throughSeq: 0,
        data: "",
      },
    });
  });

  it("reads null for a terminal.screen missing a field", () => {
    const body = golden<Record<string, unknown>>("terminal-screen");
    for (const missing of ["cardId", "cols", "rows", "throughSeq", "data"]) {
      const { [missing]: _dropped, ...rest } = body;
      expect(parseFrame(JSON.stringify(rest))).toBeNull();
    }
  });

  it("reads a terminal.refused frame, with the daemon's own sentence and reason", () => {
    const frame = parseFrame(JSON.stringify(golden("terminal-refused")));
    expect(frame).toEqual({
      kind: "terminal.refused",
      frame: {
        type: "terminal.refused",
        cardId: CARD_ID,
        error: {
          code: "refused",
          message: "This card has no terminal running. Switch it to terminal view first.",
          details: { cardId: CARD_ID, reason: "terminal_not_active" },
        },
      },
    });
  });

  it("reads null for a terminal.refused with no card id or an unreadable error", () => {
    expect(parseFrame(JSON.stringify({ type: "terminal.refused", error: {} }))).toBeNull();
    expect(parseFrame(JSON.stringify({ type: "terminal.refused", cardId: CARD_ID }))).toBeNull();
  });
});

describe("the terminal channel's client builders", () => {
  it("builds terminal.input with text, matching the daemon's own golden file", () => {
    expect(buildTerminalInput(CARD_ID, { data: "ls -la\r" })).toEqual(golden("terminal-input"));
  });

  it("builds terminal.input with a named key, matching the daemon's own golden file", () => {
    expect(buildTerminalInput(CARD_ID, { key: "ctrl-c" })).toEqual(golden("terminal-input-key"));
  });

  it("builds terminal.resize, matching the daemon's own golden file", () => {
    expect(buildTerminalResize(CARD_ID, 96, 28)).toEqual(golden("terminal-resize"));
  });

  it("builds terminal.snapshot, matching the daemon's own golden file", () => {
    expect(buildTerminalSnapshot(CARD_ID)).toEqual(golden("terminal-snapshot-request"));
  });
});
