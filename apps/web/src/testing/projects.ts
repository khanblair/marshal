/**
 * Typed builders for projects as the daemon sends them. They start from the golden `project.json`
 * that the Go tests write, so a fixture cannot drift from the wire shape, and change only the
 * fields a test cares about.
 */
import type { Project } from "@marshal/protocol";
import { type DaemonProject, toDaemonProject } from "~/data/mappers/project";
import { golden } from "~/data/testing/golden";

/** A project on the wire: the golden one, with the given fields changed. */
export function wireProject(fields: Partial<Project> & { id: string }): Project {
  return { ...golden<Project>("project"), name: fields.id, ...fields };
}

/** The same project after the mapper, the way the store receives it. */
export const daemonProject = (fields: Partial<Project> & { id: string }): DaemonProject =>
  toDaemonProject(wireProject(fields));

const MOBILE_PACKAGES = [
  "apps/ios",
  "apps/android",
  "packages/ui",
  "packages/auth",
  "packages/api-client",
];

/**
 * The three projects of the prototype, as the dev daemon's `prototype` fixture has them (same ids and
 * names), with the languages, paths, and packages the design draws. The unit tests use this list.
 */
export const PROTOTYPE_PROJECTS: readonly Project[] = [
  wireProject({
    id: "api",
    name: "api-gateway",
    language: "Go",
    path: "~/code/api-gateway",
    devCommand: "",
    bypassLocked: false,
    badges: { needs: 0, awake: 0 },
  }),
  wireProject({
    id: "web",
    name: "web-dashboard",
    language: "TypeScript",
    path: "~/code/web-dashboard",
    bypassLocked: false,
    badges: { needs: 0, awake: 0 },
  }),
  wireProject({
    id: "mobile",
    name: "mobile-app",
    language: "Monorepo",
    path: "~/code/mobile-app",
    isMonorepo: true,
    packages: MOBILE_PACKAGES,
    bypassLocked: false,
    badges: { needs: 0, awake: 0 },
  }),
];
