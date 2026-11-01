import { cleanup, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { GOLDEN_CATALOG } from "~/testing/agents";
import { optionsOf, pickValue, selectedValue } from "~/testing/dropdown";
import { useCatalog } from "~/testing/test-store";
import { showSettings } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

let restore = () => {};
beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  cleanup();
  restore();
  restore = () => {};
  vi.clearAllTimers();
  vi.useRealTimers();
});

const combo = (name: string) => screen.getByRole("combobox", { name });
const options = (name: string) => optionsOf(combo(name)).map((option) => option.textContent);
const thinking = () => screen.queryByRole("combobox", { name: "Thinking mode" });

describe("the role editor's pickers with the prototype's agents", () => {
  it("lists the agents, the models of the role's agent, every model for the backup, and thinking", () => {
    showSettings("roles");
    expect(options("Agent")).toEqual(["Claude Code", "Codex", "Gemini CLI", "Built-in agent"]);
    expect(options("Model")).toEqual(["claude-sonnet-4-5", "claude-opus-4-1", "claude-haiku-4-5"]);
    expect(options("Backup model")).toEqual([
      "claude-sonnet-4-5",
      "claude-opus-4-1",
      "claude-haiku-4-5",
      "gpt-5-codex",
      "gpt-5",
      "gpt-5-mini",
      "gemini-2.5-pro",
      "gemini-2.5-flash",
      "deepseek-chat",
      "qwen2.5-coder:32b",
    ]);
    expect(selectedValue(combo("Backup model"))).toBe("gpt-5");
    expect(options("Thinking mode")).toEqual(M.THINK);
  });

  it("gives the role the new agent's first model, and its models", () => {
    showSettings("roles");
    pickValue(combo("Agent"), "Gemini CLI");
    expect(selectedValue(combo("Model"))).toBe("gemini-2.5-pro");
    expect(options("Model")).toEqual(["gemini-2.5-pro", "gemini-2.5-flash"]);
  });

  it("offers thinking only while the model has it", () => {
    showSettings("roles");
    pickValue(combo("Agent"), "Built-in agent");
    expect(thinking()).not.toBeNull();
    pickValue(combo("Model"), "deepseek-chat");
    expect(thinking()).toBeNull();
    pickValue(combo("Model"), "gpt-5-mini");
    expect(thinking()).not.toBeNull();
  });
});

describe("the role editor's pickers with the daemon's agents", () => {
  beforeEach(() => {
    restore = useCatalog(M, GOLDEN_CATALOG);
  });

  it("lists the daemon's agents, disables the one that is not installed, and says how to install it", () => {
    showSettings("roles");
    expect(options("Agent")).toEqual([
      "Claude Code",
      "Gemini CLI",
      "Codex (not installed)",
      "Built-in agent",
    ]);
    expect(
      optionsOf(combo("Agent")).find(
        (o) => o.textContent === "Codex (not installed)",
      ) as HTMLElement,
    ).toBeDisabled();
    expect(
      screen.getByText(/Codex is not installed\. Install it with: npm install -g/),
    ).toBeVisible();
  });

  it("keeps the role's own model when the catalog lacks it, then lists the catalog's models", () => {
    showSettings("roles");
    expect(selectedValue(combo("Model"))).toBe("claude-sonnet-4-5");
    expect(options("Model")).toEqual(["claude-sonnet-4-5", "sonnet", "haiku"]);
    pickValue(combo("Model"), "haiku");
    expect(thinking()).toBeNull();
    pickValue(combo("Model"), "sonnet");
    expect(thinking()).not.toBeNull();
  });

  it("lists every model of every agent once for the backup, and keeps the role's backup", () => {
    showSettings("roles");
    expect(options("Backup model")).toEqual([
      "gpt-5",
      "sonnet",
      "haiku",
      "gemini-2.5-pro",
      "gpt-5-codex",
      "claude-sonnet-4-5",
      "gpt-5-mini",
      "deepseek-chat",
      "gemini-2.5-flash",
      "qwen2.5-coder:32b",
    ]);
  });

  it("draws a role whose agent the catalog does not know, and shows that agent", () => {
    showSettings("roles");
    const role = M.S.roles.find((r) => r.name === M.S.roleSel);
    if (!role) throw new Error("no role");
    role.agent = "Aider";
    role.model = "aider-model";
    expect(selectedValue(combo("Agent"))).toBe("Aider");
    expect(options("Model")).toEqual(["aider-model"]);
    expect(thinking()).toBeNull();
  });
});
