import type {
  ActivityOptions,
  ApiClient,
  ApiClientOptions,
  CallOptions,
  HomeActivityOptions,
  Method,
  PageOptions,
  RequestOptions,
} from "./api-client-types";
import { type ApiError, clientError, readWireError, wireError } from "./api-error";
import { isRecord } from "./guards";
import { guardRequest } from "./request-guard";

export type { ApiClient, ApiClientOptions } from "./api-client-types";

const DEFAULT_TIMEOUT_MS = 30_000;
/** Starting or resuming a card makes a worktree and starts a program, so the daemon may take minutes. */
const SLOW_TIMEOUT_MS = 180_000;
const UNAUTHORIZED = 401;

function parseJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

/**
 * A proxy in front of the daemon answers with these when the daemon is not there, and it does not
 * use the daemon's error shape. The dev server does so when the dev daemon is down. The daemon's
 * own 503 has the error shape and is read as `unavailable`.
 */
const BAD_GATEWAY = 502;
const SERVICE_UNAVAILABLE = 503;
const GATEWAY_TIMEOUT = 504;
const GATEWAY_STATUSES: ReadonlySet<number> = new Set([
  BAD_GATEWAY,
  SERVICE_UNAVAILABLE,
  GATEWAY_TIMEOUT,
]);

/**
 * The error for an answer that is not a success: the daemon's own, no daemon behind a proxy, or
 * an answer that is not the daemon's, with its status.
 */
function failure(status: number, text: string): ApiError {
  const body = parseJson(text);
  const wire = readWireError(isRecord(body) ? body.error : undefined);
  if (wire) return wireError(status, wire);
  return clientError(GATEWAY_STATUSES.has(status) ? "unreachable" : "bad_response", status);
}

interface Transport {
  request<T>(method: Method, path: string, options?: RequestOptions): Promise<T>;
  command(method: Method, path: string, options?: RequestOptions): Promise<void>;
  /** An answer that is bytes, such as an image, not JSON. */
  bytes(method: Method, path: string, options?: RequestOptions): Promise<Blob>;
}

interface Received {
  ok: boolean;
  status: number;
  text: string;
  /** The bytes of a success, for a call that asked for them. An error is always text. */
  blob: Blob | null;
  receivedAt: number;
}

/** The headers of one request: the token, what may come back, and what the body is. */
function requestHeaders(token: string | null, o: RequestOptions, wantsBytes: boolean) {
  const headers: Record<string, string> = { Accept: wantsBytes ? "image/*" : "application/json" };
  if (token) headers.Authorization = `Bearer ${token}`;
  if (o.upload) headers["Content-Type"] = o.upload.type;
  else if (o.body !== undefined) headers["Content-Type"] = "application/json";
  return headers;
}

/** The body of one request: the raw image, the JSON, or none. */
function requestBody(o: RequestOptions): Blob | string | null {
  if (o.upload) return o.upload;
  return o.body === undefined ? null : JSON.stringify(o.body);
}

/** The part that talks over the network: headers, time limits, and the way each call can fail. */
function createTransport(options: ApiClientOptions): Transport {
  const { getToken, clock, onUnauthorized, isOffline, localNow = Date.now } = options;
  const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
  const root = (options.baseUrl ?? "").replace(/\/+$/, "");
  const send = options.fetch ?? ((input, init) => globalThis.fetch(input, init));

  async function receive(
    method: Method,
    path: string,
    o: RequestOptions,
    wantsBytes = false,
  ): Promise<Received> {
    const guard = guardRequest(o.signal, o.timeoutMs ?? timeoutMs);
    try {
      const response = await send(root + path, {
        method,
        headers: requestHeaders(getToken(), o, wantsBytes),
        body: requestBody(o),
        signal: guard.signal,
        credentials: "omit",
        cache: "no-store",
      });
      const receivedAt = localNow();
      const { ok, status } = response;
      if (ok && wantsBytes)
        return { ok, status, text: "", blob: await response.blob(), receivedAt };
      return { ok, status, text: await response.text(), blob: null, receivedAt };
    } catch {
      throw clientError(guard.cause() ?? "unreachable");
    } finally {
      guard.release();
    }
  }

  /** Sends one request and returns the text of a success. Throws an `ApiError` for anything else. */
  async function exchange(method: Method, path: string, o: RequestOptions, wantsBytes = false) {
    if (method !== "GET" && isOffline?.()) throw clientError("offline");
    const sentAt = localNow();
    const answer = await receive(method, path, o, wantsBytes);
    if (!answer.ok) {
      const error = failure(answer.status, answer.text);
      if (answer.status === UNAUTHORIZED) onUnauthorized?.(error);
      throw error;
    }
    return { text: answer.text, blob: answer.blob, sentAt, receivedAt: answer.receivedAt };
  }

  return {
    async request<T>(method: Method, path: string, o: RequestOptions = {}): Promise<T> {
      const { text, sentAt, receivedAt } = await exchange(method, path, o);
      const body = parseJson(text);
      if (body === undefined) throw clientError("bad_response");
      if (isRecord(body) && typeof body.serverTime === "string") {
        clock?.observe({ serverTime: body.serverTime, sentAt, receivedAt });
      }
      // The daemon's types are generated from the Go ones and checked against golden files, so the
      // shape is trusted here instead of being checked again on every answer.
      return body as T;
    },
    async command(method, path, o = {}) {
      await exchange(method, path, o);
    },
    async bytes(method, path, o = {}) {
      const { blob } = await exchange(method, path, o, true);
      if (!blob) throw clientError("bad_response");
      return blob;
    },
  };
}

const slow = (o: CallOptions | undefined): CallOptions => ({
  ...o,
  timeoutMs: o?.timeoutMs ?? SLOW_TIMEOUT_MS,
});

/**
 * The paging a list route asks for, as a query string. A value that is not given is left out, so a
 * first page asks for nothing at all and the daemon's own default size decides.
 */
function query(page: PageOptions | ActivityOptions = {}): string {
  const params = new URLSearchParams();
  if (page.limit !== undefined) params.set("limit", String(page.limit));
  if (page.cursor) params.set("cursor", page.cursor);
  if ("kind" in page && page.kind) params.set("kind", page.kind);
  const text = params.toString();
  return text ? `?${text}` : "";
}

/** The Home dashboard's chart range, as a query string. A range that is not given is left out. */
function rangeQuery(range: number | undefined): string {
  return range === undefined ? "" : `?range=${range}`;
}

/** The filters of the Home activity list, as a query string. Only the values given are added. */
function homeActivityQuery(page: HomeActivityOptions = {}): string {
  const params = new URLSearchParams();
  if (page.limit !== undefined) params.set("limit", String(page.limit));
  if (page.cursor) params.set("cursor", page.cursor);
  if (page.kind) params.set("kind", page.kind);
  if (page.project) params.set("project", page.project);
  const text = params.toString();
  return text ? `?${text}` : "";
}

/** The text of a search as a query string. `URLSearchParams` writes `#` as `%23`, so `#41` reaches the daemon whole. */
function searchQuery(text: string): string {
  return `?${new URLSearchParams({ q: text })}`;
}

/**
 * The project a roles request is being made about, as a query string. It is left out when there is
 * none, which is what a caller that is not looking at a project asks for: the routes then report
 * every role as not overridden, because no project is being looked at.
 */
function roleQuery(project: string | undefined): string {
  return project ? `?${new URLSearchParams({ project })}` : "";
}

/** A schedules list's project filter, as a query string. Left out, every schedule is listed. */
function scheduleQuery(project: string | undefined): string {
  return project ? `?${new URLSearchParams({ project })}` : "";
}

/** The typed method of each route. Path segments are escaped, and a call with no body sends none. */
function routeMethods({
  request,
  command,
  bytes,
}: Transport): Omit<ApiClient, "request" | MergeFlowRoute> {
  const id = encodeURIComponent;
  return {
    health: (o) => request("GET", "/v1/health", o),
    whoami: (o) => request("GET", "/v1/auth/whoami", o),
    listProjects: (o) => request("GET", "/v1/projects", o),
    createProject: (body, o) => request("POST", "/v1/projects", { ...o, body }),
    getProject: (pid, o) => request("GET", `/v1/projects/${id(pid)}`, o),
    updateProject: (pid, body, o) => request("PATCH", `/v1/projects/${id(pid)}`, { ...o, body }),
    removeProject: (pid, body, o) => command("DELETE", `/v1/projects/${id(pid)}`, { ...o, body }),
    board: (pid, o) => request("GET", `/v1/projects/${id(pid)}/board`, o),
    createCard: (pid, body, o) => request("POST", `/v1/projects/${id(pid)}/cards`, { ...o, body }),
    // Everything a person does while looking at one card, restore points included: `cardMethods`.
    ...cardMethods({ request, command }),
    ...panelMethods({ request, bytes }),
    ...settingsMethods({ request, command }),
    // A project's lessons (task 7.13, B7.6): listed for the lessons screen, saved without a slug the
    // first time and with one to edit an existing lesson, removed by slug.
    listLessons: (pid, o) => request("GET", `/v1/projects/${id(pid)}/lessons`, o),
    lesson: (pid, slug, o) => request("GET", `/v1/projects/${id(pid)}/lessons/${id(slug)}`, o),
    saveLesson: (pid, body, slug, o) =>
      slug
        ? request("PUT", `/v1/projects/${id(pid)}/lessons/${id(slug)}`, { ...o, body })
        : request("POST", `/v1/projects/${id(pid)}/lessons`, { ...o, body }),
    removeLesson: (pid, slug, o) =>
      command("DELETE", `/v1/projects/${id(pid)}/lessons/${id(slug)}`, o),
    // Not a card route (`cardMethods`): the id this takes is the approval's own, not a card's (see
    // the interface's doc comment above).
    decideApproval: (aid, body, o) => command("POST", `/v1/approvals/${id(aid)}`, { ...o, body }),
    listLabels: (pid, o) => request("GET", `/v1/projects/${id(pid)}/labels`, o),
    createLabel: (pid, body, o) =>
      request("POST", `/v1/projects/${id(pid)}/labels`, { ...o, body }),
    updateLabel: (lid, body, o) => request("PATCH", `/v1/labels/${id(lid)}`, { ...o, body }),
    removeLabel: (lid, o) => command("DELETE", `/v1/labels/${id(lid)}`, o),
    home: (o) => request("GET", `/v1/home/dashboard${rangeQuery(o?.range)}`, o),
    homeActivity: (page, o) => request("GET", `/v1/home/activity${homeActivityQuery(page)}`, o),
    // Every project's CI health in one answer (section S21, B6.2 to B6.4).
    ciSnapshot: (o) => request("GET", "/v1/ci", o),
    agents: (o) => request("GET", "/v1/agents", o),
    refreshAgents: (o) => request("POST", "/v1/agents/refresh", slow(o)),
    detectTelegramChat: (body, o) =>
      request("POST", "/v1/integrations/telegram/detect-chat", { ...slow(o), body }),
    folders: (path, o) =>
      request("GET", `/v1/folders${path ? `?${new URLSearchParams({ path })}` : ""}`, o),
    testAgent: (aid, o) => request("POST", `/v1/agents/${id(aid)}/test`, slow(o)),
    listChats: (pid, archived, o) =>
      request("GET", `/v1/projects/${id(pid)}/chats${archived ? "?archived=true" : ""}`, o),
    createChat: (pid, body, o) => request("POST", `/v1/projects/${id(pid)}/chats`, { ...o, body }),
    updateChat: (cid, body, o) => request("PATCH", `/v1/chats/${id(cid)}`, { ...o, body }),
    archiveChat: (cid, o) => request("POST", `/v1/chats/${id(cid)}/archive`, slow(o)),
    restoreChat: (cid, o) => request("POST", `/v1/chats/${id(cid)}/restore`, slow(o)),
    deleteChat: (cid, o) => command("DELETE", `/v1/chats/${id(cid)}`, o),
    sendChatMessage: (cid, body, o) =>
      command("POST", `/v1/chats/${id(cid)}/messages`, { ...slow(o), body }),
    chatMessages: (cid, page, o) =>
      request("GET", `/v1/chats/${id(cid)}/messages${query(page)}`, o),
    chatMessageDetail: (cid, mid, o) =>
      request("GET", `/v1/chats/${id(cid)}/messages/${id(mid)}`, o),
    search: (text, o) => request("GET", `/v1/search${searchQuery(text)}`, o),
    me: (o) => request("GET", "/v1/me", o),
    updateMe: (body, o) => request("PATCH", "/v1/me", { ...o, body }),
    uploadAvatar: (image, o) => request("POST", "/v1/me/avatar", { ...o, upload: image }),
    removeAvatar: (o) => request("DELETE", "/v1/me/avatar", o),
    // The address is the one the profile carries, which already has the daemon's own version in it.
    avatarImage: (avatarUrl, o) => bytes("GET", avatarUrl, o),
    // A screenshot's address is the one the shot carries, with the daemon's own version in it.
    previewShotImage: (shotUrl, o) => bytes("GET", shotUrl, o),
    users: (o) => request("GET", "/v1/users", o),
    progress: (o) => request("GET", "/v1/me/progress", o),
    updateProgress: (body, o) => request("PATCH", "/v1/me/progress", { ...o, body }),
    preferences: (o) => request("GET", "/v1/me/preferences", o),
    updatePreferences: (body, o) => request("PATCH", "/v1/me/preferences", { ...o, body }),
    resetFirstLaunch: (o) => request("POST", "/v1/dev/reset-first-launch", o),
    // The paired devices and the tailnet (B9.1, B9.2, section S2b): the list a phone appears in,
    // the code a new one is paired with, revoking one, and what the daemon's own node on the tailnet
    // is doing - including whether /hooks/* was asked to be exposed to the public internet.
    listDevices: (o) => request("GET", "/v1/me/devices", o),
    createPairingCode: (o) => request("POST", "/v1/me/devices/pairing-code", o),
    pairDevice: (body, o) => request("POST", "/v1/devices/pair", { ...o, body }),
    removeDevice: (did, o) => command("DELETE", `/v1/me/devices/${id(did)}`, o),
    tailnetStatus: (o) => request("GET", "/v1/tailnet", o),
    tailnetPeers: (o) => request("GET", "/v1/tailnet/peers", o),
    listSavedViews: (pid, o) => request("GET", `/v1/projects/${id(pid)}/saved-views`, o),
    createSavedView: (pid, body, o) =>
      request("POST", `/v1/projects/${id(pid)}/saved-views`, { ...o, body }),
    updateSavedView: (vid, body, o) =>
      request("PATCH", `/v1/saved-views/${id(vid)}`, { ...o, body }),
    removeSavedView: (vid, o) => command("DELETE", `/v1/saved-views/${id(vid)}`, o),
    listProviders: (o) => request("GET", "/v1/providers", o),
    // Saving runs a test of the new key, which calls the provider, so it is given the slow limit.
    saveProvider: (pid, body, o) =>
      request("PUT", `/v1/providers/${id(pid)}`, { ...slow(o), body }),
    removeProvider: (pid, o) => request("DELETE", `/v1/providers/${id(pid)}`, o),
    testProvider: (pid, o) => request("POST", `/v1/providers/${id(pid)}/test`, slow(o)),
    // The connections Marshal can be set up with (section S29a). Saving is slow because the daemon
    // runs the connection's own test right after it stores the settings.
    listIntegrations: (o) => request("GET", "/v1/integrations", o),
    saveIntegration: (iid, body, o) =>
      request("PUT", `/v1/integrations/${id(iid)}`, { ...slow(o), body }),
    removeIntegration: (iid, o) => request("DELETE", `/v1/integrations/${id(iid)}`, o),
    testIntegration: (iid, o) => request("POST", `/v1/integrations/${id(iid)}/test`, slow(o)),
    // GitHub's sign-in (section S29a): start, read, and cancel one flow, and save or test a pasted
    // token. A token's save and test reach GitHub, so they are given the slow limit.
    startGitHubConnect: (o) => request("POST", "/v1/integrations/github/connect", o),
    readGitHubConnect: (o) => request("GET", "/v1/integrations/github/connect", o),
    cancelGitHubConnect: (o) => request("DELETE", "/v1/integrations/github/connect", o),
    saveGitHubToken: (body, o) =>
      request("PUT", "/v1/integrations/github/token", { ...slow(o), body }),
    testGitHubToken: (body, o) =>
      request("POST", "/v1/integrations/github/token/test", { ...slow(o), body }),
    authorizeGoogleCalendar: (o) => request("GET", "/v1/integrations/gcal/authorize", o),
    // Schedules and the calendar (B8.1, B8.4, N21): the Schedules screen's own list, add, edit, and
    // delete, and the one call the calendar view and Home's coming-up list read.
    listSchedules: (project, o) => request("GET", `/v1/schedules${scheduleQuery(project)}`, o),
    createSchedule: (body, o) => request("POST", "/v1/schedules", { ...o, body }),
    saveSchedule: (sid, body, o) => request("PUT", `/v1/schedules/${id(sid)}`, { ...o, body }),
    deleteSchedule: (sid, o) => command("DELETE", `/v1/schedules/${id(sid)}`, o),
    scheduleRuns: (sid, o) => request("GET", `/v1/schedules/${id(sid)}/runs`, o),
    getCalendar: (start, end, o) => request("GET", `/v1/calendar?start=${start}&end=${end}`, o),
  };
}

type MergeFlowRoute =
  | "integrationState"
  | "pauseIntegration"
  | "resumeIntegration"
  | "retryCardMerge"
  | "undoCardMerge"
  | "openCardWorktree";

/**
 * The routes of the Integration view. A retry runs a merge or a delivery again and an undo moves a
 * branch back, so both take the slow limit; opening a folder answers no body.
 */
function mergeFlowMethods({
  request,
  command,
}: Pick<Transport, "request" | "command">): Pick<ApiClient, MergeFlowRoute> {
  const id = encodeURIComponent;
  return {
    integrationState: (pid, o) => request("GET", `/v1/projects/${id(pid)}/integration`, o),
    pauseIntegration: (pid, o) => request("POST", `/v1/projects/${id(pid)}/integration/pause`, o),
    resumeIntegration: (pid, o) => request("POST", `/v1/projects/${id(pid)}/integration/resume`, o),
    retryCardMerge: (cid, o) => request("POST", `/v1/cards/${id(cid)}/merge/retry`, slow(o)),
    undoCardMerge: (cid, o) => request("POST", `/v1/cards/${id(cid)}/merge/undo`, slow(o)),
    openCardWorktree: (cid, body, o) =>
      command("POST", `/v1/cards/${id(cid)}/worktree/open`, { ...o, body }),
  };
}

/** Every route of the client that acts on one card. `cardMethods` answers exactly these. */
type CardRoute =
  | "getCard"
  | "moveCard"
  | "updateCard"
  | "removeCard"
  | "forkCard"
  | "simulateCIFailure"
  | "setCardBypass"
  | "clearCardBypass"
  | "approvePlan"
  | "rejectPlan"
  | "editPlan"
  | "checkpoints"
  | "restoreCheckpoint"
  | "preview"
  | "startPreview"
  | "stopPreview"
  | "takePreviewShot"
  | "messages"
  | "activity"
  | "messageDetail"
  | "diff"
  | "fileHunks"
  | "startCard"
  | "sendMessage"
  | "stopCard"
  | "resumeCard"
  | "pauseCard"
  | "unpauseCard"
  | "sleepCard"
  | "wakeCard"
  | "pinCard"
  | "unpinCard"
  | "setCardView"
  | "note"
  | "saveNote";

type PanelRoute =
  | "cardChecks"
  | "addCardCheck"
  | "removeCardCheck"
  | "runCardChecks"
  | "checklists"
  | "createChecklist"
  | "updateChecklist"
  | "deleteChecklist"
  | "addChecklistItem"
  | "tickChecklistItem"
  | "removeChecklistItem"
  | "comments"
  | "postComment"
  | "deleteComment"
  | "attachmentFile"
  | "cardMembers"
  | "addCardMember"
  | "removeCardMember";

/**
 * The routes that act on one card, gathered so the group can grow without crowding `routeMethods`.
 * Two of them are shaped by the rules around them: the plan routes (section S8c) name the card and
 * not the message, because the plan a card waits on is the newest one it holds
 * (docs/architecture.md 10.4); and a restore's body is optional, so putting a card's worktree back
 * to a restore point sends none. The diff path is a wildcard segment on the daemon's own route
 * (`{path...}`), so its slashes stay literal and only each segment between them is escaped.
 */
function cardMethods({
  request,
  command,
}: Pick<Transport, "request" | "command">): Pick<ApiClient, CardRoute> {
  const id = encodeURIComponent;
  return {
    getCard: (cid, o) => request("GET", `/v1/cards/${id(cid)}`, o),
    moveCard: (cid, body, o) => request("POST", `/v1/cards/${id(cid)}/move`, { ...o, body }),
    updateCard: (cid, body, o) => request("PATCH", `/v1/cards/${id(cid)}`, { ...o, body }),
    removeCard: (cid, o) => command("DELETE", `/v1/cards/${id(cid)}`, o),
    forkCard: (cid, o) => request("POST", `/v1/cards/${id(cid)}/fork`, slow(o)),
    // Starting a real simulated failure pushes a commit and waits for the forge, so both modes take
    // the slow limit; the route is the daemon's own and is refused outright in normal mode.
    simulateCIFailure: (cid, body, o) =>
      request("POST", `/v1/cards/${id(cid)}/ci-failure`, { ...slow(o), body }),
    setCardBypass: (cid, body, o) => request("POST", `/v1/cards/${id(cid)}/bypass`, { ...o, body }),
    clearCardBypass: (cid, o) => request("DELETE", `/v1/cards/${id(cid)}/bypass`, o),
    approvePlan: (cid, o) => request("POST", `/v1/cards/${id(cid)}/plan/approve`, o),
    rejectPlan: (cid, o) => request("POST", `/v1/cards/${id(cid)}/plan/reject`, o),
    editPlan: (cid, body, o) => request("PUT", `/v1/cards/${id(cid)}/plan`, { ...o, body }),
    // A card's note (section S14, B7.4, N11, task 7.12): one file per card, read and replaced whole.
    note: (cid, o) => request("GET", `/v1/cards/${id(cid)}/note`, o),
    saveNote: (cid, body, o) => request("PUT", `/v1/cards/${id(cid)}/note`, { ...o, body }),
    // The restore points of a card (B5.3, section S10): the list is a read, and a restore names the
    // checkpoint in the path.
    checkpoints: (cid, o) => request("GET", `/v1/cards/${id(cid)}/checkpoints`, o),
    restoreCheckpoint: (cid, cp, body, o) =>
      request("POST", `/v1/cards/${id(cid)}/checkpoints/${id(cp)}/restore`, { ...o, body }),
    // A card's live preview (section S13, B6.6): reading starts nothing; starting runs the project's
    // dev command, which may take a while, so it takes the slow limit; and taking a screenshot drives
    // a browser, which is slower still, so that takes the slow limit too.
    preview: (cid, o) => request("GET", `/v1/cards/${id(cid)}/preview`, o),
    startPreview: (cid, o) => request("POST", `/v1/cards/${id(cid)}/preview/start`, slow(o)),
    stopPreview: (cid, o) => request("POST", `/v1/cards/${id(cid)}/preview/stop`, o),
    takePreviewShot: (cid, body, o) =>
      request("POST", `/v1/cards/${id(cid)}/preview/shots`, { ...slow(o), body }),
    messages: (cid, page, o) => request("GET", `/v1/cards/${id(cid)}/messages${query(page)}`, o),
    activity: (cid, page, o) => request("GET", `/v1/cards/${id(cid)}/activity${query(page)}`, o),
    messageDetail: (cid, mid, o) => request("GET", `/v1/cards/${id(cid)}/messages/${id(mid)}`, o),
    diff: (cid, o) => request("GET", `/v1/cards/${id(cid)}/diff`, o),
    fileHunks: (cid, path, o) =>
      request("GET", `/v1/cards/${id(cid)}/diff/${path.split("/").map(id).join("/")}`, o),
    startCard: (cid, o) => request("POST", `/v1/cards/${id(cid)}/start`, slow(o)),
    sendMessage: (cid, body, o) =>
      command("POST", `/v1/cards/${id(cid)}/messages`, { ...slow(o), body }),
    stopCard: (cid, o) => command("POST", `/v1/cards/${id(cid)}/stop`, o),
    resumeCard: (cid, o) => command("POST", `/v1/cards/${id(cid)}/resume`, slow(o)),
    pauseCard: (cid, o) => request("POST", `/v1/cards/${id(cid)}/pause`, o),
    unpauseCard: (cid, o) => request("POST", `/v1/cards/${id(cid)}/unpause`, o),
    sleepCard: (cid, o) => command("POST", `/v1/cards/${id(cid)}/sleep`, o),
    wakeCard: (cid, o) => command("POST", `/v1/cards/${id(cid)}/wake`, slow(o)),
    pinCard: (cid, o) => request("POST", `/v1/cards/${id(cid)}/pin`, o),
    unpinCard: (cid, o) => request("POST", `/v1/cards/${id(cid)}/unpin`, o),
    setCardView: (cid, body, o) =>
      request("POST", `/v1/cards/${id(cid)}/view`, { ...slow(o), body }),
  };
}

/**
 * The one client for every call to the daemon. It sends the token, times each call out, tells
 * apart the ways a call can fail (`ApiError`), and keeps the daemon's clock. It never logs a
 * request or an answer.
 */
export function createApiClient(options: ApiClientOptions): ApiClient {
  const transport = createTransport(options);
  return { request: transport.request, ...routeMethods(transport), ...mergeFlowMethods(transport) };
}

/**
 * The routes of what a card's panel holds beyond the card (B10.2, B10.5, B10.6): checks,
 * checklists, comments, and members. Each call answers the whole list, so a screen redraws from one
 * answer.
 */
function panelMethods({
  request,
  bytes,
}: Pick<Transport, "request" | "bytes">): Pick<ApiClient, PanelRoute> {
  const id = encodeURIComponent;
  return {
    cardChecks: (cid, o) => request("GET", `/v1/cards/${id(cid)}/checks`, o),
    addCardCheck: (cid, body, o) => request("POST", `/v1/cards/${id(cid)}/checks`, { ...o, body }),
    removeCardCheck: (cid, kid, o) =>
      request("DELETE", `/v1/cards/${id(cid)}/checks/${id(kid)}`, o),
    runCardChecks: (cid, o) => request("POST", `/v1/cards/${id(cid)}/checks/run`, slow(o)),
    checklists: (cid, o) => request("GET", `/v1/cards/${id(cid)}/checklists`, o),
    createChecklist: (cid, body, o) =>
      request("POST", `/v1/cards/${id(cid)}/checklists`, { ...o, body }),
    updateChecklist: (cid, lid, body, o) =>
      request("PATCH", `/v1/cards/${id(cid)}/checklists/${id(lid)}`, { ...o, body }),
    deleteChecklist: (cid, lid, o) =>
      request("DELETE", `/v1/cards/${id(cid)}/checklists/${id(lid)}`, o),
    addChecklistItem: (cid, lid, body, o) =>
      request("POST", `/v1/cards/${id(cid)}/checklists/${id(lid)}/items`, { ...o, body }),
    tickChecklistItem: (cid, lid, iid, body, o) =>
      request("PUT", `/v1/cards/${id(cid)}/checklists/${id(lid)}/items/${id(iid)}`, { ...o, body }),
    removeChecklistItem: (cid, lid, iid, o) =>
      request("DELETE", `/v1/cards/${id(cid)}/checklists/${id(lid)}/items/${id(iid)}`, o),
    comments: (cid, o) => request("GET", `/v1/cards/${id(cid)}/comments`, o),
    postComment: (cid, body, o) => request("POST", `/v1/cards/${id(cid)}/comments`, { ...o, body }),
    deleteComment: (cid, kid, o) =>
      request("DELETE", `/v1/cards/${id(cid)}/comments/${id(kid)}`, o),
    attachmentFile: (cid, aid, o) => bytes("GET", `/v1/cards/${id(cid)}/attachments/${id(aid)}`, o),
    cardMembers: (cid, o) => request("GET", `/v1/cards/${id(cid)}/members`, o),
    addCardMember: (cid, uid, o) => request("PUT", `/v1/cards/${id(cid)}/members/${id(uid)}`, o),
    removeCardMember: (cid, uid, o) =>
      request("DELETE", `/v1/cards/${id(cid)}/members/${id(uid)}`, o),
  };
}

type SettingsRoute =
  | "listLimits"
  | "setLimit"
  | "deleteLimit"
  | "listRoles"
  | "createRole"
  | "getRole"
  | "updateRole"
  | "deleteRole"
  | "resetRole"
  | "setRoleOverride"
  | "listNotices"
  | "noticeAction"
  | "dismissNotice"
  | "sleepSettings"
  | "saveSleepSettings"
  | "alertSettings"
  | "saveAlertSettings"
  | "authorizeGmail"
  | "googleClient"
  | "googleCalendars"
  | "setGoogleCalendars";

/** The routes of the limits, the roles, the notices, and the sleep and alert settings. */
function settingsMethods({
  request,
  command,
}: Pick<Transport, "request" | "command">): Pick<ApiClient, SettingsRoute> {
  const id = encodeURIComponent;
  return {
    listLimits: (o) => request("GET", "/v1/limits", o),
    setLimit: (scope, kind, body, o) =>
      request("PUT", `/v1/limits/${id(scope)}/${id(kind)}`, { ...o, body }),
    deleteLimit: (scope, kind, o) => request("DELETE", `/v1/limits/${id(scope)}/${id(kind)}`, o),
    listRoles: (project, o) => request("GET", `/v1/roles${roleQuery(project)}`, o),
    createRole: (body, project, o) =>
      request("POST", `/v1/roles${roleQuery(project)}`, { ...o, body }),
    getRole: (name, project, o) => request("GET", `/v1/roles/${id(name)}${roleQuery(project)}`, o),
    updateRole: (name, body, project, o) =>
      request("PATCH", `/v1/roles/${id(name)}${roleQuery(project)}`, { ...o, body }),
    // A delete answers the whole list, so it is a `request` and not a `command`.
    deleteRole: (name, project, o) =>
      request("DELETE", `/v1/roles/${id(name)}${roleQuery(project)}`, o),
    resetRole: (name, project, o) =>
      request("POST", `/v1/roles/${id(name)}/reset${roleQuery(project)}`, o),
    setRoleOverride: (name, project, spec, o) =>
      request("PUT", `/v1/roles/${id(name)}/override${roleQuery(project)}`, { ...o, body: spec }),
    listNotices: (o) => request("GET", "/v1/notices", o),
    noticeAction: (notice, body, o) =>
      request("POST", `/v1/notices/${id(notice)}/actions`, { ...o, body }),
    // Taking a notice off the panel answers no body, so it is a `command`.
    dismissNotice: (notice, o) => command("DELETE", `/v1/notices/${id(notice)}`, o),
    sleepSettings: (o) => request("GET", "/v1/settings/sleep", o),
    saveSleepSettings: (body, o) => request("PUT", "/v1/settings/sleep", { ...o, body }),
    alertSettings: (o) => request("GET", "/v1/settings/alerts", o),
    saveAlertSettings: (body, o) => request("PUT", "/v1/settings/alerts", { ...o, body }),
    // Gmail's own consent, and the choice of which Google calendars Marshal reads (S29d, S29e).
    authorizeGmail: (o) => request("GET", "/v1/integrations/gmail/authorize", o),
    googleClient: (o) => request("GET", "/v1/integrations/gcal/client", o),
    googleCalendars: (o) => request("GET", "/v1/integrations/gcal/calendars", o),
    setGoogleCalendars: (body, o) =>
      request("PUT", "/v1/integrations/gcal/calendars", { ...slow(o), body }),
  };
}
