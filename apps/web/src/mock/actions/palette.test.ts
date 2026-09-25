import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FIXED_TIME, makeTwin, SLOW_TEST_MS, type Twin } from "../testing/twin";

describe("command palette", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  const labels = (): string[] => t.port.commands().map((c) => c.label);

  /** Runs the command with this label (as the prototype words it) on both stores. */
  const runBoth = (label: string): void => {
    // The port names a card with its project, `api-gateway #41`; the prototype writes `#41`.
    const port = t.port.commands().find((c) => t.shape(c.label) === label);
    const proto = (t.proto.call("commands") as { label: string; run: () => void }[]).find(
      (c) => c.label === label,
    );
    if (!port || !proto) throw new Error(`no command "${label}"`);
    proto.run();
    port.run();
  };

  it("composes groups in the prototype's order", () => {
    const groups = [...new Set(t.port.commands().map((c) => c.group))];
    expect(groups).toEqual(["Actions", "Projects", "Cards", "Settings"]);
    expect(labels().slice(0, 7)).toEqual([
      "New card",
      "Switch to chats view",
      "Switch to agents view",
      "Switch to board view",
      "Switch to list view",
      "Switch to timeline view",
      "Switch to calendar view",
    ]);
    expect(labels()).toContain("Merge #36 Remove deprecated v1 routes");
    expect(labels()).not.toContain("Fork #41");
    expect(t.port.commands()[1]?.kbd).toEqual(["⌘", "1"]);
  });

  it("adds card commands for the open card", () => {
    t.run("openCard", "web#115");
    expect(labels()).toEqual(
      expect.arrayContaining(["Pin #115", "Resume session on #115", "Fork #115"]),
    );
    t.run("openCard", "api#41");
    expect(labels()).toEqual(expect.arrayContaining(["Sleep #41"]));
  });

  it.each([
    "New card",
    "Switch to list view",
    "Go home",
    "New project",
    "New chat",
    "Replay tour",
    "Merge #36 Remove deprecated v1 routes",
    "Show notices",
    "web-dashboard",
    "#118 Dark mode for settings page",
    "Provider keys",
    "Use dark theme",
    "Use light theme",
    "Use system theme",
  ])("runs %s like the prototype", (label) => {
    runBoth(label);
    t.same();
  });

  it("runs view switches inside a project and card commands", () => {
    t.run("go", "project", "api");
    runBoth("Switch to timeline view");
    runBoth("New card");
    t.run("openCard", "api#44");
    runBoth("Pin #44");
    runBoth("Fork #44");
    runBoth("Approve on #44");
    t.run("openCard", "web#115");
    runBoth("Resume session on #115");
    t.run("openCard", "api#39");
    runBoth("Sleep #39");
    t.same();
    t.play(3000, 250);
  });

  it("simulates the CI failure from the palette", () => {
    runBoth("Simulate CI failure on #40");
    t.same();
    t.play(15_000, 500);
  });
});
