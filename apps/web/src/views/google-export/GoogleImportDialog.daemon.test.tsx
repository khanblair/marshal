// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the cards are the daemon's).
import { daemon } from "~/testing/daemon-cards-store";
import type { GoogleLinkContent } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import type { Card } from "~/mock";
import { createGoogleFilesStore } from "~/testing/fake-google-files";
import { createIntegrationStore } from "~/testing/fake-integrations";
import { CardActions } from "~/views/card/CardActions";
import { createPanelState, type Panel } from "~/views/card/panel-state";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

// The Import from Google link dialog, opened from the card's More menu, against the fake daemon.

const OPEN = "api#41";
const DOC_LINK = "https://docs.google.com/document/d/abc123/edit";
const NOTE_ROUTE = (): string => `PUT /v1/cards/${card().daemonId}/note`;

const card = (): Card => {
  const found = M.card(OPEN);
  if (!found) throw new Error("the store has no api#41");
  return found;
};

function giveNote(body: string): void {
  M.S.notes = { ...M.S.notes, [OPEN]: body };
  M.S.noteInfo = {
    ...M.S.noteInfo,
    [OPEN]: { path: "api/cards/41-x.md", author: "person", updatedAt: Date.now() },
  };
}

async function reset(): Promise<void> {
  Object.assign(daemon.integrations, createIntegrationStore());
  Object.assign(daemon.googleFiles, createGoogleFilesStore());
  delete M.S.notes;
  delete M.S.noteInfo;
  await M.refreshIntegrationList();
  daemon.calls.length = 0;
  M.set({ toasts: [] });
}

async function connect(...ids: string[]): Promise<void> {
  for (const row of daemon.integrations.rows) if (ids.includes(row.id)) row.st = "connected";
  await M.refreshIntegrationList();
}

let panel: Panel;
beforeEach(async () => {
  await reset();
  panel = createPanelState();
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  M.setViewport(1440, 900);
});

const count = (route: string): number => daemon.routes().filter((entry) => entry === route).length;
const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);

/** Draws the card's actions and opens the dialog the way a person does: More, then the item. */
function openDialog(): HTMLElement {
  render(() => <CardActions card={card()} panel={panel} />);
  fireEvent.click(screen.getByRole("button", { name: "More actions" }));
  fireEvent.click(screen.getByRole("menuitem", { name: "Import from Google link…" }));
  return screen.getByRole("dialog", { name: "Import from Google link" });
}

const field = (): HTMLInputElement => screen.getByLabelText("Google link") as HTMLInputElement;
const type = (text: string): void => {
  fireEvent.input(field(), { target: { value: text } });
};
const read = (): void => {
  fireEvent.click(screen.getByRole("button", { name: "Read" }));
};

/** Pastes a link and reads it, and waits for the preview. */
async function readLink(link = DOC_LINK): Promise<HTMLElement> {
  type(link);
  read();
  return screen.findByRole("region", { name: "What was read" });
}

describe("opening the dialog", () => {
  it("opens from the More menu with nothing to read or add yet, and closes the menu", () => {
    const dialog = openDialog();
    expect(panel.state.more).toBe(false);
    expect(field()).toHaveValue("");
    expect(within(dialog).getByRole("button", { name: "Read" })).toBeDisabled();
    expect(within(dialog).getByRole("button", { name: "Add to card note" })).toBeDisabled();
    expect(within(dialog).queryByRole("alert")).toBeNull();
  });

  it("closes on Cancel, and a new opening starts with an empty field", async () => {
    await connect("gdocs");
    openDialog();
    type(DOC_LINK);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog", { name: "Import from Google link" })).toBeNull();
    expect(panel.state.importOpen).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "More actions" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Import from Google link…" }));
    expect(field()).toHaveValue("");
  });
});

describe("reading a link", () => {
  it("shows what the file is called, its kind and a preview of its markdown", async () => {
    await connect("gdocs");
    giveNote("My note");
    openDialog();
    const preview = await readLink();
    expect(within(preview).getByRole("heading", { name: "Launch plan" })).toBeInTheDocument();
    expect(within(preview).getByText("Write the notes")).toBeInTheDocument();
    expect(screen.getByText("Google Doc")).toBeInTheDocument();
    expect(screen.queryByText("This file is long. Only the start was read.")).toBeNull();
    expect(daemon.bodies("POST /v1/google/read")).toEqual([{ url: DOC_LINK }]);
    expect(screen.getByRole("button", { name: "Add to card note" })).toBeEnabled();
  });

  it("says that only the start was read when the file was cut short", async () => {
    const cut: GoogleLinkContent = {
      kind: "sheet",
      id: "s1",
      title: "Budget",
      url: "https://docs.google.com/spreadsheets/d/s1/edit",
      markdown: "| A | B |\n| --- | --- |\n| 1 | 2 |",
      truncated: true,
    };
    vi.spyOn(M, "readGoogleLink").mockResolvedValue({ content: cut });
    await connect("gsheets");
    openDialog();
    await readLink(cut.url);
    expect(screen.getByText("This file is long. Only the start was read.")).toBeInTheDocument();
    expect(screen.getByText("Google Sheet")).toBeInTheDocument();
    expect(screen.getByRole("table")).toBeInTheDocument();
  });

  it("shows the daemon's own sentence beside the field when the address is not a Google file", async () => {
    openDialog();
    type("https://example.com/x");
    read();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Paste the address of a Google Doc, Sheet or Slides presentation.",
    );
    expect(M.S.toasts).toEqual([]);
    expect(screen.queryByRole("region", { name: "What was read" })).toBeNull();
    expect(screen.getByRole("button", { name: "Add to card note" })).toBeDisabled();
  });

  it("shows the daemon's sentence when it refuses a link the store thought was fine", async () => {
    M.set({
      integrations: M.S.integrations.map((row) =>
        row.id === "gdocs" ? { ...row, st: "connected" as const } : row,
      ),
    });
    openDialog();
    type(DOC_LINK);
    read();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Google Docs is not connected yet. Connect it in Settings, under Integrations.",
    );
    expect(M.S.toasts).toEqual([]);
  });

  it("says to connect the kind of file's service, and asks the daemon for nothing", async () => {
    await connect("gdocs");
    openDialog();
    type("https://docs.google.com/spreadsheets/d/s1/edit");
    read();
    expect(toasts()).toEqual(["Connect Google Sheets in Settings, under Integrations."]);
    expect(count("POST /v1/google/read")).toBe(0);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("clears what was read as soon as the link changes, and reads once when pressed twice", async () => {
    await connect("gdocs");
    openDialog();
    type(DOC_LINK);
    const form = field().closest("form") as HTMLFormElement;
    read();
    expect(screen.getByRole("button", { name: "Reading…" })).toBeDisabled();
    fireEvent.submit(form);
    await screen.findByRole("region", { name: "What was read" });
    expect(count("POST /v1/google/read")).toBe(1);
    type(`${DOC_LINK}#heading=h.1`);
    expect(screen.queryByRole("region", { name: "What was read" })).toBeNull();
    expect(screen.getByRole("button", { name: "Add to card note" })).toBeDisabled();
  });

  it("ignores the answer for a link that is no longer in the field", async () => {
    await connect("gdocs");
    let answer: (value: { content: GoogleLinkContent }) => void = () => undefined;
    vi.spyOn(M, "readGoogleLink").mockReturnValue(
      new Promise((resolve) => {
        answer = resolve;
      }),
    );
    openDialog();
    type(DOC_LINK);
    read();
    type("https://docs.google.com/document/d/other/edit");
    answer({
      content: {
        kind: "doc",
        id: "abc123",
        title: "Launch plan",
        url: DOC_LINK,
        markdown: "# Launch plan",
        truncated: false,
      },
    });
    await waitFor(() => expect(screen.getByRole("button", { name: "Read" })).toBeEnabled());
    expect(screen.queryByRole("region", { name: "What was read" })).toBeNull();
    expect(screen.getByRole("button", { name: "Add to card note" })).toBeDisabled();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("reads when Enter is pressed in the field", async () => {
    await connect("gdocs");
    openDialog();
    type(DOC_LINK);
    fireEvent.submit(field().closest("form") as HTMLFormElement);
    await screen.findByRole("region", { name: "What was read" });
    expect(count("POST /v1/google/read")).toBe(1);
  });
});

describe("Add to card note", () => {
  it("adds a headed block to the end of the note, through the note's own save, and closes", async () => {
    await connect("gdocs");
    giveNote("# Card\n\nMy words");
    openDialog();
    await readLink();
    fireEvent.click(screen.getByRole("button", { name: "Add to card note" }));
    const added = [
      "# Card",
      "",
      "My words",
      "",
      "## From Google Doc: Launch plan",
      "",
      "# Launch plan",
      "",
      "- Write the notes",
      "- Send them out",
      "",
      `Source: ${DOC_LINK}`,
      "",
    ].join("\n");
    await waitFor(() => expect(M.S.notes?.[OPEN]).toBe(added));
    expect(daemon.bodies(NOTE_ROUTE())).toEqual([{ body: added }]);
    expect(panel.state.importOpen).toBe(false);
    expect(screen.queryByRole("dialog", { name: "Import from Google link" })).toBeNull();
    expect(toasts()).toContain("Note saved");
  });

  it("waits while the note is still being read, so it never saves over a note it has not seen", async () => {
    await connect("gdocs");
    openDialog();
    await readLink();
    expect(screen.getByRole("button", { name: "Add to card note" })).toBeDisabled();
    giveNote("Mine");
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Add to card note" })).toBeEnabled(),
    );
    expect(count(NOTE_ROUTE())).toBe(0);
  });

  it("adds to what is typed in the note editor, and leaves the editor", async () => {
    await connect("gdocs");
    giveNote("Saved words");
    panel.set({ noteEdit: true, noteDraft: "Saved words and a draft" });
    openDialog();
    await readLink();
    fireEvent.click(screen.getByRole("button", { name: "Add to card note" }));
    await waitFor(() =>
      expect(M.S.notes?.[OPEN]).toContain("Saved words and a draft\n\n## From Google Doc"),
    );
    expect(panel.state.noteEdit).toBe(false);
  });
});

describe("on a phone", () => {
  it("is a sheet over the card, with the same fields", async () => {
    M.setViewport(390, 800);
    await connect("gdocs");
    const dialog = openDialog();
    expect(dialog.className).toContain("bottom-0");
    await readLink();
    expect(screen.getByRole("button", { name: "Add to card note" })).toBeInTheDocument();
  });
});
