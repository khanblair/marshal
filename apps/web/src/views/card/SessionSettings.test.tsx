// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (S5a is the daemon's).

import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { daemon, resetDaemonCards } from "~/testing/daemon-cards-store";
import { M } from "~/mock";
import { GOLDEN_CATALOG } from "~/testing/agents";
import { useCatalog } from "~/testing/test-store";
import { SessionSettings } from "./SessionSettings";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

let restore = () => {};
beforeEach(() => {
  resetStore();
  // The panel's changes really reach the daemon, so its card starts each test as it did the first.
  resetDaemonCards();
});
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

/** Waits for a setting the panel just changed to come back from the daemon, which is where the card's own fields say so. */
const saved = (fields: Partial<ReturnType<typeof cardOf>>) =>
  vi.waitFor(() => expect(cardOf("api#41")).toMatchObject(fields));

describe("SessionSettings with the prototype's agents", () => {
  it("lists the agents, the models of the card's agent, and thinking while the model has it", () => {
    show();
    expect(options("Agent")).toEqual(["Claude Code", "Codex", "Gemini CLI", "Built-in agent"]);
    expect(options("Model")).toEqual(["claude-sonnet-4-5", "claude-opus-4-1", "claude-haiku-4-5"]);
    expect(options("Thinking mode")).toEqual(M.THINK);
  });

  it("changes the models with the agent, and thinking with the model", async () => {
    show();
    fireEvent.change(combo("Agent"), { target: { value: "Built-in agent" } });
    await saved({ agent: "Built-in agent" });
    // The daemon's card routes take its own id, not the key the store is kept by.
    expect(daemon.routes()).toContain(
      `PATCH /v1/cards/${encodeURIComponent(cardOf("api#41").daemonId ?? "")}`,
    );
    expect(options("Model")).toEqual([
      "claude-sonnet-4-5",
      "gpt-5-mini",
      "deepseek-chat",
      "gemini-2.5-flash",
      "qwen2.5-coder:32b",
    ]);
    expect(screen.getByRole("combobox", { name: "Thinking mode" })).toBeInTheDocument();
    fireEvent.change(combo("Model"), { target: { value: "deepseek-chat" } });
    await saved({ model: "deepseek-chat" });
    expect(screen.queryByRole("combobox", { name: "Thinking mode" })).toBeNull();
    fireEvent.change(combo("Model"), { target: { value: "gpt-5-mini" } });
    await saved({ model: "gpt-5-mini" });
    // A model that can be told how hard to think, with no setting left on the card, gets the
    // prototype's own Medium back (the panel's rule travels with the change).
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

  it("lists the models of the card's agent, keeping the card's own model when the catalog lacks it", async () => {
    show();
    expect(options("Model")).toEqual(["claude-sonnet-4-5", "sonnet", "haiku"]);
    fireEvent.change(combo("Agent"), { target: { value: "Gemini CLI" } });
    await saved({ agent: "Gemini CLI" });
    // Changing the agent carries the model that agent starts its cards on, as the mock's setting
    // did, so the card does not keep another agent's model.
    expect(cardOf("api#41").model).toBe("gemini-2.5-pro");
    expect(options("Model")).toEqual(["gemini-2.5-pro"]);
  });

  it("offers thinking only for a model that has it and whose agent lets Marshal set it", async () => {
    show();
    fireEvent.change(combo("Model"), { target: { value: "sonnet" } });
    await saved({ model: "sonnet" });
    expect(screen.getByRole("combobox", { name: "Thinking mode" })).toBeInTheDocument();
    fireEvent.change(combo("Model"), { target: { value: "haiku" } });
    await saved({ model: "haiku" });
    expect(screen.queryByRole("combobox", { name: "Thinking mode" })).toBeNull();
    // The agent change already carries Gemini's own model: it thinks, but the agent cannot be told
    // how hard, so the thinking picker stays away and the thinking the card had is cleared.
    fireEvent.change(combo("Agent"), { target: { value: "Gemini CLI" } });
    await saved({ agent: "Gemini CLI", model: "gemini-2.5-pro", think: null });
    expect(screen.queryByRole("combobox", { name: "Thinking mode" })).toBeNull();
  });

  it("shows the warning of an untested agent while a card uses it", async () => {
    show();
    expect(screen.queryByText(/has not been tested with Gemini CLI/)).toBeNull();
    fireEvent.change(combo("Agent"), { target: { value: "Gemini CLI" } });
    await saved({ agent: "Gemini CLI" });
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
