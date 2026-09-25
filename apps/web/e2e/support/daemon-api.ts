import { readFileSync } from "node:fs";
import { join } from "node:path";
import type { APIRequestContext } from "@playwright/test";
import { DATA_DIR } from "./e2e-env";

/** The dev daemon's real token, read from the run's own data folder. */
export const realToken = (): string => readFileSync(join(DATA_DIR, "dev-token"), "utf8").trim();

interface WireProject {
  id: string;
  name: string;
  path: string;
  devCommand: string;
}

const auth = () => ({ authorization: `Bearer ${realToken()}` });

/** Adds a project through the API, as another device would, for a spec that needs one to start from. */
export async function createProjectViaApi(
  request: APIRequestContext,
  folder: string,
  name: string,
): Promise<WireProject> {
  const response = await request.post("/v1/projects", {
    headers: auth(),
    data: { source: "folder", path: folder, name },
  });
  if (!response.ok()) throw new Error(`could not add the project: ${response.status()}`);
  return (await response.json()) as WireProject;
}

/** The project as the daemon has it now, or null when it is gone. */
export async function getProjectViaApi(
  request: APIRequestContext,
  id: string,
): Promise<WireProject | null> {
  const response = await request.get(`/v1/projects/${id}`, { headers: auth() });
  return response.ok() ? ((await response.json()) as WireProject) : null;
}

/** Removes a project the spec added, keeping its branches and memory, so the shared daemon stays clean. */
export async function removeProjectViaApi(request: APIRequestContext, id: string): Promise<void> {
  await request.delete(`/v1/projects/${id}`, {
    headers: auth(),
    data: { keepBranches: true, keepMemory: false },
  });
}

/** One agent of the daemon's catalog, with the parts a spec checks. */
export interface WireAgent {
  kind: string;
  name: string;
  version: string;
  status: "supported" | "untested" | "missing";
  warning: string;
  installHint: string;
  models: { id: string; name: string; thinking: boolean }[];
  capabilities: { thinking: boolean };
}

/** The agents the daemon reports now, in its order, so a spec asks the daemon and never hard-codes its stub. */
export async function agentsViaApi(request: APIRequestContext): Promise<WireAgent[]> {
  const response = await request.get("/v1/agents", { headers: auth() });
  if (!response.ok()) throw new Error(`could not read the agents: ${response.status()}`);
  return ((await response.json()) as { agents: WireAgent[] }).agents;
}
