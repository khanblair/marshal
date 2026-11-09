import { describe, expect, it } from "vitest";
import { agentActivity } from "./agent-activity";

describe("agent activity follows the real session", () => {
  it("shows immediate send feedback, then thinking and streaming", () => {
    expect(agentActivity("awake", 1, [])).toBe("Sending message");
    expect(agentActivity("working", 0, [])).toBe("Thinking");
    expect(
      agentActivity("working", 0, [{ id: "answer", k: "agent", text: "Hello", streaming: true }]),
    ).toBe("Responding");
  });
  it("does not pretend an idle or blocked agent is thinking", () => {
    for (const state of ["awake", "waiting-approval", "asleep", "stopped"] as const) {
      expect(agentActivity(state, 0, [])).toBeUndefined();
    }
    expect(agentActivity("starting", 0, [])).toBe("Starting agent");
    expect(agentActivity("waking", 0, [])).toBe("Resuming agent");
  });
});
