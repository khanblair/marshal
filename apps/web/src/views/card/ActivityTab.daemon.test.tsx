// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (S10 is the daemon's).
import { daemon } from "~/testing/daemon-cards-store";
import type { Checkpoint } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toCheckpointRows } from "~/data/mappers/checkpoints";
import { M } from "~/mock";
import { ActivityTab } from "./ActivityTab";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * The Activity tab's restore points once section S10 is the daemon's: the list is the daemon's own
 * commits, and the Restore button asks first, then calls the daemon. There is no prototype for this
 * half of the tab, so the copy the dialog shows is the ruling the phase report records.
 */

const OPEN = "api#41";
const SHA = "9f2c1b7a4e6d85031234567890abcdef01234567";
const MINUTE = 60_000;

const daemonId = (): string => cardOf(OPEN).daemonId ?? "";

const restoreRoute = (id: string): string =>
  `POST /v1/cards/${encodeURIComponent(daemonId())}/checkpoints/${id}/restore`;

const cp = (fields: Partial<Checkpoint> = {}): Checkpoint => ({
  id: "01JD7Q4M2X8K9V0P5T3RB6NHAE",
  cardId: daemonId(),
  sha: SHA,
  label: "before turn 3",
  createdAt: new Date(Date.now() - 12 * MINUTE).toISOString(),
  ...fields,
});

/** Hands the daemon a card's restore points, and the store the same list the way a read would. */
const open = (list: readonly Checkpoint[]): void => {
  daemon.checkpoints[daemonId()] = list.map((one) => ({ ...one }));
  M.S.checkpoints[OPEN] = toCheckpointRows({
    cardId: daemonId(),
    checkpoints: [...list],
    serverTime: new Date().toISOString(),
  });
  render(() => <ActivityTab card={cardOf(OPEN)} c={M.deco(cardOf(OPEN))} />);
};

beforeEach(() => {
  resetStore();
  daemon.calls.splice(0, daemon.calls.length);
});
afterEach(cleanup);

describe("the daemon's restore points", () => {
  it("draws each one with the commit named short and its time", () => {
    open([cp()]);
    expect(screen.getByText("before turn 3")).toBeTruthy();
    expect(screen.getByText("9f2c1b7a")).toBeTruthy();
    expect(screen.getByText("12 min ago")).toBeTruthy();
  });

  it("names one the daemon left unnamed, so the row is never blank", () => {
    open([cp({ label: "" })]);
    expect(screen.getByText("Restore point")).toBeTruthy();
  });

  it("has no rows, and no Restore button, when the card has no restore points", () => {
    open([]);
    expect(screen.queryByText("Restore")).toBeNull();
  });

  it("asks first, saying plainly that a restore discards what came after", () => {
    open([cp()]);
    fireEvent.click(screen.getByText("Restore"));
    expect(M.S.dialog?.title).toBe("Restore checkpoint");
    expect(M.S.dialog?.message).toContain('"before turn 3"');
    expect(M.S.dialog?.message).toContain("discarded");
  });

  it("calls the daemon's restore route when the person confirms, and says so", async () => {
    open([cp()]);
    fireEvent.click(screen.getByText("Restore"));
    M.S.dialog?.run();
    // The route is recorded as the request goes out, before the daemon's answer is handled, so the
    // sentence is what to wait on; the route is asserted after it has arrived.
    await vi.waitFor(() => expect(M.S.toasts.at(-1)?.msg).toBe("Checkpoint restored"));
    expect(daemon.routes()).toContain(restoreRoute(cp().id));
  });
});
