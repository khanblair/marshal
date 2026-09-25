import * as approvals from "./actions/approvals";
import * as cardCreate from "./actions/card-create";
import * as cards from "./actions/cards";
import * as chats from "./actions/chats";
import * as checklists from "./actions/checklists";
import * as comments from "./actions/comments";
import * as filters from "./actions/filters";
import * as messages from "./actions/messages";
import * as navigation from "./actions/navigation";
import * as onboarding from "./actions/onboarding";
import { commands } from "./actions/palette";
import * as projects from "./actions/projects";
import * as sessions from "./actions/sessions";
import * as settings from "./actions/settings";
import { bindActions, bindQueries } from "./bind";
import {
  AGENTS,
  CI,
  COLUMNS,
  colOf,
  DAY_MS,
  HOUR_MS,
  isAwake,
  MINUTE_MS,
  NO_THINK,
  PERMS,
  ROLE_NAMES,
  STATUS,
  THINK,
  thinkSupported,
  tone,
  VIEWS,
} from "./constants";
import { type Ctx, createContext, type Env } from "./context";
import { deco } from "./deco";
import { decoMsgs } from "./deco-msgs";
import { dragStart } from "./dom/drag";
import { closeDialog, confirm, emit, set, toast } from "./engine";
import { full, money } from "./format";
import { diffFor } from "./seed/diffs";
import { filesFor } from "./seed/files";
import * as q from "./selectors";
import { startSimulation } from "./sim/start";

/** Keyboard navigation model the board, list, agents, and timeline views write for the shell. */
interface NavModel {
  owner: string;
  grid?: number[][];
  rows?: number[];
}

function queries(ctx: Ctx) {
  return bindQueries(ctx, {
    now: q.now,
    rel: q.rel,
    card: q.card,
    proj: q.proj,
    person: q.person,
    cardsOf: q.cardsOf,
    needs: q.needs,
    awake: q.awake,
    working: q.working,
    costs: q.costs,
    pendingApproval: q.pendingApproval,
    chatsOf: q.chatsOf,
    chatById: q.chatById,
    dupes: q.dupes,
    filtered: filters.filtered,
    deco,
    decoMsgs,
    commands,
  });
}

function appActions(ctx: Ctx) {
  return bindActions(ctx, {
    set,
    toast,
    confirm,
    closeDialog,
    go: navigation.go,
    setView: navigation.setView,
    openCard: navigation.openCard,
    closeCard: navigation.closeCard,
    setTab: navigation.setTab,
    setMode: navigation.setMode,
    setViewport: navigation.setViewport,
    finishOnboarding: onboarding.finishOnboarding,
    resetFirstLaunch: onboarding.resetFirstLaunch,
    startTour: onboarding.startTour,
    setTheme: settings.setTheme,
    runChecks: settings.runChecks,
    simulateCiFailure: settings.simulateCiFailure,
    addFilter: filters.addFilter,
    removeFilter: filters.removeFilter,
    clearFilters: filters.clearFilters,
    applyView: filters.applyView,
    saveView: filters.saveView,
    addProject: projects.addProject,
    renameProject: projects.renameProject,
    removeProject: projects.removeProject,
  });
}

function cardActions(ctx: Ctx) {
  return bindActions(ctx, {
    dragStart,
    moveCard: cards.moveCard,
    fork: cards.fork,
    deleteCard: cards.deleteCard,
    rename: cards.rename,
    setSetting: cards.setSetting,
    requestBypass: cards.requestBypass,
    turnOffBypass: cards.turnOffBypass,
    quickAdd: cardCreate.quickAdd,
    newCard: cardCreate.newCard,
    createCard: cardCreate.createCard,
    start: sessions.start,
    pause: sessions.pause,
    sleep: sessions.sleep,
    wake: sessions.wake,
    pin: sessions.pin,
    keepAwake: sessions.keepAwake,
    sleepAll: sessions.sleepAll,
    keepAllAwake: sessions.keepAllAwake,
    dismissNotice: sessions.dismissNotice,
    toggleItem: checklists.toggleItem,
    addItem: checklists.addItem,
    removeItem: checklists.removeItem,
    addChecklist: checklists.addChecklist,
    deleteChecklist: checklists.deleteChecklist,
    toggleHideDone: checklists.toggleHideDone,
    addComment: comments.addComment,
    deleteComment: comments.deleteComment,
    toggleMember: comments.toggleMember,
  });
}

function chatActions(ctx: Ctx) {
  return bindActions(ctx, {
    approve: approvals.approve,
    deny: approvals.deny,
    approvePlan: approvals.approvePlan,
    rejectPlan: approvals.rejectPlan,
    editPlan: approvals.editPlan,
    savePlan: approvals.savePlan,
    toggleTool: approvals.toggleTool,
    send: messages.send,
    chatSend: chats.chatSend,
    openChat: chats.openChat,
    newChat: chats.newChat,
    renameChat: chats.renameChat,
    archiveChat: chats.archiveChat,
    deleteChat: chats.deleteChat,
  });
}

/**
 * Creates one fake daemon: seeded state, the `M` API the views call, and the
 * simulation (unless the hash turns it off). The browser app uses the single
 * instance from `index.ts`; tests create their own.
 */
export function createMarshal(env: Env) {
  const ctx = createContext(env);
  const M = {
    S: ctx.S,
    STATUS,
    COLUMNS,
    CI,
    AGENTS,
    THINK,
    PERMS,
    ROLE_NAMES,
    VIEWS,
    NO_THINK,
    T0: ctx.today,
    D: DAY_MS,
    H: HOUR_MS,
    MIN: MINUTE_MS,
    colOf,
    tone,
    money,
    full,
    thinkSupported,
    isAwake,
    diffFor,
    filesFor,
    colCards: q.colCards,
    costTone: q.costTone,
    refuse: cards.refuse,
    emit,
    nav: null as NavModel | null,
    /** Set by the shell while it draws the app inside a device frame. */
    _framed: false,
    /** Scale of that device frame, used to convert pointer distances. */
    _scale: 1,
    get mobile(): boolean {
      return q.isMobile(ctx.S);
    },
    ...queries(ctx),
    ...appActions(ctx),
    ...cardActions(ctx),
    ...chatActions(ctx),
  };
  startSimulation(ctx, env.hash);
  return M;
}

export type Marshal = ReturnType<typeof createMarshal>;
