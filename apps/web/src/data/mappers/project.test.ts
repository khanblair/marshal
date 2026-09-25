import type { Project, ProjectListSnapshot } from "@marshal/protocol";
import { describe, expect, expectTypeOf, it } from "vitest";
import type { Project as MockProject } from "~/mock/types";
import { golden } from "../testing/golden";
import { type DaemonProject, toDaemonProject } from "./project";

const project = golden<Project>("project");
const list = golden<ProjectListSnapshot>("project-list");

describe("toDaemonProject", () => {
  it("maps the golden project", () => {
    expect(toDaemonProject(project)).toEqual({
      id: "web-dashboard",
      name: "web-dashboard",
      lang: "TypeScript",
      path: "/home/ada/code/web-dashboard",
      branch: "main",
      dev: "pnpm dev",
      lockBypass: true,
      packages: undefined,
      createdAt: Date.UTC(2026, 8, 25, 10, 15, 30, 123),
      needs: 2,
      awake: 1,
    });
  });

  it("keeps the packages of a monorepo, and only of a monorepo", () => {
    const [, monorepo] = list.projects;
    expect(monorepo && toDaemonProject(monorepo).packages).toEqual([
      "apps/android",
      "apps/ios",
      "packages/ui",
    ]);
    expect(toDaemonProject({ ...project, packages: ["stray"] }).packages).toBeUndefined();
  });

  it("does not share the packages array with the wire object", () => {
    const [, monorepo] = list.projects;
    if (!monorepo) throw new Error("the golden list has no monorepo");
    const mapped = toDaemonProject(monorepo);
    mapped.packages?.push("changed");
    expect(monorepo.packages).toHaveLength(3);
  });

  it("maps every project of the golden list, with the name and the id apart", () => {
    const mapped = list.projects.map(toDaemonProject);
    expect(mapped.map((p) => [p.id, p.name, p.lang, p.needs, p.awake])).toEqual([
      ["web-dashboard", "web-dashboard", "TypeScript", 2, 1],
      ["mobile", "mobile-app", "Monorepo", 0, 0],
    ]);
    expect(mapped[1]?.dev).toBe("");
    expect(mapped[1]?.lockBypass).toBe(false);
  });

  it("fills every field of the mock's Project except the four the daemon does not have yet", () => {
    // This line fails to compile when a field of the mock's Project is neither mapped nor listed here.
    type Missing = "ci" | "ciAgo" | "monthBase" | "runs";
    expectTypeOf<DaemonProject>().toExtend<Omit<MockProject, Missing>>();
    expectTypeOf<Missing>().toEqualTypeOf<Exclude<keyof MockProject, keyof DaemonProject>>();
    expect(Object.keys(toDaemonProject(project))).not.toContain("ci");
  });
});
