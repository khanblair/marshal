import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Marshal } from "~/mock";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { createSyncedMarshal, createTestMarshal } from "~/testing/test-store";
import { commitStep } from "./commit-step";
import { initialDraft, type OnboardingDraft } from "./onboarding-draft";

let m: Marshal;
let draft: OnboardingDraft;

beforeEach(() => {
  m = createTestMarshal({
    hash: "#nosim",
    storage: null,
    viewport: { w: 1440, h: 900 },
    applyTheme: () => {},
  });
  draft = initialDraft();
});

describe("commitStep on the profile screen", () => {
  it("saves the trimmed name and email and the time zone", () => {
    Object.assign(draft, {
      name: "  Grace Hopper ",
      email: " grace@navy.mil ",
      tz: "Asia/Singapore",
    });
    commitStep(1, draft, m);
    expect(m.S.profile).toMatchObject({
      name: "Grace Hopper",
      email: "grace@navy.mil",
      tz: "Asia/Singapore",
    });
  });

  it("keeps the current name when the field is blank", () => {
    const before = m.S.profile.name;
    draft.name = "   ";
    commitStep(1, draft, m);
    expect(m.S.profile.name).toBe(before);
  });
});

describe("commitStep on the agents screen", () => {
  it("saves a key longer than eight characters, masked", () => {
    draft.keys.anthropic = "sk-ant-api03-abcdefgh1234";
    commitStep(2, draft, m);
    const provider = m.S.providers.find((p) => p.id === "anthropic");
    expect(provider).toMatchObject({ st: "saved", masked: "sk-ant…1234" });
  });

  it("ignores a key of eight characters or fewer", () => {
    const before = m.S.providers.find((p) => p.id === "deepseek");
    draft.keys.openai = "12345678";
    commitStep(2, draft, m);
    expect(m.S.providers.find((p) => p.id === "openai")?.masked).not.toBe("123456…5678");
    expect(m.S.providers.find((p) => p.id === "deepseek")).toEqual(before);
  });
});

describe("commitStep on the project screen", () => {
  let daemon: FakeDaemon;
  let synced: Marshal;
  beforeEach(async () => {
    daemon = createFakeDaemon();
    synced = await createSyncedMarshal(daemon);
  });
  afterEach(() => daemon.data.stop());

  const created = () => daemon.bodies("POST /v1/projects");
  const names = () => synced.S.projects.map((p) => p.name);

  it("starts on the sample, so a screen the person did not touch adds the sample", async () => {
    expect(draft.source).toBe("sample");
    commitStep(3, draft, synced);
    await vi.waitFor(() => expect(names()).toContain("marshal-sample"));
    expect(created()).toEqual([{ source: "sample" }]);
  });

  it("adds a project from a folder, keeping the typed path", async () => {
    Object.assign(draft, { source: "folder", path: " ~/code/my-repo " });
    commitStep(3, draft, synced);
    await vi.waitFor(() => expect(names()).toContain("my-repo"));
    expect(created()).toEqual([{ source: "folder", path: "~/code/my-repo", name: "my-repo" }]);
  });

  it("clones a GitHub URL into ~/code", async () => {
    Object.assign(draft, { source: "github", url: "https://github.com/acme/web-monorepo.git" });
    commitStep(3, draft, synced);
    await vi.waitFor(() => expect(created()).toHaveLength(1));
    expect(created()[0]).toEqual({
      source: "clone",
      url: "https://github.com/acme/web-monorepo.git",
      dest: "~/code/web-monorepo",
      name: "web-monorepo",
    });
  });

  it("shows the daemon's refusal as a toast and lets setup carry on", async () => {
    const sentence = "That folder does not exist. Check the path and try again.";
    daemon.refuseNext("POST /v1/projects", 400, "invalid_argument", sentence);
    Object.assign(draft, { source: "folder", path: "~/code/nowhere" });
    commitStep(3, draft, synced);
    await vi.waitFor(() => expect(synced.S.toasts.map((t) => t.msg)).toContain(sentence));
    expect(synced.S.projects).toHaveLength(0);
  });

  it("adds nothing for an empty or nameless path", () => {
    Object.assign(draft, { source: "folder", path: "" });
    commitStep(3, draft, synced);
    Object.assign(draft, { source: "github", url: "///" });
    commitStep(3, draft, synced);
    expect(created()).toEqual([]);
  });
});

describe("commitStep on the last screen", () => {
  it("marks the chosen chat apps connected", () => {
    draft.chatApps.discord = true;
    commitStep(4, draft, m);
    expect(m.S.integrations.find((x) => x.id === "discord")?.st).toBe("connected");
  });

  it("leaves apps that were not connected as they were", () => {
    const before = m.S.integrations.find((x) => x.id === "discord")?.st;
    commitStep(4, draft, m);
    expect(m.S.integrations.find((x) => x.id === "discord")?.st).toBe(before);
  });

  it("does nothing on the welcome screen", () => {
    const profile = { ...m.S.profile };
    commitStep(0, draft, m);
    expect(m.S.profile).toEqual(profile);
  });
});
