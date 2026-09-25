import type { Seed } from "./seed";
import type { State } from "./state-types";

export interface StateOptions {
  vw: number;
  vh: number;
  onboarded: boolean;
  /** Midnight today, where the calendar opens. */
  today: number;
}

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
    listCols: {
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
    },
    sort: { agents: { k: "state", dir: 1 }, list: { k: "id", dir: -1 } },
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
    route: { page: "home", pid: "api", view: "board" },
    ...projectState(),
    ...uiState(),
    ...seed,
    chatOpen: {},
    chatQuery: {},
    archOpen: {},
    calCursor: opts.today,
    onboarding: !opts.onboarded,
  };
}
