import { describe, expect, it } from "vitest";
import type {
  Card,
  CardKey,
  CreateLabelRequest,
  ErrorFrame,
  ErrorResponse,
  EventBatch,
  Health,
  Hello,
  HomeSnapshot,
  LabelSnapshot,
  MoveCardRequest,
  Page,
  Resync,
  ServerFrame,
  UpdateCardRequest,
  UpdateLabelRequest,
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
    // `type` narrows a ServerFrame to one of the five, and the compiler checks that each branch
    // reads a field that frame really has.
    const describeFrame = (frame: ServerFrame): string => {
      switch (frame.type) {
        case "events":
          return `${frame.events.length} events`;
        case "resync":
          return frame.reason;
        case "error":
          return frame.error.code;
        case "terminal.screen":
          return `${frame.cols}x${frame.rows}`;
        case "terminal.refused":
          return frame.error.code;
      }
    };
    expect(describeFrame(golden("resync") as ServerFrame)).toBe("epoch-changed");
    expect(describeFrame(golden("error-frame") as ServerFrame)).toBe("invalid_argument");
    expect(describeFrame(golden("event-batch") as ServerFrame)).toBe("2 events");
    expect(describeFrame(golden("terminal-screen") as ServerFrame)).toBe("120x32");
    expect(describeFrame(golden("terminal-refused") as ServerFrame)).toBe("refused");
  });

  it("has a card whose thinking setting is always present, and null when the card has none", () => {
    // `thinking` is required and nullable, not optional: the Go field is a pointer without
    // `omitempty`, so it is always in the JSON, as null when there is no setting
    // (`protocol/card.go`). Both shapes below must compile; the golden card has a setting.
    const withSetting: Card = {
      id: "01M3C107JB041061050R3GG28A",
      projectId: "web-dashboard",
      number: 12,
      key: "web-dashboard#12",
      title: "Add a health check endpoint",
      body: "Serve GET /health with the version.",
      state: "backlog",
      agent: "claude",
      model: "claude-sonnet-4-5",
      thinking: "high",
      permissionMode: "auto-edits",
      role: "",
      // A list on the wire is never null, so a card with no labels carries [].
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
      // The session state is required and nullable in the same way: null until the card has a
      // session, and the stored state after that (`protocol/card.go`).
      session: null,
      // Which view the card opens in (chat or terminal, section S9): chat until the person
      // switches it.
      viewMode: "chat",
      branch: "",
      createdAt: "2026-09-25T10:15:30.123Z",
      updatedAt: "2026-09-25T10:16:30.123Z",
    };
    expect(golden("card")).toEqual(withSetting);
    const withoutSetting: Card = { ...withSetting, thinking: null };
    expect(withoutSetting.thinking).toBeNull();
    const asleep: Card = { ...withSetting, session: "asleep" };
    expect(asleep.session).toBe("asleep");
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

// The card edits, moves, labels, and the Home answer. Each sample is checked against the generated
// type by the compiler, and against the daemon's golden file by the assertion, so the two sides
// cannot drift apart.
describe("the Phase 2 golden files", () => {
  it("has an update-card request that sets only what changed", () => {
    const sample: UpdateCardRequest = {
      title: "Add a health check endpoint",
      agent: "claude",
      permissionMode: "auto-edits",
      role: "Implementer",
      package: "packages/api",
      labels: ["01M3C107JB04106105A"],
      plannedStart: { at: "2026-09-30T12:00:00.000Z" },
      due: { clear: true },
    };
    expect(golden("update-card-request")).toEqual(sample);
  });

  it("has a request that clears a date without inventing a moment for it", () => {
    const sample: UpdateCardRequest = { due: { clear: true } };
    expect(golden("update-card-request-clear-date")).toEqual(sample);
  });

  it("has a move request that carries the target column and nothing else", () => {
    const sample: MoveCardRequest = { state: "working" };
    expect(golden("move-card-request")).toEqual(sample);
  });

  it("has a project's labels", () => {
    const sample: LabelSnapshot = {
      projectId: "web-dashboard",
      labels: [
        {
          id: "01M3C107JB041061050R3GG28B",
          projectId: "web-dashboard",
          name: "backend",
          color: "blue",
          createdAt: "2026-09-30T12:00:00.000Z",
        },
        {
          id: "01M3C107JB041061050R3GG28C",
          projectId: "web-dashboard",
          name: "urgent",
          color: "red",
          createdAt: "2026-09-30T12:00:00.000Z",
        },
      ],
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("label-snapshot")).toEqual(sample);
  });

  it("has the label requests", () => {
    const create: CreateLabelRequest = { name: "urgent", color: "red" };
    expect(golden("create-label-request")).toEqual(create);
    const update: UpdateLabelRequest = { name: "urgent" };
    expect(golden("update-label-request")).toEqual(update);
  });

  it("has the Home answer with a waiting card and an awake one", () => {
    const sample: HomeSnapshot = {
      needs: [
        {
          cardId: "01M3C107JB041061050R3GG28A",
          key: "web-dashboard#12",
          number: 12,
          projectId: "web-dashboard",
          projectName: "web-dashboard",
          title: "Add a health check endpoint",
          reason: { kind: "plan-ready", text: "The plan is ready for review." },
          waitingSince: "2026-09-30T12:00:00.000Z",
          role: "Implementer",
        },
      ],
      awake: [
        {
          cardId: "01M3C107JB041061050R3GG28A",
          key: "web-dashboard#12",
          number: 12,
          projectId: "web-dashboard",
          projectName: "web-dashboard",
          title: "Add a health check endpoint",
          state: "working",
          session: "working",
          doingNow: "Writing the handler",
          pinned: true,
          paused: false,
          contextUsed: 42,
          awakeSince: "2026-09-30T12:00:00.000Z",
        },
      ],
      tiles: { needs: 1, working: 1, mergedToday: 3 },
      // A hand-built answer without a range carries no stored numbers, which is what null means.
      stats: null,
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("home-snapshot")).toEqual(sample);
  });

  it("has an empty Home answer with empty lists, never null", () => {
    const empty: HomeSnapshot = {
      needs: [],
      awake: [],
      tiles: { needs: 0, working: 0, mergedToday: 0 },
      stats: null,
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("home-snapshot-empty")).toEqual(empty);
  });
});
