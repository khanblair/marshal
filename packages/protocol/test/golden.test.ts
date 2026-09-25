import { describe, expect, it } from "vitest";
import type {
  CardKey,
  ErrorFrame,
  ErrorResponse,
  EventBatch,
  Health,
  Hello,
  Page,
  Resync,
  ServerFrame,
  WhoAmI,
  Error as WireError,
  Event as WireEvent,
} from "../src";
import { golden } from "./golden";

// Each sample below is checked against the generated type by the compiler. If a Go type gains or
// loses a field, its golden file changes, and the test stops compiling until the sample matches
// again, so the two sides cannot drift apart unnoticed. `Error` and `Event` are renamed on
// import because they would otherwise hide the built-in types of the same name.
describe("golden files from the daemon", () => {
  it("has the health answer the generated type describes", () => {
    const sample: Health = {
      status: "ok",
      version: "0.0.0",
      mode: "dev",
      serverTime: "2026-09-25T10:00:00.000Z",
    };
    expect(golden("health")).toEqual(sample);
  });

  it("has the one error shape", () => {
    const wire: WireError = {
      code: "not_found",
      message: "Marshal cannot find that card. It may have been removed.",
      details: { id: "01M3C107JB041061050R3GG28A" },
    };
    const sample: ErrorResponse = { error: wire };
    expect(golden("error")).toEqual(sample);
  });

  it("has a page with its cursor and the daemon's time", () => {
    const sample: Page<CardKey> = {
      items: [
        { projectId: "api", number: 12 },
        { projectId: "web-dashboard", number: 3 },
      ],
      nextCursor: "eyJuIjoxM30",
      serverTime: "2026-09-25T10:15:30.123Z",
    };
    expect(golden("page")).toEqual(sample);
  });

  it("has a batch of events under an epoch", () => {
    const events: WireEvent[] = [
      {
        seq: 41,
        topic: "project:web-dashboard",
        type: "card.created",
        at: "2026-09-25T10:15:30.123Z",
        data: { cardId: "01M3C107JB041061050R3GG28A" },
      },
      {
        seq: 42,
        topic: "card:01M3C107JB041061050R3GG28A",
        type: "session.state_changed",
        at: "2026-09-25T10:15:30.143Z",
        data: { state: "working" },
      },
    ];
    const sample: EventBatch = { type: "events", epoch: "01M3C0ZZZZ000000000000000A", events };
    expect(golden("event-batch")).toEqual(sample);
  });

  it("has the hello a client sends first", () => {
    const sample: Hello = {
      type: "hello",
      subscribe: ["home", "project:web-dashboard", "card:01M3C107JB041061050R3GG28A"],
      sinceSeq: 41,
      epoch: "01M3C0ZZZZ000000000000000A",
    };
    expect(golden("hello")).toEqual(sample);
  });

  it("has the resync frame", () => {
    const sample: Resync = {
      type: "resync",
      epoch: "01M3C0ZZZZ000000000000000B",
      reason: "epoch-changed",
      seq: 0,
    };
    expect(golden("resync")).toEqual(sample);
  });

  it("has the error frame that comes before a close", () => {
    const wire: WireError = {
      code: "invalid_argument",
      message: "That topic is not one Marshal knows. Check the topic and connect again.",
      details: { topic: "project:Bad_ID" },
    };
    const sample: ErrorFrame = { type: "error", error: wire };
    expect(golden("error-frame")).toEqual(sample);
  });

  it("has frames a client can tell apart by their type", () => {
    // `type` narrows a ServerFrame to one of the three, and the compiler checks that each branch
    // reads a field that frame really has.
    const describeFrame = (frame: ServerFrame): string => {
      switch (frame.type) {
        case "events":
          return `${frame.events.length} events`;
        case "resync":
          return frame.reason;
        case "error":
          return frame.error.code;
      }
    };
    expect(describeFrame(golden("resync") as ServerFrame)).toBe("epoch-changed");
    expect(describeFrame(golden("error-frame") as ServerFrame)).toBe("invalid_argument");
    expect(describeFrame(golden("event-batch") as ServerFrame)).toBe("2 events");
  });

  it("has the answer to whoami", () => {
    const sample: WhoAmI = {
      deviceId: "01M3C107JD041061050R3GG28A",
      deviceKind: "cli",
      userId: "01M3C107JE0R3GG28A04106105",
      mode: "normal",
      serverTime: "2026-09-25T10:15:30.123Z",
    };
    expect(golden("whoami")).toEqual(sample);
  });
});
