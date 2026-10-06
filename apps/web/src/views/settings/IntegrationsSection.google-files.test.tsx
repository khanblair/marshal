import { cleanup, fireEvent, screen, within } from "@solidjs/testing-library";
import { afterEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { INTEGRATIONS_EXPANDED_KEY, INTEGRATIONS_VIEW_KEY } from "./integrations-view";
import { PHONE_WIDTH_PX, showSettings } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

afterEach(() => {
  cleanup();
  window.localStorage.removeItem(INTEGRATIONS_VIEW_KEY);
  window.localStorage.removeItem(INTEGRATIONS_EXPANDED_KEY);
});

// Google Drive, Docs, Sheets and Slides sit after Gmail, each a row like every other: a fold, a
// status, a Test button once something is stored, and a Connect or Manage button that opens its dialog.

const IDS = ["gdrive", "gdocs", "gsheets", "gslides"];
const NAMES = ["Google Drive", "Google Docs", "Google Sheets", "Google Slides"];
const ORDER = [
  "github",
  "trello",
  "gcal",
  "gmail",
  ...IDS,
  "telegram",
  "discord",
  "ntfy",
  "obsidian",
];

const shownIds = (): string[] =>
  [...document.querySelectorAll("[data-integration]")].map(
    (element) => element.getAttribute("data-integration") ?? "",
  );
const panel = (id: string): HTMLElement => {
  const found = document.querySelector(`[data-integration="${id}"]`);
  if (!(found instanceof HTMLElement)) throw new Error(`the ${id} row is not drawn`);
  return found;
};
const choose = (name: "List view" | "Card view") =>
  fireEvent.click(screen.getByRole("button", { name }));
const connect = (id: string, st: "none" | "connected" | "error") =>
  M.set({
    integrations: M.S.integrations.map((entry) => (entry.id === id ? { ...entry, st } : entry)),
  });

describe("the four Google rows", () => {
  it("are listed right after Gmail, in this order, in the list layout", () => {
    showSettings("integrations");
    expect(shownIds()).toEqual(ORDER);
    for (const name of NAMES) expect(screen.getByText(name)).toBeInTheDocument();
  });

  it("are listed right after Gmail in the card layout too", () => {
    showSettings("integrations");
    choose("Card view");
    expect(document.querySelector("[data-view]")?.getAttribute("data-view")).toBe("cards");
    expect(shownIds()).toEqual(ORDER);
  });

  it("start not connected, with a Connect button and no Test button", () => {
    showSettings("integrations");
    for (const id of IDS) {
      expect(within(panel(id)).getByText("Not connected")).toBeInTheDocument();
      expect(within(panel(id)).getByRole("button", { name: "Connect" })).toBeInTheDocument();
      expect(within(panel(id)).queryByRole("button", { name: "Test" })).toBeNull();
    }
  });

  it("fold from a chevron of their own, like every other row", () => {
    showSettings("integrations");
    for (const name of NAMES) {
      expect(screen.getByRole("button", { name: `Expand ${name}` })).toHaveAttribute(
        "aria-expanded",
        "false",
      );
    }
    fireEvent.click(screen.getByRole("button", { name: "Expand Google Slides" }));
    expect(screen.getByRole("button", { name: "Collapse Google Slides" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("show a Test button, and Manage, once something is stored", () => {
    showSettings("integrations");
    connect("gsheets", "connected");
    expect(within(panel("gsheets")).getByRole("button", { name: "Manage" })).toBeInTheDocument();
    expect(within(panel("gsheets")).getByRole("button", { name: "Test" })).toBeInTheDocument();
  });
});

describe("the dialog of each", () => {
  it.each(IDS.map((id, at) => [id, NAMES[at] ?? ""]))(
    "opens %s from Connect and closes again",
    (id, name) => {
      showSettings("integrations");
      fireEvent.click(within(panel(id)).getByRole("button", { name: "Connect" }));
      const dialog = screen.getByRole("dialog", { name: `Connect ${name}` });
      expect(within(dialog).getByRole("tab", { name: "Google access" })).toBeInTheDocument();
      expect(within(dialog).getByRole("tab", { name: "Files" })).toBeInTheDocument();
      fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
      expect(screen.queryByRole("dialog")).toBeNull();
    },
  );

  it("opens from Manage once connected, with Reconnect, Test connection and Disconnect", () => {
    showSettings("integrations");
    connect("gdocs", "connected");
    fireEvent.click(within(panel("gdocs")).getByRole("button", { name: "Manage" }));
    const dialog = screen.getByRole("dialog", { name: "Connect Google Docs" });
    expect(
      within(dialog).getByRole("button", { name: "Reconnect Google Docs" }),
    ).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Test connection" })).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Disconnect" })).toBeInTheDocument();
  });

  it("opens in the card layout too, and only one dialog at a time", () => {
    showSettings("integrations");
    choose("Card view");
    fireEvent.click(within(panel("gdrive")).getByRole("button", { name: "Connect" }));
    expect(screen.getByRole("dialog", { name: "Connect Google Drive" })).toBeInTheDocument();
    fireEvent.click(within(panel("gslides")).getByRole("button", { name: "Connect" }));
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.getByRole("dialog", { name: "Connect Google Slides" })).toBeInTheDocument();
  });

  it("switches between its two tabs", () => {
    showSettings("integrations");
    fireEvent.click(within(panel("gdrive")).getByRole("button", { name: "Connect" }));
    fireEvent.click(screen.getByRole("tab", { name: "Files" }));
    expect(screen.getByLabelText(/^Folder/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Google access" }));
    expect(screen.queryByLabelText(/^Folder/)).toBeNull();
    expect(screen.getByText(/make files in your Drive/)).toBeInTheDocument();
  });
});

describe("on a phone", () => {
  it("says the connection is made on the desktop app, with no way to connect", () => {
    showSettings("integrations", {}, PHONE_WIDTH_PX);
    fireEvent.click(within(panel("gslides")).getByRole("button", { name: "Connect" }));
    const dialog = screen.getByRole("dialog", { name: "Connect Google Slides" });
    expect(
      within(dialog).getByText(/Connect Google Slides on the desktop app/),
    ).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Connect with Google" })).toBeNull();
  });
});
