import type { Profile } from "@marshal/protocol";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { golden } from "~/data/testing/golden";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { wireProfile } from "~/testing/fake-me";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import {
  contextOf,
  createSyncedMarshal,
  createTestMarshal,
  DAEMON_PERSON,
  MOCK_PERSON,
} from "~/testing/test-store";
import { profileSyncer } from "./profile";

const ada = golden<Profile>("profile");
const AVATAR = "/v1/users/01M3C107JB041061050R3GG2U1/avatar?v=1759233600000";

let daemon: FakeDaemon | null = null;
const created: string[] = [];
const revoked: string[] = [];

beforeEach(() => {
  created.length = 0;
  revoked.length = 0;
  // jsdom has no object URLs, so these count how many were made and let go.
  URL.createObjectURL = vi.fn(() => {
    const url = `blob:test/${created.length + 1}`;
    created.push(url);
    return url;
  });
  URL.revokeObjectURL = vi.fn((url: string) => {
    revoked.push(url);
  });
});
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const open = (options: Parameters<typeof createFakeDaemon>[0] = {}): FakeDaemon => {
  daemon = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, ...options });
  return daemon;
};

async function synced(d: FakeDaemon) {
  const M = await createSyncedMarshal(d, { sections: DAEMON_PERSON });
  return { M, ctx: contextOf(M) };
}

const avatarCalls = (d: FakeDaemon): string[] =>
  d.routes().filter((route) => route.startsWith("GET /v1/users/") && route.includes("/avatar"));

describe("the profile section", () => {
  it("is section S2a and follows the me topic", () => {
    expect(profileSyncer.section).toBe("S2a");
    expect(profileSyncer.topics).toEqual(["me"]);
  });
});

describe("loading the person", () => {
  it("puts the daemon's profile in the store, with its initials, and keeps the tailnet and devices", async () => {
    const d = open({ profile: wireProfile({ avatarUrl: null }) });
    const { M } = await synced(d);
    expect(M.S.profile).toMatchObject({
      id: ada.id,
      name: "Ada Okafor",
      email: "ada@example.com",
      tz: "Europe/London",
      initials: "AO",
      avatar: null,
      tailnet: "ada@kolaborate.co",
      node: "marshal-laptop.tail3f2a.ts.net",
    });
    expect(M.S.profile.devices.map((device) => device.name)).toEqual(["Pixel 8", "iPad Air"]);
  });

  it("shows a person who has no email or time zone as empty", async () => {
    const d = open({
      profile: wireProfile({ email: "", timeZone: "", name: "Owner", initials: "O" }),
    });
    const { M } = await synced(d);
    expect(M.S.profile).toMatchObject({ name: "Owner", email: "", tz: "", initials: "O" });
  });

  it("puts the daemon's users where the member pickers read the people", async () => {
    const d = open();
    const { M } = await synced(d);
    expect(M.S.people).toEqual([{ id: ada.id, name: "Ada Okafor" }]);
    expect(M.person(ada.id)?.name).toBe("Ada Okafor");
    expect(M.person("blair")).toBeUndefined();
  });

  it("does not show the prototype's people or profile before the daemon answers, nor at all without one", () => {
    const M = createTestMarshal({ sections: DAEMON_PERSON, data: null });
    expect(M.S.people).toEqual([]);
    expect(M.S.profile).toMatchObject({ name: "", email: "", tz: "", avatar: null });
    expect(M.S.profile.tailnet).toBe("ada@kolaborate.co");
  });

  it("keeps the prototype's people and profile while the section is on the mock", () => {
    const M = createTestMarshal({ sections: MOCK_PERSON, data: null });
    expect(M.S.people.map((person) => person.id)).toEqual(["ada", "blair", "godana", "angella"]);
    expect(M.S.profile.name).toBe("Ada Okafor");
    expect(M.S.profile.id).toBeUndefined();
  });
});

describe("me.updated", () => {
  it("changes the profile and the person's name in the list, and changes nothing twice", async () => {
    const d = open();
    const { M } = await synced(d);
    const next = { ...d.me.profile, name: "Ada Lovelace", initials: "AL", email: "" };
    const send = () =>
      d.emit("me", "me.updated", {
        profile: next,
        preferences: d.me.preferences,
        progress: d.me.progress,
      });
    send();
    send();
    expect(M.S.profile).toMatchObject({ name: "Ada Lovelace", initials: "AL", email: "" });
    expect(M.S.people).toEqual([{ id: ada.id, name: "Ada Lovelace" }]);
  });

  it("ignores an event that has no profile", async () => {
    const d = open();
    const { M } = await synced(d);
    d.emit("me", "me.updated", { preferences: d.me.preferences });
    d.emit("me", "other", { profile: { ...d.me.profile, name: "X" } });
    expect(M.S.profile.name).toBe("Ada Okafor");
  });
});

describe("the avatar", () => {
  it("is fetched with the token and drawn from an object URL, once", async () => {
    const d = open({ profile: wireProfile({ avatarUrl: AVATAR }) });
    const { M } = await synced(d);
    await vi.waitFor(() => expect(M.S.profile.avatar).toBe("blob:test/1"));
    expect(avatarCalls(d)).toEqual([`GET ${AVATAR}`]);
    expect(d.calls.find((call) => call.url === AVATAR)?.headers.authorization).toMatch(/^Bearer /);
    // The same address again, as in the echo of an unrelated change, fetches nothing more.
    d.emit("me", "me.updated", {
      profile: d.me.profile,
      preferences: d.me.preferences,
      progress: d.me.progress,
    });
    expect(avatarCalls(d)).toHaveLength(1);
    expect(created).toHaveLength(1);
  });

  it("shows the initials when there is no avatar", async () => {
    const d = open();
    const { M } = await synced(d);
    expect(M.S.profile.avatar).toBeNull();
    expect(avatarCalls(d)).toEqual([]);
    expect(created).toEqual([]);
  });

  it("swaps to a new picture and lets the old object URL go", async () => {
    const d = open({ profile: wireProfile({ avatarUrl: AVATAR }) });
    const { M } = await synced(d);
    await vi.waitFor(() => expect(M.S.profile.avatar).toBe("blob:test/1"));
    const next = `${AVATAR.replace("1759233600000", "1759233700000")}`;
    d.me.profile.avatarUrl = next;
    d.emit("me", "me.updated", {
      profile: d.me.profile,
      preferences: d.me.preferences,
      progress: d.me.progress,
    });
    await vi.waitFor(() => expect(M.S.profile.avatar).toBe("blob:test/2"));
    expect(revoked).toEqual(["blob:test/1"]);
  });

  it("goes back to the initials when the avatar is removed, and lets the object URL go", async () => {
    const d = open({ profile: wireProfile({ avatarUrl: AVATAR }) });
    const { M } = await synced(d);
    await vi.waitFor(() => expect(M.S.profile.avatar).toBe("blob:test/1"));
    d.me.profile.avatarUrl = null;
    d.emit("me", "me.updated", {
      profile: d.me.profile,
      preferences: d.me.preferences,
      progress: d.me.progress,
    });
    expect(M.S.profile.avatar).toBeNull();
    expect(revoked).toEqual(["blob:test/1"]);
  });

  it("shows the initials for a picture that cannot be read, and tries again the next time", async () => {
    const d = open({ profile: wireProfile({ avatarUrl: AVATAR }) });
    d.refuseNext(
      `GET ${AVATAR}`,
      404,
      "not_found",
      "Marshal cannot find that avatar. It may have been removed.",
    );
    const { M } = await synced(d);
    await vi.waitFor(() => expect(avatarCalls(d)).toHaveLength(1));
    expect(M.S.profile.avatar).toBeNull();
    d.emit("me", "me.updated", {
      profile: d.me.profile,
      preferences: d.me.preferences,
      progress: d.me.progress,
    });
    await vi.waitFor(() => expect(M.S.profile.avatar).toBe("blob:test/1"));
    expect(avatarCalls(d)).toHaveLength(2);
  });

  it("drops a slow picture that was asked for before a newer one", async () => {
    const d = open({ profile: wireProfile({ avatarUrl: AVATAR }) });
    const release = d.holdNext(`GET ${AVATAR}`);
    const { M } = await synced(d);
    d.me.profile.avatarUrl = null;
    d.emit("me", "me.updated", {
      profile: d.me.profile,
      preferences: d.me.preferences,
      progress: d.me.progress,
    });
    release();
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(M.S.profile.avatar).toBeNull();
    expect(created).toEqual([]);
  });

  it("lets the object URL go when the store stops following the daemon", async () => {
    const d = open({ profile: wireProfile({ avatarUrl: AVATAR }) });
    const { M, ctx } = await synced(d);
    await vi.waitFor(() => expect(M.S.profile.avatar).toBe("blob:test/1"));
    ctx.sync?.stop();
    expect(revoked).toEqual(["blob:test/1"]);
    expect(M.S.profile.avatar).toBeNull();
  });
});
