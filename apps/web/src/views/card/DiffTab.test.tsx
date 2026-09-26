// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (S5a and S11 are the daemon's).
import { daemon } from "~/testing/daemon-cards-store";
import type { ChangedFile, DiffHunk } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { CardPanel } from "./CardPanel";
import { DiffTab } from "./DiffTab";
import { createDiffList } from "./diff-list";
import { createPanelState } from "./panel-state";
import { createTerminalState } from "./terminal-state";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const FILES = 2000;
const VIEWPORT_PX = 600;
/** The rows a screen of this height and its overscan can hold, with room to spare: far below FILES. */
const MOST_ROWS = 60;

const card = () => cardOf("api#41");
const diffRoute = () => `GET /v1/cards/${card().daemonId}/diff`;
/** How many calls the daemon had had when the test began: it is one daemon for the whole file. */
let start = 0;
const hunkCalls = (from = start) =>
  daemon
    .routes()
    .slice(from)
    .filter((route) => route.includes("/diff/"));

const pathOf = (i: number) => `src/file-${String(i).padStart(4, "0")}.go`;
const changed = (path: string, large = false): ChangedFile => ({
  path,
  oldPath: "",
  status: "modified",
  additions: 3,
  deletions: 1,
  binary: false,
  large,
});
const hunk = (text: string): DiffHunk => ({
  header: "@@ -1,1 +1,2 @@",
  lines: [{ kind: "added", oldLine: 0, newLine: 1, text }],
});

/** Gives the daemon this card's diff: `count` files, and hunks for the ones a test names. */
function setDiff(count: number, hunks: Record<string, DiffHunk[]> = {}, larger: string[] = []) {
  const files = Array.from({ length: count }, (_, i) =>
    changed(pathOf(i), larger.includes(pathOf(i))),
  );
  for (const key of Object.keys(daemon.diffs)) delete daemon.diffs[key];
  daemon.diffs[card().daemonId ?? ""] = { files, hunks };
}

beforeEach(() => {
  resetStore();
  start = daemon.routes().length;
  Object.defineProperty(HTMLElement.prototype, "clientHeight", {
    configurable: true,
    get: () => VIEWPORT_PX,
  });
});
afterEach(() => {
  cleanup();
  Reflect.deleteProperty(HTMLElement.prototype, "clientHeight");
});

function Harness(props: { panel: ReturnType<typeof createPanelState> }) {
  const list = createDiffList(card);
  return <DiffTab card={card()} panel={props.panel} list={list} />;
}
const show = () => {
  const panel = createPanelState();
  return render(() => <Harness panel={panel} />);
};
const diffListCalls = () =>
  daemon
    .routes()
    .slice(start)
    .filter((route) => route === diffRoute());
const rowCount = () => screen.queryAllByRole("listitem").length;
const scroller = () =>
  screen.getByRole("list", { name: "Changed files" }).parentElement as HTMLElement;
const headerOf = (path: string) => screen.getByText(path).closest("button") as HTMLButtonElement;

describe("DiffTab on the daemon", () => {
  it("draws far fewer rows than a diff of two thousand files has", async () => {
    setDiff(FILES);
    show();
    expect(await screen.findByText(`${FILES} files changed`)).toBeInTheDocument();
    expect(rowCount()).toBeGreaterThan(3);
    expect(rowCount()).toBeLessThan(MOST_ROWS);
    expect(screen.getAllByRole("button", { expanded: false }).length).toBeLessThan(MOST_ROWS);
    expect(screen.queryByText(pathOf(FILES - 1))).toBeNull();
    // The list is as tall as the files it has, so the scroll bar is the diff's.
    const list = screen.getByRole("list", { name: "Changed files" });
    expect(Number.parseFloat(list.style.height)).toBeGreaterThan(FILES * 38);
  });

  it("draws the files at the end of the list once it is scrolled there", async () => {
    setDiff(FILES);
    show();
    await screen.findByText(`${FILES} files changed`);
    scroller().scrollTop = 1_000_000;
    fireEvent.scroll(scroller());
    expect(screen.getByText(pathOf(FILES - 1))).toBeInTheDocument();
    expect(screen.queryByText(pathOf(0))).toBeNull();
    expect(rowCount()).toBeLessThan(MOST_ROWS);
    // Each row says which file of how many it is.
    const last = screen.getByText(pathOf(FILES - 1)).closest("li");
    expect(last).toHaveAttribute("aria-posinset", String(FILES));
    expect(last).toHaveAttribute("aria-setsize", String(FILES));
  });

  it("loads a file's hunks when it is opened, once, and no other file's", async () => {
    setDiff(FILES, { [pathOf(5)]: [hunk("func opened() {}")] });
    show();
    await screen.findByText(`${FILES} files changed`);
    await waitFor(() => expect(hunkCalls().length).toBeGreaterThanOrEqual(2));
    const before = daemon.routes().length;
    expect(screen.queryByText("func opened() {}")).toBeNull();
    fireEvent.click(headerOf(pathOf(5)));
    expect(await screen.findByText("func opened() {}")).toBeInTheDocument();
    const id = card().daemonId;
    expect(hunkCalls(before)).toEqual([`GET /v1/cards/${id}/diff/${pathOf(5)}`]);
    // Closing it and opening it again keeps what was fetched.
    fireEvent.click(headerOf(pathOf(5)));
    fireEvent.click(headerOf(pathOf(5)));
    expect(screen.getByText("func opened() {}")).toBeInTheDocument();
    expect(hunkCalls(before)).toHaveLength(1);
  });

  it("loads the first two small files' hunks as the mock's were always there", async () => {
    setDiff(FILES, { [pathOf(0)]: [hunk("func first() {}")] });
    show();
    expect(await screen.findByText("func first() {}")).toBeInTheDocument();
    expect(headerOf(pathOf(0))).toHaveAttribute("aria-expanded", "true");
    expect(headerOf(pathOf(2))).toHaveAttribute("aria-expanded", "false");
  });

  it("asks for the files it draws, not all of them, when every file is expanded", async () => {
    setDiff(FILES);
    show();
    await screen.findByText(`${FILES} files changed`);
    await waitFor(() => expect(hunkCalls().length).toBeGreaterThanOrEqual(2));
    const before = daemon.routes().length;
    fireEvent.click(screen.getByRole("button", { name: "Expand all" }));
    await waitFor(() => expect(hunkCalls(before).length).toBeGreaterThan(0));
    await new Promise((resolve) => setTimeout(resolve, 50));
    const asked = hunkCalls(before);
    expect(asked.length).toBeLessThan(MOST_ROWS);
    expect(new Set(asked).size).toBe(asked.length);
    expect(screen.getByRole("button", { name: "Collapse all" })).toBeInTheDocument();
  });

  it("keeps a large file collapsed until Load diff is pressed", async () => {
    setDiff(FILES, { [pathOf(4)]: [hunk("sum line")] }, [pathOf(4)]);
    show();
    await screen.findByText(`${FILES} files changed`);
    const before = daemon.routes().length;
    fireEvent.click(headerOf(pathOf(4)));
    expect(screen.getByText(/Large file: 3 changed lines/)).toBeInTheDocument();
    expect(hunkCalls(before)).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Load diff" }));
    // The daemon's own lines, not a stand-in.
    expect(await screen.findByText("sum line")).toBeInTheDocument();
    expect(hunkCalls(before)).toEqual([`GET /v1/cards/${card().daemonId}/diff/${pathOf(4)}`]);
  });

  it("keeps the header button drawn, with its focus, while it is scrolled out of view", async () => {
    setDiff(FILES);
    show();
    await screen.findByText(`${FILES} files changed`);
    const button = headerOf(pathOf(3));
    button.focus();
    scroller().scrollTop = 1_000_000;
    fireEvent.scroll(scroller());
    expect(document.activeElement).toBe(button);
    expect(screen.getByText(pathOf(3)).closest("button")).toBe(button);
    button.blur();
    expect(screen.queryByText(pathOf(3))).toBeNull();
  });

  it("toggles a file from the keyboard, since its header is a button", async () => {
    setDiff(3);
    show();
    await screen.findByText("3 files changed");
    const button = headerOf(pathOf(2));
    expect(button).toHaveAttribute("aria-expanded", "false");
    button.focus();
    fireEvent.click(button);
    expect(headerOf(pathOf(2))).toHaveAttribute("aria-expanded", "true");
    expect(document.activeElement).toBe(button);
  });

  it("shows the counts of every file in the summary", async () => {
    setDiff(3);
    show();
    expect(await screen.findByText("3 files changed")).toBeInTheDocument();
    const summary = screen.getByText("3 files changed").parentElement as HTMLElement;
    expect(within(summary).getByText("+9")).toBeInTheDocument();
  });

  it("says there are no changes yet, once the daemon answers with none", async () => {
    setDiff(0);
    show();
    expect(await screen.findByText(/No changes yet/)).toBeInTheDocument();
    expect(screen.queryByRole("list")).toBeNull();
    expect(screen.queryByRole("button", { name: "Expand all" })).toBeNull();
  });

  it("shows a loading state while the daemon has not answered", async () => {
    setDiff(3);
    const release = daemon.holdNext(diffRoute());
    const { container } = show();
    expect(screen.getByRole("status")).toHaveTextContent("Loading changes");
    expect(container.firstElementChild).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByRole("list")).toBeNull();
    expect(screen.queryByText(/No changes yet/)).toBeNull();
    release();
    expect(await screen.findByText("3 files changed")).toBeInTheDocument();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("shows the daemon's own sentence when the list cannot be read, and tries again", async () => {
    setDiff(3);
    daemon.refuseNext(diffRoute(), 500, "internal", "Marshal ran into a problem. Try again.");
    show();
    expect(await screen.findByText("Marshal ran into a problem. Try again.")).toBeInTheDocument();
    expect(screen.queryByText(/No changes yet/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("3 files changed")).toBeInTheDocument();
    expect(screen.queryByText("Marshal ran into a problem. Try again.")).toBeNull();
  });

  it("tells the person when a file's hunks cannot be read, and leaves the file as it was", async () => {
    setDiff(3);
    daemon.refuseNext(
      `GET /v1/cards/${card().daemonId}/diff/${pathOf(2)}`,
      500,
      "internal",
      "Marshal ran into a problem. Try again.",
    );
    show();
    await screen.findByText("3 files changed");
    const before = daemon.routes().length;
    fireEvent.click(headerOf(pathOf(2)));
    // The toast region is the app shell's, so the toast is looked for in the store.
    await waitFor(() =>
      expect(M.S.toasts.map((toast) => toast.msg)).toEqual([
        "Marshal ran into a problem. Try again.",
      ]),
    );
    // Opening it again asks again.
    fireEvent.click(headerOf(pathOf(2)));
    fireEvent.click(headerOf(pathOf(2)));
    await waitFor(() =>
      expect(hunkCalls(before).filter((route) => route.endsWith(pathOf(2)))).toHaveLength(2),
    );
  });

  it("shows no made-up lines for a large file the daemon has no hunks for", async () => {
    setDiff(3, {}, [pathOf(2)]);
    show();
    await screen.findByText("3 files changed");
    fireEvent.click(headerOf(pathOf(2)));
    fireEvent.click(screen.getByRole("button", { name: "Load diff" }));
    await waitFor(() =>
      expect(hunkCalls()).toContain(`GET /v1/cards/${card().daemonId}/diff/${pathOf(2)}`),
    );
    await waitFor(() => expect(screen.queryByText(/Large file:/)).toBeNull());
    expect(screen.queryByText(/cloud\.google\.com/)).toBeNull();
    expect(screen.queryByText("...")).toBeNull();
  });

  it("does not ask for the list again when the card changes, only for another card", async () => {
    setDiff(3);
    show();
    await screen.findByText("3 files changed");
    expect(diffListCalls()).toHaveLength(1);
    const wire = daemon.cards.find((one) => one.id === card().daemonId);
    if (!wire) throw new Error("the daemon does not hold api#41");
    daemon.emit(`project:${wire.projectId}`, "card.updated", {
      card: { ...wire, title: "Renamed while the diff is open" },
    });
    await waitFor(() => expect(card().title).toBe("Renamed while the diff is open"));
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(diffListCalls()).toHaveLength(1);
  });

  it("counts the daemon's files on the Diff tab, not the mock's, and asks again when the tab opens", async () => {
    setDiff(7);
    render(() => <CardPanel card={card()} terminal={createTerminalState()} />);
    const tab = () => screen.getByRole("tab", { name: /^Diff/ });
    await waitFor(() => expect(within(tab()).getByText("7")).toBeInTheDocument());
    expect(diffListCalls()).toHaveLength(1);
    fireEvent.click(tab());
    expect(await screen.findByText("7 files changed")).toBeInTheDocument();
    await waitFor(() => expect(diffListCalls()).toHaveLength(2));
    expect(within(tab()).getByText("7")).toBeInTheDocument();
  });

  it("shows no count on the Diff tab of a card with no changes, whatever the mock has for it", async () => {
    setDiff(0);
    render(() => <CardPanel card={card()} terminal={createTerminalState()} />);
    await waitFor(() => expect(diffListCalls()).toHaveLength(1));
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(within(screen.getByRole("tab", { name: /^Diff/ })).queryByText(/\d/)).toBeNull();
  });

  // Last, since it stops the shared daemon, which the store notices and tries to reconnect to.
  it("says the daemon cannot be reached when it is off, and shows the list once it is back", async () => {
    setDiff(3);
    daemon.stop();
    try {
      show();
      expect(await screen.findByText(/can't reach the daemon/i)).toBeInTheDocument();
      expect(screen.queryByText(/No changes yet/)).toBeNull();
    } finally {
      daemon.start();
    }
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("3 files changed")).toBeInTheDocument();
  });
});
