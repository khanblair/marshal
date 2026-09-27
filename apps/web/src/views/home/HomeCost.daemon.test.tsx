// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the cost numbers and the limits are the daemon's).
import { ctx, daemon } from "~/testing/daemon-limits-store";
import { cleanup, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createLimitsStore } from "~/testing/fake-limits";
import { M } from "~/mock";
import { applyLimitList } from "~/sync/limits";
import { HomeView } from "./HomeView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * Section S19b: Home's cost tile and cost chart against the fake daemon. The numbers are the
 * daemon's stored days, in `M.S.stats` — the mock's fabricated spend and its four seeded ceilings
 * are gone — so the same stored day drives both, and a ceiling the daemon has not set leaves no
 * limit line, no legend entry, and no "of $X" in the tile. Rendering all of Home in jsdom is slow.
 */
const SLOW_TEST_MS = 30_000;

/** The daemon's stored days, as the syncer would have put them in the store. */
const storedStats = (todayMicros: number, projectMicros: number) => ({
  range: 7,
  days: [{ day: ctx.today, cardsFinished: 2, merges: 1, ciFailures: 0, costMicros: todayMicros }],
  projects: [
    {
      projectId: "api",
      days: [
        { day: ctx.today, cardsFinished: 1, merges: 0, ciFailures: 0, costMicros: projectMicros },
      ],
    },
  ],
});

const tile = (name: RegExp) => screen.getByRole("button", { name });

beforeEach(() => {
  // A fresh install: the daemon has no ceiling at all until the test gives it one.
  Object.assign(daemon.limits, createLimitsStore({ limits: [] }));
  applyLimitList(ctx, { limits: [] });
  M.S.stats = storedStats(12_340_000, 5_000_000);
  M.set({ dashRange: 7, toasts: [], dialog: null });
});
afterEach(cleanup);

describe("Home's cost on the daemon", { timeout: SLOW_TEST_MS }, () => {
  it("shows the daemon's stored spend alone, with no ceiling furniture anywhere", () => {
    render(() => <HomeView />);
    expect(tile(/Cost today$/)).toHaveTextContent("$12.34Cost today");
    expect(screen.queryByText(/^Cost today of/)).not.toBeInTheDocument();
    expect(screen.queryByText("Daily limit")).not.toBeInTheDocument();
    expect(screen.queryByText(/^Limit \$/)).not.toBeInTheDocument();
    const chart = screen.getByRole("img", { name: "All projects today $12.34" });
    expect(chart).toBeInTheDocument();
    // The chart is keyed by the store's projects, so its legend names them and nothing else.
    const legend = chart.closest("figure");
    if (!legend) throw new Error("the cost chart has no figure");
    expect(within(legend).getAllByText("api-gateway").length).toBeGreaterThan(0);
    expect(within(legend).getAllByText("mobile-app").length).toBeGreaterThan(0);
  });

  it("draws the daemon's own ceiling and names it when the daemon has one", () => {
    Object.assign(
      daemon.limits,
      createLimitsStore({ limits: [{ scope: "global", kind: "cost-day", value: 15_000_000 }] }),
    );
    applyLimitList(ctx, { limits: daemon.limits.limits });
    render(() => <HomeView />);
    // The ceiling and the spend both come from the daemon, so the tile says which ceiling it is.
    expect(tile(/Cost today of/)).toHaveTextContent("$12.34Cost today of $15.00");
    expect(
      screen.getByRole("img", { name: "All projects today $12.34 of the $15.00 daily limit" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Daily limit")).toBeInTheDocument();
    expect(screen.getByText("Limit $15.00")).toBeInTheDocument();
  });
});
