import { cleanup, fireEvent, render } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { useGlobalKeys } from "./keyboard";
import { PHONE_PX, resetShell } from "./shell-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/** A card that waits for approval (seed: #44 "Upgrade grpc-go to 1.66"). */
const APPROVAL_CARD = "api#44";
const CARD_A = "api#41";
const CARD_B = "api#42";
const CARD_C = "api#43";

function Keys() {
  useGlobalKeys();
  return (
    <div>
      <input aria-label="field" />
      <button type="button" data-search="1">
        search
      </button>
    </div>
  );
}

const press = (key: string, init: KeyboardEventInit = {}) =>
  fireEvent.keyDown(window, { key, ...init });

beforeEach(() => {
  resetShell();
  M.nav = null;
  render(() => <Keys />);
});
afterEach(cleanup);

describe("search and view shortcuts", () => {
  it("Ctrl+K opens the palette and a second press closes it", () => {
    press("k", { ctrlKey: true });
    expect(M.S.palette).toBe(true);
    press("k", { metaKey: true });
    expect(M.S.palette).toBe(false);
  });

  it("Ctrl+1 to 6 open the views of the current project", () => {
    M.go("project", "api", "board");
    press("4", { ctrlKey: true });
    expect(M.S.route.view).toBe("list");
    press("1", { ctrlKey: true });
    expect(M.S.route.view).toBe("chat");
  });

  it("a view shortcut on another page goes to the project", () => {
    press("3", { ctrlKey: true });
    expect(M.S.route).toMatchObject({ page: "project", view: "board" });
  });

  it("does nothing for a digit past the last view", () => {
    press("9", { ctrlKey: true });
    expect(M.S.route.page).toBe("home");
  });

  it("ignores every key while onboarding or the tour is on", () => {
    M.set({ onboarding: true });
    press("k", { ctrlKey: true });
    expect(M.S.palette).toBe(false);
    M.set({ onboarding: false, tour: { step: 0 } });
    press("k", { ctrlKey: true });
    expect(M.S.palette).toBe(false);
  });
});

describe("Escape closes one layer at a time", () => {
  it("goes through palette, dialog, new card, new project, remove project, menu, notices, sidebar", () => {
    M.set({
      palette: true,
      newCard: {
        title: "",
        body: "",
        template: "Blank",
        role: "Worker",
        agent: "Claude Code",
        start: true,
      },
      newProject: { source: "folder", path: "", url: "", name: "", branch: "main" },
      removeProject: { id: "api", keepBranches: true, keepMemory: true },
      menu: "avatar",
      noticesOpen: true,
      sideOpen: true,
    });
    press("Escape");
    expect(M.S.palette).toBe(false);
    press("Escape");
    expect(M.S.newCard).toBeNull();
    press("Escape");
    expect(M.S.newProject).toBeNull();
    press("Escape");
    expect(M.S.removeProject).toBeNull();
    press("Escape");
    expect(M.S.menu).toBeNull();
    press("Escape");
    expect(M.S.noticesOpen).toBe(false);
    press("Escape");
    expect(M.S.sideOpen).toBe(false);
  });

  it("closes a confirm dialog", () => {
    M.confirm({ title: "Sure?", message: "m", action: "Yes", run: () => undefined });
    press("Escape");
    expect(M.S.dialog).toBeNull();
  });

  it("blurs a focused field before closing the card", () => {
    M.openCard(CARD_A);
    const field = document.querySelector<HTMLInputElement>("input");
    field?.focus();
    press("Escape");
    expect(document.activeElement).not.toBe(field);
    expect(M.S.openId).toBe(CARD_A);
    press("Escape");
    expect(M.S.openId).toBeNull();
  });
});

describe("single-key card shortcuts", () => {
  it("N starts a new card, on the board", () => {
    press("n");
    expect(M.S.route).toMatchObject({ page: "project", view: "board" });
    expect(M.S.newCard).not.toBeNull();
  });

  it("A approves the pending approval of the open card", () => {
    M.openCard(APPROVAL_CARD);
    expect(M.pendingApproval(APPROVAL_CARD)).toBeTruthy();
    press("a");
    expect(M.pendingApproval(APPROVAL_CARD)).toBeFalsy();
  });

  it("A does nothing for a card without one", () => {
    M.openCard(CARD_A);
    const state = M.card(CARD_A)?.state;
    press("a");
    expect(M.card(CARD_A)?.state).toBe(state);
  });

  it("S puts an awake card to sleep and wakes a sleeping one", () => {
    const sleep = vi.spyOn(M, "sleep").mockImplementation(() => undefined);
    const wake = vi.spyOn(M, "wake").mockImplementation(() => undefined);
    M.openCard(CARD_A);
    const card = M.card(CARD_A);
    if (!card) throw new Error("seed card missing");
    const asleep = card.asleep;
    card.asleep = false;
    press("s");
    expect(sleep).toHaveBeenCalledWith(CARD_A);
    card.asleep = true;
    press("s");
    expect(wake).toHaveBeenCalledWith(CARD_A);
    card.asleep = asleep;
    sleep.mockRestore();
    wake.mockRestore();
  });

  it("P pins the card", () => {
    M.openCard(CARD_A);
    const pinned = !!M.card(CARD_A)?.pinned;
    press("p");
    expect(!!M.card(CARD_A)?.pinned).toBe(!pinned);
  });

  it("/ focuses the search field", () => {
    press("/");
    expect(document.activeElement).toBe(document.querySelector("[data-search]"));
  });

  it("Enter opens the focused card when nothing is focused", () => {
    M.set({ focusId: CARD_B });
    press("Enter");
    expect(M.S.openId).toBe(CARD_B);
  });

  it("ignores single keys while typing in a field", () => {
    document.querySelector<HTMLInputElement>("input")?.focus();
    press("n");
    expect(M.S.newCard).toBeNull();
  });

  it("ignores single keys while a dialog is open or a modifier is held", () => {
    M.set({ palette: true });
    press("n");
    expect(M.S.newCard).toBeNull();
    M.set({ palette: false });
    press("n", { altKey: true });
    expect(M.S.newCard).toBeNull();
    press("n", { ctrlKey: true });
    expect(M.S.newCard).toBeNull();
  });
});

describe("arrow keys move card focus", () => {
  it("moves through a grid of columns", () => {
    M.nav = { owner: "board", grid: [[CARD_A, CARD_B], [], [CARD_C]] };
    M.set({ focusId: CARD_A });
    press("ArrowDown");
    expect(M.S.focusId).toBe(CARD_B);
    press("ArrowUp");
    expect(M.S.focusId).toBe(CARD_A);
    press("ArrowRight");
    expect(M.S.focusId).toBe(CARD_C);
    press("ArrowLeft");
    expect(M.S.focusId).toBe(CARD_A);
  });

  it("starts at the first card when none is focused", () => {
    M.nav = { owner: "board", grid: [[], [CARD_B]] };
    M.set({ focusId: null });
    press("ArrowDown");
    expect(M.S.focusId).toBe(CARD_B);
  });

  it("moves up and down a list, and ignores left and right", () => {
    M.nav = { owner: "list", rows: [CARD_A, CARD_B, CARD_C] };
    M.set({ focusId: CARD_A });
    press("ArrowDown");
    press("ArrowDown");
    press("ArrowDown");
    expect(M.S.focusId).toBe(CARD_C);
    press("ArrowLeft");
    expect(M.S.focusId).toBe(CARD_C);
    press("ArrowUp");
    expect(M.S.focusId).toBe(CARD_B);
  });

  it("focuses the card element after a moment", () => {
    vi.useFakeTimers();
    const card = document.createElement("div");
    card.tabIndex = 0;
    card.dataset.card = String(CARD_B);
    document.body.append(card);
    M.nav = { owner: "list", rows: [CARD_A, CARD_B] };
    M.set({ focusId: CARD_A });
    press("ArrowDown");
    vi.advanceTimersByTime(30);
    expect(document.activeElement).toBe(card);
    card.remove();
    vi.useRealTimers();
  });

  it("does nothing without a navigation model", () => {
    M.set({ focusId: CARD_A });
    press("ArrowDown");
    expect(M.S.focusId).toBe(CARD_A);
  });

  it("leaves arrows to a control marked data-no-nav", () => {
    M.nav = { owner: "list", rows: [CARD_A, CARD_B] };
    M.set({ focusId: CARD_A });
    const box = document.createElement("div");
    box.dataset.noNav = "1";
    const inner = document.createElement("button");
    box.append(inner);
    document.body.append(box);
    inner.focus();
    press("ArrowDown");
    expect(M.S.focusId).toBe(CARD_A);
    box.remove();
  });
});

describe("on a phone", () => {
  it("still opens the palette with Ctrl+K", () => {
    M.setViewport(PHONE_PX, 900);
    press("k", { ctrlKey: true });
    expect(M.S.palette).toBe(true);
  });
});
