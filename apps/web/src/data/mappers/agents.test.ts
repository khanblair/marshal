import type { AgentCatalog } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { withBuiltIn } from "~/sync/agents";
import { PROTOTYPE_CATALOG } from "~/testing/agents";
import { golden } from "../testing/golden";
import { thinkSupportedIn, toAgentOptions, toLegacyAgents } from "./agents";

const catalog = golden<AgentCatalog>("agents");

describe("toAgentOptions", () => {
  const options = toAgentOptions(catalog);

  it("lists every agent in the daemon's order, missing ones too", () => {
    expect(options.map((o) => [o.kind, o.name, o.version, o.status, o.missing])).toEqual([
      ["claude", "Claude Code", "2.1.282", "supported", false],
      ["gemini", "Gemini CLI", "0.36.0", "untested", false],
      ["codex", "Codex", "", "missing", true],
    ]);
  });

  it("keeps the warning, the install hint, the models, and the capabilities", () => {
    const [claude, gemini, codex] = options;
    expect(claude?.models).toEqual([
      { id: "sonnet", name: "Sonnet (latest)", thinking: true },
      { id: "haiku", name: "Haiku (latest)", thinking: false },
    ]);
    expect(claude?.capabilities).toEqual(catalog.agents[0]?.capabilities);
    expect(gemini?.warning).toContain("has not been tested with Gemini CLI 0.36.0");
    expect(gemini?.installHint).toBe("");
    expect(codex?.installHint).toContain("npm install -g @openai/codex");
    expect(codex?.warning).toBe("");
  });

  it("does not share objects with the wire catalog", () => {
    options[0]?.models.push({ id: "extra", name: "Extra", thinking: false });
    expect(catalog.agents[0]?.models).toHaveLength(2);
  });

  it("gives an empty list for an empty catalog", () => {
    expect(toAgentOptions({ agents: [], serverTime: catalog.serverTime })).toEqual([]);
  });
});

describe("toLegacyAgents", () => {
  const legacy = toLegacyAgents(catalog);

  it("is keyed by the name shown to people, with model ids, an icon, and the version", () => {
    expect(legacy).toEqual({
      "Claude Code": { models: ["sonnet", "haiku"], icon: "terminal-square", version: "2.1.282" },
      "Gemini CLI": { models: ["gemini-2.5-pro"], icon: "terminal-square", version: "0.36.0" },
      Codex: { models: ["gpt-5-codex"], icon: "terminal-square", version: "" },
    });
  });

  it("uses the same icons and the same names as the prototype's table", () => {
    const prototype = toLegacyAgents(PROTOTYPE_CATALOG);
    expect(prototype).toEqual({
      "Claude Code": {
        models: ["claude-sonnet-4-5", "claude-opus-4-1", "claude-haiku-4-5"],
        icon: "terminal-square",
        version: "2.0.14",
      },
      Codex: {
        models: ["gpt-5-codex", "gpt-5", "gpt-5-mini"],
        icon: "terminal-square",
        version: "0.42.0",
      },
      "Gemini CLI": {
        models: ["gemini-2.5-pro", "gemini-2.5-flash"],
        icon: "terminal-square",
        version: "0.8.1",
      },
    });
    for (const [name, info] of Object.entries(legacy)) {
      expect(prototype[name]?.icon).toBe(info.icon);
    }
    expect(Object.keys(prototype)).toEqual(expect.arrayContaining(Object.keys(legacy)));
  });

  it("has no built-in agent, because the daemon's catalog does not list it", () => {
    expect(legacy["Built-in agent"]).toBeUndefined();
  });

  it("keeps a missing agent in the map", () => {
    expect(legacy.Codex).toBeDefined();
  });
});

describe("thinkSupportedIn", () => {
  const supports = thinkSupportedIn(catalog);

  it("follows the thinking flag of each model", () => {
    expect(supports("sonnet")).toBe(true);
    expect(supports("haiku")).toBe(false);
  });

  it("needs the agent to let Marshal set thinking too, as the Go types say", () => {
    // Gemini's model thinks, but the agent gives Marshal no way to say how hard.
    expect(catalog.agents[1]?.models[0]?.thinking).toBe(true);
    expect(catalog.agents[1]?.capabilities.thinking).toBe(false);
    expect(supports("gemini-2.5-pro")).toBe(false);
    expect(supports("gpt-5-codex")).toBe(false);
  });

  it("says no for a model the catalog does not list", () => {
    expect(supports("claude-sonnet-4-5")).toBe(false);
    expect(supports("")).toBe(false);
  });

  it("keeps the two built-in models without a thinking setting off, as the prototype's fixed list did", () => {
    const withBuiltInAgent = thinkSupportedIn(withBuiltIn(catalog.agents));
    expect(withBuiltInAgent("deepseek-chat")).toBe(false);
    expect(withBuiltInAgent("qwen2.5-coder:32b")).toBe(false);
    expect(withBuiltInAgent("gpt-5-mini")).toBe(true);
    expect(thinkSupportedIn({ agents: [], serverTime: catalog.serverTime })("sonnet")).toBe(false);
  });
});
