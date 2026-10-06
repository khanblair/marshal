import type { GoogleFile, GoogleFiles, GoogleLinkContent } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { failureOf, make, TOKEN } from "./api-client-test-helpers";
import { errorAnswer, jsonAnswer } from "./testing/fake-fetch";

// The routes of Google Drive, Docs, Sheets and Slides (sections S29i to S29l), one typed method each.

const file: GoogleFile = {
  id: "doc-1",
  name: "Plan",
  kind: "doc",
  url: "https://docs.google.com/document/d/doc-1/edit",
};
const files: GoogleFiles = { files: [file], folder: "Marshal" };
const content: GoogleLinkContent = {
  kind: "doc",
  id: "doc-1",
  title: "Plan",
  url: file.url,
  markdown: "# Plan",
  truncated: false,
};
const CREATED = 201;

const routes = {
  "GET /v1/integrations/gdocs/authorize": () =>
    jsonAnswer({ url: "https://accounts.google.com/x" }),
  "GET /v1/google/files": () => jsonAnswer(files),
  "GET /v1/google/files?kind=slides": () => jsonAnswer({ files: [], folder: "Marshal" }),
  "POST /v1/google/docs": () => jsonAnswer(file, CREATED),
  "POST /v1/google/sheets": () => jsonAnswer({ ...file, kind: "sheet" }, CREATED),
  "POST /v1/google/slides": () => jsonAnswer({ ...file, kind: "slides" }, CREATED),
  "POST /v1/google/drive/files": () => jsonAnswer({ ...file, kind: "file" }, CREATED),
  "POST /v1/google/read": () => jsonAnswer(content),
  "PUT /v1/integrations/gdrive": () => jsonAnswer({ integrations: [], serverTime: "" }),
};

describe("the Google file routes", () => {
  it("asks for one service's consent address, with the id in the path", async () => {
    const { client, calls } = make(routes);
    expect(await client.authorizeGoogleService("gdocs")).toEqual({
      url: "https://accounts.google.com/x",
    });
    expect(calls[0]).toMatchObject({
      method: "GET",
      url: "/v1/integrations/gdocs/authorize",
      body: null,
    });
    expect(calls[0]?.headers.authorization).toBe(`Bearer ${TOKEN}`);
  });

  it("escapes the id it puts in the consent path", async () => {
    const { client, calls } = make({
      "GET /v1/integrations/a%2Fb/authorize": () => jsonAnswer({ url: "x" }),
    });
    await client.authorizeGoogleService("a/b");
    expect(calls[0]?.url).toBe("/v1/integrations/a%2Fb/authorize");
  });

  it("lists the files with no query for every kind, and one kind in the query", async () => {
    const { client, calls } = make(routes);
    expect(await client.googleFiles()).toEqual(files);
    expect(await client.googleFiles("slides")).toEqual({ files: [], folder: "Marshal" });
    expect(calls.map((call) => `${call.method} ${call.url}`)).toEqual([
      "GET /v1/google/files",
      "GET /v1/google/files?kind=slides",
    ]);
  });

  it("sends each create as JSON to its own route and answers the file", async () => {
    const { client, calls } = make(routes);
    expect(await client.createGoogleDoc({ title: "Plan", html: "<p>x</p>" })).toEqual(file);
    expect(await client.createGoogleSheet({ title: "Board", rows: [["a"]] })).toMatchObject({
      kind: "sheet",
    });
    expect(
      await client.createGoogleSlides({ title: "Deck", slides: [{ title: "Hi", bullets: [] }] }),
    ).toMatchObject({ kind: "slides" });
    expect(await client.uploadGoogleFile({ name: "n.md", content: "x" })).toMatchObject({
      kind: "file",
    });
    expect(calls.map((call) => `${call.method} ${call.url}`)).toEqual([
      "POST /v1/google/docs",
      "POST /v1/google/sheets",
      "POST /v1/google/slides",
      "POST /v1/google/drive/files",
    ]);
    expect(JSON.parse(calls[0]?.body ?? "")).toEqual({ title: "Plan", html: "<p>x</p>" });
    expect(calls[0]?.headers["content-type"]).toBe("application/json");
  });

  it("sends the link to read and answers its markdown", async () => {
    const { client, calls } = make(routes);
    expect(await client.readGoogleLink({ url: file.url })).toEqual(content);
    expect(JSON.parse(calls[0]?.body ?? "")).toEqual({ url: file.url });
  });

  it("saves Drive's folder through the generic save route", async () => {
    const { client, calls } = make(routes);
    await client.saveIntegration("gdrive", { folder: "Work" });
    expect(calls[0]).toMatchObject({ method: "PUT", url: "/v1/integrations/gdrive" });
    expect(JSON.parse(calls[0]?.body ?? "")).toEqual({ folder: "Work" });
  });

  it("fails with the daemon's own sentence when it refuses", async () => {
    const sentence =
      "Google Docs is not connected yet. Connect it in Settings, under Integrations.";
    const { client } = make({
      "GET /v1/google/files?kind=doc": () => errorAnswer(422, "refused", sentence),
    });
    const error = await failureOf(client.googleFiles("doc"));
    expect(error.message).toBe(sentence);
  });
});
