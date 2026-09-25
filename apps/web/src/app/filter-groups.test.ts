import { beforeEach, describe, expect, it, vi } from "vitest";
import { toDaemonProject } from "~/data/mappers/project";
import { M } from "~/mock";
import { applyProject } from "~/sync/projects";
import { daemonProject, PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf } from "~/testing/test-store";
import { chipValue, FILTER_KEY_LABEL, filterGroups } from "./filter-groups";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const optionsOf = (label: string) => filterGroups().find((g) => g.label === label)?.options ?? [];

describe("filterGroups", () => {
  beforeEach(() => {
    M.go("project", "api", "board");
  });

  it("lists Status, Role, Agent, and Label for a project without packages", () => {
    expect(filterGroups().map((g) => g.label)).toEqual(["Status", "Role", "Agent", "Label"]);
  });

  it("counts every card once across the seven status columns", () => {
    const options = optionsOf("Status");
    expect(options.map((o) => o.value)).toEqual([...M.COLUMNS]);
    const total = options.reduce((sum, o) => sum + o.count, 0);
    expect(total).toBe(M.cardsOf("api").length);
  });

  it("gives status options their icon and a solid status color class", () => {
    const [backlog, , , needs] = optionsOf("Status");
    expect(backlog).toMatchObject({ icon: "st-backlog", iconClass: "text-border-strong" });
    expect(needs).toMatchObject({ label: "Needs you", iconClass: "text-status-needs-you-solid" });
  });

  it("sorts role, agent, and label values and counts the cards that have each", () => {
    const roles = optionsOf("Role");
    expect(roles.map((o) => o.value)).toEqual([...roles.map((o) => o.value)].sort());
    for (const option of roles) {
      const expected = M.cardsOf("api").filter((c) => c.role === option.value).length;
      expect(option.count).toBe(expected);
    }
    expect(optionsOf("Label").some((o) => o.value === "auth")).toBe(true);
    expect(optionsOf("Agent").every((o) => o.kind === "agent")).toBe(true);
  });

  it("adds a Package group with every package for a monorepo project", () => {
    M.go("project", "mobile", "board");
    const packages = optionsOf("Package");
    expect(packages.map((o) => o.value)).toEqual([
      "apps/ios",
      "apps/android",
      "packages/ui",
      "packages/auth",
      "packages/api-client",
    ]);
    expect(packages.find((o) => o.value === "packages/api-client")?.count).toBe(
      M.cardsOf("mobile").filter((c) => c.pkg === "packages/api-client").length,
    );
  });
});

describe("filterGroups for a monorepo whose packages the daemon found on disk", () => {
  it("also offers a package a card names that the project does not list, so no card is out of reach", () => {
    const ctx = contextOf(M);
    const daemonPackages = ["packages/api", "packages/shared", "packages/web"];
    applyProject(
      ctx,
      daemonProject({
        id: "mobile",
        name: "mobile-app",
        language: "Monorepo",
        isMonorepo: true,
        packages: daemonPackages,
      }),
    );
    try {
      M.go("project", "mobile", "board");
      const values = optionsOf("Package").map((o) => o.value);
      expect(values.slice(0, 3)).toEqual(daemonPackages);
      expect(values).toContain("apps/ios");
      expect(optionsOf("Package").find((o) => o.value === "apps/ios")?.count).toBe(
        M.cardsOf("mobile").filter((c) => c.pkg === "apps/ios").length,
      );
    } finally {
      const original = PROTOTYPE_PROJECTS.find((p) => p.id === "mobile");
      if (original) applyProject(ctx, toDaemonProject(original));
    }
  });
});

describe("chipValue and FILTER_KEY_LABEL", () => {
  it("shows a status filter by its label and other filters as stored", () => {
    expect(chipValue("status", "needs")).toBe("Needs you");
    expect(chipValue("role", "Worker")).toBe("Worker");
  });

  it("names every filter kind", () => {
    expect(FILTER_KEY_LABEL.package).toBe("Package");
    expect(FILTER_KEY_LABEL.model).toBe("Model");
  });
});
