import type { Profile, Progress, SavedView, User } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "~/data/testing/golden";
import {
  ALL_CARDS,
  toLaneKey,
  toOnboardingState,
  toPerson,
  toSortSpec,
  toStoredFilters,
  toStoredProfile,
  toStoredView,
  toViewList,
  toWireFilters,
  toWireLanes,
  toWireSort,
} from "./me-mapper";

const view = golden<SavedView>("saved-view");

describe("sort", () => {
  it("turns asc into 1 and desc into -1, and back", () => {
    expect(toSortSpec({ key: "id", direction: "desc" }, { k: "state", dir: 1 })).toEqual({
      k: "id",
      dir: -1,
    });
    expect(toSortSpec({ key: "cost", direction: "asc" }, { k: "state", dir: 1 })).toEqual({
      k: "cost",
      dir: 1,
    });
    expect(toWireSort({ k: "id", dir: -1 })).toEqual({ key: "id", direction: "desc" });
    expect(toWireSort({ k: "cost", dir: 1 })).toEqual({ key: "cost", direction: "asc" });
  });

  it("round trips both ways", () => {
    for (const spec of [
      { k: "id", dir: -1 },
      { k: "state", dir: 1 },
    ]) {
      expect(toSortSpec(toWireSort(spec), { k: "x", dir: 1 })).toEqual(spec);
    }
    for (const order of [
      { key: "id", direction: "desc" },
      { key: "upd", direction: "asc" },
    ] as const) {
      expect(toWireSort(toSortSpec(order, { k: "x", dir: 1 }))).toEqual(order);
    }
  });

  it("keeps the table's own default when the daemon has none, and never shares it", () => {
    const fallback = { k: "state", dir: 1 };
    const spec = toSortSpec(null, fallback);
    expect(spec).toEqual(fallback);
    expect(spec).not.toBe(fallback);
    expect(toSortSpec(undefined, fallback)).toEqual(fallback);
  });
});

describe("filters", () => {
  it("renames key and value to k and v, and back", () => {
    const wire = [
      { key: "status", value: "needs" },
      { key: "label", value: "ui" },
    ] as const;
    const stored = toStoredFilters(wire);
    expect(stored).toEqual([
      { k: "status", v: "needs" },
      { k: "label", v: "ui" },
    ]);
    expect(toWireFilters(stored)).toEqual(wire);
  });
});

describe("folded lanes", () => {
  it("lists a project's folded lanes without the project, sorted, and skips unfolded ones", () => {
    const folded = {
      "api:role:Tester": true,
      "api:role:Builder": true,
      "api:agent:Codex": false,
      "web:role:Tester": true,
      "apis:role:X": true,
    };
    expect(toWireLanes("api", folded)).toEqual(["role:Builder", "role:Tester"]);
    expect(toWireLanes("web", folded)).toEqual(["role:Tester"]);
    expect(toWireLanes("none", folded)).toEqual([]);
  });

  it("puts the project back in front of a lane, so it round trips", () => {
    expect(toLaneKey("api", "role:Tester")).toBe("api:role:Tester");
    const key = toLaneKey("api", "role:Tester");
    expect(toWireLanes("api", { [key]: true })).toEqual(["role:Tester"]);
  });
});

describe("saved views", () => {
  it("keeps the id and turns filters and the swimlane into the menu's names", () => {
    expect(toStoredView(view)).toEqual({
      id: view.id,
      name: "Needs me",
      f: [{ k: "status", v: "needs" }],
      swim: "none",
    });
  });

  it("puts the client's own All cards first when the daemon has no view of that name", () => {
    const list = toViewList([view]);
    expect(list.map((one) => one.name)).toEqual([ALL_CARDS, "Needs me"]);
    expect(list[0]).toEqual({ name: ALL_CARDS, f: [], swim: "none" });
    expect(list[0]?.id).toBeUndefined();
    expect(toViewList([]).map((one) => one.name)).toEqual([ALL_CARDS]);
  });

  it("does not list All cards twice when the daemon has one, whatever its case or spaces", () => {
    const mine = { ...view, id: "01M3C107JB041061050R3GG2V9", name: " all cards " };
    const list = toViewList([view, mine]);
    expect(list.map((one) => one.name)).toEqual(["Needs me", " all cards "]);
    expect(list.every((one) => one.id !== undefined)).toBe(true);
  });
});

describe("the profile and the people", () => {
  const profile = golden<Profile>("profile");
  const users = golden<{ users: User[] }>("user-list").users;

  it("reads the time zone as tz and keeps the daemon's initials and id", () => {
    expect(toStoredProfile(profile)).toEqual({
      id: profile.id,
      name: "Ada Okafor",
      email: "ada@example.com",
      tz: "Europe/London",
      initials: "AO",
    });
  });

  it("reads a person as an id and a name", () => {
    expect(users.map(toPerson)).toEqual([
      { id: users[0]?.id, name: "Ada Okafor" },
      { id: users[1]?.id, name: "Sam Reyes" },
    ]);
  });
});

describe("the onboarding progress", () => {
  const progress = (
    onboarding: Progress["onboarding"]["status"],
    tutorial: Progress["tutorial"]["status"],
    step = 0,
  ): Progress => ({
    onboarding: { status: onboarding, step, finishedAt: null },
    tutorial: { status: tutorial, finishedAt: null },
  });

  it("shows onboarding while it is pending, at the step it stopped at, and no tour yet", () => {
    expect(toOnboardingState(progress("pending", "pending", 2))).toEqual({
      onboarding: true,
      obStep: 2,
      tourPending: false,
    });
  });

  it("runs the tour once onboarding is finished or skipped and the tour is pending", () => {
    expect(toOnboardingState(progress("done", "pending"))).toMatchObject({
      onboarding: false,
      tourPending: true,
    });
    expect(toOnboardingState(progress("skipped", "pending")).tourPending).toBe(true);
  });

  it("runs no tour once it was finished or skipped", () => {
    expect(toOnboardingState(progress("done", "done")).tourPending).toBe(false);
    expect(toOnboardingState(progress("done", "skipped")).tourPending).toBe(false);
  });
});
