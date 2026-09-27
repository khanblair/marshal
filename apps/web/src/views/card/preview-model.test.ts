import type { Preview } from "@marshal/protocol";
import { beforeEach, describe, expect, it } from "vitest";
import { M } from "~/mock";
import { DARK_PREVIEW_CARD_ID, previewState, previewStatus, previewUrl } from "./preview-model";
import { resetStore } from "./test-helpers";

/*
 * The Preview tab's own read of a card's preview (section S13). The daemon owns the state, the
 * address, and the command, so these are the words for what the daemon last said — not the mock's
 * old stand-in, which computed a port from the card's id and a made-up URL.
 */

const preview = (fields: Partial<Preview>): Preview => ({
  cardId: "card_web119",
  state: "stopped",
  url: "",
  port: 0,
  command: "",
  startedAt: null,
  error: "",
  shots: [],
  ...fields,
});

const STOPPED = "Stopped. Start the preview to run the app from this card's worktree.";

describe("preview model", () => {
  beforeEach(() => resetStore());

  it("draws a card nothing has been read for yet as stopped, with no address", () => {
    expect(previewState("web#119")).toBe("stopped");
    expect(previewUrl("web#119")).toBe("");
    expect(previewStatus("web#119")).toBe(STOPPED);
  });

  it("shows the daemon's own address and command while the preview is running", () => {
    M.S.preview = {
      "web#119": preview({
        state: "running",
        url: "http://127.0.0.1:5103",
        port: 5103,
        command: "pnpm dev",
      }),
    };
    expect(previewState("web#119")).toBe("running");
    expect(previewUrl("web#119")).toBe("http://127.0.0.1:5103");
    expect(previewStatus("web#119")).toBe("Running pnpm dev on port 5103");
  });

  it("still names the port when the project has no dev command worth showing", () => {
    M.S.preview = { "web#119": preview({ state: "running", port: 5103 }) };
    expect(previewStatus("web#119")).toBe("Running on port 5103");
  });

  it("says a preview is starting, with the command it was started with", () => {
    M.S.preview = { "web#119": preview({ state: "starting", port: 5103, command: "pnpm dev" }) };
    expect(previewState("web#119")).toBe("starting");
    expect(previewUrl("web#119")).toBe("");
    expect(previewStatus("web#119")).toBe("Starting the dev server with pnpm dev");
  });

  it("says a preview that could not start is stopped, in the daemon's own sentence", () => {
    M.S.preview = {
      "web#119": preview({ state: "stopped", error: "The dev server stopped before it answered." }),
    };
    expect(previewState("web#119")).toBe("stopped");
    expect(previewStatus("web#119")).toBe("The dev server stopped before it answered.");
  });

  it("keeps card #118 as the one that previews a dark page", () => {
    expect(DARK_PREVIEW_CARD_ID).toBe("web#118");
  });
});
