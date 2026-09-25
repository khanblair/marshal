import { afterEach, describe, expect, it, vi } from "vitest";
import { M, type NewProjectDraft } from "~/mock";
import {
  addDraftProject,
  autoName,
  canAdd,
  followName,
  IDLE_MESSAGE,
  patchDraft,
  requestOf,
  sourceOf,
} from "./new-project";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const draft = (over: Partial<NewProjectDraft> = {}): NewProjectDraft => ({
  source: "folder",
  path: "",
  url: "",
  name: "",
  branch: "",
  ...over,
});

afterEach(() => {
  vi.restoreAllMocks();
  M.set({ newProject: null, toasts: [] });
  M.go("home");
});

describe("sourceOf", () => {
  it("is the path for a folder and the URL for GitHub", () => {
    expect(sourceOf(draft({ path: "~/a", url: "https://x/y" }))).toBe("~/a");
    expect(sourceOf(draft({ source: "github", path: "~/a", url: "https://x/y" }))).toBe(
      "https://x/y",
    );
  });
});

describe("autoName", () => {
  it("takes the last path or URL segment without .git", () => {
    expect(autoName("~/code/billing-service")).toBe("billing-service");
    expect(autoName("https://github.com/acme/ledger.git")).toBe("ledger");
    expect(autoName("git@github.com:acme/ledger.git")).toBe("ledger");
    expect(autoName("~/code/repo/")).toBe("repo");
    expect(autoName("")).toBe("");
  });
});

describe("the line under the fields", () => {
  it("says what Marshal does, and never guesses a language or a monorepo from the text typed", () => {
    expect(IDLE_MESSAGE).toBe(
      "Choose a folder or paste a URL. Marshal detects the language and monorepo tools.",
    );
  });
});

describe("requestOf", () => {
  it("sends a folder as typed, with the trimmed name", () => {
    expect(requestOf(draft({ path: " ~/code/billing-service ", name: "  billing  " }))).toEqual({
      source: "folder",
      path: "~/code/billing-service",
      name: "billing",
    });
  });

  it("clones a GitHub URL into ~/code/<repository name>, with the branch only when one is typed", () => {
    const github = {
      source: "github",
      url: "https://github.com/acme/ledger.git",
      name: "ledger",
    } as const;
    expect(requestOf(draft(github))).toEqual({
      source: "clone",
      url: "https://github.com/acme/ledger.git",
      dest: "~/code/ledger",
      name: "ledger",
    });
    expect(requestOf(draft({ ...github, branch: " develop " }))).toMatchObject({
      branch: "develop",
    });
  });
});

describe("canAdd and followName", () => {
  it("needs a source and a name", () => {
    expect(canAdd(draft())).toBe(false);
    expect(canAdd(draft({ path: "~/a" }))).toBe(false);
    expect(canAdd(draft({ name: "a" }))).toBe(false);
    expect(canAdd(draft({ path: "~/a", name: " " }))).toBe(false);
    expect(canAdd(draft({ path: "~/a", name: "a" }))).toBe(true);
    expect(canAdd(draft({ source: "github", path: "~/a", name: "a" }))).toBe(false);
  });

  it("names the project after the source until the name has been typed", () => {
    expect(followName(draft({ name: "" }), "~/code/thing")).toBe("thing");
    expect(followName(draft({ name: "Mine", nameTouched: true }), "~/code/thing")).toBe("Mine");
  });

  it("merges changes into the draft", () => {
    const d = draft();
    patchDraft(d, { path: "~/a", branch: "dev" });
    expect(d).toMatchObject({ path: "~/a", branch: "dev", source: "folder" });
  });
});

describe("addDraftProject", () => {
  it("adds a folder project, opens its board, closes, and confirms with a toast", async () => {
    const add = vi.spyOn(M, "addProject").mockResolvedValue({ id: "billing" });
    const d = draft({ path: "~/code/billing-service", name: "  billing  " });
    M.set({ newProject: d });
    await addDraftProject(d);
    expect(add).toHaveBeenCalledWith({
      source: "folder",
      path: "~/code/billing-service",
      name: "billing",
    });
    expect(M.S.newProject).toBeNull();
    expect(M.S.route).toMatchObject({ page: "project", pid: "billing", view: "board" });
    expect(M.S.toasts.map((t) => t.msg)).toContain("Project added");
  });

  it("clones a GitHub project into ~/code", async () => {
    const add = vi.spyOn(M, "addProject").mockResolvedValue({ id: "monorepo" });
    await addDraftProject(
      draft({ source: "github", url: "https://github.com/acme/monorepo.git", name: "mono" }),
    );
    expect(add).toHaveBeenCalledWith({
      source: "clone",
      url: "https://github.com/acme/monorepo.git",
      dest: "~/code/monorepo",
      name: "mono",
    });
  });

  it("keeps the dialog open with its fields when the daemon refuses, and shows its sentence", async () => {
    const sentence = "That folder is not a Git repository. Choose the top folder of a repository.";
    vi.spyOn(M, "addProject").mockResolvedValue({ error: sentence });
    const d = draft({ path: "~/code/plain", name: "plain" });
    M.set({ newProject: d });
    await addDraftProject(d);
    expect(M.S.newProject).toMatchObject({ path: "~/code/plain", name: "plain", error: sentence });
    expect(M.S.newProject?.busy).toBe(false);
    expect(M.S.toasts).toHaveLength(0);
  });

  it("refuses a second press while the first is still running", async () => {
    let finish: (value: { id: string }) => void = () => {};
    const add = vi
      .spyOn(M, "addProject")
      .mockReturnValue(new Promise((resolve) => (finish = resolve)));
    const d = draft({ path: "~/a", name: "a" });
    M.set({ newProject: d });
    const first = addDraftProject(d);
    await addDraftProject(d);
    expect(add).toHaveBeenCalledOnce();
    finish({ id: "a" });
    await first;
  });

  it("does nothing without a source and a name", async () => {
    const add = vi.spyOn(M, "addProject");
    await addDraftProject(draft({ path: "~/a" }));
    expect(add).not.toHaveBeenCalled();
  });
});
