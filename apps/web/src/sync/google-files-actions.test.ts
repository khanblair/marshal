import type { GoogleFile } from "@marshal/protocol";
import { afterEach, describe, expect, it } from "vitest";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { createSyncedMarshal, createTestMarshal } from "~/testing/test-store";

// Sections S29i to S29l: Google Drive, Docs, Sheets and Slides. Every call here goes through the
// real client to the fake daemon, which answers the way the daemon's own routes do.

const NOT_CONNECTED = (name: string) =>
  `${name} is not connected yet. Connect it in Settings, under Integrations.`;

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

/** A synced store over a fake daemon that has the given connections already made. */
async function start(connected: readonly string[] = [], files: readonly GoogleFile[] = []) {
  const d = createFakeDaemon({ googleFiles: files });
  daemon = d;
  for (const row of d.integrations.rows) if (connected.includes(row.id)) row.st = "connected";
  return { d, M: await createSyncedMarshal(d) };
}

const row = (M: Awaited<ReturnType<typeof start>>["M"], id: string) => {
  const found = M.S.integrations.find((integration) => integration.id === id);
  if (!found) throw new Error(`the store has no ${id} connection`);
  return found;
};

const NOTES: GoogleFile = {
  id: "doc-9",
  name: "Release notes",
  kind: "doc",
  url: "https://docs.google.com/document/d/doc-9/edit",
  modifiedAt: "2026-10-05T10:00:00Z",
};
const BOARD: GoogleFile = {
  id: "sheet-9",
  name: "Board",
  kind: "sheet",
  url: "https://docs.google.com/spreadsheets/d/sheet-9/edit",
};

describe("the four Google rows", () => {
  it("are listed after Gmail, not connected, and each filed under its own kind", async () => {
    const { d, M } = await start();
    expect(d.integrations.rows.map((entry) => [entry.id, entry.kind]).slice(3, 8)).toEqual([
      ["gmail", "gmail"],
      ["gdrive", "drive"],
      ["gdocs", "docs"],
      ["gsheets", "sheets"],
      ["gslides", "slides"],
    ]);
    for (const id of ["gdrive", "gdocs", "gsheets", "gslides"]) {
      expect(row(M, id).st).toBe("none");
      expect(M.connectionOnDaemon(id)).toBe(true);
    }
  });

  it("run their own test on the daemon, with checks of their own", async () => {
    const { M } = await start(["gdocs"]);
    expect(await M.testIntegration("gdocs")).toBe(true);
    expect(row(M, "gdocs").lastTest?.checks.map((check) => check.name)).toEqual([
      "Summary",
      "Access",
      "Drive",
      "Docs",
    ]);
    expect(row(M, "gdocs").detail).toBe(
      "Google Docs works. Marshal makes documents in the folder “Marshal” and reads any you share by link.",
    );
  });

  it("add a Folder check to Drive's own test", async () => {
    const { M } = await start(["gdrive"]);
    expect(await M.testIntegration("gdrive")).toBe(true);
    expect(row(M, "gdrive").lastTest?.checks.at(-1)).toMatchObject({
      name: "Folder",
      message: "The folder “Marshal” is there.",
    });
  });
});

describe("the consent of a Google service", () => {
  it("answers the address Google's page is at, for the service asked", async () => {
    const { d, M } = await start();
    expect(await M.authorizeGoogleService("gdocs")).toEqual({
      url: "https://accounts.google.com/o/oauth2/auth?fake=gdocs",
    });
    expect(d.routes()).toContain("GET /v1/integrations/gdocs/authorize");
  });

  it("answers nothing for a connection that is not a Google service, and shows no toast", async () => {
    const { M } = await start();
    expect(await M.authorizeGoogleService("github")).toBeNull();
    expect(M.S.toasts).toEqual([]);
  });
});

describe("the files Marshal made", () => {
  it("lists every kind from Drive's access when no kind is asked for, and says the folder", async () => {
    const { d, M } = await start(["gdrive"], [NOTES, BOARD]);
    expect(await M.googleFiles()).toEqual({ files: { files: [NOTES, BOARD], folder: "Marshal" } });
    expect(d.routes()).toContain("GET /v1/google/files");
  });

  it("lists one kind with that service's access, and leaves other kinds out", async () => {
    const { d, M } = await start(["gdocs"], [NOTES, BOARD]);
    expect(await M.googleFiles("doc")).toEqual({ files: { files: [NOTES], folder: "Marshal" } });
    expect(d.routes()).toContain("GET /v1/google/files?kind=doc");
  });

  it("answers the daemon's own sentence, and no toast, when the service is not connected", async () => {
    const { M } = await start(["gdrive"]);
    expect(await M.googleFiles("sheet")).toEqual({ error: NOT_CONNECTED("Google Sheets") });
    expect(M.S.toasts).toEqual([]);
  });

  it("says to reconnect when Google no longer accepts the access", async () => {
    const { d, M } = await start();
    const drive = d.integrations.rows.find((entry) => entry.id === "gdrive");
    if (drive) drive.st = "error";
    expect(await M.googleFiles()).toEqual({
      error: "Google no longer accepts Marshal's access to Google Drive. Reconnect it in Settings.",
    });
  });

  it("is told there is no daemon, in a sentence, when the store has none", async () => {
    const M = createTestMarshal();
    expect(await M.googleFiles()).toEqual({ error: "Marshal is not connected to its daemon." });
    expect(await M.authorizeGoogleService("gdocs")).toBeNull();
    expect(await M.saveGoogleDrive({ folder: "Work" })).toEqual({
      error: "Marshal is not connected to its daemon.",
    });
  });
});

describe("Google Drive's folder", () => {
  it("saves the name, answers the whole list, and says to grant access while none is given", async () => {
    const { d, M } = await start();
    expect(await M.saveGoogleDrive({ folder: "Work" })).toEqual({ saved: true });
    expect(d.bodies("PUT /v1/integrations/gdrive")).toEqual([{ folder: "Work" }]);
    expect(d.integrations.driveFolder).toBe("Work");
    expect(row(M, "gdrive")).toMatchObject({
      st: "none",
      detail: "Grant access to finish connecting Google Drive.",
    });
  });

  it("is the folder the list then names, once Drive is connected", async () => {
    const { M } = await start(["gdrive"]);
    await M.saveGoogleDrive({ folder: "Work" });
    expect(await M.googleFiles()).toEqual({ files: { files: [], folder: "Work" } });
  });

  it("answers the daemon's sentence for a name it refuses, and keeps the folder", async () => {
    const { d, M } = await start();
    for (const folder of ["", "a\u0007b", "x".repeat(101)]) {
      expect(await M.saveGoogleDrive({ folder })).toEqual({
        error: "The folder name must be 1 to 100 characters, with no control characters.",
      });
    }
    expect(d.integrations.driveFolder).toBe("Marshal");
    expect(M.S.toasts).toEqual([]);
  });
});

describe("making files", () => {
  it("makes a document, a sheet, a presentation and a plain file, newest first", async () => {
    const { d, M } = await start(["gdrive", "gdocs", "gsheets", "gslides"]);
    const doc = await M.createGoogleDoc({ title: "Plan", html: "<h1>Plan</h1>" });
    const sheet = await M.createGoogleSheet({ title: "Board", rows: [["Title"], ["One"]] });
    const slides = await M.createGoogleSlides({
      title: "Deck",
      slides: [{ title: "Hello", bullets: ["One"] }],
    });
    const plain = await M.uploadGoogleFile({ name: "notes.md", content: "# Notes" });
    expect(doc).toMatchObject({ file: { name: "Plan", kind: "doc" } });
    expect(sheet).toMatchObject({ file: { name: "Board", kind: "sheet" } });
    expect(slides).toMatchObject({ file: { name: "Deck", kind: "slides" } });
    expect(plain).toMatchObject({ file: { name: "notes.md", kind: "file" } });
    expect("file" in doc && doc.file.url).toMatch(/^https:\/\/docs\.google\.com\/document\/d\//);
    expect(d.routes()).toEqual(
      expect.arrayContaining([
        "POST /v1/google/docs",
        "POST /v1/google/sheets",
        "POST /v1/google/slides",
        "POST /v1/google/drive/files",
      ]),
    );
    const listed = await M.googleFiles();
    expect("files" in listed && listed.files.files.map((file) => file.name)).toEqual([
      "notes.md",
      "Deck",
      "Board",
      "Plan",
    ]);
  });

  it("answers the daemon's sentence when the service is not connected, and makes nothing", async () => {
    const { d, M } = await start(["gdrive"]);
    expect(await M.createGoogleDoc({ title: "Plan", text: "x" })).toEqual({
      error: NOT_CONNECTED("Google Docs"),
    });
    expect(d.googleFiles.files).toEqual([]);
    expect(M.S.toasts).toEqual([]);
  });

  it("asks for a title", async () => {
    const { M } = await start(["gdocs"]);
    expect(await M.createGoogleDoc({ title: " ", text: "x" })).toEqual({
      error: "The title must be 1 to 200 characters.",
    });
  });
});

describe("reading a Google link", () => {
  const link = (path: string) => `https://docs.google.com${path}`;

  it("reads a document, a sheet and a presentation by the address, with the matching access", async () => {
    const { d, M } = await start(["gdocs", "gsheets", "gslides"]);
    const doc = await M.readGoogleLink({ url: link("/document/d/abc123/edit") });
    expect(doc).toMatchObject({ content: { kind: "doc", id: "abc123", truncated: false } });
    const sheet = await M.readGoogleLink({ url: link("/spreadsheets/u/0/d/s1/edit#gid=0") });
    expect(sheet).toMatchObject({ content: { kind: "sheet", id: "s1" } });
    const slides = await M.readGoogleLink({ url: link("/u/0/presentation/d/p1/edit") });
    expect(slides).toMatchObject({ content: { kind: "slides", id: "p1" } });
    expect(d.bodies("POST /v1/google/read")).toHaveLength(3);
  });

  it("says what to paste when the address is not one of those", async () => {
    const { M } = await start(["gdocs"]);
    for (const url of ["https://example.com/document/d/x", "not a link", link("/forms/d/x")]) {
      expect(await M.readGoogleLink({ url })).toEqual({
        error: "Paste the address of a Google Doc, Sheet or Slides presentation.",
      });
    }
  });

  it("needs the connection of that kind of file", async () => {
    const { M } = await start(["gdocs"]);
    expect(await M.readGoogleLink({ url: link("/spreadsheets/d/s1/edit") })).toEqual({
      error: NOT_CONNECTED("Google Sheets"),
    });
  });
});
