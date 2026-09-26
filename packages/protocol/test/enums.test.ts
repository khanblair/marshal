import { readdirSync, readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import * as protocol from "../src";
import { GOLDEN_DIR, golden } from "./golden";

/** Every readonly array the generator added, by the name of its type. */
const ARRAYS: Record<string, readonly string[]> = {
  ActivityKind: protocol.ActivityKindValues,
  ActivityState: protocol.ActivityStateValues,
  AgentKind: protocol.AgentKindValues,
  AgentStatus: protocol.AgentStatusValues,
  CIState: protocol.CIStateValues,
  CardState: protocol.CardStateValues,
  CardViewMode: protocol.CardViewModeValues,
  ChatApprovalState: protocol.ChatApprovalStateValues,
  ChatMessageKind: protocol.ChatMessageKindValues,
  ChatPlanState: protocol.ChatPlanStateValues,
  ChatRefusalReason: protocol.ChatRefusalReasonValues,
  ChatTargetKind: protocol.ChatTargetKindValues,
  DeviceKind: protocol.DeviceKindValues,
  DiffFileStatus: protocol.DiffFileStatusValues,
  DiffLineKind: protocol.DiffLineKindValues,
  ErrorCode: protocol.ErrorCodeValues,
  EventType: protocol.EventTypeValues,
  FilterKey: protocol.FilterKeyValues,
  FeedKind: protocol.FeedKindValues,
  FrameType: protocol.FrameTypeValues,
  HoldRefusalReason: protocol.HoldRefusalReasonValues,
  LabelColor: protocol.LabelColorValues,
  MoveRefusalReason: protocol.MoveRefusalReasonValues,
  NeedsReasonKind: protocol.NeedsReasonKindValues,
  NoticeKind: protocol.NoticeKindValues,
  PermissionMode: protocol.PermissionModeValues,
  ProgressStatus: protocol.ProgressStatusValues,
  ProjectSource: protocol.ProjectSourceValues,
  ProjectView: protocol.ProjectViewValues,
  ResyncReason: protocol.ResyncReasonValues,
  SessionState: protocol.SessionStateValues,
  SortDirection: protocol.SortDirectionValues,
  Swimlane: protocol.SwimlaneValues,
  TerminalKey: protocol.TerminalKeyValues,
  TerminalRefusalReason: protocol.TerminalRefusalReasonValues,
  Theme: protocol.ThemeValues,
  ThinkingMode: protocol.ThinkingModeValues,
  TopicKind: protocol.TopicKindValues,
  ViewRefusalReason: protocol.ViewRefusalReasonValues,
};

describe("the fixed lists", () => {
  it("are the same in the generated arrays as in the Go lists", () => {
    // Adding a list in Go changes this golden file, which fails here until it is added above.
    expect(ARRAYS).toEqual(golden("enums"));
  });
});

describe("the wire rules", () => {
  it("keep the subprotocol names and the replay size the docs give", () => {
    expect(protocol.WebSocketSubprotocol).toBe("marshal.v1");
    expect(protocol.BearerSubprotocolPrefix).toBe("bearer.");
    expect(protocol.ReplayBufferSize).toBe(2000);
    expect(protocol.HomeTopic).toBe("home");
  });

  it("write every timestamp in every golden file as UTC with milliseconds", () => {
    const looksLikeATime = /^\d{4}-\d{2}-\d{2}T/;
    const wireForm = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/;
    const times: string[] = [];
    const walk = (value: unknown): void => {
      if (typeof value === "string" && looksLikeATime.test(value)) times.push(value);
      else if (Array.isArray(value)) for (const item of value) walk(item);
      else if (value && typeof value === "object")
        for (const item of Object.values(value)) walk(item);
    };
    for (const file of readdirSync(GOLDEN_DIR).filter((name) => name.endsWith(".json"))) {
      walk(JSON.parse(readFileSync(`${GOLDEN_DIR}${file}`, "utf8")));
    }
    expect(times.length).toBeGreaterThan(0);
    for (const time of times) expect(time).toMatch(wireForm);
  });
});
