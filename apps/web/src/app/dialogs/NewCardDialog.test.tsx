import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { GOLDEN_CATALOG, wireAgent, wireCatalog } from "~/testing/agents";
import { useCatalog } from "~/testing/test-store";
import { NewCardDialog } from "./NewCardDialog";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seedCards = JSON.parse(JSON.stringify(M.S.cards));
const DESKTOP_PX = 1440;
const PHONE_PX = 390;
const HEIGHT_PX = 900;
const FOCUS_MS = 20;

beforeEach(() => {
  vi.useFakeTimers();
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  M.go("project", "api", "board");
  M.set({ newCard: null, openId: null, toasts: [] });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  M.set({ newCard: null, openId: null, toasts: [] });
  M.S.cards = structuredClone(seedCards);
});

const titleField = () => screen.getByRole("textbox", { name: "Title" });
const create = () => screen.getByRole("button", { name: "Create card" });
const combo = (name: string) => screen.getByRole("combobox", { name });

describe("NewCardDialog", () => {
  it("renders nothing while there is no draft", () => {
    render(() => <NewCardDialog />);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("is a form named after the project, with every field at its default", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    const dialog = screen.getByRole("dialog", { name: "New card in api-gateway" });
    expect(dialog.tagName).toBe("FORM");
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(titleField()).toHaveAttribute("placeholder", "Fix token refresh on login");
    expect(screen.getByRole("textbox", { name: /Description/ })).toHaveAttribute(
      "placeholder",
      "What should the agent do, and what does done mean?",
    );
    expect(
      screen.getByText("This becomes the first message in the card's session."),
    ).toBeInTheDocument();
    expect(combo("Template")).toHaveValue("Blank");
    expect(combo("Role")).toHaveValue("Worker");
    expect(combo("Agent")).toHaveValue("Claude Code");
    expect(screen.getByRole("checkbox", { name: "Start the card right away" })).not.toBeChecked();
    expect(create()).toBeDisabled();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("offers the template, role, and agent lists", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    const names = (label: string) =>
      within(combo(label))
        .getAllByRole("option")
        .map((o) => o.textContent);
    expect(names("Template")).toEqual([
      "Blank",
      "Bug fix",
      "New endpoint",
      "Refactor",
      "Plan first",
    ]);
    expect(names("Role")).toEqual(M.ROLE_NAMES);
    expect(names("Agent")).toEqual(Object.keys(M.AGENTS));
  });

  it("focuses the title once it is painted", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    vi.advanceTimersByTime(FOCUS_MS);
    expect(titleField()).toHaveFocus();
  });

  it("writes every field into the draft and enables Create once there is a title", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    fireEvent.input(titleField(), { target: { value: "Fix token refresh" } });
    fireEvent.input(screen.getByRole("textbox", { name: /Description/ }), {
      target: { value: "Refresh before expiry." },
    });
    fireEvent.change(combo("Template"), { target: { value: "Bug fix" } });
    fireEvent.change(combo("Role"), { target: { value: M.ROLE_NAMES[1] } });
    fireEvent.change(combo("Agent"), { target: { value: Object.keys(M.AGENTS)[1] } });
    fireEvent.click(screen.getByRole("checkbox", { name: "Start the card right away" }));
    expect(M.S.newCard).toMatchObject({
      title: "Fix token refresh",
      body: "Refresh before expiry.",
      template: "Bug fix",
      role: M.ROLE_NAMES[1],
      agent: Object.keys(M.AGENTS)[1],
      start: true,
    });
    expect(create()).toBeEnabled();
  });

  it("keeps Create disabled for a blank title", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    fireEvent.input(titleField(), { target: { value: "   " } });
    expect(create()).toBeDisabled();
  });

  it("warns about a title that looks like an existing card and opens that card", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    fireEvent.input(titleField(), { target: { value: "rate limiting API" } });
    const warning = screen.getByRole("status");
    expect(warning).toHaveTextContent("This looks like an existing card");
    fireEvent.click(
      within(warning).getByRole("button", { name: "#43 Add rate limiting per API key" }),
    );
    expect(M.S.newCard).toBeNull();
    expect(M.S.openId).toBe("api#43");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("creates the card, closes, and toasts with an Open action", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    const before = M.S.cards.length;
    fireEvent.input(titleField(), { target: { value: "Brand new thing" } });
    fireEvent.submit(screen.getByRole("dialog"));
    expect(M.S.cards.length).toBe(before + 1);
    expect(M.S.cards.at(-1)?.title).toBe("Brand new thing");
    expect(M.S.newCard).toBeNull();
    expect(M.S.toasts.map((t) => t.msg)).toContain("Card created");
  });

  it("does not create a card from an empty title", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    const before = M.S.cards.length;
    fireEvent.submit(screen.getByRole("dialog"));
    expect(M.S.cards.length).toBe(before);
    expect(M.S.newCard).not.toBeNull();
  });

  it("closes from Cancel, from the X, from a scrim click, and on Escape", () => {
    const { container } = render(() => <NewCardDialog />);
    const open = () => M.newCard();
    open();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(M.S.newCard).toBeNull();
    open();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(M.S.newCard).toBeNull();
    open();
    fireEvent.click(container.querySelector(".bg-scrim-dialog") as HTMLElement);
    expect(M.S.newCard).toBeNull();
    open();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(M.S.newCard).toBeNull();
  });

  it("is a bottom sheet on a phone", () => {
    M.setViewport(PHONE_PX, HEIGHT_PX);
    M.newCard();
    render(() => <NewCardDialog />);
    expect(screen.getByRole("dialog")).toHaveClass("bottom-0", "rounded-t-xl");
  });
});

describe("NewCardDialog with the daemon's agents", () => {
  let restore = () => {};
  beforeEach(() => {
    restore = useCatalog(M, GOLDEN_CATALOG);
  });
  afterEach(() => restore());

  const agentOptions = () =>
    within(combo("Agent"))
      .getAllByRole("option")
      .map((option) => ({
        text: option.textContent,
        disabled: (option as HTMLOptionElement).disabled,
      }));

  it("lists the agents the daemon reports, then the built-in agent, and disables the one that is not installed", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    expect(agentOptions()).toEqual([
      { text: "Claude Code", disabled: false },
      { text: "Gemini CLI", disabled: false },
      { text: "Codex (not installed)", disabled: true },
      { text: "Built-in agent", disabled: false },
    ]);
    expect(combo("Agent")).toHaveValue("Claude Code");
  });

  it("says how to install an agent that is missing, in the daemon's own sentence, under the pickers", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    const hint = screen.getByText(/Codex is not installed\. Install it with: npm install -g/);
    expect(hint).toBeVisible();
    expect(combo("Agent")).toHaveAttribute("aria-describedby", hint.parentElement?.id);
  });

  it("shows the warning of an untested agent while it is chosen, and only then", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    const warning = /Marshal has not been tested with Gemini CLI 0\.36\.0/;
    expect(screen.queryByText(warning)).toBeNull();
    fireEvent.change(combo("Agent"), { target: { value: "Gemini CLI" } });
    expect(screen.getByText(warning)).toBeInTheDocument();
    fireEvent.change(combo("Agent"), { target: { value: "Claude Code" } });
    expect(screen.queryByText(warning)).toBeNull();
  });

  it("lets an untested agent be chosen, and the card it makes gets that agent's first model", () => {
    M.newCard();
    render(() => <NewCardDialog />);
    fireEvent.change(combo("Agent"), { target: { value: "Gemini CLI" } });
    fireEvent.input(titleField(), { target: { value: "Try the untested agent" } });
    fireEvent.submit(screen.getByRole("dialog"));
    expect(M.S.cards.at(-1)).toMatchObject({ agent: "Gemini CLI", model: "gemini-2.5-pro" });
  });

  it("shows no notes when every agent can be used", () => {
    restore();
    M.newCard();
    render(() => <NewCardDialog />);
    expect(screen.queryByText(/is not installed/)).toBeNull();
    expect(agentOptions().every((option) => !option.disabled)).toBe(true);
  });

  it("starts on the first agent that can be used when Claude Code is not installed", () => {
    useCatalog(
      M,
      wireCatalog([
        wireAgent({ kind: "claude", name: "Claude Code", status: "missing", version: "" }),
        wireAgent({ kind: "codex", name: "Codex", status: "supported", version: "1.0.0" }),
      ]),
    );
    M.newCard();
    render(() => <NewCardDialog />);
    expect(combo("Agent")).toHaveValue("Codex");
    expect(M.S.newCard?.agent).toBe("Codex");
  });
});
