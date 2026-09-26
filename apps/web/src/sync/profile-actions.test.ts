import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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

/** The file chooser, which a test answers by hand: a file, or null for a chooser that was closed. */
const picker = { pickImage: vi.fn<() => Promise<File | null>>() };

let daemon: FakeDaemon | null = null;
beforeEach(() => {
  picker.pickImage.mockReset();
  let made = 0;
  URL.createObjectURL = vi.fn(() => `blob:test/${++made}`);
  URL.revokeObjectURL = vi.fn();
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
  const M = await createSyncedMarshal(d, { sections: DAEMON_PERSON, pickImage: picker.pickImage });
  return { M, ctx: contextOf(M) };
}

const toasts = (M: { S: { toasts: { msg: string }[] } }) => M.S.toasts.map((toast) => toast.msg);
const fields = { name: "Ada Okafor", email: "ada@example.com", tz: "Europe/London" };
const image = (type = "image/png", bytes = 8) =>
  new File([new Uint8Array(bytes)], "me.png", { type });

describe("saving the profile on the daemon", () => {
  it("sends only the fields that changed, trimmed, and shows what the daemon answered", async () => {
    const d = open();
    const { M } = await synced(d);
    const saved = await M.saveProfile({
      name: "  Grace Hopper ",
      email: "ada@example.com",
      tz: "Asia/Singapore",
    });
    expect(saved).toBe(true);
    expect(d.bodies("PATCH /v1/me")).toEqual([
      { name: "Grace Hopper", timeZone: "Asia/Singapore" },
    ]);
    // The initials are the daemon's: the screen did not work them out.
    expect(M.S.profile).toMatchObject({
      name: "Grace Hopper",
      tz: "Asia/Singapore",
      initials: "GH",
    });
    expect(toasts(M)).toEqual(["Profile saved"]);
  });

  it("clears an email with the empty string", async () => {
    const d = open();
    const { M } = await synced(d);
    await M.saveProfile({ ...fields, email: "  " });
    expect(d.bodies("PATCH /v1/me")).toEqual([{ email: "" }]);
    expect(M.S.profile.email).toBe("");
  });

  it("asks the daemon nothing when nothing changed", async () => {
    const d = open();
    const { M } = await synced(d);
    expect(await M.saveProfile(fields)).toBe(true);
    expect(d.routes()).not.toContain("PATCH /v1/me");
    expect(toasts(M)).toEqual([]);
  });

  it("shows the daemon's sentence for a time zone it does not know, and keeps what was saved", async () => {
    const d = open();
    const { M } = await synced(d);
    const saved = await M.saveProfile({ ...fields, tz: "Europe/Atlantis" });
    expect(saved).toBe(false);
    expect(toasts(M)).toEqual(["Marshal does not know that time zone. Choose one from the list."]);
    expect(M.S.profile.tz).toBe("Europe/London");
    expect(d.me.profile.timeZone).toBe("Europe/London");
  });

  it("shows the daemon's sentence for an email it refuses", async () => {
    const d = open();
    const { M } = await synced(d);
    expect(await M.saveProfile({ ...fields, email: "not an email" })).toBe(false);
    expect(toasts(M)).toEqual([
      "That does not look like an email address. Check it and try again.",
    ]);
    expect(M.S.profile.email).toBe("ada@example.com");
  });

  it("shows the daemon's own words for an empty name too, and does not judge it itself", async () => {
    const d = open();
    const { M } = await synced(d);
    expect(await M.saveProfile({ ...fields, name: "   " })).toBe(false);
    expect(toasts(M)).toEqual(["Enter a name. It shows on cards you comment on."]);
    expect(d.bodies("PATCH /v1/me")).toEqual([{ name: "" }]);
  });

  it("says the daemon cannot be reached, and saves nothing, when it is not there", async () => {
    const d = open();
    const { M } = await synced(d);
    d.stop();
    expect(await M.saveProfile({ ...fields, name: "Grace" })).toBe(false);
    expect(toasts(M)).toEqual(["Marshal can't reach the daemon. Check that it is running."]);
    expect(M.S.profile.name).toBe("Ada Okafor");
  });

  it("is not saved when there is no daemon to save to", async () => {
    const M = createTestMarshal({
      sections: DAEMON_PERSON,
      data: null,
      pickImage: picker.pickImage,
    });
    expect(await M.saveProfile({ ...fields, name: "Grace" })).toBe(false);
  });
});

describe("the avatar", () => {
  it("uploads the picked image as it is, with its own type, and draws it", async () => {
    const d = open();
    const { M } = await synced(d);
    picker.pickImage.mockResolvedValue(image("image/jpeg", 12));
    await M.chooseAvatar();
    const call = d.calls.find((one) => one.method === "POST" && one.url === "/v1/me/avatar");
    expect(call?.headers["content-type"]).toBe("image/jpeg");
    expect((call?.raw as Blob | undefined)?.size).toBe(12);
    expect(d.me.avatar).toEqual({ type: "image/jpeg", size: 12 });
    await vi.waitFor(() => expect(M.S.profile.avatar).toBe("blob:test/1"));
    expect(toasts(M)).toEqual(["Avatar saved"]);
  });

  it("shows the daemon's sentence for an image that is too large, and keeps the avatar it had", async () => {
    const d = open();
    const { M } = await synced(d);
    picker.pickImage.mockResolvedValue(image("image/png", 2 * 1024 * 1024 + 1));
    await M.chooseAvatar();
    expect(toasts(M)).toEqual(["That image is larger than 2 MB. Choose a smaller one."]);
    expect(M.S.profile.avatar).toBeNull();
    expect(d.me.avatar).toBeNull();
  });

  it("shows the daemon's sentence for a kind of image it does not take", async () => {
    const d = open();
    const { M } = await synced(d);
    picker.pickImage.mockResolvedValue(image("image/gif"));
    await M.chooseAvatar();
    expect(toasts(M)).toEqual(["Marshal accepts PNG, JPEG, and WebP images. Choose one of those."]);
  });

  it("changes nothing when the chooser is closed without a file", async () => {
    const d = open();
    const { M } = await synced(d);
    picker.pickImage.mockResolvedValue(null);
    await M.chooseAvatar();
    expect(d.routes()).not.toContain("POST /v1/me/avatar");
    expect(toasts(M)).toEqual([]);
  });

  it("uploads nothing when there is no daemon", async () => {
    const M = createTestMarshal({
      sections: DAEMON_PERSON,
      data: null,
      pickImage: picker.pickImage,
    });
    picker.pickImage.mockResolvedValue(image());
    await M.chooseAvatar();
    expect(toasts(M)).toEqual([]);
  });

  it("is only a toast on the mock, which has no picker", async () => {
    const M = createTestMarshal({ sections: MOCK_PERSON, data: null, pickImage: picker.pickImage });
    M.chooseAvatar();
    expect(picker.pickImage).not.toHaveBeenCalled();
    expect(toasts(M)).toEqual(["Choose an image to use as your avatar"]);
  });

  it("is saved as the mock saved it, with nothing to refuse", () => {
    const M = createTestMarshal({ sections: MOCK_PERSON, data: null, pickImage: picker.pickImage });
    expect(M.saveProfile({ name: " Grace ", email: " g@navy.mil ", tz: "Asia/Singapore" })).toBe(
      true,
    );
    expect(M.S.profile).toMatchObject({ name: "Grace", email: "g@navy.mil", tz: "Asia/Singapore" });
    expect(toasts(M)).toEqual(["Profile saved"]);
  });
});

describe("the profile the daemon starts with", () => {
  it("is what a fresh daemon has: a person named Owner with no email or time zone", async () => {
    const d = open({
      profile: wireProfile({ name: "Owner", initials: "O", email: "", timeZone: "" }),
    });
    const { M } = await synced(d);
    expect(await M.saveProfile({ name: "Owner", email: "", tz: "" })).toBe(true);
    expect(M.S.profile.initials).toBe("O");
  });
});
