import type { Card as WireCard } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import type { Marshal } from "~/mock";
import { cardId, wireCard } from "~/testing/fake-cards";
import { createFakeDaemon } from "~/testing/fake-daemon";
import { wireProject } from "~/testing/projects";
import { contextOf, createSyncedMarshal } from "~/testing/test-store";
import { previewOf, startCardPreview, stopCardPreview, takeCardPreviewShot } from "./preview";

/*
 * A card's live preview, through the store the app writes with (section S13, docs/backend-checklist.md
 * B6.6 and B6.7). The daemon owns whether a server runs, on which port, and which shots exist, so the
 * store only ever holds what the daemon last answered - the start and stop routes answer the preview
 * as it is now (a `PreviewSnapshot`, whose `preview` is what the tab draws). Two cards are previewed
 * at once here, each on its own port, which is the one property the phase promises they never share.
 *
 * The daemon's own shooter sits behind a fake (`~/testing/fake-previews`), so no browser is launched.
 */

/** A project Marshal can preview: it has a dev command. The prototype's own projects have none. */
const WEB = wireProject({ id: "web", name: "web-dashboard", devCommand: "pnpm dev" });

const cardOf = (number: number): WireCard =>
  wireCard({ projectId: "web", number, title: `Card ${String(number)}`, state: "working" });

const key = (number: number): string => `web#${String(number)}`;

/** A store following a daemon that holds the given cards of the previewable project. */
async function setup(cards: readonly WireCard[] = [cardOf(118), cardOf(119)]) {
  const d = createFakeDaemon({ projects: [WEB], cards });
  const M = await createSyncedMarshal(d);
  return { M, ctx: contextOf(M), d };
}

const said = (M: Marshal): string => M.S.toasts.at(-1)?.msg ?? "";

describe("a card's preview in the store", () => {
  it("previews two cards at once, each on its own port, without shared state", async () => {
    const { ctx, d } = await setup();
    try {
      expect(await startCardPreview(ctx, key(118))).toBe(true);
      expect(await startCardPreview(ctx, key(119))).toBe(true);
      expect(previewOf(ctx, key(118))).toMatchObject({
        state: "running",
        port: 5100,
        url: "http://127.0.0.1:5100",
        command: "pnpm dev",
      });
      expect(previewOf(ctx, key(119))).toMatchObject({
        state: "running",
        port: 5101,
        url: "http://127.0.0.1:5101",
      });
      // Stopping one leaves the other running, on its own port.
      expect(await stopCardPreview(ctx, key(118))).toBe(true);
      expect(previewOf(ctx, key(118))?.state).toBe("stopped");
      expect(previewOf(ctx, key(119))?.state).toBe("running");
    } finally {
      d.data.stop();
    }
  });

  it("keeps a refused start out of the store and shows the daemon's own sentence", async () => {
    const commandless = wireProject({ id: "api", name: "api-gateway", devCommand: "" });
    const card = wireCard({ projectId: "api", number: 41, title: "Fix it", state: "working" });
    const d = createFakeDaemon({ projects: [commandless], cards: [card] });
    const M = await createSyncedMarshal(d);
    try {
      const ctx = contextOf(M);
      expect(await startCardPreview(ctx, "api#41")).toBe(false);
      expect(previewOf(ctx, "api#41")).toBeUndefined();
      expect(said(M)).toBe(
        "This project has no dev command, so Marshal does not know how to start it. Add one in project settings.",
      );
    } finally {
      d.data.stop();
    }
  });

  it("takes a screenshot through the daemon, keeps it on the card, and says so", async () => {
    const { M, ctx, d } = await setup([cardOf(118)]);
    try {
      await startCardPreview(ctx, key(118));
      expect(await takeCardPreviewShot(ctx, key(118), "before")).toBe(true);
      expect(previewOf(ctx, key(118))?.shots.map((shot) => shot.kind)).toEqual(["before"]);
      expect(d.bodies(`POST /v1/cards/${cardId(118)}/preview/shots`)).toEqual([{ kind: "before" }]);
      expect(said(M)).toBe("The screenshot was taken.");
    } finally {
      d.data.stop();
    }
  });

  it("refuses a screenshot of a preview that is not running, and changes nothing", async () => {
    const { ctx, d } = await setup([cardOf(118)]);
    try {
      expect(await takeCardPreviewShot(ctx, key(118), "before")).toBe(false);
      expect(previewOf(ctx, key(118))).toBeUndefined();
    } finally {
      d.data.stop();
    }
  });
});
