import { cleanup, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { CardMeta } from "./CardMeta";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => resetStore());
afterEach(cleanup);

const show = (id = "api#41") => {
  const card = cardOf(id);
  return render(() => <CardMeta card={card} c={M.deco(card)} />);
};

describe("CardMeta branch line", () => {
  it("shows the card's branch and the branch it merges into", () => {
    Object.assign(M.proj("api") ?? {}, { integrationBranch: "development" });
    show();
    const line = screen.getByTitle("Finished work merges into development");
    expect(line).toHaveTextContent("marshal/41-fix-token-refresh→development");
  });

  it("shows only the card's branch when the project has no integration branch", () => {
    const project = M.proj("api");
    if (project) delete project.integrationBranch;
    show();
    expect(screen.getByText("marshal/41-fix-token-refresh")).toBeInTheDocument();
    expect(screen.queryByText("→")).toBeNull();
    expect(screen.queryByTitle(/Finished work merges into/)).toBeNull();
  });

  it("shows no branch line for a card that has not started", () => {
    Object.assign(M.proj("api") ?? {}, { integrationBranch: "development" });
    show("api#45");
    expect(screen.queryByText("→")).toBeNull();
  });
});

describe("CardMeta files shared with other cards", () => {
  const flowWith = (overlaps: { path: string; cards: string[]; cardIds: string[] }[]) => ({
    api: { flow: { overlaps }, error: "", loading: false },
  });

  it("names the files another card has also changed, and which cards", () => {
    cardOf("api#41").daemonId = "01CARD41";
    M.set({
      integration: flowWith([
        { path: "src/auth.ts", cards: ["api#36", "api#41"], cardIds: ["01CARD36", "01CARD41"] },
        { path: "src/db.ts", cards: ["api#36", "api#41"], cardIds: ["01CARD36", "01CARD41"] },
      ]) as never,
    });
    show();
    expect(screen.getByText("Shares 2 files with api#36")).toBeInTheDocument();
    expect(screen.getByTitle(/src\/auth\.ts/)).toBeInTheDocument();
  });

  it("says nothing for a card whose files no other card has changed", () => {
    cardOf("api#41").daemonId = "01CARD41";
    M.set({
      integration: flowWith([
        { path: "src/auth.ts", cards: ["api#36", "api#37"], cardIds: ["01CARD36", "01CARD37"] },
      ]) as never,
    });
    show();
    expect(screen.queryByText(/Shares/)).toBeNull();
  });
});

it("shows measured context and identifies missing readings", () => {
  const card = cardOf("api#41");
  card.ctx = 0;
  show();
  expect(screen.getByText("Context unavailable")).toBeInTheDocument();
  card.ctx = 0.42;
  expect(screen.getByText("42%")).toBeInTheDocument();
});
