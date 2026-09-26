import type { Preferences } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "~/data/testing/golden";
import { contextOf, createTestMarshal, MOCK_PERSON } from "~/testing/test-store";
import {
  changedKeys,
  completeBase,
  defaultBase,
  dropProjectKeys,
  prefsFromWire,
  projectKey,
  readPrefs,
  savedViewIdOf,
  toRequest,
} from "./prefs-model";

const wire = golden<Preferences>("preferences");

/** A store with the prototype's three projects and no daemon: the mock's own person, so the defaults are the mock's. */
function store() {
  const M = createTestMarshal({ sections: MOCK_PERSON });
  return { M, S: contextOf(M).S };
}

describe("reading the preferences from the store", () => {
  it("has every preference of every project, and the theme, the columns, and both sorts", () => {
    const { S } = store();
    const map = readPrefs(S);
    expect(map.get("#theme")).toBe("system");
    expect(map.get("#col:pkg")).toBe(false);
    expect(map.get("#col:title")).toBe(true);
    expect(map.get("#sort:agents")).toEqual({ k: "state", dir: 1 });
    expect(map.get("#sort:list")).toEqual({ k: "id", dir: -1 });
    expect(map.get(projectKey("api", "lastView"))).toBe("board");
    expect(map.get(projectKey("api", "filters"))).toEqual([]);
    expect(map.get(projectKey("mobile", "swim"))).toBe("package");
    expect(map.get(projectKey("web", "lanes"))).toEqual([]);
    expect(map.get(projectKey("web", "showAllDone"))).toBe(false);
    expect(map.get(projectKey("web", "savedViewId"))).toBeNull();
  });

  it("reads a folded lane as the wire writes it, and leaves out one that was unfolded", () => {
    const { S } = store();
    S.laneCollapsed["api:role:Tester"] = true;
    S.laneCollapsed["api:role:Builder"] = false;
    expect(readPrefs(S).get(projectKey("api", "lanes"))).toEqual(["role:Tester"]);
  });

  it("names the saved view in use by its id, and the client's own All cards by none", () => {
    const { S } = store();
    S.savedViews.api = [
      { name: "All cards", f: [], swim: "none" },
      { id: "v1", name: "Needs me", f: [], swim: "none" },
    ];
    S.savedView.api = "All cards";
    expect(savedViewIdOf(S, "api")).toBeNull();
    S.savedView.api = "Needs me";
    expect(savedViewIdOf(S, "api")).toBe("v1");
    S.savedView.api = null;
    expect(savedViewIdOf(S, "api")).toBeNull();
  });
});

/** The daemon's defaults, with the swimlane it holds for the monorepo once it was sent the one the client starts on. */
function settledBase(S: ReturnType<typeof store>["S"]) {
  const base = defaultBase(S);
  base.set(projectKey("mobile", "swim"), "package");
  return base;
}

describe("the defaults", () => {
  it("are the daemon's, so the only thing a fresh store differs in is a monorepo's package swimlane", () => {
    const { S } = store();
    expect(changedKeys(defaultBase(S), readPrefs(S))).toEqual([projectKey("mobile", "swim")]);
    expect(changedKeys(settledBase(S), readPrefs(S))).toEqual([]);
  });

  it("give a project that arrives later the daemon's defaults, whether or not it is a monorepo", () => {
    const { S } = store();
    const base = defaultBase(S);
    const later = { id: "late", name: "late", lang: "Go", path: "/late", packages: ["a", "b"] };
    completeBase(base, [later]);
    expect(base.get(projectKey("late", "swim"))).toBe("none");
    expect(base.get(projectKey("late", "lastView"))).toBe("board");
    dropProjectKeys(base, "late");
    expect(base.has(projectKey("late", "swim"))).toBe(false);
  });
});

describe("comparing", () => {
  it("names only the keys that changed, and compares lists and objects by value", () => {
    const { S } = store();
    const base = settledBase(S);
    S.theme = "dark";
    S.sort.list = { k: "id", dir: -1 };
    S.filters.api = [{ k: "status", v: "needs" }];
    S.filters.web = [];
    expect(changedKeys(base, readPrefs(S)).sort()).toEqual(
      ["#theme", projectKey("api", "filters")].sort(),
    );
  });
});

describe("the daemon's preferences", () => {
  it("are read key by key, with the wire's words turned into the store's", () => {
    const { S } = store();
    S.savedViews.api = [
      { id: "01M3C107JB041061050R3GG2V1", name: "Needs me", f: [], swim: "none" },
    ];
    const map = prefsFromWire(wire, S);
    expect(map.get("#theme")).toBe("dark");
    expect(map.get("#col:cost")).toBe(false);
    expect(map.get("#col:pkg")).toBe(true);
    expect(map.get("#sort:list")).toEqual({ k: "id", dir: -1 });
    expect(map.get(projectKey("api", "filters"))).toEqual([{ k: "status", v: "needs" }]);
    expect(map.get(projectKey("api", "swim"))).toBe("role");
    expect(map.get(projectKey("api", "lanes"))).toEqual(["role:Tester"]);
    expect(map.get(projectKey("api", "savedViewId"))).toBe("01M3C107JB041061050R3GG2V1");
  });

  it("have no opinion on a table that was never sorted or a project with nothing saved", () => {
    const { S } = store();
    const map = prefsFromWire(wire, S);
    expect(map.has("#sort:agents")).toBe(false);
    expect(map.has(projectKey("mobile", "swim"))).toBe(false);
  });

  it("leave out a project the store does not have, and read a saved view that is not there as none", () => {
    const { S } = store();
    const extra: Preferences = {
      ...wire,
      projects: { ...wire.projects, ghost: { ...wire.projects.web! } },
    };
    const map = prefsFromWire(extra, S);
    expect([...map.keys()].some((key) => key.startsWith("ghost:"))).toBe(false);
    // The list of api's saved views has no view of that id.
    expect(map.get(projectKey("api", "savedViewId"))).toBeNull();
  });
});

describe("the request", () => {
  it("sends the theme, the columns, and the sorts in the wire's words", () => {
    const { S } = store();
    const base = settledBase(S);
    S.theme = "dark";
    S.listCols.pkg = true;
    S.listCols.cost = false;
    S.sort.agents = { k: "cost", dir: 1 };
    S.sort.list = { k: "upd", dir: -1 };
    const now = readPrefs(S);
    expect(toRequest(now, changedKeys(base, now))).toEqual({
      theme: "dark",
      listColumns: { pkg: true, cost: false },
      sort: {
        agents: { key: "cost", direction: "asc" },
        list: { key: "upd", direction: "desc" },
      },
    });
  });

  it("sends only the fields of a project that changed, and the empty string for a view let go", () => {
    const { S } = store();
    S.savedViews.api = [{ id: "v1", name: "Needs me", f: [], swim: "none" }];
    S.savedView.api = "Needs me";
    const base = settledBase(S);
    base.set(projectKey("api", "savedViewId"), "v1");
    S.savedView.api = null;
    S.filters.api = [{ k: "role", v: "Tester" }];
    S.laneCollapsed["api:role:Tester"] = true;
    S.showAllDone.web = true;
    S.query.web = "cache";
    S.lastView.web = "list";
    S.swim.web = "agent";
    const now = readPrefs(S);
    expect(toRequest(now, changedKeys(base, now))).toEqual({
      projects: {
        api: {
          savedViewId: "",
          filters: [{ key: "role", value: "Tester" }],
          collapsedLanes: ["role:Tester"],
        },
        web: { showAllDone: true, query: "cache", lastView: "list", swimlane: "agent" },
      },
    });
  });
});
