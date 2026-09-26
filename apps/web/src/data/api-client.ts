import type {
  ActivityItem,
  AgentCatalog,
  BoardSnapshot,
  Card,
  CardDiff,
  CardView,
  Chat,
  ChatListSnapshot,
  ChatMessage,
  ChatMessageDetail,
  CreateCardRequest,
  CreateChatRequest,
  CreateLabelRequest,
  CreateProjectRequest,
  CreateSavedViewRequest,
  FeedEntry,
  FileHunks,
  Health,
  HomeSnapshot,
  Label,
  LabelSnapshot,
  MoveCardRequest,
  Page,
  Preferences,
  Profile,
  Progress,
  Project,
  ProjectListSnapshot,
  RemoveProjectRequest,
  SavedView,
  SavedViewListSnapshot,
  SearchSnapshot,
  SendMessageRequest,
  SetViewRequest,
  UpdateCardRequest,
  UpdateChatRequest,
  UpdateLabelRequest,
  UpdatePreferencesRequest,
  UpdateProfileRequest,
  UpdateProgressRequest,
  UpdateProjectRequest,
  UpdateSavedViewRequest,
  UserListSnapshot,
  WhoAmI,
} from "@marshal/protocol";
import { type ApiError, clientError, readWireError, wireError } from "./api-error";
import type { DaemonClock } from "./daemon-clock";
import { isRecord } from "./guards";
import { guardRequest } from "./request-guard";

type Method = "GET" | "POST" | "PATCH" | "DELETE";

const DEFAULT_TIMEOUT_MS = 30_000;
/** Starting or resuming a card makes a worktree and starts a program, so the daemon may take minutes. */
const SLOW_TIMEOUT_MS = 180_000;
const UNAUTHORIZED = 401;

/** Options of one call. */
interface CallOptions {
  /** Cancels the request. The call then fails with the code `aborted`. */
  signal?: AbortSignal | undefined;
  /** Replaces the time limit of the client for this call. */
  timeoutMs?: number | undefined;
}

/** The paging a list route takes: a size, and the cursor of the last page read. */
interface PageOptions {
  limit?: number;
  cursor?: string;
}

/** The activity list, which can be narrowed to one kind of entry. */
interface ActivityOptions extends PageOptions {
  kind?: string;
}

/** The Home dashboard, whose `range` says how many days the charts cover. */
interface HomeOptions extends CallOptions {
  /** 7, 30, or 90 days. Left out, the daemon answers with seven. */
  range?: number;
}

/** The Home activity list, which can be narrowed to one kind and one project. */
interface HomeActivityOptions extends PageOptions {
  kind?: string;
  project?: string;
}

interface RequestOptions extends CallOptions {
  /** Sent as JSON. Leave it out for a request with no body. */
  body?: unknown;
  /** Sent as it is, with the image's own `Content-Type` (an avatar upload), and not as JSON. */
  upload?: Blob;
}

export interface ApiClientOptions {
  /** The daemon's address, such as `http://127.0.0.1:47800`. Empty means the address of the page. */
  baseUrl?: string;
  /** The token to send. With none, the request is still sent, and the daemon answers 401. */
  getToken: () => string | null;
  fetch?: typeof fetch;
  /** Learns the daemon's clock from every answer that has a `serverTime`. */
  clock?: Pick<DaemonClock, "observe">;
  /** The time limit of a call, in ms. Starting and resuming a card have their own, longer one. */
  timeoutMs?: number;
  /** Called once for every 401 answer, before the error is thrown. */
  onUnauthorized?: (error: ApiError) => void;
  /** The local clock in ms. It is only for the clock offset, and tests replace it. */
  localNow?: () => number;
}

/** One typed method for each route of the daemon (architecture.md section 11.1). */
export interface ApiClient {
  /** The path every method below is built on. Use it for a route that has no method yet. */
  request<T>(method: Method, path: string, options?: RequestOptions): Promise<T>;
  health(options?: CallOptions): Promise<Health>;
  whoami(options?: CallOptions): Promise<WhoAmI>;
  listProjects(options?: CallOptions): Promise<ProjectListSnapshot>;
  createProject(body: CreateProjectRequest, options?: CallOptions): Promise<Project>;
  getProject(id: string, options?: CallOptions): Promise<Project>;
  updateProject(id: string, body: UpdateProjectRequest, options?: CallOptions): Promise<Project>;
  removeProject(id: string, body?: RemoveProjectRequest, options?: CallOptions): Promise<void>;
  board(projectId: string, options?: CallOptions): Promise<BoardSnapshot>;
  createCard(projectId: string, body: CreateCardRequest, options?: CallOptions): Promise<Card>;
  getCard(id: string, options?: CallOptions): Promise<Card>;
  /** Moves a card by hand. A refused move fails with the reason the app shows. */
  moveCard(id: string, body: MoveCardRequest, options?: CallOptions): Promise<Card>;
  /** Changes the fields a person set. A field that is left out is not touched. */
  updateCard(id: string, body: UpdateCardRequest, options?: CallOptions): Promise<Card>;
  /** Removes the card, its session, its worktree, and its branch. */
  removeCard(id: string, options?: CallOptions): Promise<void>;
  /** Adds a card in the backlog that starts from this card's latest commit. */
  forkCard(id: string, options?: CallOptions): Promise<Card>;
  listLabels(projectId: string, options?: CallOptions): Promise<LabelSnapshot>;
  createLabel(projectId: string, body: CreateLabelRequest, options?: CallOptions): Promise<Label>;
  updateLabel(id: string, body: UpdateLabelRequest, options?: CallOptions): Promise<Label>;
  removeLabel(id: string, options?: CallOptions): Promise<void>;
  /** The numbers Home draws, and the cards behind them. `range` picks 7, 30, or 90 days. */
  home(options?: HomeOptions): Promise<HomeSnapshot>;
  /** The Home activity stream, newest first, one page at a time, filtered by kind and project. */
  homeActivity(page?: HomeActivityOptions, options?: CallOptions): Promise<Page<FeedEntry>>;
  /** A card's chat, newest first, one page at a time. */
  messages(cardId: string, page?: PageOptions, options?: CallOptions): Promise<Page<ChatMessage>>;
  /** A card's activity, newest first, one page at a time, optionally one kind of it. */
  activity(
    cardId: string,
    page?: ActivityOptions,
    options?: CallOptions,
  ): Promise<Page<ActivityItem>>;
  /** One chat message in full: a tool call's output and the files it changed. */
  messageDetail(
    cardId: string,
    messageId: string,
    options?: CallOptions,
  ): Promise<ChatMessageDetail>;
  /** A card's changed files with their counts, and no hunks. */
  diff(cardId: string, options?: CallOptions): Promise<CardDiff>;
  /** One changed file's hunks, loaded when the screen opens it. */
  fileHunks(cardId: string, path: string, options?: CallOptions): Promise<FileHunks>;
  startCard(id: string, options?: CallOptions): Promise<Card>;
  sendMessage(id: string, body: SendMessageRequest, options?: CallOptions): Promise<void>;
  stopCard(id: string, options?: CallOptions): Promise<void>;
  resumeCard(id: string, options?: CallOptions): Promise<void>;
  /** Holds a working card between turns: the turn running finishes, and a message sent meanwhile
   * waits until the card is resumed. Refused when the card is not working. */
  pauseCard(id: string, options?: CallOptions): Promise<Card>;
  /** Releases a pause and delivers the message it was holding. Safe to call on a card that is not paused. */
  unpauseCard(id: string, options?: CallOptions): Promise<Card>;
  /** Stops the card's agent process and keeps the session id, so Wake or Start brings it back. */
  sleepCard(id: string, options?: CallOptions): Promise<void>;
  /** Resumes a sleeping card's session through its saved id. */
  wakeCard(id: string, options?: CallOptions): Promise<void>;
  /** Keeps a card from sleeping on its own. */
  pinCard(id: string, options?: CallOptions): Promise<Card>;
  /** Undoes a pin. Safe to call on a card that is not pinned. */
  unpinCard(id: string, options?: CallOptions): Promise<Card>;
  /** Switches a card between the chat view and the terminal view (section S9, docs/architecture.md
   * 4.3): it stops the current process and resumes the same session in the other mode, so it gets
   * the same long timeout as `startCard`/`resumeCard`/`wakeCard`. Asking for the view the card is
   * already in changes nothing. A refusal carries the daemon's own sentence and a stable reason. */
  setCardView(id: string, body: SetViewRequest, options?: CallOptions): Promise<CardView>;
  agents(options?: CallOptions): Promise<AgentCatalog>;
  refreshAgents(options?: CallOptions): Promise<AgentCatalog>;
  /** A project's chats. `archived` asks for the archived ones instead of the live ones. */
  listChats(
    projectId: string,
    archived?: boolean,
    options?: CallOptions,
  ): Promise<ChatListSnapshot>;
  /** Makes a chat in a project, with its own session. */
  createChat(projectId: string, body: CreateChatRequest, options?: CallOptions): Promise<Chat>;
  /** Renames a chat. A body with no title leaves its name alone. */
  updateChat(id: string, body: UpdateChatRequest, options?: CallOptions): Promise<Chat>;
  /** Takes a chat out of the main list and puts its session to sleep. */
  archiveChat(id: string, options?: CallOptions): Promise<Chat>;
  /** Brings an archived chat back to the main list. */
  restoreChat(id: string, options?: CallOptions): Promise<Chat>;
  /** Deletes a chat: its session stops, its logs go, and the chat goes. Its cards stay. */
  deleteChat(id: string, options?: CallOptions): Promise<void>;
  /**
   * Sends a person's message into a chat's own session. The answer arrives on the chat's topic, so
   * this has none. A chat's first message starts its agent, and a message to a sleeping chat
   * resumes it, so the call gets the same long time to answer a card's start does.
   */
  sendChatMessage(chatId: string, body: SendMessageRequest, options?: CallOptions): Promise<void>;
  /** A chat's messages, newest first, one page at a time: the same shape a card's chat has. */
  chatMessages(
    chatId: string,
    page?: PageOptions,
    options?: CallOptions,
  ): Promise<Page<ChatMessage>>;
  /** One chat message in full: a tool call's output and the files it changed. */
  chatMessageDetail(
    chatId: string,
    messageId: string,
    options?: CallOptions,
  ): Promise<ChatMessageDetail>;
  /**
   * The projects, cards, and chats that match what was typed, each kind best match first and cut to
   * a few. An empty query matches nothing. The answer's `query` is the text the daemon searched
   * for, cleaned, so a caller that types ahead can tell an answer that is not the latest.
   */
  search(query: string, options?: CallOptions): Promise<SearchSnapshot>;
  /** The person the token belongs to. */
  me(options?: CallOptions): Promise<Profile>;
  /** Changes the fields that are sent. A field that is left out is not touched, and `""` clears an email or a time zone. */
  updateMe(body: UpdateProfileRequest, options?: CallOptions): Promise<Profile>;
  /** Sets the avatar. The image is the body as it is, and its type (PNG, JPEG, or WebP) is the `Content-Type`. */
  uploadAvatar(image: Blob, options?: CallOptions): Promise<Profile>;
  /** Removes the avatar. Removing one that is not there is not an error. */
  removeAvatar(options?: CallOptions): Promise<Profile>;
  /**
   * The bytes of an avatar, from the `avatarUrl` a profile or a user carries. The route needs the
   * token, so an `<img>` cannot load it; the caller shows the bytes through an object URL.
   */
  avatarImage(avatarUrl: string, options?: CallOptions): Promise<Blob>;
  /** Everyone who can be put on a card. Solo use lists the owner only. */
  users(options?: CallOptions): Promise<UserListSnapshot>;
  /** How far the person is with onboarding and with the tour. */
  progress(options?: CallOptions): Promise<Progress>;
  updateProgress(body: UpdateProgressRequest, options?: CallOptions): Promise<Progress>;
  /** The screen preferences that follow the person between devices. */
  preferences(options?: CallOptions): Promise<Preferences>;
  /** Saves what is sent and merges it with the rest: columns by key, projects by id and by field. */
  updatePreferences(body: UpdatePreferencesRequest, options?: CallOptions): Promise<Preferences>;
  /** Puts onboarding and the tour back to the start. It exists only on a daemon that runs in dev mode. */
  resetFirstLaunch(options?: CallOptions): Promise<Progress>;
  listSavedViews(projectId: string, options?: CallOptions): Promise<SavedViewListSnapshot>;
  /** Saves a view. A name the project already uses replaces that view and keeps its id. */
  createSavedView(
    projectId: string,
    body: CreateSavedViewRequest,
    options?: CallOptions,
  ): Promise<SavedView>;
  updateSavedView(
    id: string,
    body: UpdateSavedViewRequest,
    options?: CallOptions,
  ): Promise<SavedView>;
  removeSavedView(id: string, options?: CallOptions): Promise<void>;
}

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
  const { getToken, clock, onUnauthorized, localNow = Date.now } = options;
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

/** The typed method of each route. Path segments are escaped, and a call with no body sends none. */
function routeMethods({ request, command, bytes }: Transport): Omit<ApiClient, "request"> {
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
    getCard: (cid, o) => request("GET", `/v1/cards/${id(cid)}`, o),
    moveCard: (cid, body, o) => request("POST", `/v1/cards/${id(cid)}/move`, { ...o, body }),
    updateCard: (cid, body, o) => request("PATCH", `/v1/cards/${id(cid)}`, { ...o, body }),
    removeCard: (cid, o) => command("DELETE", `/v1/cards/${id(cid)}`, o),
    forkCard: (cid, o) => request("POST", `/v1/cards/${id(cid)}/fork`, slow(o)),
    listLabels: (pid, o) => request("GET", `/v1/projects/${id(pid)}/labels`, o),
    createLabel: (pid, body, o) =>
      request("POST", `/v1/projects/${id(pid)}/labels`, { ...o, body }),
    updateLabel: (lid, body, o) => request("PATCH", `/v1/labels/${id(lid)}`, { ...o, body }),
    removeLabel: (lid, o) => command("DELETE", `/v1/labels/${id(lid)}`, o),
    home: (o) => request("GET", `/v1/home/dashboard${rangeQuery(o?.range)}`, o),
    homeActivity: (page, o) => request("GET", `/v1/home/activity${homeActivityQuery(page)}`, o),
    messages: (cid, page, o) => request("GET", `/v1/cards/${id(cid)}/messages${query(page)}`, o),
    activity: (cid, page, o) => request("GET", `/v1/cards/${id(cid)}/activity${query(page)}`, o),
    messageDetail: (cid, mid, o) => request("GET", `/v1/cards/${id(cid)}/messages/${id(mid)}`, o),
    diff: (cid, o) => request("GET", `/v1/cards/${id(cid)}/diff`, o),
    // The path is a wildcard segment on the daemon's own route (`{path...}`), so its slashes stay
    // literal; only each segment between them is escaped.
    fileHunks: (cid, path, o) =>
      request("GET", `/v1/cards/${id(cid)}/diff/${path.split("/").map(id).join("/")}`, o),
    startCard: (cid, o) => request("POST", `/v1/cards/${id(cid)}/start`, slow(o)),
    sendMessage: (cid, body, o) => command("POST", `/v1/cards/${id(cid)}/messages`, { ...o, body }),
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
    agents: (o) => request("GET", "/v1/agents", o),
    refreshAgents: (o) => request("POST", "/v1/agents/refresh", o),
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
    users: (o) => request("GET", "/v1/users", o),
    progress: (o) => request("GET", "/v1/me/progress", o),
    updateProgress: (body, o) => request("PATCH", "/v1/me/progress", { ...o, body }),
    preferences: (o) => request("GET", "/v1/me/preferences", o),
    updatePreferences: (body, o) => request("PATCH", "/v1/me/preferences", { ...o, body }),
    resetFirstLaunch: (o) => request("POST", "/v1/dev/reset-first-launch", o),
    listSavedViews: (pid, o) => request("GET", `/v1/projects/${id(pid)}/saved-views`, o),
    createSavedView: (pid, body, o) =>
      request("POST", `/v1/projects/${id(pid)}/saved-views`, { ...o, body }),
    updateSavedView: (vid, body, o) =>
      request("PATCH", `/v1/saved-views/${id(vid)}`, { ...o, body }),
    removeSavedView: (vid, o) => command("DELETE", `/v1/saved-views/${id(vid)}`, o),
  };
}

/**
 * The one client for every call to the daemon. It sends the token, times each call out, tells
 * apart the ways a call can fail (`ApiError`), and keeps the daemon's clock. It never logs a
 * request or an answer.
 */
export function createApiClient(options: ApiClientOptions): ApiClient {
  const transport = createTransport(options);
  return { request: transport.request, ...routeMethods(transport) };
}
