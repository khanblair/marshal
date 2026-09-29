import type { FolderListing } from "@marshal/protocol";
import { fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { FolderInput } from "./FolderInput";

const listing = (over: Partial<FolderListing>): FolderListing => ({
  path: "/Users/ada",
  parent: "/Users",
  home: "/Users/ada",
  isGitRepo: false,
  folders: [],
  truncated: false,
  serverTime: "2026-09-30T12:00:00.000Z",
  ...over,
});

const HOME = listing({
  folders: [
    { name: "code", path: "/Users/ada/code", isGitRepo: false },
    { name: "marshal", path: "/Users/ada/marshal", isGitRepo: true },
  ],
});
const CODE = listing({ path: "/Users/ada/code", parent: "/Users/ada", isGitRepo: true });

afterEach(() => vi.restoreAllMocks());

const press = (name: string | RegExp): void => {
  fireEvent.click(screen.getByRole("button", { name }));
};

describe("FolderInput", () => {
  it("opens a browser of the daemon's folders at home, and marks the Git repositories", async () => {
    const browse = vi.spyOn(M, "browseFolders").mockResolvedValue(HOME);
    render(() => <FolderInput value="" onChange={() => undefined} />);
    press("Browse…");
    expect(await screen.findByText("code")).toBeVisible();
    expect(browse).toHaveBeenCalledWith(undefined);
    expect(screen.getByText("marshal").closest("button")).toHaveTextContent("Git");
    expect(screen.getByRole("button", { name: "Up one folder" })).toBeEnabled();
  });

  it("opens a folder when it is pressed, goes back up, and picks the open folder", async () => {
    const browse = vi
      .spyOn(M, "browseFolders")
      .mockResolvedValueOnce(HOME)
      .mockResolvedValueOnce(CODE)
      .mockResolvedValueOnce(HOME);
    const chosen = vi.fn();
    render(() => <FolderInput value="" onChange={chosen} />);
    press("Browse…");
    fireEvent.click(await screen.findByRole("button", { name: /^code/ }));
    expect(await screen.findByText("Git repository")).toBeVisible();
    expect(browse).toHaveBeenLastCalledWith("/Users/ada/code");
    press("Up one folder");
    await screen.findByText("marshal");
    expect(browse).toHaveBeenLastCalledWith("/Users/ada");
    press("Use this folder");
    expect(chosen).toHaveBeenCalledWith("/Users/ada");
    expect(screen.queryByText("marshal")).toBeNull();
  });

  it("starts where the typed path points, and Cancel closes it without choosing", async () => {
    const browse = vi.spyOn(M, "browseFolders").mockResolvedValue(CODE);
    const chosen = vi.fn();
    render(() => <FolderInput value="/Users/ada/code" onChange={chosen} />);
    press("Browse…");
    await screen.findByText("No folders in here.");
    expect(browse).toHaveBeenCalledWith("/Users/ada/code");
    press("Cancel");
    expect(screen.queryByText("No folders in here.")).toBeNull();
    expect(chosen).not.toHaveBeenCalled();
  });

  it("falls back to home when the typed path is not a folder that opens", async () => {
    const browse = vi
      .spyOn(M, "browseFolders")
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(HOME);
    render(() => <FolderInput value="/nope" onChange={() => undefined} />);
    press("Browse…");
    await screen.findByText("code");
    expect(browse).toHaveBeenNthCalledWith(1, "/nope");
    expect(browse).toHaveBeenNthCalledWith(2, undefined);
  });

  it("cannot go up from the top of the disk", async () => {
    vi.spyOn(M, "browseFolders").mockResolvedValue(listing({ path: "/", parent: "" }));
    render(() => <FolderInput value="" onChange={() => undefined} />);
    press("Browse…");
    await screen.findByText("No folders in here.");
    expect(screen.getByRole("button", { name: "Up one folder" })).toBeDisabled();
  });

  it("still lets a path be typed", () => {
    const changed = vi.fn();
    render(() => <FolderInput value="" onChange={changed} />);
    fireEvent.input(screen.getByPlaceholderText("~/code/my-repo"), { target: { value: "~/x" } });
    expect(changed).toHaveBeenCalledWith("~/x");
  });
});
