// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the person's sections are the daemon's).
import { ctx, daemon } from "~/testing/daemon-person-store";
import { cleanup, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { wireProfile } from "~/testing/fake-me";
import { AvatarMenu } from "./AvatarMenu";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const AVATAR = "/v1/users/01M3C107JB041061050R3GG2U1/avatar?v=1759233600000";

beforeEach(() => {
  URL.createObjectURL = vi.fn(() => "blob:test/1");
  URL.revokeObjectURL = vi.fn();
});
afterEach(cleanup);

const send = () =>
  daemon.emit("me", "me.updated", {
    profile: daemon.me.profile,
    preferences: daemon.me.preferences,
    progress: daemon.me.progress,
  });
const button = () => screen.getByRole("button", { name: "Profile and settings" });

/** Bumps the profile's `updatedAt` past its current value, the way every real change does. */
const bump = (): string => {
  const at = Date.parse(daemon.me.profile.updatedAt);
  return new Date(Math.max(Date.now(), Number.isNaN(at) ? 0 : at) + 1_000).toISOString();
};

describe("AvatarMenu on the daemon", () => {
  it("shows the initials the daemon made from the name, and follows a change of name", async () => {
    Object.assign(daemon.me.profile, wireProfile({ name: "Ada Okafor", initials: "AO" }));
    send();
    render(() => <AvatarMenu />);
    expect(button()).toHaveTextContent("AO");
    daemon.me.profile.name = "Grace Hopper";
    daemon.me.profile.initials = "GH";
    send();
    await waitFor(() => expect(button()).toHaveTextContent("GH"));
  });

  it("draws the picture when there is an avatar, and the initials again when it is removed", async () => {
    // Set the avatar the way a real change arrives: `updatedAt` bumped, as every write does.
    daemon.me.profile.avatarUrl = AVATAR;
    daemon.me.profile.updatedAt = bump();
    daemon.me.avatar = { type: "image/png", size: 70 };
    send();
    render(() => <AvatarMenu />);
    await waitFor(() =>
      expect(button().querySelector("img")).toHaveAttribute("src", "blob:test/1"),
    );
    expect(button()).not.toHaveTextContent("GH");
    daemon.me.profile.avatarUrl = null;
    daemon.me.profile.updatedAt = bump();
    daemon.me.avatar = null;
    send();
    await waitFor(() => expect(button().querySelector("img")).toBeNull());
    expect(ctx.S.profile.avatar).toBeNull();
  });
});
