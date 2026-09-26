import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { SEARCH_DEBOUNCE_MS } from "~/sync/search";
import { CommandPalette } from "./CommandPalette";

/*
 * The palette with a daemon connected and section S24a still on the mock (docs/backend-checklist.md
 * B2.11: "While S24a is mock the palette keeps searching the store"). The other palette tests run
 * with no daemon at all, which is the daemon-mode fallback, so this is the only file where the
 * section, and not the missing daemon, is what keeps the search off the daemon. The store is built
 * here, before any import runs, because the palette reads the one store `~/mock` hands out.
 */
const host = await vi.hoisted(async () => {
  window.location.hash = "#nosim";
  const { sectionStatus } = await import("~/data/sections");
  const { wireCard } = await import("~/testing/fake-cards");
  const { createFakeDaemon } = await import("~/testing/fake-daemon");
  const { PROTOTYPE_PROJECTS } = await import("~/testing/projects");
  const { createTestMarshal } = await import("~/testing/test-store");
  const daemon = createFakeDaemon({
    projects: PROTOTYPE_PROJECTS,
    cards: [
      wireCard({ projectId: "api", number: 41, title: "Fix token refresh on login" }),
      wireCard({ projectId: "api", number: 43, title: "Add rate limiting per API key" }),
    ],
  });
  window.M = createTestMarshal({
    data: daemon.data,
    sections: { ...sectionStatus, S5a: "daemon", S24a: "mock" },
  });
  return { daemon };
});

const { daemon } = host;

const DESKTOP_PX = 1440;
const HEIGHT_PX = 900;
const SEARCH = "Search actions, projects, cards, and settings";

beforeAll(async () => {
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  await vi.waitFor(() => expect(M.S.connection?.state).toBe("online"));
});
afterAll(() => daemon.data.stop());

beforeEach(() => {
  vi.useFakeTimers();
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  M.go("project", "api", "board");
  M.set({ palette: true, menu: null, openId: null });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  M.set({ palette: false, openId: null });
});

const input = (): HTMLInputElement => screen.getByRole("textbox", { name: SEARCH });
const options = (): HTMLElement[] => [...document.querySelectorAll<HTMLElement>('[role="option"]')];
const type = (text: string): void => {
  fireEvent.input(input(), { target: { value: text } });
};
const searches = (): string[] => daemon.routes().filter((r) => r.startsWith("GET /v1/search"));

describe("CommandPalette with a daemon connected and S24a on the mock", () => {
  it("searches the store and never asks the daemon, however long typing pauses", async () => {
    render(() => <CommandPalette />);
    type("rate limiting");
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 3);
    expect(searches()).toEqual([]);
    expect(options().map((o) => o.textContent)).toEqual([
      expect.stringContaining("api-gateway #43 Add rate limiting per API key"),
    ]);
  });

  it("finds nothing by a card's key, which only the daemon's search reads", async () => {
    render(() => <CommandPalette />);
    type("api#41");
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 3);
    expect(searches()).toEqual([]);
    expect(options()).toHaveLength(0);
    expect(screen.getByText('No results for "api#41".')).toBeInTheDocument();
  });
});
