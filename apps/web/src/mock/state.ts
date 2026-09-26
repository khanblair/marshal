import type { Seed } from "./seed";
import type { State } from "./state-types";
import type { SortSpec } from "./types";

export interface StateOptions {
  vw: number;
  vh: number;
  onboarded: boolean;
  /** Midnight today, where the calendar opens. */
  today: number;
  /** True when the saved views are the daemon's (S6a): the prototype's seeded ones are then not the store's. */
  savedViewsOnDaemon?: boolean;
}

/** The List's columns before anyone chose. The daemon keeps only the columns a person changed. */
export const DEFAULT_LIST_COLS: Readonly<Record<string, boolean>> = {
  id: true,
  title: true,
  state: true,
  role: true,
  agent: true,
  model: true,
  branch: true,
  ci: true,
  cost: true,
  upd: true,
  pkg: false,
  think: false,
};

/** How the Agents table and the List are sorted before anyone chose. */
export const DEFAULT_SORT: Readonly<{ agents: SortSpec; list: SortSpec }> = {
  agents: { k: "state", dir: 1 },
  list: { k: "id", dir: -1 },
};

/** Per-project defaults for the three seeded projects. */
const projectState = () =>
  ({
    lastView: { api: "board", web: "board", mobile: "board" },
    filters: { api: [], web: [], mobile: [] },
    query: { api: "", web: "", mobile: "" },
    swim: { api: "none", web: "none", mobile: "package" },
    savedViews: {
      api: [
        { name: "All cards", f: [], swim: "none" },
        { name: "Needs me", f: [{ k: "status", v: "needs" }], swim: "none" },
        { name: "Claude Code by role", f: [{ k: "agent", v: "Claude Code" }], swim: "role" },
      ],
      web: [
        { name: "All cards", f: [], swim: "none" },
        { name: "UI work", f: [{ k: "label", v: "ui" }], swim: "none" },
        { name: "By agent", f: [], swim: "agent" },
      ],
      mobile: [
        { name: "By package", f: [], swim: "package" },
        { name: "All cards", f: [], swim: "none" },
        {
          name: "api-client only",
          f: [{ k: "package", v: "packages/api-client" }],
          swim: "none",
        },
      ],
    },
    savedView: { api: "All cards", web: "All cards", mobile: "By package" },
    limits: {
      global: { day: 25, month: 400, awake: 18 },
      api: { day: 10, month: 150, awake: 8 },
      web: { day: 8, month: 120, awake: 6 },
      mobile: { day: 8, month: 150, awake: 8 },
    },
  }) satisfies Partial<State>;

const uiState = () =>
  ({
    openId: null,
    focusId: null,
    tab: "chat",
    mode: "chat",
    switching: false,
    detailW: 600,
    detailExpanded: false,
    laneCollapsed: {},
    showAllDone: {},
    noticesOpen: false,
    noticesSeen: 0,
    toasts: [],
    dialog: null,
    palette: false,
    newCard: null,
    sidebarCollapsed: false,
    mobileTab: "home",
    menu: null,
    sleep: { idle: 15, warn: 2, channel: "In app only", restore: "Auto-restore on startup" },
    settingsSection: "general",
    roleSel: "Worker",
    listCols: { ...DEFAULT_LIST_COLS },
    sort: { agents: { ...DEFAULT_SORT.agents }, list: { ...DEFAULT_SORT.list } },
    calMode: "month",
    announce: "",
    obStep: 0,
    tour: null,
    split: [],
    dashRange: 7,
  }) satisfies Partial<State>;

/** The prototype's initial `S`, as a plain object ready to become a store. */
export function initialState(seed: Seed, opts: StateOptions): State {
  return {
    ready: false,
    vw: opts.vw,
    vh: opts.vh,
    theme: "system",
    reduced: false,
    route: { page: "home", pid: null, view: "board" },
    // Projects come from the daemon (`sync/projects.ts`), so there are none until it answers.
    projects: [],
    // Agents come from the daemon too (`sync/agents.ts`).
    agents: [],
    ...projectState(),
    // The prototype's saved views are its own: on the daemon they are the daemon's, and the client's
    // own "All cards" (`ensureProjectState`) is all a project has until they are loaded.
    ...(opts.savedViewsOnDaemon ? { savedViews: {}, savedView: {} } : {}),
    ...uiState(),
    ...seed,
    // The Home numbers come from the daemon (`sync/home-stats.ts`); there are none until it answers,
    // so a chart draws the range the screen asked for with every day at zero.
    stats: { range: 7, days: [], projects: [] },
    chatOpen: {},
    chatQuery: {},
    archOpen: {},
    calCursor: opts.today,
    onboarding: !opts.onboarded,
  };
}
