import type { Agent, Preview } from "@marshal/protocol";
import type { ConnectionState } from "~/data/connection-machine";
import type { CheckpointRow } from "~/data/mappers/checkpoints";
import type { CardKey } from "./card-key";
import type { CalEvent, Integration, Profile, Provider, Role, Schedule } from "./settings-types";
import type {
  Activity,
  Card,
  CardTab,
  Chat,
  Check,
  Column,
  DailyStats,
  Dialog,
  FeedItem,
  Filter,
  LimitsByScope,
  MobileTab,
  Mode,
  Msg,
  NewCardDraft,
  NewProjectDraft,
  Notice,
  Person,
  Project,
  RemoveProjectDraft,
  ResolvedTheme,
  Route,
  SavedView,
  SleepChoice,
  SortSpec,
  SwimKey,
  Theme,
  Toast,
  ViewKey,
} from "./types";

/** The link with the daemon as the screens see it. The store follows `data.connection` into this. */
interface ConnectionView {
  state: ConnectionState;
  /** When the next check happens, in ms on this device's clock, or null. */
  retryAt: number | null;
  /** The technical reason for the last failure, for the collapsed "Details". Never a token. */
  detail: string;
  /** The daemon's plain sentence when it refused the token that is stored, else empty. */
  rejection: string;
  /** True from the moment a token is submitted until the daemon has answered. */
  busy: boolean;
}

/** The single mutable app state (`M.S`). Field names and shapes match the prototype. */
export interface State {
  ready: boolean;
  vw: number;
  theme: Theme;
  reduced: boolean;
  route: Route;
  lastView: Record<string, ViewKey>;
  people: Person[];
  projects: Project[];
  /** The daemon's agent catalog, as mirrored by `sync/agents.ts`. The built-in agent is added by `mock/agents.ts`. */
  agents: Agent[];
  cards: Card[];
  chat: Record<CardKey, Msg[]>;
  act: Record<CardKey, Activity[]>;
  /** A card's restore points, newest first (section S10, B5.3), read from the daemon when it opens. */
  checkpoints: Record<CardKey, CheckpointRow[]>;
  chats: Record<string, Chat[]>;
  checks: Record<CardKey, Check[]>;
  chatOpen: Record<string, string | null>;
  chatQuery: Record<string, string>;
  archOpen: Record<string, boolean>;
  openId: CardKey | null;
  focusId: CardKey | null;
  tab: CardTab;
  mode: Mode;
  switching: false | Mode;
  detailW: number;
  detailExpanded: boolean;
  filters: Record<string, Filter[]>;
  query: Record<string, string>;
  swim: Record<string, SwimKey>;
  laneCollapsed: Record<string, boolean>;
  showAllDone: Record<string, boolean>;
  savedViews: Record<string, SavedView[]>;
  savedView: Record<string, string | null>;
  notices: Notice[];
  noticesOpen: boolean;
  noticesSeen: number;
  toasts: Toast[];
  dialog: Dialog | null;
  palette: boolean;
  newCard: NewCardDraft | null;
  sidebarCollapsed: boolean;
  mobileTab: MobileTab;
  menu: string | null;
  limits: LimitsByScope;
  sleep: SleepChoice;
  roles: Role[];
  providers: Provider[];
  integrations: Integration[];
  schedules: Schedule[];
  calEvents: CalEvent[];
  settingsSection: string;
  roleSel: string;
  listCols: Record<string, boolean>;
  sort: { agents: SortSpec; list: SortSpec };
  calMode: "month" | "week";
  calCursor: number;
  announce: string;
  profile: Profile;
  onboarding: boolean;
  obStep: number;
  tour: { step: number } | null;
  feed: FeedItem[];
  /** The stored numbers the Home charts draw. Empty until `sync/home-stats.ts` answers. */
  stats: DailyStats;
  split: ViewKey[];
  dashRange: number;
  vh: number;
  /* The fields below are written later by the shell, the views, or drag and drop.
     They stay absent until first written, exactly like the prototype, because some
     views tell `undefined` apart from `null`. */
  resolvedTheme?: ResolvedTheme;
  dragId?: CardKey | null;
  dropCol?: string | null;
  newProject?: NewProjectDraft | null;
  removeProject?: RemoveProjectDraft | null;
  newChatOpen?: boolean;
  renaming?: string | null;
  sideOpen?: boolean;
  settingsPid?: string;
  schedEdit?: string | null;
  /**
   * A card's live preview (section S13): the daemon's own value, kept whole so a snapshot and a
   * `preview.state_changed` event are applied the same way and can never disagree. A card with no
   * entry has no preview read for it yet, which the tab draws as stopped.
   */
  preview?: Record<CardKey, Preview>;
  notes?: Record<CardKey, string>;
  allKind?: "activity" | "ci";
  quickAddAt?: string | null;
  mobileCol?: Column;
  calExpand?: string | null;
  /** Absent when no daemon is connected (a test). It is set as soon as the store starts following the daemon. */
  connection?: ConnectionView;
  /** A plain sentence when the first snapshots could not be loaded, which the app shows with a Try again. */
  loadError?: string;
}
