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
  DecideApprovalRequest,
  EditPlanRequest,
  FeedEntry,
  FileHunks,
  Health,
  HomeSnapshot,
  IntegrationList,
  Label,
  LabelSnapshot,
  Lesson,
  LimitList,
  MoveCardRequest,
  Note,
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
  SaveLessonRequest,
  SaveNoteRequest,
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
import type { ApiError } from "./api-error";
import type { DaemonClock } from "./daemon-clock";

export type Method = "GET" | "POST" | "PATCH" | "PUT" | "DELETE";

/** Options of one call. */
export interface CallOptions {
  /** Cancels the request. The call then fails with the code `aborted`. */
  signal?: AbortSignal | undefined;
  /** Replaces the time limit of the client for this call. */
  timeoutMs?: number | undefined;
}

/** The paging a list route takes: a size, and the cursor of the last page read. */
export interface PageOptions {
  limit?: number;
  cursor?: string;
}

/** The activity list, which can be narrowed to one kind of entry. */
export interface ActivityOptions extends PageOptions {
  kind?: string;
}

/** The Home dashboard, whose `range` says how many days the charts cover. */
interface HomeOptions extends CallOptions {
  /** 7, 30, or 90 days. Left out, the daemon answers with seven. */
  range?: number;
}

/** The Home activity list, which can be narrowed to one kind and one project. */
export interface HomeActivityOptions extends PageOptions {
  kind?: string;
  project?: string;
}

export interface RequestOptions extends CallOptions {
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
  /**
   * A card's note, whole (section S14, B7.4, N11, task 7.12). A card nothing has been saved for
   * answers the note the daemon would start one from, with no save time; reading never writes one.
   */
  note(cardId: string, options?: CallOptions): Promise<Note>;
  /** Replaces a card's note with the body, and answers it as a read would. */
  saveNote(cardId: string, body: SaveNoteRequest, options?: CallOptions): Promise<Note>;
  /**
   * A project's lessons, newest first (section 7.13, B7.6): what an agent has learned worth
   * remembering next time, for the lessons screen.
   */
  listLessons(projectId: string, options?: CallOptions): Promise<Lesson[]>;
  /** One project's lesson by its slug. */
  lesson(projectId: string, slug: string, options?: CallOptions): Promise<Lesson>;
  /**
   * Writes a lesson, making it the first time when slug is omitted. The slug that is actually saved
   * to is made fresh from the title in the body, so retitling one moves it to a new slug; read the
   * new one back from the answer.
   */
  saveLesson(
    projectId: string,
    body: SaveLessonRequest,
    slug?: string,
    options?: CallOptions,
  ): Promise<Lesson>;
  /** Removes a lesson outright, file and row. */
  removeLesson(projectId: string, slug: string, options?: CallOptions): Promise<void>;
  /**
   * Answers one permission request an agent is blocked on (section S8b, B3.4, N7). The id is the
   * approval's own, not the card's - it is what the chat's approval block and Home's needs-you card
   * both carry (`ChatApproval.id`, `Card.needsReason.approvalId`). A request already answered is
   * `conflict`, and one that does not exist is `not_found`.
   */
  decideApproval(id: string, body: DecideApprovalRequest, options?: CallOptions): Promise<void>;
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
