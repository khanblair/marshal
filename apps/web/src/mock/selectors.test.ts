import { describe, expect, it } from "vitest";
import { sectionStatus } from "~/data/sections";
import { applyProject } from "~/sync/projects";
import { daemonProject } from "~/testing/projects";
import { testEnv } from "~/testing/test-store";
import { createContext } from "./context";
import { costs } from "./selectors";

// These tests are about the mock's own fabricated money, so they say out loud that the cost section
// (S19b) is still on the mock. The daemon's own stored spend is covered below.
const mockCosts = () => testEnv({ hash: "#nosim", sections: { ...sectionStatus, S19b: "mock" } });

describe("costs for projects with no cost data", () => {
  const store = () => {
    const ctx = createContext(mockCosts());
    applyProject(ctx, daemonProject({ id: "billing" }));
    applyProject(ctx, daemonProject({ id: "ledger" }));
    return ctx;
  };

  it("counts a project the daemon has no month cost for as zero, not as made-up money", () => {
    const ctx = store();
    const only = costs(ctx, "billing");
    expect(only.month).toBe(only.today);
    expect(Number.isNaN(costs(ctx).month)).toBe(false);
  });

  it("adds the month cost of the projects that have one, and skips the rest", () => {
    const ctx = store();
    const billing = ctx.S.projects[0];
    if (!billing) throw new Error("no project");
    billing.monthBase = 10;
    const all = costs(ctx);
    expect(all.month - all.today).toBe(10);
  });
});

describe("costs from the daemon's stored numbers", () => {
  /** A store whose cost section is the daemon's, with one stored day that is today. */
  const store = () => {
    const ctx = createContext(testEnv({ hash: "#nosim" }));
    ctx.S.stats = {
      range: 7,
      days: [{ day: ctx.today, cardsFinished: 0, merges: 0, ciFailures: 0, costMicros: 2_500_000 }],
      projects: [
        {
          projectId: "billing",
          days: [
            { day: ctx.today, cardsFinished: 0, merges: 0, ciFailures: 0, costMicros: 1_000_000 },
          ],
        },
      ],
    };
    return ctx;
  };

  it("reads today's and this month's spend from the stored micro-dollars", () => {
    const ctx = store();
    const all = costs(ctx);
    expect(all.today).toBe(2.5);
    expect(all.month).toBe(2.5);
    expect(costs(ctx, "billing").today).toBe(1);
    expect(costs(ctx, "billing").month).toBe(1);
    // A project the daemon has no days for spends nothing.
    expect(costs(ctx, "ledger").today).toBe(0);
  });

  it("reads a scope with no ceiling as zero, which means no ceiling is set", () => {
    const all = costs(store());
    expect(all.day).toBe(0);
    expect(all.monthL).toBe(0);
    expect(all.awakeL).toBe(0);
  });
});
