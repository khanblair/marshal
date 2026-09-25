import type { AgentCatalog } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { applyAgentCatalog } from "~/sync/agents";
import { GOLDEN_CATALOG, wireAgent, wireCatalog } from "~/testing/agents";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { agentOptions, defaultAgent, defaultAgentOf, defaultModel, thinkSupported } from "./agents";
import type { Card } from "./types";

/** A store whose catalog is the golden one: Claude Code supported, Gemini CLI untested, Codex missing. */
function storeWith(catalog?: AgentCatalog) {
  const M = createTestMarshal({ hash: "#nosim" });
  const ctx = contextOf(M);
  if (catalog) applyAgentCatalog(ctx, catalog);
  return { M, ctx };
}

describe("M.AGENTS with the prototype's agents", () => {
  const { M } = storeWith();

  it("has the prototype's table, in its order, with the built-in agent last", () => {
    expect(M.AGENTS).toEqual({
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
      "Built-in agent": {
        models: [
          "claude-sonnet-4-5",
          "gpt-5-mini",
          "deepseek-chat",
          "gemini-2.5-flash",
          "qwen2.5-coder:32b",
        ],
        icon: "cpu",
        version: "Marshal 0.9",
      },
    });
    expect(Object.keys(M.AGENTS)).toEqual(["Claude Code", "Codex", "Gemini CLI", "Built-in agent"]);
  });

  it("lists the two models without a thinking setting as NO_THINK, like the fixed list did", () => {
    expect(M.NO_THINK).toEqual(["deepseek-chat", "qwen2.5-coder:32b"]);
  });

  it("offers thinking for every model but those two", () => {
    for (const model of ["claude-sonnet-4-5", "gpt-5-codex", "gemini-2.5-pro", "gpt-5-mini"]) {
      expect(M.thinkSupported(model)).toBe(true);
    }
    for (const model of ["deepseek-chat", "qwen2.5-coder:32b"]) {
      expect(M.thinkSupported(model)).toBe(false);
    }
  });
});

describe("M.AGENTS with the daemon's catalog", () => {
  const { M, ctx } = storeWith(GOLDEN_CATALOG);

  it("lists the daemon's agents by name, then the built-in agent", () => {
    expect(Object.keys(M.AGENTS)).toEqual(["Claude Code", "Gemini CLI", "Codex", "Built-in agent"]);
    expect(M.AGENTS["Claude Code"]).toEqual({
      models: ["sonnet", "haiku"],
      icon: "terminal-square",
      version: "2.1.282",
    });
  });

  it("keeps a missing agent in the list, marked, with its hint", () => {
    const codex = M.agentOptions().find((agent) => agent.name === "Codex");
    expect(codex).toMatchObject({ missing: true, status: "missing" });
    expect(codex?.installHint).toContain("npm install -g @openai/codex");
  });

  it("marks an untested agent with its warning, and it is not missing", () => {
    const gemini = agentOptions(ctx).find((agent) => agent.name === "Gemini CLI");
    expect(gemini).toMatchObject({ missing: false, status: "untested" });
    expect(gemini?.warning).toContain("has not been tested");
  });

  it("offers thinking only when the model has it and its agent lets Marshal set it", () => {
    expect(thinkSupported(ctx, "sonnet")).toBe(true);
    expect(thinkSupported(ctx, "haiku")).toBe(false);
    // Gemini's model thinks, but the agent gives Marshal no way to say how hard.
    expect(thinkSupported(ctx, "gemini-2.5-pro")).toBe(false);
    expect(thinkSupported(ctx, "gpt-5-codex")).toBe(false);
    // The built-in agent's models keep the prototype's rule.
    expect(thinkSupported(ctx, "gpt-5-mini")).toBe(true);
    expect(thinkSupported(ctx, "deepseek-chat")).toBe(false);
  });

  it("says no thinking for a model no agent lists", () => {
    expect(thinkSupported(ctx, "never-heard-of-it")).toBe(false);
    expect(thinkSupported(ctx, "")).toBe(false);
  });

  it("lists as NO_THINK the known models that cannot think, each once", () => {
    expect(M.NO_THINK).toEqual([
      "haiku",
      "gemini-2.5-pro",
      "gpt-5-codex",
      "deepseek-chat",
      "qwen2.5-coder:32b",
    ]);
  });

  it("gives the first model of an agent as its default, and nothing for an agent it does not know", () => {
    expect(defaultModel(ctx, "Claude Code")).toBe("sonnet");
    expect(defaultModel(ctx, "Gemini CLI")).toBe("gemini-2.5-pro");
    expect(defaultModel(ctx, "Built-in agent")).toBe("claude-sonnet-4-5");
    expect(defaultModel(ctx, "Aider")).toBeUndefined();
  });
});

describe("the agent a new card starts with", () => {
  it("is Claude Code while it can be used", () => {
    expect(defaultAgent(storeWith(GOLDEN_CATALOG).ctx)).toBe("Claude Code");
    expect(defaultAgent(storeWith().ctx)).toBe("Claude Code");
  });

  it("is the first agent that can be used when Claude Code is missing", () => {
    const missingClaude = wireCatalog([
      wireAgent({ kind: "claude", name: "Claude Code", status: "missing", version: "" }),
      wireAgent({ kind: "codex", name: "Codex", status: "supported", version: "1.0.0" }),
    ]);
    const { M, ctx } = storeWith(missingClaude);
    expect(defaultAgent(ctx)).toBe("Codex");
    M.newCard();
    expect(M.S.newCard?.agent).toBe("Codex");
  });

  it("is the built-in agent when the daemon has none that can be used, and never a missing one", () => {
    const none = wireCatalog([
      wireAgent({ kind: "claude", name: "Claude Code", status: "missing" }),
    ]);
    expect(defaultAgent(storeWith(none).ctx)).toBe("Built-in agent");
    expect(defaultAgentOf([])).toBe("Claude Code");
  });
});

describe("a card that names an agent or model the catalog does not know", () => {
  const card = (store: ReturnType<typeof storeWith>): Card => {
    const first = store.M.S.cards[0];
    if (!first) throw new Error("no card");
    return first;
  };

  it("still draws: its view model keeps the agent and model, and shows no thinking", () => {
    const store = storeWith(GOLDEN_CATALOG);
    const c = card(store);
    c.agent = "Aider";
    c.model = "some-model";
    c.think = "High";
    const view = store.M.deco(c);
    expect(view).toMatchObject({ agent: "Aider", model: "some-model", think: "", hasThink: false });
  });

  it("does not stop a setting change: the model stays when the agent is unknown", () => {
    const store = storeWith(GOLDEN_CATALOG);
    const c = card(store);
    store.M.setSetting(c.id, "agent", "Aider");
    expect(c.agent).toBe("Aider");
    expect(c.model).not.toBeUndefined();
  });
});
