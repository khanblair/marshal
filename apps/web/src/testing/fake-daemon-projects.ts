import type { Project } from "@marshal/protocol";
import { emptyAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { baseName, bodyOf, type Publish, refuse, STATUS, slug } from "./fake-daemon-support";
import { wireProject } from "./projects";

export interface ProjectState {
  projects: Project[];
  publish: Publish;
  now: () => string;
}

const notFound = () =>
  refuse(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that project. It may have been removed.",
  );

/** The sample ships inside the daemon: one folder, so the request names only the source. */
const SAMPLE_NAME = "marshal-sample";
const SAMPLE_PATH = "~/.marshal/sample";

function createSampleProject(
  { projects, publish, now }: ProjectState,
  body: Record<string, unknown>,
): Response {
  const already = projects.find((p) => p.name === SAMPLE_NAME || p.path === SAMPLE_PATH);
  if (already) {
    return refuse(STATUS.conflict, "conflict", "The sample project is already in Marshal.");
  }
  const name = String(body.name ?? "").trim() || SAMPLE_NAME;
  const project = wireProject({
    id: slug(name),
    name,
    path: SAMPLE_PATH,
    language: "TypeScript",
    devCommand: "",
    bypassLocked: false,
    badges: { needs: 0, awake: 0 },
    createdAt: now(),
  });
  projects.push(project);
  publish("home", "project.created", { project });
  return jsonAnswer(project, STATUS.created);
}

export function createProject(
  { projects, publish, now }: ProjectState,
  request: FakeRequest,
): Response {
  const body = bodyOf(request);
  if (body.source === "sample") return createSampleProject({ projects, publish, now }, body);
  const path = String(body.path ?? body.dest ?? "").trim();
  if (!path)
    return refuse(STATUS.badRequest, "invalid_argument", "Choose the folder of the repository.");
  if (projects.some((p) => p.path === path)) {
    return refuse(STATUS.conflict, "conflict", "That repository is already a project in Marshal.");
  }
  const name = String(body.name ?? "").trim() || baseName(path);
  const project = wireProject({
    id: slug(name),
    name,
    path,
    language: "Unknown",
    devCommand: "",
    bypassLocked: false,
    badges: { needs: 0, awake: 0 },
    createdAt: now(),
  });
  projects.push(project);
  publish("home", "project.created", { project });
  return jsonAnswer(project, STATUS.created);
}

export function updateProject(
  { projects, publish }: ProjectState,
  id: string,
  request: FakeRequest,
): Response {
  const project = projects.find((p) => p.id === id);
  if (!project) return notFound();
  const body = bodyOf(request);
  if (typeof body.name === "string") {
    if (!body.name.trim()) {
      return refuse(
        STATUS.badRequest,
        "invalid_argument",
        "Project names can't be empty. The old name is kept.",
      );
    }
    project.name = body.name.trim();
  }
  if (typeof body.devCommand === "string") project.devCommand = body.devCommand;
  if (typeof body.defaultBranch === "string") project.defaultBranch = body.defaultBranch;
  // An empty string clears the choice, so the default branch stands in for it again.
  if (typeof body.integrationBranch === "string") {
    project.integrationBranch = body.integrationBranch.trim() || project.defaultBranch;
  }
  if (typeof body.bypassLocked === "boolean") project.bypassLocked = body.bypassLocked;
  publish("home", "project.updated", { project });
  return jsonAnswer(project);
}

export function removeProject({ projects, publish }: ProjectState, id: string): Response {
  const at = projects.findIndex((p) => p.id === id);
  if (at < 0) return notFound();
  projects.splice(at, 1);
  publish("home", "project.removed", { projectId: id });
  return emptyAnswer();
}
