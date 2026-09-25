/* Domain model of the fake daemon. Shapes match design/store.js exactly. */

import type { CardKey } from "./card-key";

export type Status =
  | "backlog"
  | "planning"
  | "working"
  | "needs"
  | "review"
  | "ready"
  | "merging"
  | "done";
export type Column = Exclude<Status, "merging">;
export type CiState = "queued" | "running" | "passed" | "failed" | "cancelled";
export type ToneKind = "solid" | "text" | "subtle";
export type ViewKey = "chat" | "agents" | "board" | "list" | "timeline" | "calendar";
export type Page = "home" | "project" | "settings" | "all";
export type Theme = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";
export type SwimKey = "none" | "role" | "agent" | "package" | "label";
export type FilterKey = "status" | "role" | "agent" | "model" | "label" | "package";
export type CardTab = "chat" | "comments" | "activity" | "diff" | "checks" | "preview" | "notes";
export type Mode = "chat" | "terminal";
export type MobileTab = "home" | "chat" | "board";

export interface Person {
  id: string;
  name: string;
}

interface CiRun {
  wf: string;
  st: CiState;
  ago: number;
  pkg?: string;
}

export interface Project {
  id: string;
  name: string;
  lang: string;
  path: string;
  /*
   * The four fields below come from CI and cost data the daemon does not have until a later phase, so
   * a project from the daemon has none of them. Read them defensively, and show an honest empty state
   * instead of a made-up state (backend-checklist.md 2.3, rule 4).
   */
  ci?: CiState;
  ciAgo?: number;
  monthBase?: number;
  runs?: CiRun[];
  packages?: string[] | undefined;
  branch?: string;
  dev?: string;
  lockBypass?: boolean;
}

export interface ChecklistItem {
  id: string;
  text: string;
  done: boolean;
  by: string | null;
  doneAt?: number;
}

export interface Checklist {
  id: string;
  title: string;
  hideDone: boolean;
  items: ChecklistItem[];
}

export interface Attachment {
  kind: "image" | "file" | "link";
  name: string;
  size?: string;
  url?: string;
  src?: string;
}

export interface Comment {
  id: string;
  author: string;
  text: string;
  ts: number;
  att: Attachment[];
  read: boolean;
}

export interface Card {
  /** The card's key, `<projectId>#<number>`. It is the same in every map, message, and route. */
  id: CardKey;
  /** The number within its project, which is what the label `#41` shows. */
  n: number;
  p: string;
  title: string;
  state: Status;
  role: string;
  agent: string;
  model: string;
  think: string | null;
  perm: string;
  branch: string | null;
  ci: CiState | null;
  cost: number;
  doing: string;
  reason: string;
  asleep: boolean;
  pinned: boolean;
  bypass: boolean;
  paused: boolean;
  pkg: string | null;
  labels: string[];
  deps: CardKey[];
  members: string[];
  checklists: Checklist[];
  comments: Comment[];
  s: number | null;
  e: number | null;
  due: number | null;
  ctx: number;
  upd: number;
  pr: number | null;
  mergePct: number;
  waking: boolean;
}

export type ToolState = "ok" | "running" | "fail";
type PlanState = "waiting" | "approved" | "rejected";
export type ApprovalState = "waiting" | "approved" | "denied";

export interface UserMsg {
  id: string;
  k: "user";
  text: string;
}
export interface AgentMsg {
  id: string;
  k: "agent";
  text: string;
  streaming?: boolean;
}
export interface ToolMsg {
  id: string;
  k: "tool";
  icon: string;
  action: string;
  result: string;
  st: ToolState;
  detail: string;
  open: boolean;
}
export interface SystemMsg {
  id: string;
  k: "system";
  text: string;
}
export interface DiffMsg {
  id: string;
  k: "diff";
  files: number;
  add: number;
  del: number;
}
export interface PlanMsg {
  id: string;
  k: "plan";
  st: PlanState;
  editing: boolean;
  edited?: boolean;
  steps: string[];
  files: string[];
  risks: string[];
  checks: string[];
}
export interface ApprovalMsg {
  id: string;
  k: "approval";
  st: ApprovalState;
  cmd: string;
  why: string;
  cardId?: CardKey;
}
export interface CardRefMsg {
  id: string;
  k: "card";
  cardId: CardKey;
}
export interface LinksMsg {
  id: string;
  k: "links";
  text: string;
  cards: CardKey[];
}
export type Msg =
  | UserMsg
  | AgentMsg
  | ToolMsg
  | SystemMsg
  | DiffMsg
  | PlanMsg
  | ApprovalMsg
  | CardRefMsg
  | LinksMsg;

export type ActivityKind = "file" | "command" | "test" | "tool" | "approval";
export type ActivityState = "ok" | "running" | "fail" | "waiting";
export interface Activity {
  id: string;
  kind: ActivityKind;
  text: string;
  result: string;
  st: ActivityState;
  ts: number;
  fresh?: boolean;
}

export type CheckState = "pending" | "running" | "passed" | "failed";
export interface Check {
  id: string;
  name: string;
  cmd: string;
  st: CheckState;
}

type DiffLine = [sign: " " | "+" | "-", line: number, text: string];
interface DiffHunk {
  h: string;
  lines: DiffLine[];
}
export interface DiffFile {
  path: string;
  add: number;
  del: number;
  large?: boolean;
  hunks: DiffHunk[];
}

export interface Chat {
  id: string;
  pid: string;
  title: string;
  target: string;
  msgs: Msg[];
  last: number;
  archived: boolean;
  fresh?: boolean;
}

type FeedKind = "brief" | "merge" | "schedule" | "approval" | "plan" | "ci" | "tool";
export interface FeedItem {
  id: string;
  kind: FeedKind;
  text: string;
  pid: string | null;
  ts: number;
  cardId?: CardKey;
  job?: string;
}

export interface SleepNotice {
  id: string;
  kind: "sleep";
  cards: CardKey[];
  deadline: number;
  ts: number;
}
export interface InfoNotice {
  id: string;
  kind: "ci-main" | "cost" | "plan" | "ci";
  pid?: string;
  cardId?: CardKey;
  text: string;
  sub: string;
  ts: number;
}
export type Notice = SleepNotice | InfoNotice;

export interface ToastAction {
  label: string;
  run: () => void;
}
export interface Toast {
  id: string;
  msg: string;
  action: ToastAction | undefined;
  dismiss: () => void;
}

export interface DialogSpec {
  title: string;
  message: string;
  action: string;
  destructive?: boolean;
  ack?: string;
  run: () => void;
}
export interface Dialog extends DialogSpec {
  acked: boolean;
}

export interface NewCardDraft {
  title: string;
  body: string;
  template: string;
  role: string;
  agent: string;
  start: boolean;
}
export interface NewProjectDraft {
  source: "folder" | "github";
  path: string;
  url: string;
  name: string;
  /** The branch to check out when cloning. Empty means the repository's own default. */
  branch: string;
  nameTouched?: boolean;
  /** True while the daemon is adding the project, so a second press is refused. */
  busy?: boolean;
  /** The daemon's plain sentence when it refused, shown under the fields. */
  error?: string;
}
export interface RemoveProjectDraft {
  id: string;
  keepBranches: boolean;
  keepMemory: boolean;
}

export interface Limits {
  day: number;
  month: number;
  awake: number;
}
export interface Filter {
  k: FilterKey;
  v: string;
}
export interface SavedView {
  name: string;
  f: Filter[];
  swim: SwimKey;
}
export interface Route {
  page: Page;
  pid: string | null;
  view: ViewKey;
}
export interface SortSpec {
  k: string;
  dir: number;
}

/* One agent in the shape the prototype's `AGENTS` table had: the model ids, an icon, and a
   version. The shape lives in the data layer, because the mapper that fills it must not depend on
   the mock; it is re-exported here for the store's own modules. */
export type { AgentInfo } from "~/data/mappers/agents";
