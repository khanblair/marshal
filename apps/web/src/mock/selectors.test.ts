import { describe, expect, it } from "vitest";
import { applyProject } from "~/sync/projects";
import { daemonProject } from "~/testing/projects";
import { testEnv } from "~/testing/test-store";
import { createContext } from "./context";
import { costs } from "./selectors";

describe("costs for projects with no cost data", () => {
  const store = () => {
    const ctx = createContext(testEnv({ hash: "#nosim" }));
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
