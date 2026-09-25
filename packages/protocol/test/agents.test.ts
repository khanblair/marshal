import { describe, expect, it } from "vitest";
import type { Agent, AgentCatalog } from "../src";
import { golden } from "./golden";

// The sample is checked against the generated types by the compiler. If a field of the agent
// catalog changes in Go, its golden file changes, and this stops compiling until the sample
// matches again.
describe("the agent catalog golden file from the daemon", () => {
  it("has one agent of each status, with the daemon's time", () => {
    const claude: Agent = {
      kind: "claude",
      name: "Claude Code",
      version: "2.1.282",
      status: "supported",
      warning: "",
      installHint: "",
      models: [
        { id: "sonnet", name: "Sonnet (latest)", thinking: true },
        { id: "haiku", name: "Haiku (latest)", thinking: false },
      ],
      capabilities: {
        resume: true,
        structuredEvents: true,
        modelSwitching: true,
        thinking: true,
        mcp: true,
        approvals: false,
      },
    };
    const gemini: Agent = {
      kind: "gemini",
      name: "Gemini CLI",
      version: "0.36.0",
      status: "untested",
      warning:
        "Marshal has not been tested with Gemini CLI 0.36.0. It usually works, but if something looks wrong, try version 0.35.1.",
      installHint: "",
      models: [{ id: "gemini-2.5-pro", name: "Gemini 2.5 Pro", thinking: true }],
      capabilities: {
        resume: true,
        structuredEvents: true,
        modelSwitching: true,
        thinking: false,
        mcp: true,
        approvals: true,
      },
    };
    const codex: Agent = {
      kind: "codex",
      name: "Codex",
      version: "",
      status: "missing",
      warning: "",
      installHint: "Codex is not installed. Install it with: npm install -g @openai/codex",
      models: [{ id: "gpt-5-codex", name: "GPT-5 Codex", thinking: true }],
      capabilities: {
        resume: false,
        structuredEvents: false,
        modelSwitching: false,
        thinking: false,
        mcp: false,
        approvals: false,
      },
    };
    const sample: AgentCatalog = {
      agents: [claude, gemini, codex],
      serverTime: "2026-09-25T10:20:00.000Z",
    };
    expect(golden("agents")).toEqual(sample);
  });

  it("never sends the place of a program on disk", () => {
    const catalog = golden("agents") as AgentCatalog;
    for (const agent of catalog.agents) {
      expect(Object.keys(agent)).not.toContain("path");
    }
  });
});
