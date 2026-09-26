import { describe, expect, it } from "vitest";
import type {
  CreateSavedViewRequest,
  EventBatch,
  MeUpdatedEventData,
  Preferences,
  Profile,
  Progress,
  SavedView,
  SavedViewListSnapshot,
  SavedViewUpdatedEventData,
  UpdatePreferencesRequest,
  UpdateProfileRequest,
  UpdateProgressRequest,
  UpdateSavedViewRequest,
  UserListSnapshot,
} from "../src";
import { MeTopic } from "../src";
import { golden } from "./golden";

// The person using Marshal and a project's saved views (docs/backend-checklist.md B2.2, B2.5, and
// B2.13, inventory N20, N27, and N30). Each sample is checked against the generated type by the
// compiler, and against the daemon's golden file by the assertion, so the two sides cannot drift.
// Every field is set in at least one sample, and both a null and a value appear for each field that
// can be null.

const AVATAR = "/v1/users/01M3C107JB041061050R3GG2U1/avatar?v=1759233600000";

const profile: Profile = {
  id: "01M3C107JB041061050R3GG2U1",
  name: "Ada Okafor",
  email: "ada@example.com",
  initials: "AO",
  timeZone: "Europe/London",
  avatarUrl: AVATAR,
  tailnetIdentity: "",
  createdAt: "2026-09-27T12:00:00.000Z",
  updatedAt: "2026-09-30T12:00:00.000Z",
};

const progress: Progress = {
  onboarding: { status: "pending", step: 2, finishedAt: null },
  tutorial: { status: "skipped", finishedAt: "2026-09-30T11:00:00.000Z" },
};

const preferences: Preferences = {
  theme: "dark",
  listColumns: { cost: false, pkg: true },
  sort: { agents: null, list: { key: "id", direction: "desc" } },
  projects: {
    api: {
      lastView: "list",
      filters: [{ key: "status", value: "needs" }],
      query: "cache",
      swimlane: "role",
      collapsedLanes: ["role:Tester"],
      showAllDone: true,
      savedViewId: "01M3C107JB041061050R3GG2V1",
    },
    web: {
      lastView: "board",
      filters: [],
      query: "",
      swimlane: "none",
      collapsedLanes: [],
      showAllDone: false,
      savedViewId: null,
    },
  },
};

const needsMe: SavedView = {
  id: "01M3C107JB041061050R3GG2V1",
  projectId: "api",
  name: "Needs me",
  filters: [{ key: "status", value: "needs" }],
  swimlane: "none",
  createdAt: "2026-09-28T12:00:00.000Z",
  updatedAt: "2026-09-30T11:00:00.000Z",
};

const byRole: SavedView = {
  id: "01M3C107JB041061050R3GG2V2",
  projectId: "api",
  name: "Claude Code by role",
  filters: [{ key: "agent", value: "Claude Code" }],
  swimlane: "role",
  createdAt: "2026-09-30T11:30:00.000Z",
  updatedAt: "2026-09-30T11:30:00.000Z",
};

describe("the profile golden files", () => {
  it("has a profile with an avatar", () => {
    expect(golden("profile")).toEqual(profile);
  });

  it("has a profile change that sets only what it names, and clears with the empty string", () => {
    const sample: UpdateProfileRequest = {
      name: "Ada Okafor",
      email: "",
      timeZone: "Africa/Lagos",
    };
    expect(golden("update-profile-request")).toEqual(sample);
  });

  it("has the users list, one person with an avatar and one without", () => {
    const sample: UserListSnapshot = {
      users: [
        { id: "01M3C107JB041061050R3GG2U1", name: "Ada Okafor", initials: "AO", avatarUrl: AVATAR },
        { id: "01M3C107JB041061050R3GG2U2", name: "Sam Reyes", initials: "SR", avatarUrl: null },
      ],
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("user-list")).toEqual(sample);
  });
});

describe("the progress and preferences golden files", () => {
  it("has progress with onboarding pending at a screen and the tour skipped", () => {
    expect(golden("progress")).toEqual(progress);
  });

  it("has a progress change that finishes onboarding and replays the tour", () => {
    const sample: UpdateProgressRequest = {
      onboarding: { step: 3, status: "done" },
      tutorial: { status: "pending" },
    };
    expect(golden("update-progress-request")).toEqual(sample);
  });

  it("has preferences with a project that has everything set and one that has the defaults", () => {
    expect(golden("preferences")).toEqual(preferences);
  });

  it("has a preferences change that names one project and clears its saved view", () => {
    const sample: UpdatePreferencesRequest = {
      theme: "light",
      listColumns: { think: true },
      sort: { agents: { key: "state", direction: "asc" } },
      projects: {
        mobile: {
          lastView: "timeline",
          filters: [{ key: "label", value: "ui" }],
          query: "",
          swimlane: "package",
          collapsedLanes: ["package:packages/api-client"],
          showAllDone: false,
          savedViewId: "",
        },
      },
    };
    expect(golden("update-preferences-request")).toEqual(sample);
  });
});

describe("the saved view golden files", () => {
  it("has one saved view", () => {
    expect(golden("saved-view")).toEqual(needsMe);
  });

  it("has a project's saved views in the order they were saved", () => {
    const sample: SavedViewListSnapshot = {
      projectId: "api",
      views: [needsMe, byRole],
      serverTime: "2026-09-30T12:00:00.000Z",
    };
    expect(golden("saved-view-list")).toEqual(sample);
  });

  it("has the requests that save and change a view", () => {
    const create: CreateSavedViewRequest = {
      name: "UI work",
      filters: [{ key: "label", value: "ui" }],
      swimlane: "agent",
    };
    expect(golden("create-saved-view-request")).toEqual(create);
    const update: UpdateSavedViewRequest = { name: "Needs me now", filters: [], swimlane: "label" };
    expect(golden("update-saved-view-request")).toEqual(update);
  });
});

describe("the events golden file", () => {
  it("sends me.updated on the me topic, whole, and saved_view.updated on the project's topic", () => {
    const batch = golden("me-events") as EventBatch;
    expect(batch.events.map((ev) => [ev.type, ev.topic])).toEqual([
      ["me.updated", MeTopic],
      ["saved_view.updated", "project:api"],
    ]);
    const me: MeUpdatedEventData = { profile, preferences, progress };
    expect(batch.events[0]?.data).toEqual(me);
    const views: SavedViewUpdatedEventData = { projectId: "api", views: [needsMe, byRole] };
    expect(batch.events[1]?.data).toEqual(views);
  });
});
