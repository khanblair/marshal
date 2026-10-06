import type { GoogleFile } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Integration } from "~/mock";
import type { GoogleFilesAnswer } from "~/sync/google-files-actions";
import { createEditState } from "./edit-state";
import { GoogleFilesDialog } from "./GoogleFilesDialog";
import type { GoogleFilesProps } from "./google-files-connect";
import { GOOGLE_FILE_SPECS, type GoogleFileSpec, googleFileSpec } from "./google-files-spec";

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const DRIVE = googleFileSpec("gdrive") as GoogleFileSpec;
const DOCS = googleFileSpec("gdocs") as GoogleFileSpec;

const ONE_CLICK = async () => ({ bundled: true, own: false });
const NO_CLIENT = async () => ({ bundled: false, own: false });
const NOT_SURE = async () => null;

const row = (spec: GoogleFileSpec, st: Integration["st"], detail = ""): Integration => ({
  id: spec.id,
  name: spec.name,
  icon: "plug",
  st,
  detail,
});

const NOTES: GoogleFile = {
  id: "n",
  name: "Release notes",
  kind: "doc",
  url: "https://docs.google.com/document/d/n/edit",
};
const list = (...files: GoogleFile[]): GoogleFilesAnswer => ({
  files: { files, folder: "Marshal" },
});

function show(spec: GoogleFileSpec, st: Integration["st"], extra: Partial<GoogleFilesProps> = {}) {
  const edit = createEditState();
  edit.open(spec.id);
  render(() => (
    <GoogleFilesDialog
      spec={spec}
      integration={row(spec, st)}
      edit={edit}
      info={ONE_CLICK}
      refresh={async () => undefined}
      load={async () => list()}
      phone={false}
      {...extra}
    />
  ));
  return edit;
}

const tab = (name: string) => screen.getByRole("tab", { name });
const press = (name: string) => fireEvent.click(screen.getByRole("button", { name }));

describe("the two tabs", () => {
  it("opens on Google access, titled by the connection, with Files one click away", () => {
    show(DOCS, "none");
    expect(screen.getByRole("dialog", { name: "Connect Google Docs" })).toBeInTheDocument();
    expect(tab("Google access")).toHaveAttribute("aria-selected", "true");
    expect(tab("Files")).toHaveAttribute("aria-selected", "false");
    fireEvent.click(tab("Files"));
    expect(tab("Files")).toHaveAttribute("aria-selected", "true");
    fireEvent.click(tab("Google access"));
    expect(screen.getByRole("button", { name: "Connect with Google" })).toBeInTheDocument();
  });

  it("closes from the Close button", () => {
    const edit = show(DOCS, "none");
    press("Close");
    expect(edit.id()).toBeNull();
  });

  it.each(GOOGLE_FILE_SPECS)(
    "says what $name is for and what Google will ask, in plain words",
    (spec) => {
      show(spec, "none");
      expect(screen.getByText(spec.purpose)).toBeInTheDocument();
      expect(screen.getByText(spec.permission)).toBeInTheDocument();
      expect(document.body.textContent).not.toMatch(/oauth/i);
    },
  );
});

describe("Connect with Google", () => {
  it("opens Google on one click and then asks the row how it went until the grant lands", async () => {
    const grant = vi.fn(async () => true);
    const refresh = vi.fn(async () => undefined);
    show(DOCS, "none", { grant, refresh });
    press("Connect with Google");
    await waitFor(() => expect(grant).toHaveBeenCalledTimes(1));
    await vi.advanceTimersByTimeAsync(4100);
    expect(refresh.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("says so, and does not keep asking, when Google could not be started", async () => {
    const refresh = vi.fn(async () => undefined);
    const edit = show(DOCS, "none", { grant: async () => false, refresh });
    press("Connect with Google");
    await waitFor(() =>
      expect(edit.errorFor("gdocs")).toBe("Marshal could not start the Google sign-in. Try again."),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      /could not start the Google sign-in/,
    );
    await vi.advanceTimersByTimeAsync(6000);
    expect(refresh).not.toHaveBeenCalled();
  });

  it("says to use the computer that runs Marshal, and offers no Test or Disconnect before it is connected", () => {
    show(DOCS, "none");
    expect(screen.getByText(/Use the computer that runs Marshal/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Test connection" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Disconnect" })).toBeNull();
  });
});

describe("a build with no Google client", () => {
  it("points to Google Calendar, and offers no Connect button", async () => {
    show(DOCS, "none", { info: NO_CLIENT });
    expect(
      await screen.findByText(
        "Save a Google client under Google Calendar first, then connect here.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect with Google" })).toBeNull();
  });

  it("still offers Connect when the daemon could not say which client it has", async () => {
    const info = vi.fn(NOT_SURE);
    show(DOCS, "none", { info });
    await vi.advanceTimersByTimeAsync(0);
    expect(info).toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Connect with Google" })).toBeInTheDocument();
    expect(screen.queryByText(/Save a Google client/)).toBeNull();
  });

  it("offers Connect when a client of the person's own is saved", async () => {
    const info = vi.fn(async () => ({ bundled: false, own: true }));
    show(DOCS, "none", { info });
    await vi.advanceTimersByTimeAsync(0);
    expect(info).toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Connect with Google" })).toBeInTheDocument();
    expect(screen.queryByText(/Save a Google client/)).toBeNull();
  });
});

describe("once connected", () => {
  it("offers Reconnect, Test connection and Disconnect, and no Connect with Google", () => {
    show(DOCS, "connected");
    expect(screen.getByRole("button", { name: "Reconnect Google Docs" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Test connection" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect with Google" })).toBeNull();
  });

  it("asks Google again from Reconnect", async () => {
    const grant = vi.fn(async () => true);
    show(DOCS, "connected", { grant });
    press("Reconnect Google Docs");
    await waitFor(() => expect(grant).toHaveBeenCalledTimes(1));
  });

  it("offers Reconnect, with the daemon's sentence, when Google no longer accepts the access", () => {
    const edit = createEditState();
    edit.open("gdocs");
    render(() => (
      <GoogleFilesDialog
        spec={DOCS}
        integration={row(
          DOCS,
          "error",
          "Google no longer accepts Marshal's access to Google Docs.",
        )}
        edit={edit}
        info={ONE_CLICK}
        phone={false}
      />
    ));
    expect(screen.getByRole("alert")).toHaveTextContent(/no longer accepts/);
    expect(screen.getByRole("button", { name: "Reconnect Google Docs" })).toBeInTheDocument();
  });
});

describe("on a phone", () => {
  it("says the connection is made on the desktop app, and offers no way to connect", () => {
    show(DOCS, "none", { phone: true });
    expect(
      screen.getByText(/Connect Google Docs on the desktop app\. This phone uses the connection/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Connect with Google" })).toBeNull();
  });

  it("keeps Test and Disconnect for a connection that is made, but not Reconnect", () => {
    show(DOCS, "connected", { phone: true });
    expect(screen.getByText(/on the desktop app/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Test connection" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reconnect Google Docs" })).toBeNull();
  });
});

describe("the Files tab", () => {
  const open = () => fireEvent.click(tab("Files"));

  it("lists the files of the connection's own kind, each with a link to open it", async () => {
    const load = vi.fn(async () => list(NOTES));
    show(DOCS, "connected", { load });
    open();
    await screen.findByText("Release notes");
    expect(load).toHaveBeenCalledWith("doc");
    expect(screen.getByRole("link", { name: "Open Release notes" })).toHaveAttribute(
      "href",
      NOTES.url,
    );
  });

  it("asks for every kind on Drive's tab, and for each kind on the others", async () => {
    for (const [spec, kind] of [
      [DRIVE, undefined],
      [googleFileSpec("gsheets"), "sheet"],
      [googleFileSpec("gslides"), "slides"],
    ] as const) {
      const load = vi.fn(async () => list());
      show(spec as GoogleFileSpec, "connected", { load });
      open();
      await waitFor(() => expect(load).toHaveBeenCalledWith(kind));
      cleanup();
    }
  });

  it("says Marshal has made none yet", async () => {
    show(DOCS, "connected");
    open();
    expect(await screen.findByText("Marshal has not made any files here yet.")).toBeInTheDocument();
  });

  it("shows the daemon's sentence when the files could not be read", async () => {
    show(DOCS, "connected", {
      load: async () => ({ error: "Google could not be reached. Try again." }),
    });
    open();
    expect(await screen.findByRole("alert")).toHaveTextContent("Google could not be reached.");
  });

  it("says to connect first, and reads nothing, while the connection is not made", () => {
    const load = vi.fn(async () => list());
    show(DOCS, "none", { load });
    open();
    expect(screen.getByText(/Connect Google Docs first/)).toBeInTheDocument();
    expect(load).not.toHaveBeenCalled();
  });

  it("has no Folder field on Docs", () => {
    show(DOCS, "connected");
    open();
    expect(screen.queryByLabelText(/^Folder/)).toBeNull();
  });
});

describe("Google Drive's folder", () => {
  const open = () => fireEvent.click(tab("Files"));
  const field = () => screen.getByLabelText(/^Folder/) as HTMLInputElement;

  it("starts as Marshal, even before Drive is connected, and has a Save", () => {
    show(DRIVE, "none");
    open();
    expect(field()).toHaveValue("Marshal");
    expect(screen.getByRole("button", { name: "Save folder" })).toBeInTheDocument();
  });

  it("shows the folder the daemon has saved once the files are read, until it is edited", async () => {
    show(DRIVE, "connected", { load: async () => ({ files: { files: [], folder: "Work" } }) });
    open();
    await waitFor(() => expect(field()).toHaveValue("Work"));
    fireEvent.input(field(), { target: { value: "Mine" } });
    expect(field()).toHaveValue("Mine");
  });

  it("saves the name typed, without its spaces, and says it is saved", async () => {
    const saveFolder = vi.fn(async (_: string) => ({ saved: true as const }));
    show(DRIVE, "none", { saveFolder });
    open();
    fireEvent.input(field(), { target: { value: " Notes " } });
    press("Save folder");
    await waitFor(() => expect(saveFolder).toHaveBeenCalledWith("Notes"));
    expect(await screen.findByText("Folder saved.")).toBeInTheDocument();
  });

  it("shows the daemon's sentence under the field when it refuses the name", async () => {
    const sentence = "The folder name must be 1 to 100 characters, with no control characters.";
    show(DRIVE, "none", { saveFolder: async () => ({ error: sentence }) });
    open();
    fireEvent.input(field(), { target: { value: "" } });
    press("Save folder");
    const alert = await screen.findByRole("alert");
    expect(within(alert.parentElement as HTMLElement).getByText(sentence)).toBeInTheDocument();
    expect(screen.queryByText("Folder saved.")).toBeNull();
  });
});
