// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the person's sections are the daemon's).
import { chooser, ctx, daemon } from "~/testing/daemon-person-store";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { wireProfile } from "~/testing/fake-me";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const made: string[] = [];

beforeEach(() => {
  made.length = 0;
  URL.createObjectURL = vi.fn(() => {
    made.push(`blob:test/${made.length + 1}`);
    return made.at(-1) as string;
  });
  URL.revokeObjectURL = vi.fn();
  // Every test starts from the golden person, the way the daemon was made.
  Object.assign(daemon.me.profile, wireProfile());
  daemon.me.avatar = null;
  daemon.emit("me", "me.updated", {
    profile: daemon.me.profile,
    preferences: daemon.me.preferences,
    progress: daemon.me.progress,
  });
  M.set({ settingsSection: "profile", toasts: [] });
});
afterEach(() => {
  cleanup();
  chooser.pick.mockReset();
});

const nameField = () => screen.getByLabelText<HTMLInputElement>("Name");
const emailField = () => screen.getByLabelText<HTMLInputElement>("Email", { exact: false });
const zoneField = () => screen.getByLabelText<HTMLSelectElement>("Time zone", { exact: false });
const saveButton = () => screen.getByRole("button", { name: "Save profile" });
const toasts = () => M.S.toasts.map((toast) => toast.msg);

describe("the Profile section on the daemon", () => {
  it("shows the daemon's profile, with its initials, and keeps the paired devices and the tailnet the mock's", () => {
    render(() => <SettingsView />);
    expect(nameField()).toHaveValue("Ada Okafor");
    expect(emailField()).toHaveValue("ada@example.com");
    expect(zoneField()).toHaveValue("Europe/London");
    expect(saveButton()).toBeDisabled();
    expect(screen.getByLabelText("Your avatar")).toHaveTextContent("AO");
    expect(screen.getByText("Pixel 8")).toBeInTheDocument();
    expect(screen.getByText("ada@kolaborate.co")).toBeInTheDocument();
  });

  it("saves what was changed on the daemon, and then shows what the daemon answered", async () => {
    render(() => <SettingsView />);
    fireEvent.input(nameField(), { target: { value: "  Grace Hopper " } });
    expect(screen.getByLabelText("Your avatar")).toHaveTextContent("GH");
    fireEvent.click(saveButton());
    await waitFor(() => expect(toasts()).toContain("Profile saved"));
    expect(daemon.bodies("PATCH /v1/me").at(-1)).toEqual({ name: "Grace Hopper" });
    expect(daemon.me.profile.name).toBe("Grace Hopper");
    expect(nameField()).toHaveValue("Grace Hopper");
    expect(saveButton()).toBeDisabled();
  });

  it("shows the daemon's sentence for a name it refuses, and keeps what was typed", async () => {
    render(() => <SettingsView />);
    const long = "A".repeat(101);
    fireEvent.input(nameField(), { target: { value: long } });
    fireEvent.click(saveButton());
    await waitFor(() => expect(toasts()).toContain("A name can have at most 100 characters."));
    expect(nameField()).toHaveValue(long);
    expect(saveButton()).toBeEnabled();
    expect(M.S.profile.name).toBe("Ada Okafor");
  });

  it("says the daemon cannot be reached, and keeps what was typed, when it is away", async () => {
    render(() => <SettingsView />);
    fireEvent.input(nameField(), { target: { value: "Grace" } });
    daemon.stop();
    fireEvent.click(saveButton());
    await waitFor(() =>
      expect(toasts()).toContain("Marshal can't reach the daemon. Check that it is running."),
    );
    daemon.start();
    expect(nameField()).toHaveValue("Grace");
    expect(saveButton()).toBeEnabled();
  });

  it("keeps what was typed after the save began", async () => {
    render(() => <SettingsView />);
    fireEvent.input(nameField(), { target: { value: "Grace" } });
    const release = daemon.holdNext("PATCH /v1/me");
    fireEvent.click(saveButton());
    fireEvent.input(nameField(), { target: { value: "Grace Hopper" } });
    release();
    await waitFor(() => expect(daemon.me.profile.name).toBe("Grace"));
    await waitFor(() => expect(toasts()).toContain("Profile saved"));
    expect(nameField()).toHaveValue("Grace Hopper");
    expect(saveButton()).toBeEnabled();
  });

  it("offers a time zone that is not one of the six, so it can be shown", async () => {
    render(() => <SettingsView />);
    daemon.me.profile.timeZone = "Asia/Tokyo";
    daemon.emit("me", "me.updated", {
      profile: daemon.me.profile,
      preferences: daemon.me.preferences,
      progress: daemon.me.progress,
    });
    await waitFor(() => expect(zoneField()).toHaveValue("Asia/Tokyo"));
    expect(Array.from(zoneField().options).map((option) => option.value)).toEqual([
      "Asia/Tokyo",
      "Europe/London",
      "Europe/Lisbon",
      "Africa/Lagos",
      "America/New_York",
      "America/Los_Angeles",
      "Asia/Singapore",
    ]);
  });

  it("says no time zone is set when the daemon has none, rather than showing the first", async () => {
    render(() => <SettingsView />);
    daemon.me.profile.timeZone = "";
    daemon.emit("me", "me.updated", {
      profile: daemon.me.profile,
      preferences: daemon.me.preferences,
      progress: daemon.me.progress,
    });
    await waitFor(() => expect(zoneField()).toHaveValue(""));
    expect(zoneField().selectedOptions[0]).toHaveTextContent("Not set");
    expect(saveButton()).toBeDisabled();
    fireEvent.change(zoneField(), { target: { value: "Africa/Lagos" } });
    expect(saveButton()).toBeEnabled();
  });

  it("opens the file chooser from Upload image, uploads what is picked, and draws it", async () => {
    render(() => <SettingsView />);
    chooser.pick.mockResolvedValue(
      new File([new Uint8Array(9)], "me.webp", { type: "image/webp" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Upload image" }));
    await waitFor(() => expect(daemon.me.avatar).toEqual({ type: "image/webp", size: 9 }));
    const avatar = screen.getByLabelText("Your avatar");
    await waitFor(() => expect(avatar.querySelector("img")).not.toBeNull());
    expect(avatar.querySelector("img")).toHaveAttribute("src", made.at(-1));
    expect(avatar).not.toHaveTextContent("AO");
    expect(toasts()).toContain("Avatar saved");
  });

  it("shows the daemon's sentence for an image it refuses and keeps the initials", async () => {
    render(() => <SettingsView />);
    chooser.pick.mockResolvedValue(new File([new Uint8Array(9)], "me.gif", { type: "image/gif" }));
    fireEvent.click(screen.getByRole("button", { name: "Upload image" }));
    await waitFor(() =>
      expect(toasts()).toContain(
        "Marshal accepts PNG, JPEG, and WebP images. Choose one of those.",
      ),
    );
    expect(screen.getByLabelText("Your avatar")).toHaveTextContent("AO");
    expect(ctx.S.profile.avatar).toBeNull();
  });
});
