import { describe, expect, it } from "vitest";
import { toAgentOptions } from "~/data/mappers/agents";
import { GOLDEN_CATALOG, wireAgent, wireCatalog } from "~/testing/agents";
import { agentNotes, agentSelectOptions, modelOptions } from "./agent-picker";

const golden = toAgentOptions(GOLDEN_CATALOG);

describe("agentSelectOptions", () => {
  it("lists every agent in order and disables one that is not installed, saying so in its label", () => {
    expect(agentSelectOptions(golden)).toEqual([
      { value: "Claude Code", label: "Claude Code", disabled: false },
      { value: "Gemini CLI", label: "Gemini CLI", disabled: false },
      { value: "Codex", label: "Codex (not installed)", disabled: true },
    ]);
  });

  it("keeps an untested agent enabled", () => {
    expect(agentSelectOptions(golden).find((row) => row.value === "Gemini CLI")?.disabled).toBe(
      false,
    );
  });

  it("puts an agent the catalog does not know first, so the picker shows what is set", () => {
    const rows = agentSelectOptions(golden, "Aider");
    expect(rows[0]).toEqual({ value: "Aider" });
    expect(rows).toHaveLength(4);
  });

  it("adds nothing for an agent it knows, or for none", () => {
    expect(agentSelectOptions(golden, "Codex")).toHaveLength(3);
    expect(agentSelectOptions(golden, "")).toHaveLength(3);
    expect(agentSelectOptions(golden, undefined)).toHaveLength(3);
  });

  it("is empty for an empty catalog", () => {
    expect(agentSelectOptions([])).toEqual([]);
  });
});

describe("agentNotes", () => {
  it("gives how to install each missing agent", () => {
    expect(agentNotes(golden)).toEqual([
      expect.stringContaining("Codex is not installed. Install it with: npm install -g"),
    ]);
  });

  it("puts the warning of the chosen agent first when its version is untested", () => {
    const notes = agentNotes(golden, "Gemini CLI");
    expect(notes).toHaveLength(2);
    expect(notes[0]).toContain("has not been tested with Gemini CLI 0.36.0");
    expect(notes[1]).toContain("Codex is not installed");
  });

  it("shows no warning for an agent that is not chosen", () => {
    expect(agentNotes(golden, "Claude Code").join(" ")).not.toContain("has not been tested");
  });

  it("says nothing when every agent can be used, and drops empty and repeated sentences", () => {
    const fine = toAgentOptions(
      wireCatalog([wireAgent({ kind: "claude", name: "Claude Code", warning: "" })]),
    );
    expect(agentNotes(fine, "Claude Code")).toEqual([]);
    const twice = toAgentOptions(
      wireCatalog([
        wireAgent({ kind: "codex", name: "Codex", status: "missing", installHint: "Install it." }),
        wireAgent({
          kind: "gemini",
          name: "Gemini CLI",
          status: "missing",
          installHint: "Install it.",
        }),
      ]),
    );
    expect(agentNotes(twice)).toEqual(["Install it."]);
  });
});

describe("modelOptions", () => {
  it("is the agent's models as they are", () => {
    expect(modelOptions(["a", "b"], "b")).toEqual(["a", "b"]);
  });

  it("puts a model the agent does not list first, so the picker shows what is set", () => {
    expect(modelOptions(["a", "b"], "old")).toEqual(["old", "a", "b"]);
  });

  it("adds nothing for an empty current model", () => {
    expect(modelOptions(["a"], "")).toEqual(["a"]);
    expect(modelOptions([], "")).toEqual([]);
  });
});
