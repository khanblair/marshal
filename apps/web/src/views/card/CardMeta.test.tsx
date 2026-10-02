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
