import { readFileSync } from "node:fs";
import { join } from "node:path";
import type { APIRequestContext } from "@playwright/test";
import { DAEMON_ORIGIN, DATA_DIR } from "./e2e-env";

/** The dev daemon's real token, read from the run's own data folder. */
export const realToken = (): string => readFileSync(join(DATA_DIR, "dev-token"), "utf8").trim();

interface WireProject {
  id: string;
  name: string;
  path: string;
  devCommand: string;
}

const auth = () => ({ authorization: `Bearer ${realToken()}` });

/** One call to the daemon from outside a page, with this run's own token. */
async function daemonCall(path: string, method: string, body?: unknown): Promise<Response> {
  return fetch(`${DAEMON_ORIGIN}${path}`, {
    method,
    headers: { ...auth(), "content-type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

/**
 * Ends the first launch the way a person who skipped it does, so every spec starts inside the app
 * instead of on the first-launch screens. This run's daemon is made new each time, so its first
 * launch is pending until something ends it: the run itself, once, from `global-setup`.
 */
export async function finishFirstLaunch(): Promise<void> {
  const response = await daemonCall("/v1/me/progress", "PATCH", {
    onboarding: { status: "skipped" },
    tutorial: { status: "skipped" },
  });
  if (!response.ok) throw new Error(`could not finish the first launch: ${response.status}`);
}

/** Puts this person's first launch back to the start, for the spec that walks it. A dev daemon answers this. */
export async function resetFirstLaunch(): Promise<void> {
  const response = await daemonCall("/v1/dev/reset-first-launch", "POST");
  if (!response.ok) throw new Error(`could not reset the first launch: ${response.status}`);
}

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

/** The projects the daemon has now, so a spec can count them instead of trusting the screen. */
export async function projectsViaApi(request: APIRequestContext): Promise<WireProject[]> {
  const response = await request.get("/v1/projects", { headers: auth() });
  if (!response.ok()) throw new Error(`could not read the projects: ${response.status()}`);
  return ((await response.json()) as { projects: WireProject[] }).projects;
}

/** What the daemon says when the first-launch screen asks it for the sample project. */
export interface SampleAnswer {
  ok: boolean;
  /** The project it made, or the one already there. */
  id: string;
  /** Its own sentence when it refused. */
  message: string;
}

/** Asks the daemon for the sample project, the way the first-launch screen does. */
export async function askForSampleViaApi(request: APIRequestContext): Promise<SampleAnswer> {
  const response = await request.post("/v1/projects", {
    headers: auth(),
    data: { source: "sample" },
  });
  const body = (await response.json().catch(() => ({}))) as {
    id?: string;
    message?: string;
    details?: { projectId?: string };
  };
  return {
    ok: response.ok(),
    id: body.id ?? body.details?.projectId ?? "",
    message: body.message ?? "",
  };
}

/** The daemon's answer to `GET /v1/me`, with the profile and the preferences a spec changed. */
export async function meViaApi(
  request: APIRequestContext,
): Promise<{ preferences: { theme: string }; profile: { name: string } }> {
  const response = await request.get("/v1/me", { headers: auth() });
  if (!response.ok()) throw new Error(`could not read me: ${response.status()}`);
  return (await response.json()) as { preferences: { theme: string }; profile: { name: string } };
}

/** One card of a project's board, with the parts a spec checks. */
export interface WireCard {
  id: string;
  key: string;
  number: number;
  projectId: string;
  title: string;
  state: string;
  /** The session's state, or null when the card has never had one. */
  session: string | null;
  paused: boolean;
  pinned: boolean;
}

/** Adds a card to a project through the API, as another device would, for a spec that needs one. */
export async function createCardViaApi(
  request: APIRequestContext,
  projectId: string,
  title: string,
): Promise<WireCard> {
  const response = await request.post(`/v1/projects/${projectId}/cards`, {
    headers: auth(),
    data: { title },
  });
  if (!response.ok()) throw new Error(`could not add the card: ${response.status()}`);
  return (await response.json()) as WireCard;
}

/** Home's numbers as the daemon has them now: the two lists and the tile counts. */
export async function homeViaApi(
  request: APIRequestContext,
): Promise<{ needs: { key: string }[]; awake: { key: string }[]; tiles: Record<string, number> }> {
  const response = await request.get("/v1/home/dashboard", { headers: auth() });
  if (!response.ok()) throw new Error(`could not read Home: ${response.status()}`);
  return (await response.json()) as {
    needs: { key: string }[];
    awake: { key: string }[];
    tiles: Record<string, number>;
  };
}

/** The cards of a project's board as the daemon has them now. */
export async function boardViaApi(
  request: APIRequestContext,
  projectId: string,
): Promise<WireCard[]> {
  const response = await request.get(`/v1/projects/${projectId}/board`, { headers: auth() });
  if (!response.ok()) throw new Error(`could not read the board: ${response.status()}`);
  return ((await response.json()) as { cards: WireCard[] }).cards;
}

/** One card as the daemon has it now, so a spec can ask again after a click and a reload. */
export async function cardViaApi(
  request: APIRequestContext,
  projectId: string,
  id: string,
): Promise<WireCard | null> {
  const response = await request.get(`/v1/projects/${projectId}/cards/${id}`, { headers: auth() });
  return response.ok() ? ((await response.json()) as WireCard) : null;
}

/** One project chat, with the parts a spec checks. */
export interface WireChat {
  id: string;
  projectId: string;
  title: string;
  archived: boolean;
}

/** The chats a project has now, most recently active first. */
export async function chatsViaApi(
  request: APIRequestContext,
  projectId: string,
): Promise<WireChat[]> {
  const response = await request.get(`/v1/projects/${projectId}/chats`, { headers: auth() });
  if (!response.ok()) throw new Error(`could not read the chats: ${response.status()}`);
  return ((await response.json()) as { chats: WireChat[] }).chats;
}

/** A chat's messages as the daemon has them, newest first. */
export async function chatMessagesViaApi(
  request: APIRequestContext,
  chatId: string,
): Promise<{ kind: string; text: string }[]> {
  const response = await request.get(`/v1/chats/${chatId}/messages`, { headers: auth() });
  if (!response.ok()) throw new Error(`could not read the messages: ${response.status()}`);
  return ((await response.json()) as { items: { kind: string; text: string }[] }).items;
}
