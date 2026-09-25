import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { cardLabel } from "~/mock/card-key";
import { ciInfo, hasCi, homeCiAgo, runAgo, runsOf } from "./ci-rows";
import { type HomeSnapshot, homeSnapshot, resetHome } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const snapshot: HomeSnapshot = homeSnapshot();

beforeEach(() => {
  vi.useFakeTimers();
  resetHome(snapshot);
});
afterEach(() => {
  vi.useRealTimers();
});

describe("homeCiAgo", () => {
  it("says minutes under an hour, hours after, and nothing for zero", () => {
    expect(homeCiAgo(0)).toBe("");
    expect(homeCiAgo(2)).toBe("2 min ago");
    expect(homeCiAgo(59)).toBe("59 min ago");
    expect(homeCiAgo(60)).toBe("1 h ago");
    expect(homeCiAgo(150)).toBe("3 h ago");
    expect(homeCiAgo(3000)).toBe("50 h ago");
  });
});

describe("runAgo", () => {
  it("says minutes, hours, then days", () => {
    expect(runAgo(1)).toBe("1 min ago");
    expect(runAgo(59)).toBe("59 min ago");
    expect(runAgo(95)).toBe("2 h ago");
    expect(runAgo(1439)).toBe("24 h ago");
    expect(runAgo(1440)).toBe("1 days ago");
    expect(runAgo(4320)).toBe("3 days ago");
  });
});

describe("ciInfo", () => {
  it("describes each state and falls back to queued", () => {
    expect(ciInfo("passed").label).toBe("Passed");
    expect(ciInfo("failed").label).toBe("Failed");
    expect(ciInfo("nonsense" as never)).toBe(M.CI.queued);
  });
});

describe("runsOf", () => {
  it("joins the workflow runs on main with the runs of the project's cards, newest first", () => {
    const project = M.proj("mobile");
    if (!project) throw new Error("seed has no mobile project");
    const runs = runsOf(project);
    const minutes = runs.map((r) => r.minutesAgo);
    expect(minutes).toEqual([...minutes].sort((a, b) => a - b));
    expect(runs.filter((r) => r.where === "main").map((r) => r.name)).toEqual([
      "android apps/android",
      "ios apps/ios",
      "packages packages/*",
    ]);
    const cardRuns = runs.filter((r) => r.where !== "main");
    expect(cardRuns.length).toBe(M.cardsOf("mobile").filter((c) => c.ci).length);
    for (const run of cardRuns) expect(run.where).toMatch(/^#\d+ /);
  });

  it("never reports a card run as less than a minute old", () => {
    const project = M.proj("api");
    const card = M.cardsOf("api").find((c) => c.ci);
    if (!project || !card) throw new Error("seed has no api card with CI");
    card.upd = M.now();
    const own = runsOf(project).find((r) => r.where.startsWith(`${cardLabel(card)} `));
    expect(own?.minutesAgo).toBe(1);
  });

  it("opens the board for a workflow run and the card for a card run", () => {
    const project = M.proj("api");
    if (!project) throw new Error("seed has no api project");
    const runs = runsOf(project);
    runs.find((r) => r.where === "main")?.open();
    expect(M.S.route).toMatchObject({ page: "project", pid: "api", view: "board" });
    const own = runs.find((r) => r.where !== "main");
    own?.open();
    const owner = M.cardsOf("api").find((c) => own?.where.startsWith(`${cardLabel(c)} `));
    expect(M.S.openId).toBe(owner?.id);
  });
});

describe("a project with no CI data", () => {
  it("is told apart by its missing state, and has no runs of its own", () => {
    const bare = { id: "billing", name: "billing", lang: "Go", path: "~/code/billing" };
    expect(hasCi(bare)).toBe(false);
    expect(hasCi({ ...bare, ci: "passed" })).toBe(true);
    expect(runsOf(bare)).toEqual([]);
    expect(homeCiAgo(undefined)).toBe("");
  });
});
