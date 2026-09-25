import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { GOLDEN_CATALOG } from "~/testing/agents";
import { useCatalog } from "~/testing/test-store";
import { SessionSettings } from "./SessionSettings";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

let restore = () => {};
beforeEach(() => resetStore());
afterEach(() => {
  cleanup();
  restore();
  restore = () => {};
});

const show = (id = "api#41") => render(() => <SessionSettings card={cardOf(id)} />);
const combo = (name: string) => screen.getByRole("combobox", { name });
const options = (name: string) =>
  within(combo(name))
    .getAllByRole("option")
    .map((option) => option.textContent);

describe("SessionSettings with the prototype's agents", () => {
  it("lists the agents, the models of the card's agent, and thinking while the model has it", () => {
    show();
    expect(options("Agent")).toEqual(["Claude Code", "Codex", "Gemini CLI", "Built-in agent"]);
    expect(options("Model")).toEqual(["claude-sonnet-4-5", "claude-opus-4-1", "claude-haiku-4-5"]);
    expect(options("Thinking mode")).toEqual(M.THINK);
  });

  it("changes the models with the agent, and thinking with the model", () => {
    show();
    fireEvent.change(combo("Agent"), { target: { value: "Built-in agent" } });
    expect(options("Model")).toEqual([
      "claude-sonnet-4-5",
      "gpt-5-mini",
      "deepseek-chat",
      "gemini-2.5-flash",
      "qwen2.5-coder:32b",
    ]);
    expect(screen.getByRole("combobox", { name: "Thinking mode" })).toBeInTheDocument();
    fireEvent.change(combo("Model"), { target: { value: "deepseek-chat" } });
    expect(screen.queryByRole("combobox", { name: "Thinking mode" })).toBeNull();
    fireEvent.change(combo("Model"), { target: { value: "gpt-5-mini" } });
    expect(combo("Thinking mode")).toHaveValue("Medium");
  });

  it("shows no note while every agent can be used", () => {
    show();
    expect(screen.queryByText(/is not installed/)).toBeNull();
  });
});

describe("SessionSettings with the daemon's agents", () => {
  beforeEach(() => {
    restore = useCatalog(M, GOLDEN_CATALOG);
  });

  it("lists the daemon's agents and the built-in agent, and the one not installed is disabled with its hint", () => {
    show();
    expect(options("Agent")).toEqual([
      "Claude Code",
      "Gemini CLI",
      "Codex (not installed)",
      "Built-in agent",
    ]);
    const codex = within(combo("Agent")).getByRole("option", { name: "Codex (not installed)" });
    expect(codex).toBeDisabled();
    expect(
      screen.getByText(/Codex is not installed\. Install it with: npm install -g/),
    ).toBeVisible();
  });

  it("lists the models of the card's agent, keeping the card's own model when the catalog lacks it", () => {
    show();
    expect(options("Model")).toEqual(["claude-sonnet-4-5", "sonnet", "haiku"]);
    fireEvent.change(combo("Agent"), { target: { value: "Gemini CLI" } });
    expect(cardOf("api#41").model).toBe("gemini-2.5-pro");
    expect(options("Model")).toEqual(["gemini-2.5-pro"]);
  });

  it("offers thinking only for a model that has it and whose agent lets Marshal set it", () => {
    show();
    fireEvent.change(combo("Model"), { target: { value: "sonnet" } });
    expect(screen.getByRole("combobox", { name: "Thinking mode" })).toBeInTheDocument();
    fireEvent.change(combo("Model"), { target: { value: "haiku" } });
    expect(screen.queryByRole("combobox", { name: "Thinking mode" })).toBeNull();
    // Gemini's model thinks, but the agent cannot be told how hard.
    fireEvent.change(combo("Agent"), { target: { value: "Gemini CLI" } });
    expect(screen.queryByRole("combobox", { name: "Thinking mode" })).toBeNull();
  });

  it("shows the warning of an untested agent while a card uses it", () => {
    show();
    expect(screen.queryByText(/has not been tested with Gemini CLI/)).toBeNull();
    fireEvent.change(combo("Agent"), { target: { value: "Gemini CLI" } });
    expect(screen.getByText(/Marshal has not been tested with Gemini CLI 0\.36\.0/)).toBeVisible();
  });

  it("still draws a card that names an agent the catalog does not know, and shows that agent", () => {
    const card = cardOf("api#41");
    card.agent = "Aider";
    card.model = "aider-model";
    show();
    expect(options("Agent")).toEqual([
      "Aider",
      "Claude Code",
      "Gemini CLI",
      "Codex (not installed)",
      "Built-in agent",
    ]);
    expect(combo("Agent")).toHaveValue("Aider");
    expect(options("Model")).toEqual(["aider-model"]);
    expect(screen.queryByRole("combobox", { name: "Thinking mode" })).toBeNull();
  });

  it("shows a card whose agent has since gone missing as that agent, disabled but selected", () => {
    cardOf("api#41").agent = "Codex";
    show();
    expect(combo("Agent")).toHaveValue("Codex");
    expect(
      within(combo("Agent")).getByRole("option", { name: "Codex (not installed)" }),
    ).toBeDisabled();
  });
});
