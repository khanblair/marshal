import { afterEach, describe, expect, it, vi } from "vitest";
import { M, type NewProjectDraft } from "~/mock";
import {
  addDraftProject,
  autoName,
  canAdd,
  detectKind,
  detectMessage,
  followName,
  patchDraft,
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
  branch: "main",
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

describe("detectKind", () => {
  it("detects nothing from nothing", () => {
    expect(detectKind("")).toBeNull();
  });

  it("guesses a monorepo, Go, or TypeScript from the text, in that order", () => {
    expect(detectKind("~/code/my-monorepo")).toBe("mono");
    expect(detectKind("~/code/apps")).toBe("mono");
    expect(detectKind("~/code/workspace-tools")).toBe("mono");
    expect(detectKind("~/code/billing-service")).toBe("Go");
    expect(detectKind("~/code/API")).toBe("Go");
    expect(detectKind("~/code/website")).toBe("TypeScript");
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

describe("detectMessage", () => {
  it("asks for a folder or URL before anything is typed", () => {
    expect(detectMessage(null, "main")).toBe(
      "Choose a folder or paste a URL. Marshal detects the language and monorepo tools.",
    );
  });

  it("describes a monorepo, and names the language and branch otherwise", () => {
    expect(detectMessage("mono", "main")).toBe(
      "Detected a monorepo: pnpm workspaces with 3 packages. It gets one board with package swimlanes.",
    );
    expect(detectMessage("Go", "develop")).toBe("Detected a Go project on branch develop.");
    expect(detectMessage("TypeScript", "")).toBe("Detected a TypeScript project on branch main.");
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
  it("adds a folder project, opens its board, and confirms with a toast", () => {
    const add = vi.spyOn(M, "addProject").mockReturnValue("p99");
    M.set({ newProject: draft() });
    addDraftProject(draft({ path: "~/code/billing-service", name: "  billing  ", branch: "dev" }));
    expect(add).toHaveBeenCalledWith({
      name: "billing",
      path: "~/code/billing-service",
      branch: "dev",
      mono: false,
      lang: "Go",
    });
    expect(M.S.newProject).toBeNull();
    expect(M.S.route).toMatchObject({ page: "project", pid: "p99", view: "board" });
    expect(M.S.toasts.map((t) => t.msg)).toContain("Project added");
  });

  it("clones a GitHub project into ~/code and passes monorepo detection on", () => {
    const add = vi.spyOn(M, "addProject").mockReturnValue("p98");
    addDraftProject(
      draft({ source: "github", url: "https://github.com/acme/monorepo.git", name: "mono" }),
    );
    expect(add).toHaveBeenCalledWith({
      name: "mono",
      path: "~/code/monorepo",
      branch: "main",
      mono: true,
      lang: "TypeScript",
    });
  });

  it("does nothing without a source and a name", () => {
    const add = vi.spyOn(M, "addProject");
    addDraftProject(draft({ path: "~/a" }));
    expect(add).not.toHaveBeenCalled();
  });
});
