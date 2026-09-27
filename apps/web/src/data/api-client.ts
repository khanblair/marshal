import type {
  ActivityItem,
  AgentCatalog,
  BoardSnapshot,
  BypassRequest,
  Card,
  CardDiff,
  CardView,
  Chat,
  ChatListSnapshot,
  ChatMessage,
  ChatMessageDetail,
  CheckpointList,
  CISnapshot,
  CreateCardRequest,
  CreateChatRequest,
  CreateLabelRequest,
  CreateProjectRequest,
  CreateRoleRequest,
  CreateSavedViewRequest,
  EditPlanRequest,
  FeedEntry,
  FileHunks,
  Health,
  HomeSnapshot,
  IntegrationList,
  Label,
  LabelSnapshot,
  LimitList,
  MoveCardRequest,
  NoticeActionRequest,
  NoticeActionResult,
  NoticeList,
  Page,
  Preferences,
  PreviewShotRequest,
  PreviewShotResult,
  PreviewSnapshot,
  Profile,
  Progress,
  Project,
  ProjectListSnapshot,
  ProviderList,
  RemoveProjectRequest,
  RestoreCheckpointRequest,
  Role,
  RoleList,
  RoleSpec,
  SavedView,
  SavedViewListSnapshot,
  SaveGitHubRequest,
  SaveProviderRequest,
  SearchSnapshot,
  SendMessageRequest,
  SetLimitRequest,
  SetViewRequest,
  SimulateCIFailureRequest,
  SimulateCIFailureResult,
  SleepSettings,
  TestResult,
  UpdateCardRequest,
  UpdateChatRequest,
  UpdateLabelRequest,
  UpdatePreferencesRequest,
  UpdateProfileRequest,
  UpdateProgressRequest,
  UpdateProjectRequest,
  UpdateRoleRequest,
  UpdateSavedViewRequest,
  UserListSnapshot,
  WhoAmI,
} from "@marshal/protocol";
import { type ApiError, clientError, readWireError, wireError } from "./api-error";
import type { DaemonClock } from "./daemon-clock";
import { isRecord } from "./guards";
import { guardRequest } from "./request-guard";

type Method = "GET" | "POST" | "PATCH" | "PUT" | "DELETE";

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
  /**
   * Simulates a CI failure on the card (N28, B6.4). `synthetic` injects a failed run through the CI
   * monitor's own path - the same rerun, the same trimmed log, the same loop limits - and never
   * touches GitHub; `real` pushes a deliberately failing change to the card's own branch, which runs
   * for real and uses Actions minutes. Both answer the run they made. A daemon that is not in dev
   * mode does not have this route at all, so the answer is `not_found`.
   */
  simulateCIFailure(
    id: string,
    body: SimulateCIFailureRequest,
    options?: CallOptions,
  ): Promise<SimulateCIFailureResult>;
  /**
   * Turns bypass permissions on for a card (B3.2). The body carries the acknowledgement the person
   * gave: a request without it is refused with the reason `unacknowledged`, and a card of a project
   * that locks bypass is refused with `locked`.
   */
  setCardBypass(id: string, body: BypassRequest, options?: CallOptions): Promise<Card>;
  /** Turns bypass permissions off, leaving the card in full auto. Always allowed. */
  clearCardBypass(id: string, options?: CallOptions): Promise<Card>;
  /**
   * Answers the plan a card is waiting on (section S8c, B5.2): the plan is stored as approved, the
   * card starts working on it, and it leaves plan-only mode. A card with no plan is `not_found`, and
   * one whose plan has already been answered is `conflict`.
   */
  approvePlan(id: string, options?: CallOptions): Promise<Card>;
  /** Sends the plan back: it is stored as rejected and the card returns to planning. Refused the same way. */
  rejectPlan(id: string, options?: CallOptions): Promise<Card>;
  /**
   * Replaces the steps of the plan a card is waiting on. It stays waiting, so the card is not
   * moved, and an empty list of steps is refused.
   */
  editPlan(id: string, body: EditPlanRequest, options?: CallOptions): Promise<Card>;
  listLabels(projectId: string, options?: CallOptions): Promise<LabelSnapshot>;
  createLabel(projectId: string, body: CreateLabelRequest, options?: CallOptions): Promise<Label>;
  updateLabel(id: string, body: UpdateLabelRequest, options?: CallOptions): Promise<Label>;
  removeLabel(id: string, options?: CallOptions): Promise<void>;
  /** The numbers Home draws, and the cards behind them. `range` picks 7, 30, or 90 days. */
  home(options?: HomeOptions): Promise<HomeSnapshot>;
  /** The Home activity stream, newest first, one page at a time, filtered by kind and project. */
  homeActivity(page?: HomeActivityOptions, options?: CallOptions): Promise<Page<FeedEntry>>;
  /**
   * Every project's CI health, and the moment the daemon answered, so ages are counted from the
   * daemon's own clock (section S21, docs/backend-checklist.md B6.2 to B6.4). A project Marshal has
   * no run for is left out on purpose, which is what Home draws as "GitHub is not connected".
   */
  ciSnapshot(options?: CallOptions): Promise<CISnapshot>;
  /** A card's chat, newest first, one page at a time. */
  messages(cardId: string, page?: PageOptions, options?: CallOptions): Promise<Page<ChatMessage>>;
  /** A card's activity, newest first, one page at a time, optionally one kind of it. */
  activity(
    cardId: string,
    page?: ActivityOptions,
    options?: CallOptions,
  ): Promise<Page<ActivityItem>>;
  /**
   * A card's restore points, newest first (section S10, docs/backend-checklist B5.3): the commits
   * Marshal made before its turns, which its worktree can be put back to. A card that never started
   * has none.
   */
  checkpoints(cardId: string, options?: CallOptions): Promise<CheckpointList>;
  /**
   * Puts a card's worktree and branch back to one of its restore points, and answers the card as it
   * now is. Refused while the card's agent is running a turn, with the daemon's own sentence.
   */
  restoreCheckpoint(
    cardId: string,
    checkpointId: string,
    body?: RestoreCheckpointRequest,
    options?: CallOptions,
  ): Promise<Card>;
  /**
   * A card's live preview (section S13, docs/backend-checklist.md B6.6): its state, the address it
   * answers on, and the before and after screenshots taken of it. Reading it starts nothing, so
   * looking at the tab never starts a dev server on the person's machine.
   */
  preview(cardId: string, options?: CallOptions): Promise<PreviewSnapshot>;
  /**
   * Runs the project's dev command for the card in the card's own worktree, on a port picked for
   * it, and answers the preview as it is now. Starting a dev server may take a while, so it takes
   * the slow limit. A card with no worktree, or a project with no dev command, is refused with the
   * daemon's own sentence rather than shown a spinner that never ends.
   */
  startPreview(cardId: string, options?: CallOptions): Promise<PreviewSnapshot>;
  /** Stops the card's dev server. Stopping a preview that is not running is not an error. */
  stopPreview(cardId: string, options?: CallOptions): Promise<PreviewSnapshot>;
  /**
   * Takes one half of a running preview's before and after screenshot pair. A browser Marshal
   * cannot find is not an error: the answer says the check was skipped and why.
   */
  takePreviewShot(
    cardId: string,
    body: PreviewShotRequest,
    options?: CallOptions,
  ): Promise<PreviewShotResult>;
  /**
   * The bytes of a screenshot, from the address a shot carries. Like an avatar it needs the token,
   * so a bare `img` tag cannot fetch it; the client fetches it and the screen shows the bytes.
   */
  previewShotImage(shotUrl: string, options?: CallOptions): Promise<Blob>;
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
  /** Every model provider Marshal knows, in the order the screen shows them (sections S28, S4). */
  listProviders(options?: CallOptions): Promise<ProviderList>;
  /**
   * Stores a provider's secret: an API key, or the server URL of a local provider. The key is never
   * sent back, only the masked form. The daemon tests the key as part of this call, so the answer
   * already says whether it works.
   */
  saveProvider(id: string, body: SaveProviderRequest, options?: CallOptions): Promise<ProviderList>;
  /** Forgets a stored key. Removing one that was never stored is not an error. */
  removeProvider(id: string, options?: CallOptions): Promise<ProviderList>;
  /**
   * Runs one connection test now and answers what it found. A test that ran and found a bad key is
   * a success with a failed check; only a test that could not run, or one asked for inside the
   * cooldown, is an error.
   */
  testProvider(id: string, options?: CallOptions): Promise<TestResult>;
  /**
   * Every connection Marshal can be set up with (section S29a): GitHub, and the connections later
   * phases build, each read as "not connected" until it is. The answer is the whole list, whether or
   * not a connection is set up, and never carries a secret.
   */
  listIntegrations(options?: CallOptions): Promise<IntegrationList>;
  /**
   * Stores a connection's settings. Today that is the GitHub App's whole setup - the App's id, its
   * installation id, its private key, and its webhook secret - which arrives together because no
   * part of it is any use alone. The key and the secret go to the OS keychain and never come back.
   * The daemon tests the connection as part of this call, so the answered row already carries the
   * last test's result.
   */
  saveIntegration(
    id: string,
    body: SaveGitHubRequest,
    options?: CallOptions,
  ): Promise<IntegrationList>;
  /** Forgets a connection's settings and its secret. Removing one that was never set up is not an error. */
  removeIntegration(id: string, options?: CallOptions): Promise<IntegrationList>;
  /**
   * Runs one connection's test now and answers what it found. A test that ran and found something
   * wrong is a success with a failed check; only a test that could not run, or one asked for inside
   * the daemon's cooldown, is an error.
   */
  testIntegration(id: string, options?: CallOptions): Promise<TestResult>;
  /** Every cost and awake ceiling that is set, global ones first (sections S26b, S19b). */
  listLimits(options?: CallOptions): Promise<LimitList>;
  /** Sets one ceiling. The scope and kind come from the path; the body carries only the number. */
  setLimit(
    scope: string,
    kind: string,
    body: SetLimitRequest,
    options?: CallOptions,
  ): Promise<LimitList>;
  /** Removes one ceiling. A scope that had none of that kind is not an error. */
  deleteLimit(scope: string, kind: string, options?: CallOptions): Promise<LimitList>;
  /**
   * Every role Marshal knows (section S27): Marshal's starter roles first, then the roles a person
   * made. The project is optional and changes only whether a role reads as overridden, because the
   * roles themselves are global.
   */
  listRoles(project?: string, options?: CallOptions): Promise<RoleList>;
  /** Adds a role, or imports one. The body is the export document; a name another role has is a conflict. */
  createRole(body: CreateRoleRequest, project?: string, options?: CallOptions): Promise<RoleList>;
  /** One role by its name. */
  getRole(name: string, project?: string, options?: CallOptions): Promise<Role>;
  /** Renames a role, replaces its body, or both. A field left out is not touched. */
  updateRole(
    name: string,
    body: UpdateRoleRequest,
    project?: string,
    options?: CallOptions,
  ): Promise<RoleList>;
  /** Removes a role a person made. One of Marshal's own is refused; reset it instead. */
  deleteRole(name: string, project?: string, options?: CallOptions): Promise<RoleList>;
  /** Clears one project's own version of a role, leaving the role itself exactly as it is. */
  resetRole(name: string, project: string, options?: CallOptions): Promise<RoleList>;
  /** Gives one project its own version of a role. The body is the spec alone, not an export document. */
  setRoleOverride(
    name: string,
    project: string,
    spec: RoleSpec,
    options?: CallOptions,
  ): Promise<RoleList>;
  /**
   * Every notice that is standing (section S23), the idle-card sleep groups today. It is never null,
   * so an empty answer is an empty array and a client never handles both.
   */
  listNotices(options?: CallOptions): Promise<NoticeList>;
  /**
   * One of the four calls a person makes on a notice (inventory N5): keep one card awake, sleep one
   * card now, keep every card the notice names awake, or sleep them all now. The notice id names the
   * group and the two per-card calls carry the card. The answer is how many cards it changed, so a
   * toast can say a number.
   */
  noticeAction(
    notice: string,
    body: NoticeActionRequest,
    options?: CallOptions,
  ): Promise<NoticeActionResult>;
  /** Takes one notice off the panel. A notice that is already gone is not an error. */
  dismissNotice(notice: string, options?: CallOptions): Promise<void>;
  /** The numbers and choices behind automatic sleep (section S26a). */
  sleepSettings(options?: CallOptions): Promise<SleepSettings>;
  /** Saves them. A value the screen would not offer is refused with the sentence the form shows. */
  saveSleepSettings(body: SleepSettings, options?: CallOptions): Promise<SleepSettings>;
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

/**
 * The project a roles request is being made about, as a query string. It is left out when there is
 * none, which is what a caller that is not looking at a project asks for: the routes then report
 * every role as not overridden, because no project is being looked at.
 */
function roleQuery(project: string | undefined): string {
  return project ? `?${new URLSearchParams({ project })}` : "";
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
    // Everything a person does while looking at one card, restore points included: `cardMethods`.
    ...cardMethods({ request, command }),
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
    // A screenshot's address is the one the shot carries, with the daemon's own version in it.
    previewShotImage: (shotUrl, o) => bytes("GET", shotUrl, o),
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
  | "setCardView";

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
