import type { GoogleFile } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { GoogleFilesAnswer } from "~/sync/google-files-actions";
import { GoogleFilesList } from "./GoogleFilesList";

afterEach(cleanup);

const file = (over: Partial<GoogleFile>): GoogleFile => ({
  id: "x",
  name: "X",
  kind: "doc",
  url: "https://docs.google.com/document/d/x/edit",
  ...over,
});

const NOTES = file({ id: "n", name: "Release notes" });
const BOARD = file({
  id: "b",
  name: "Board",
  kind: "sheet",
  url: "https://docs.google.com/spreadsheets/d/b/edit",
});

const list = (...files: GoogleFile[]): GoogleFilesAnswer => ({
  files: { files, folder: "Marshal" },
});

describe("the files Marshal made", () => {
  it("says to connect first, and asks Google nothing, until the service is connected", () => {
    const load = vi.fn();
    render(() => <GoogleFilesList name="Google Docs" connected={false} load={load} />);
    expect(
      screen.getByText("Connect Google Docs first. The files Marshal makes show up here."),
    ).toBeInTheDocument();
    expect(load).not.toHaveBeenCalled();
  });

  it("lists each file with a link that opens it in a new tab, and says where they are kept", async () => {
    render(() => (
      <GoogleFilesList name="Google Docs" kind="doc" connected load={async () => list(NOTES)} />
    ));
    await screen.findByText("Release notes");
    const link = screen.getByRole("link", { name: "Open Release notes" });
    expect(link).toHaveAttribute("href", "https://docs.google.com/document/d/x/edit");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByText("Saved in the folder “Marshal” in your Drive.")).toBeInTheDocument();
  });

  it("asks for the kind it was given, and for every kind when it has none", async () => {
    const load = vi.fn(async () => list());
    render(() => <GoogleFilesList name="Google Sheets" kind="sheet" connected load={load} />);
    await waitFor(() => expect(load).toHaveBeenCalledWith("sheet"));
    cleanup();
    const every = vi.fn(async () => list());
    render(() => <GoogleFilesList name="Google Drive" connected load={every} />);
    await waitFor(() => expect(every).toHaveBeenCalledWith(undefined));
  });

  it("names each file's kind on Drive's list, and leaves it off a list of one kind", async () => {
    render(() => (
      <GoogleFilesList name="Google Drive" connected load={async () => list(NOTES, BOARD)} />
    ));
    await screen.findByText("Board");
    expect(screen.getByText("Doc")).toBeInTheDocument();
    expect(screen.getByText("Sheet")).toBeInTheDocument();
    cleanup();
    render(() => (
      <GoogleFilesList name="Google Docs" kind="doc" connected load={async () => list(NOTES)} />
    ));
    await screen.findByText("Release notes");
    expect(screen.queryByText("Doc")).toBeNull();
  });

  it("says how long ago a file changed, when Google said", async () => {
    const recent = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();
    render(() => (
      <GoogleFilesList
        name="Google Docs"
        kind="doc"
        connected
        load={async () => list(file({ name: "Recent", modifiedAt: recent }))}
      />
    ));
    await screen.findByText("Recent");
    expect(screen.getByText(/2 h/i)).toBeInTheDocument();
  });

  it("says plainly when Marshal has made none yet", async () => {
    render(() => (
      <GoogleFilesList name="Google Docs" kind="doc" connected load={async () => list()} />
    ));
    expect(await screen.findByText("Marshal has not made any files here yet.")).toBeInTheDocument();
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("shows the daemon's own sentence when the files could not be read, and tries again", async () => {
    const load = vi
      .fn<() => Promise<GoogleFilesAnswer>>()
      .mockResolvedValueOnce({ error: "Google Docs is not connected yet." })
      .mockResolvedValueOnce(list(NOTES));
    render(() => <GoogleFilesList name="Google Docs" kind="doc" connected load={load} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Google Docs is not connected yet.");
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByText("Release notes");
    expect(load).toHaveBeenCalledTimes(2);
  });

  it("draws no link for an address that is not a web address", async () => {
    render(() => (
      <GoogleFilesList
        name="Google Docs"
        kind="doc"
        connected
        load={async () => list(file({ name: "Odd", url: "javascript:alert(1)" }))}
      />
    ));
    await screen.findByText("Odd");
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("tells the caller the folder the daemon keeps the files in", async () => {
    const onFolder = vi.fn();
    render(() => (
      <GoogleFilesList
        name="Google Drive"
        connected
        onFolder={onFolder}
        load={async () => ({ files: { files: [], folder: "Work" } })}
      />
    ));
    await waitFor(() => expect(onFolder).toHaveBeenCalledWith("Work"));
  });
});
