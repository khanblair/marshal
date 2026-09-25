import type { CalEvent, Integration, Profile, Provider, Role, Schedule } from "./settings-types";
import type {
  Activity,
  Card,
  CardTab,
  Chat,
  Check,
  Column,
  Dialog,
  FeedItem,
  Filter,
  Limits,
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
  SortSpec,
  SwimKey,
  Theme,
  Toast,
  ViewKey,
} from "./types";

type PreviewState = "stopped" | "starting" | "running";

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
  cards: Card[];
  chat: Record<number, Msg[]>;
  act: Record<number, Activity[]>;
  chats: Record<string, Chat[]>;
  checks: Record<number, Check[]>;
  chatOpen: Record<string, string | null>;
  chatQuery: Record<string, string>;
  archOpen: Record<string, boolean>;
  openId: number | null;
  focusId: number | null;
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
  limits: { global: Limits; [pid: string]: Limits };
  sleep: { idle: number; warn: number; channel: string; restore: string };
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
  split: ViewKey[];
  dashRange: number;
  vh: number;
  /* The fields below are written later by the shell, the views, or drag and drop.
     They stay absent until first written, exactly like the prototype, because some
     views tell `undefined` apart from `null`. */
  resolvedTheme?: ResolvedTheme;
  dragId?: number | null;
  dropCol?: string | null;
  newProject?: NewProjectDraft | null;
  removeProject?: RemoveProjectDraft | null;
  newChatOpen?: boolean;
  renaming?: string | null;
  sideOpen?: boolean;
  settingsPid?: string;
  schedEdit?: string | null;
  preview?: Record<number, PreviewState>;
  notes?: Record<number, string>;
  allKind?: "activity" | "ci";
  quickAddAt?: string | null;
  mobileCol?: Column;
  calExpand?: string | null;
}
