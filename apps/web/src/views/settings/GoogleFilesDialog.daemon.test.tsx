// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it.
import { daemon } from "~/testing/daemon-integrations-store";
import type { GoogleFile } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { createGoogleFilesStore } from "~/testing/fake-google-files";
import { createIntegrationStore } from "~/testing/fake-integrations";
import { INTEGRATIONS_EXPANDED_KEY, INTEGRATIONS_VIEW_KEY } from "./integrations-view";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

// The four Google file dialogs against the fake daemon: the real store, client, and routes, with
// only Google's own page (`window.open`) replaced.

const NOTES: GoogleFile = {
  id: "doc-1",
  name: "Release notes",
  kind: "doc",
  url: "https://docs.google.com/document/d/doc-1/edit",
};

/** Puts the daemon and the store back to the state they start in, so one test cannot see another's. */
async function reset(): Promise<void> {
  Object.assign(daemon.integrations, createIntegrationStore());
  Object.assign(daemon.googleFiles, createGoogleFilesStore());
  // This build has Marshal's own Google client, so Connect with Google is one click.
  daemon.schedules.google.client = { bundled: true, own: false };
  await M.refreshIntegrationList();
  daemon.calls.length = 0;
  M.set({ settingsSection: "integrations", toasts: [] });
}

/** Makes one connection connected on the daemon, and lets the store read the list again. */
async function connect(id: string): Promise<void> {
  const row = daemon.integrations.rows.find((entry) => entry.id === id);
  if (row) row.st = "connected";
  await M.refreshIntegrationList();
}

beforeEach(reset);
afterEach(() => {
  cleanup();
  window.localStorage.removeItem(INTEGRATIONS_VIEW_KEY);
  window.localStorage.removeItem(INTEGRATIONS_EXPANDED_KEY);
  vi.restoreAllMocks();
});

const panel = (id: string): HTMLElement => {
  const found = document.querySelector(`[data-integration="${id}"]`);
  if (!(found instanceof HTMLElement)) throw new Error(`the ${id} row is not drawn`);
  return found;
};
const press = (id: string, name: string): void => {
  fireEvent.click(within(panel(id)).getByRole("button", { name }));
};
const dialog = (name: string): HTMLElement => screen.getByRole("dialog", { name });
const count = (route: string): number => daemon.routes().filter((entry) => entry === route).length;

describe("Connect with Google", () => {
  it("asks the daemon for this service's own address and sends the person there", async () => {
    const tab = { closed: false, opener: {}, location: { href: "about:blank" }, close: vi.fn() };
    vi.spyOn(window, "open").mockReturnValue(tab as unknown as Window);
    render(() => <SettingsView />);
    press("gdocs", "Connect");
    fireEvent.click(await screen.findByRole("button", { name: "Connect with Google" }));
    await vi.waitFor(() =>
      expect(tab.location.href).toBe("https://accounts.google.com/o/oauth2/auth?fake=gdocs"),
    );
    expect(count("GET /v1/integrations/gdocs/authorize")).toBe(1);
  });

  it("says to save a Google client first when the daemon has none", async () => {
    daemon.schedules.google.client = { bundled: false, own: false };
    render(() => <SettingsView />);
    press("gslides", "Connect");
    expect(
      await screen.findByText(
        "Save a Google client under Google Calendar first, then connect here.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect with Google" })).toBeNull();
  });
});

describe("a connected service", () => {
  it("runs its own test on the daemon and shows the checks it found", async () => {
    await connect("gdocs");
    render(() => <SettingsView />);
    press("gdocs", "Manage");
    fireEvent.click(
      within(dialog("Connect Google Docs")).getByRole("button", { name: "Test connection" }),
    );
    await vi.waitFor(() => expect(count("POST /v1/integrations/gdocs/test")).toBe(1));
    expect(await screen.findAllByText("Google Docs answered.")).not.toHaveLength(0);
    expect(M.S.toasts.map((toast) => toast.msg)).toContain("Google Docs test passed");
  });

  it("lists the files Marshal made, from the daemon, on the Files tab", async () => {
    daemon.googleFiles.files.push(NOTES);
    await connect("gdocs");
    render(() => <SettingsView />);
    press("gdocs", "Manage");
    fireEvent.click(screen.getByRole("tab", { name: "Files" }));
    expect(await screen.findByText("Release notes")).toBeInTheDocument();
    expect(count("GET /v1/google/files?kind=doc")).toBe(1);
    expect(screen.getByRole("link", { name: "Open Release notes" })).toHaveAttribute(
      "href",
      NOTES.url,
    );
  });

  it("says the daemon's own sentence on the Files tab when it will not list the files", async () => {
    // The store thinks Docs is connected while the daemon does not, as after a stale read.
    M.set({
      integrations: M.S.integrations.map((entry) =>
        entry.id === "gdocs" ? { ...entry, st: "connected" as const } : entry,
      ),
    });
    render(() => <SettingsView />);
    press("gdocs", "Manage");
    fireEvent.click(screen.getByRole("tab", { name: "Files" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Google Docs is not connected yet. Connect it in Settings, under Integrations.",
    );
    expect(M.S.toasts).toEqual([]);
  });
});

describe("Google Drive's folder", () => {
  const open = async (): Promise<void> => {
    render(() => <SettingsView />);
    press("gdrive", "Connect");
    fireEvent.click(await screen.findByRole("tab", { name: "Files" }));
  };
  const field = () => screen.getByLabelText(/^Folder/) as HTMLInputElement;

  it("saves the folder through the daemon, and the row says to grant access", async () => {
    await open();
    fireEvent.input(field(), { target: { value: "Work" } });
    fireEvent.click(screen.getByRole("button", { name: "Save folder" }));
    expect(await screen.findByText("Folder saved.")).toBeInTheDocument();
    expect(daemon.bodies("PUT /v1/integrations/gdrive")).toEqual([{ folder: "Work" }]);
    expect(daemon.integrations.driveFolder).toBe("Work");
    expect(M.S.integrations.find((entry) => entry.id === "gdrive")?.detail).toBe(
      "Grant access to finish connecting Google Drive.",
    );
  });

  it("shows the daemon's sentence under the field when it refuses the name, with no toast", async () => {
    await open();
    fireEvent.input(field(), { target: { value: "   " } });
    fireEvent.click(screen.getByRole("button", { name: "Save folder" }));
    expect(
      await screen.findByText(
        "The folder name must be 1 to 100 characters, with no control characters.",
      ),
    ).toBeInTheDocument();
    expect(M.S.toasts).toEqual([]);
    await waitFor(() => expect(count("PUT /v1/integrations/gdrive")).toBe(1));
  });

  it("shows the saved folder once Drive is connected and the files are read", async () => {
    daemon.integrations.driveFolder = "Work";
    await connect("gdrive");
    render(() => <SettingsView />);
    press("gdrive", "Manage");
    fireEvent.click(screen.getByRole("tab", { name: "Files" }));
    await waitFor(() => expect(field()).toHaveValue("Work"));
    expect(count("GET /v1/google/files")).toBe(1);
  });
});
