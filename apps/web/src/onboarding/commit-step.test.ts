import { beforeEach, describe, expect, it } from "vitest";
import type { Marshal } from "~/mock";
import { createMarshal } from "~/mock/marshal";
import { commitStep } from "./commit-step";
import { initialDraft, type OnboardingDraft } from "./onboarding-draft";

let m: Marshal;
let draft: OnboardingDraft;

beforeEach(() => {
  m = createMarshal({
    hash: "#nosim",
    storage: null,
    viewport: { w: 1440, h: 900 },
    applyTheme: () => {},
  });
  draft = initialDraft();
});

const projectNames = (): string[] => m.S.projects.map((p) => p.name);

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
  it("adds the sample project with its cards, once", () => {
    const cards = m.S.cards.length;
    commitStep(3, draft, m);
    expect(projectNames()).toContain("marshal-sample");
    expect(m.S.cards.length).toBe(cards + 3);
    commitStep(3, draft, m);
    expect(projectNames().filter((n) => n === "marshal-sample")).toHaveLength(1);
  });

  it("adds a project from a folder, keeping the typed path", () => {
    Object.assign(draft, { source: "folder", path: "~/code/my-repo" });
    commitStep(3, draft, m);
    expect(m.S.projects.at(-1)).toMatchObject({ name: "my-repo", path: "~/code/my-repo" });
  });

  it("clones a GitHub URL into ~/code and detects monorepos", () => {
    Object.assign(draft, { source: "github", url: "https://github.com/acme/web-monorepo.git" });
    commitStep(3, draft, m);
    expect(m.S.projects.at(-1)).toMatchObject({
      name: "web-monorepo",
      path: "~/code/web-monorepo",
      lang: "Monorepo",
    });
  });

  it("adds nothing for an empty or nameless path", () => {
    const count = m.S.projects.length;
    Object.assign(draft, { source: "folder", path: "" });
    commitStep(3, draft, m);
    Object.assign(draft, { source: "github", url: "///" });
    commitStep(3, draft, m);
    expect(m.S.projects).toHaveLength(count);
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
