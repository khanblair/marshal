import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { createMemo } from "solid-js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type Card, M } from "~/mock";
import type { CardKey } from "~/mock/card-key";
import { CardItem } from "./CardItem";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seed = JSON.parse(JSON.stringify(M.S.cards));
const DESKTOP_PX = 1440;
const CARD_WORKING = "api#41";
const CARD_NEEDS = "api#43";
const CARD_MERGING = "api#35";
const CARD_PACKAGE = "mobile#209";

function findCard(id: CardKey): Card {
  const card = M.card(id);
  if (!card) throw new Error(`no card #${id}`);
  return card;
}

/** A card the way the board draws it: its view model in a memo, so store changes reach it. */
function LiveCard(props: { id: CardKey }) {
  const view = createMemo(() => M.deco(findCard(props.id)));
  return <CardItem c={view()} />;
}

const cardEl = (id: CardKey): HTMLElement => {
  const el = document.querySelector<HTMLElement>(`[data-card="${id}"]`);
  if (!el) throw new Error(`no element for #${id}`);
  return el;
};

beforeEach(() => {
  vi.useFakeTimers();
  M.S.cards = structuredClone(seed);
  M.S.openId = null;
  M.S.focusId = null;
  M.S.dragId = null;
  M.setViewport(DESKTOP_PX, 900);
  M.go("project", "api", "board");
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

describe("CardItem", () => {
  it("is a focusable button named after the card", () => {
    render(() => <LiveCard id={CARD_WORKING} />);
    const el = cardEl(CARD_WORKING);
    expect(el).toHaveAttribute("role", "button");
    expect(el).toHaveAttribute("tabindex", "0");
    expect(el).toHaveAttribute("aria-label", M.deco(findCard(CARD_WORKING)).aria);
    expect(screen.getByRole("button", { name: /^#41 / })).toBe(el);
  });

  it("shows the title, number, state, role, model, and what it is doing", () => {
    const view = M.deco(findCard(CARD_WORKING));
    render(() => <LiveCard id={CARD_WORKING} />);
    expect(screen.getByText(view.title)).toBeInTheDocument();
    expect(screen.getByText("#41")).toBeInTheDocument();
    expect(screen.getByText("Working")).toBeInTheDocument();
    expect(screen.getByText(view.role)).toBeInTheDocument();
    expect(screen.getByText(view.model)).toBeInTheDocument();
    expect(screen.getByText(view.doing)).toBeInTheDocument();
  });

  it("gives a Needs you card its reason and a state colored edge", () => {
    const view = M.deco(findCard(CARD_NEEDS));
    render(() => <LiveCard id={CARD_NEEDS} />);
    expect(screen.getByText(view.reason)).toHaveClass("line-clamp-2", "text-status-needs-you-text");
    expect(cardEl(CARD_NEEDS).style.borderLeftColor).toBe("var(--color-status-needs-you-solid)");
  });

  it("shows merge progress on a merging card", () => {
    render(() => <LiveCard id={CARD_MERGING} />);
    const bar = screen.getByRole("progressbar", { name: "Merge progress" });
    expect(bar).toHaveAttribute("aria-valuenow", String(findCard(CARD_MERGING).mergePct));
  });

  it("shows the bypass badge only when bypass is on", () => {
    render(() => <LiveCard id={CARD_WORKING} />);
    expect(screen.queryByText("Bypass")).not.toBeInTheDocument();
    findCard(CARD_WORKING).bypass = true;
    expect(screen.getByText("Bypass")).toBeInTheDocument();
    expect(cardEl(CARD_WORKING).getAttribute("aria-label")).toContain("Bypass permissions on");
  });

  it("shows the package and the members", () => {
    const view = M.deco(findCard(CARD_PACKAGE));
    render(() => <LiveCard id={CARD_PACKAGE} />);
    expect(screen.getByText(view.pkg ?? "")).toBeInTheDocument();
    const members = screen.getByLabelText("Members");
    expect(members.children).toHaveLength(view.avatars.length);
    expect(members.querySelector(`[title="${view.avatars[0]?.title}"]`)).not.toBeNull();
  });

  it("shows the footer meta: branch, CI, cost, checklist, comments, attachments, and Pinned", () => {
    const card = findCard(CARD_WORKING);
    Object.assign(card, { pinned: true, cost: 1.25, branch: "feat/board", ci: "failed" });
    render(() => <LiveCard id={CARD_WORKING} />);
    expect(screen.getByText("feat/board")).toHaveClass("font-mono", "truncate");
    expect(screen.getByTitle("CI failed")).toHaveTextContent("Failed");
    expect(screen.getByTitle("Cost of this card")).toHaveTextContent("$1.25");
    expect(screen.getByTitle("Pinned: this card never sleeps")).toHaveTextContent("Pinned");
  });

  it("shows an asleep marker and the paused line", () => {
    const card = findCard(CARD_WORKING);
    render(() => <LiveCard id={CARD_WORKING} />);
    expect(screen.queryByText("Paused by you")).not.toBeInTheDocument();
    card.paused = true;
    expect(screen.getByText("Paused by you")).toBeInTheDocument();
    card.asleep = true;
    expect(screen.getByText("Asleep")).toBeInTheDocument();
  });

  it("opens the card on click and on Enter", () => {
    render(() => <LiveCard id={CARD_WORKING} />);
    fireEvent.click(cardEl(CARD_WORKING));
    expect(M.S.openId).toBe(CARD_WORKING);
    M.S.openId = null;
    fireEvent.keyDown(cardEl(CARD_WORKING), { key: "Enter" });
    expect(M.S.openId).toBe(CARD_WORKING);
    M.S.openId = null;
    fireEvent.keyDown(cardEl(CARD_WORKING), { key: "a" });
    expect(M.S.openId).toBeNull();
  });

  it("rings the selected card, and the focused one with a lighter ring", () => {
    render(() => <LiveCard id={CARD_WORKING} />);
    expect(cardEl(CARD_WORKING).style.boxShadow).toBe("none");
    M.S.focusId = CARD_WORKING;
    expect(cardEl(CARD_WORKING).style.boxShadow).toBe("0 0 0 2px var(--color-border-strong)");
    M.S.openId = CARD_WORKING;
    expect(cardEl(CARD_WORKING).style.boxShadow).toBe("0 0 0 2px var(--color-ink)");
  });

  it("fades while it is being dragged", () => {
    render(() => <LiveCard id={CARD_WORKING} />);
    expect(cardEl(CARD_WORKING).style.opacity).toBe("1");
    M.S.dragId = CARD_WORKING;
    expect(cardEl(CARD_WORKING).style.opacity).toBe("0.4");
  });

  it("updates in place when the card changes", () => {
    render(() => <LiveCard id={CARD_WORKING} />);
    const el = cardEl(CARD_WORKING);
    findCard(CARD_WORKING).title = "Renamed on the board";
    expect(screen.getByText("Renamed on the board")).toBeInTheDocument();
    expect(cardEl(CARD_WORKING)).toBe(el);
  });
});
