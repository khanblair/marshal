// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the cards are the daemon's).
import { daemon } from "~/testing/daemon-cards-store";
import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toasts } from "~/app/toasts/Toasts";
import { M } from "~/mock";
import type { Card } from "~/mock";
import { createGoogleFilesStore } from "~/testing/fake-google-files";
import { createIntegrationStore } from "~/testing/fake-integrations";
import { ViewHeader } from "~/app/ViewHeader";
import { moreItems } from "~/views/card/card-actions";
import { CardActions } from "~/views/card/CardActions";
import { createPanelState } from "~/views/card/panel-state";
import {
  exportBoardToSheet,
  exportBoardToSlides,
  exportCardToDoc,
  exportKey,
  saveNoteToDrive,
} from "./export-actions";
import { isExporting } from "./export-run";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

// The five buttons that make or read a Google file, against the fake daemon: the real store, client
// and routes. Only the tab that Google's own page opens in (`window.open`) is replaced.

const OPEN = "api#41";
const NOT_CONNECTED = (name: string) => `Connect ${name} in Settings, under Integrations.`;
const DAEMON_REFUSES = (name: string) =>
  `${name} is not connected yet. Connect it in Settings, under Integrations.`;

const card = (): Card => {
  const found = M.card(OPEN);
  if (!found) throw new Error("the store has no api#41");
  return found;
};

/** Gives the card a note the way opening it does: the body, and when it was last saved. */
function giveNote(body: string, saved = true): void {
  M.S.notes = { ...M.S.notes, [OPEN]: body };
  M.S.noteInfo = {
    ...M.S.noteInfo,
    [OPEN]: { path: "api/cards/41-x.md", author: "person", updatedAt: saved ? Date.now() : null },
  };
}

/** Puts the daemon and the store back to the state they start in, so one test cannot see another's. */
async function reset(): Promise<void> {
  Object.assign(daemon.integrations, createIntegrationStore());
  Object.assign(daemon.googleFiles, createGoogleFilesStore());
  delete M.S.notes;
  delete M.S.noteInfo;
  await M.refreshIntegrationList();
  daemon.calls.length = 0;
  M.set({ toasts: [] });
}

/** Changes what the store reads for one connection, as a list read from the daemon would. */
function setState(id: string, st: "connected" | "error"): void {
  M.set({
    integrations: M.S.integrations.map((entry) => (entry.id === id ? { ...entry, st } : entry)),
  });
}

/** Makes one connection connected on the daemon, and lets the store read the list again. */
async function connect(...ids: string[]): Promise<void> {
  for (const row of daemon.integrations.rows) if (ids.includes(row.id)) row.st = "connected";
  await M.refreshIntegrationList();
}

const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);
const count = (route: string): number => daemon.routes().filter((entry) => entry === route).length;

beforeEach(reset);
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  M.setViewport(1440, 900);
});

describe("Export to Google Doc", () => {
  it("sends the card as a document, says it is saved, and opens the file from the toast's click", async () => {
    const tab = { opener: {} };
    const open = vi.spyOn(window, "open").mockReturnValue(tab as unknown as Window);
    await connect("gdocs");
    giveNote("# Plan\n\nMy <words>");
    render(() => <Toasts />);
    expect(await exportCardToDoc(card())).toBe(true);
    const [body] = daemon.bodies("POST /v1/google/docs") as { title: string; html: string }[];
    expect(body?.title).toBe(card().title);
    expect(body?.html).toContain(`<h1>${card().title}</h1>`);
    expect(body?.html).toContain("<strong>Project:</strong> api-gateway");
    expect(body?.html).toContain("<h2>Note</h2>");
    expect(body?.html).toContain("My &lt;words&gt;");
    expect(toasts()).toEqual(["Saved to Google Docs"]);
    // The tab opens on the click, not after the request.
    expect(open).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: "Open" }));
    expect(open).toHaveBeenCalledWith("https://docs.google.com/document/d/doc-1/edit", "_blank");
    expect(M.S.toasts).toEqual([]);
  });

  it("leaves the note out when nothing was saved for the card, and waits while it is still read", async () => {
    await connect("gdocs");
    expect(await exportCardToDoc(card())).toBe(false);
    expect(toasts()).toEqual(["The note is still loading. Try again in a moment."]);
    expect(count("POST /v1/google/docs")).toBe(0);
    giveNote("A starter note", false);
    expect(await exportCardToDoc(card())).toBe(true);
    const [body] = daemon.bodies("POST /v1/google/docs") as { html: string }[];
    expect(body?.html).not.toContain("<h2>Note</h2>");
    expect(body?.html).not.toContain("starter");
  });
});

describe("Save note to Google Drive", () => {
  it("sends the note as a markdown file named after the card", async () => {
    await connect("gdrive");
    giveNote("# Plan\n\n- one");
    expect(await saveNoteToDrive(card())).toBe(true);
    expect(daemon.bodies("POST /v1/google/drive/files")).toEqual([
      { name: `${card().title}.md`, content: "# Plan\n\n- one", mimeType: "text/markdown" },
    ]);
    expect(toasts()).toEqual(["Saved to Google Drive"]);
    expect(daemon.googleFiles.files[0]).toMatchObject({ kind: "file" });
  });

  it("says so, and asks for nothing, when the card has no note", async () => {
    await connect("gdrive");
    giveNote("A starter note", false);
    expect(await saveNoteToDrive(card())).toBe(false);
    giveNote("   ");
    expect(await saveNoteToDrive(card())).toBe(false);
    expect(toasts()).toEqual(["This card has no note yet.", "This card has no note yet."]);
    expect(count("POST /v1/google/drive/files")).toBe(0);
  });
});

describe("Export board to Google Sheet", () => {
  it("sends a header and a row for each card of the board", async () => {
    await connect("gsheets");
    expect(await exportBoardToSheet("api")).toBe(true);
    const [body] = daemon.bodies("POST /v1/google/sheets") as { title: string; rows: string[][] }[];
    expect(body?.title).toBe("api-gateway board");
    expect(body?.rows[0]).toEqual(expect.arrayContaining(["Card", "Title", "Column", "State"]));
    expect(body?.rows).toHaveLength(1 + M.cardsOf("api").length);
    const mine = body?.rows.find((row) => row[0] === "#41");
    expect(mine?.slice(0, 4)).toEqual(["#41", card().title, "Working", "Working"]);
    expect(mine).toHaveLength(body?.rows[0]?.length ?? 0);
    expect(toasts()).toEqual(["Saved to Google Sheets"]);
  });
});

describe("Export board to Google Slides", () => {
  it("sends a title slide, then a slide for each column", async () => {
    await connect("gslides");
    expect(await exportBoardToSlides("api")).toBe(true);
    const [body] = daemon.bodies("POST /v1/google/slides") as {
      title: string;
      slides: { title: string; bullets: string[] }[];
    }[];
    expect(body?.title).toBe("api-gateway board");
    expect(body?.slides).toHaveLength(1 + M.COLUMNS.length);
    expect(body?.slides[0]).toMatchObject({ title: "api-gateway" });
    expect(body?.slides[0]?.bullets[0]).toBe(`${M.cardsOf("api").length} cards`);
    const working = body?.slides.find((slide) => slide.title.startsWith("Working ("));
    expect(working?.bullets).toContain(card().title);
    expect(body?.slides.map((slide) => slide.title.replace(/ \(\d+\)$/, "")).slice(1)).toEqual(
      M.COLUMNS.map((col) => M.STATUS[col].label),
    );
    expect(toasts()).toEqual(["Saved to Google Slides"]);
  });
});

describe("a connection that is not ready", () => {
  const actions: [string, string, () => Promise<boolean>, string][] = [
    ["Export to Google Doc", "Google Docs", () => exportCardToDoc(card()), "POST /v1/google/docs"],
    [
      "Save note to Google Drive",
      "Google Drive",
      () => saveNoteToDrive(card()),
      "POST /v1/google/drive/files",
    ],
    [
      "Export board to Google Sheet",
      "Google Sheets",
      () => exportBoardToSheet("api"),
      "POST /v1/google/sheets",
    ],
    [
      "Export board to Google Slides",
      "Google Slides",
      () => exportBoardToSlides("api"),
      "POST /v1/google/slides",
    ],
  ];

  it.each(actions)(
    "%s says to connect %s in Settings, and asks the daemon for nothing",
    async (_label, name, run, route) => {
      giveNote("A note");
      expect(await run()).toBe(false);
      expect(toasts()).toEqual([NOT_CONNECTED(name)]);
      expect(count(route)).toBe(0);
      expect(isExporting(exportKey("doc", OPEN))).toBe(false);
    },
  );

  it("says to reconnect when Google no longer accepts the access", async () => {
    setState("gdocs", "error");
    expect(await exportCardToDoc(card())).toBe(false);
    expect(toasts()).toEqual(["Reconnect Google Docs in Settings, under Integrations."]);
    expect(count("POST /v1/google/docs")).toBe(0);
  });

  it("shows the daemon's own sentence when it refuses, and can be asked again", async () => {
    // The store thinks Docs is connected while the daemon does not, as after a stale read.
    setState("gdocs", "connected");
    giveNote("A note");
    expect(await exportCardToDoc(card())).toBe(false);
    expect(toasts()).toEqual([DAEMON_REFUSES("Google Docs")]);
    expect(isExporting(exportKey("doc", OPEN))).toBe(false);
    expect(await exportCardToDoc(card())).toBe(false);
    expect(count("POST /v1/google/docs")).toBe(2);
  });
});

describe("pressing twice", () => {
  it("makes one file, and shows the item as working until the answer is back", async () => {
    await connect("gdocs");
    giveNote("A note");
    const find = () =>
      moreItems(card(), () => undefined).find((item) => item.label === "Export to Google Doc");
    expect(find()).toMatchObject({ disabled: false, hint: undefined });
    const first = exportCardToDoc(card());
    const second = exportCardToDoc(card());
    expect(find()).toMatchObject({ disabled: true, hint: "Working…" });
    expect(await Promise.all([first, second])).toEqual([true, false]);
    expect(count("POST /v1/google/docs")).toBe(1);
    expect(find()).toMatchObject({ disabled: false, hint: undefined });
    expect(toasts()).toEqual(["Saved to Google Docs"]);
  });

  it("keeps two boards apart, so one export does not hold the other", async () => {
    await connect("gsheets");
    const [a, b] = await Promise.all([exportBoardToSheet("api"), exportBoardToSheet("web")]);
    expect([a, b]).toEqual([true, true]);
    expect(count("POST /v1/google/sheets")).toBe(2);
  });
});

describe("from the buttons a person presses", () => {
  const pressMore = (name: string) => {
    fireEvent.click(screen.getByRole("button", { name }));
  };

  it("makes the document and the file from the card's More menu", async () => {
    await connect("gdocs", "gdrive");
    giveNote("# Plan");
    const panel = createPanelState();
    render(() => <CardActions card={card()} panel={panel} />);
    pressMore("More actions");
    fireEvent.click(screen.getByRole("menuitem", { name: "Export to Google Doc" }));
    await vi.waitFor(() => expect(count("POST /v1/google/docs")).toBe(1));
    await vi.waitFor(() => expect(toasts()).toEqual(["Saved to Google Docs"]));
    pressMore("More actions");
    fireEvent.click(screen.getByRole("menuitem", { name: "Save note to Google Drive" }));
    await vi.waitFor(() => expect(count("POST /v1/google/drive/files")).toBe(1));
    await vi.waitFor(() => expect(toasts()).toContain("Saved to Google Drive"));
  });

  it.each(["board", "list", "timeline"] as const)(
    "makes the sheet from the project's More menu in the %s view, and holds the button while it runs",
    async (view) => {
      await connect("gsheets");
      M.setViewport(1440, 900);
      M.go("project", "api", view);
      render(() => <ViewHeader />);
      pressMore("More project actions");
      fireEvent.click(screen.getByRole("menuitem", { name: "Export board to Google Sheet" }));
      pressMore("More project actions");
      expect(screen.getByRole("menuitem", { name: "Export board to Google Sheet" })).toBeDisabled();
      expect(screen.getByRole("menuitem", { name: "Export board to Google Slides" })).toBeEnabled();
      await vi.waitFor(() => expect(toasts()).toEqual(["Saved to Google Sheets"]));
      expect(count("POST /v1/google/sheets")).toBe(1);
      expect(screen.getByRole("menuitem", { name: "Export board to Google Sheet" })).toBeEnabled();
    },
  );

  it("works on a phone's card too, since the daemon does the work", async () => {
    M.setViewport(390, 800);
    await connect("gdocs");
    giveNote("# Plan");
    const panel = createPanelState();
    render(() => <CardActions card={card()} panel={panel} />);
    pressMore("More actions");
    fireEvent.click(screen.getByRole("menuitem", { name: "Export to Google Doc" }));
    await vi.waitFor(() => expect(toasts()).toEqual(["Saved to Google Docs"]));
  });
});
