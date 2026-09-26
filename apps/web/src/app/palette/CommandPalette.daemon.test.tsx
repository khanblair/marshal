import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { SEARCH_DEBOUNCE_MS } from "~/sync/search";
import { CommandPalette } from "./CommandPalette";

/*
 * The palette with section S24a on the daemon (docs/backend-checklist.md B2.11). The store and the
 * fake daemon it follows are built here, before any import runs, because the palette reads the one
 * store `~/mock` hands out. The daemon holds the prototype's projects, three cards, and one chat.
 * The chats are the daemon's too, or their rows would be left out (they are a section of their own).
 */
const host = await vi.hoisted(async () => {
  window.location.hash = "#nosim";
  const { sectionStatus } = await import("~/data/sections");
  const { wireCard } = await import("~/testing/fake-cards");
  const { wireChat } = await import("~/testing/fake-chats");
  const { createFakeDaemon } = await import("~/testing/fake-daemon");
  const { PROTOTYPE_PROJECTS } = await import("~/testing/projects");
  const { createTestMarshal } = await import("~/testing/test-store");
  const daemon = createFakeDaemon({
    projects: PROTOTYPE_PROJECTS,
    cards: [
      wireCard({
        projectId: "api",
        number: 41,
        title: "Fix token refresh on login",
        state: "working",
      }),
      wireCard({
        projectId: "api",
        number: 43,
        title: "Add rate limiting per API key",
        state: "needs",
      }),
      wireCard({
        projectId: "web",
        number: 7,
        title: "Show the session timeout",
        state: "backlog",
      }),
    ],
    chats: [
      wireChat({
        id: "01M3CHAT00000000000000000A",
        projectId: "api",
        title: "Token rotation question",
      }),
    ],
  });
  window.M = createTestMarshal({
    data: daemon.data,
    sections: { ...sectionStatus, S5a: "daemon", S17: "daemon", S24a: "daemon" },
  });
  return { daemon };
});

const { daemon } = host;

const DESKTOP_PX = 1440;
const HEIGHT_PX = 900;
const SEARCH = "Search actions, projects, cards, and settings";
const TOKEN_CHAT = "Token rotation question";

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
  daemon.start();
});

const input = (): HTMLInputElement => screen.getByRole("textbox", { name: SEARCH });
/* Plain DOM queries: a role query with 60 rows is slow in jsdom. */
const options = (): HTMLElement[] => [...document.querySelectorAll<HTMLElement>('[role="option"]')];
const labels = (): string[] => options().map((o) => o.textContent ?? "");
const has = (text: string): boolean => labels().some((label) => label.includes(text));
const selected = (): HTMLElement | undefined =>
  options().find((o) => o.getAttribute("aria-selected") === "true");
const type = (text: string): void => {
  fireEvent.input(input(), { target: { value: text } });
};
const searches = (): string[] => daemon.routes().filter((r) => r.startsWith("GET /v1/search"));

/** Lets typing pause long enough for the search to be sent, and its answer to come back. */
async function answered(): Promise<void> {
  await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
  await vi.advanceTimersByTimeAsync(0);
}

describe("CommandPalette with the daemon", () => {
  it("lists what the daemon found as projects, cards, and chats, once typing pauses", async () => {
    render(() => <CommandPalette />);
    const before = searches().length;
    type("token");
    expect(searches()).toHaveLength(before);
    await answered();
    await vi.waitFor(() => expect(has(TOKEN_CHAT)).toBe(true));
    expect(searches().slice(before)).toEqual(["GET /v1/search?q=token"]);
    expect(labels()).toEqual([
      expect.stringContaining("api-gateway #41 Fix token refresh on login"),
      expect.stringContaining(TOKEN_CHAT),
    ]);
    expect(screen.getByText("Cards")).toBeInTheDocument();
    expect(screen.getByText("Chats")).toBeInTheDocument();
    // A chat row names its project where a project row shows its language.
    expect(options()[1]).toHaveTextContent("api-gateway");
  });

  it("shows the same rows as the mock while the answer is on its way", async () => {
    render(() => <CommandPalette />);
    type("rate limiting");
    expect(labels()).toEqual([
      expect.stringContaining("api-gateway #43 Add rate limiting per API key"),
    ]);
    expect(options()[0]).toHaveAttribute("data-pi", "0");
    await answered();
    await vi.waitFor(() => expect(searches().at(-1)).toBe("GET /v1/search?q=rate+limiting"));
    expect(has("api-gateway #43 Add rate limiting per API key")).toBe(true);
  });

  it("sends nothing while the field is empty, and lists the palette's own commands", async () => {
    render(() => <CommandPalette />);
    const before = searches().length;
    type("rate");
    type("");
    await answered();
    expect(searches()).toHaveLength(before);
    expect(has("New card")).toBe(true);
    expect(screen.getByText("Actions")).toBeInTheDocument();
  });

  it("keeps the palette's actions and settings from the client while the daemon fills the rest", async () => {
    render(() => <CommandPalette />);
    type("theme");
    await answered();
    expect(has("Use dark theme")).toBe(true);
    type("go home");
    await answered();
    expect(labels()).toEqual(["Go home"]);
  });

  it("opens a card the daemon found that the store does not hold, on Enter", async () => {
    daemon.cards.push({
      ...daemon.cards[0]!,
      id: "01M3CARD000000000000000060",
      key: "api#60",
      number: 60,
      title: "Made elsewhere",
    });
    render(() => <CommandPalette />);
    type("elsewhere");
    await answered();
    await vi.waitFor(() => expect(has("api-gateway #60 Made elsewhere")).toBe(true));
    fireEvent.keyDown(input(), { key: "Enter" });
    expect(M.S.palette).toBe(false);
    await vi.waitFor(() => expect(M.S.openId).toBe("api#60"));
    expect(daemon.routes()).toContain("GET /v1/cards/01M3CARD000000000000000060");
  });

  it("opens a chat in its project's chat view when its row is clicked", async () => {
    M.go("home");
    render(() => <CommandPalette />);
    type("rotation");
    await answered();
    await vi.waitFor(() => expect(has(TOKEN_CHAT)).toBe(true));
    fireEvent.click(screen.getByText(TOKEN_CHAT));
    expect(M.S.palette).toBe(false);
    await vi.waitFor(() => expect(M.S.chatOpen.api).toBe("01M3CHAT00000000000000000A"));
    expect(M.S.route).toMatchObject({ page: "project", pid: "api", view: "chat" });
  });

  it("opens a project the daemon found on its page", async () => {
    M.go("home");
    render(() => <CommandPalette />);
    type("dashboard");
    await answered();
    await vi.waitFor(() => expect(has("web-dashboard")).toBe(true));
    fireEvent.click(screen.getByText("web-dashboard"));
    await vi.waitFor(() => expect(M.S.route).toMatchObject({ page: "project", pid: "web" }));
  });

  it("switches project from the keyboard: type, arrow to the project's row, and Enter", async () => {
    M.go("home");
    render(() => <CommandPalette />);
    type("dashboard");
    await answered();
    await vi.waitFor(() => expect(has("web-dashboard")).toBe(true));
    // The rows are the matching actions first (none match), then the daemon's projects.
    expect(options()[0]).toHaveTextContent("web-dashboard");
    expect(options()[0]).toHaveTextContent("TypeScript");
    fireEvent.keyDown(input(), { key: "Enter" });
    expect(M.S.palette).toBe(false);
    await vi.waitFor(() => expect(M.S.route).toMatchObject({ page: "project", pid: "web" }));
  });

  it("arrows down to a project row that is not first, and opens that one", async () => {
    M.go("home");
    render(() => <CommandPalette />);
    type("a");
    await answered();
    await vi.waitFor(() => expect(searches().at(-1)).toBe("GET /v1/search?q=a"));
    const at = options().findIndex((o) => o.textContent?.startsWith("web-dashboard"));
    expect(at).toBeGreaterThan(0);
    for (let i = 0; i < at; i++) fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(options()[at]).toHaveAttribute("aria-selected", "true");
    fireEvent.keyDown(input(), { key: "Enter" });
    await vi.waitFor(() => expect(M.S.route).toMatchObject({ page: "project", pid: "web" }));
  });

  it("falls back to what the store holds when the search fails, and Enter still opens the row", async () => {
    daemon.refuseNext("GET /v1/search?q=rate", 500, "internal", "Marshal hit a problem.");
    render(() => <CommandPalette />);
    type("rate");
    await answered();
    expect(labels()).toEqual([
      expect.stringContaining("api-gateway #43 Add rate limiting per API key"),
    ]);
    fireEvent.keyDown(input(), { key: "Enter" });
    expect(M.S.openId).toBe("api#43");
  });

  it("falls back to what the store holds when the daemon does not answer", async () => {
    daemon.stop();
    render(() => <CommandPalette />);
    type("timeout");
    await answered();
    expect(labels()).toEqual([
      expect.stringContaining("web-dashboard #7 Show the session timeout"),
    ]);
    type("zzzz");
    await answered();
    expect(screen.getByText('No results for "zzzz".')).toBeInTheDocument();
  });

  it("moves the selection with the arrow keys over the daemon's rows, and starts from the top when they land", async () => {
    render(() => <CommandPalette />);
    type("api");
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(selected()).toBe(options()[2]);
    await answered();
    await vi.waitFor(() => expect(has("api-gateway")).toBe(true));
    expect(selected()).toBe(options()[0]);
    fireEvent.keyDown(input(), { key: "ArrowDown" });
    expect(selected()).toBe(options()[1]);
    fireEvent.keyDown(input(), { key: "ArrowUp" });
    fireEvent.keyDown(input(), { key: "ArrowUp" });
    expect(selected()).toBe(options()[0]);
  });

  it("does not send a search that was typed just before the palette closed", async () => {
    render(() => <CommandPalette />);
    const before = searches().length;
    type("token");
    M.set({ palette: false });
    await answered();
    expect(searches()).toHaveLength(before);
  });

  it("starts a new search session every time it opens", async () => {
    render(() => <CommandPalette />);
    type("token");
    await answered();
    await vi.waitFor(() => expect(has(TOKEN_CHAT)).toBe(true));
    M.set({ palette: false });
    M.set({ palette: true });
    expect(input()).toHaveValue("");
    expect(has(TOKEN_CHAT)).toBe(false);
    expect(has("New card")).toBe(true);
  });
});
