import { isDaemon } from "~/data/sections";
import { startSync } from "~/sync";
import * as cardWrites from "~/sync/card-actions";
import * as cardHold from "~/sync/card-hold";
import { sendToCard } from "~/sync/card-session";
import * as cardView from "~/sync/card-view";
import * as chatWrites from "~/sync/chat-actions";
import { retryChat, sendToChat } from "~/sync/chat-session";
import * as connection from "~/sync/connection-actions";
import { loadCardDiff, loadFileHunks } from "~/sync/diff";
import { homeActivityPage } from "~/sync/home-feed";
import * as onboardingWrites from "~/sync/onboarding-actions";
import * as profileWrites from "~/sync/profile-actions";
import * as projects from "~/sync/project-actions";
import * as savedViews from "~/sync/saved-views";
import { createPaletteSearch } from "~/sync/search";
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
import * as sessions from "./actions/sessions";
import * as settings from "./actions/settings";
import * as agents from "./agents";
import { bindActions, bindQueries } from "./bind";
import type { CardKey } from "./card-key";
import {
  CI,
  COLUMNS,
  colOf,
  DAY_MS,
  HOUR_MS,
  isAwake,
  MINUTE_MS,
  PERMS,
  ROLE_NAMES,
  STATUS,
  THINK,
  tone,
  VIEWS,
} from "./constants";
import { type Ctx, createContext, type Env, sectionsOf } from "./context";
import { deco } from "./deco";
import { decoMsgs } from "./deco-msgs";
import { dragStart } from "./dom/drag";
import { closeDialog, confirm, emit, set, toast } from "./engine";
import { full, money } from "./format";
import { diffFor } from "./seed/diffs";
import { filesFor } from "./seed/files";
import * as q from "./selectors";
import { startSimulation } from "./sim/start";
import type { AgentInfo, Mode } from "./types";

/** Keyboard navigation model the board, list, agents, and timeline views write for the shell. */
interface NavModel {
  owner: string;
  grid?: CardKey[][];
  rows?: CardKey[];
}

function queries(ctx: Ctx) {
  return bindQueries(ctx, {
    now: q.now,
    rel: q.rel,
    card: q.card,
    cardLabelOf: q.cardLabelOf,
    proj: q.proj,
    agentOptions: agents.agentOptions,
    thinkSupported: agents.thinkSupported,
    person: q.person,
    meId: q.meId,
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
    startSearch: createPaletteSearch,
    loadActivityPage: homeActivityPage,
    loadCardDiff,
    loadFileHunks,
    // The terminal view's decoded, escape-stripped text (section S9): empty for a mock-only card,
    // or a real one nothing has ever applied to.
    terminalText: cardView.terminalTextOf,
  });
}

function appActions(ctx: Ctx) {
  // The profile (S2a) and the saved views (S6a) are the daemon's once their sections are switched,
  // and each is switched on its own, so a store can have one on the daemon and the other on the mock.
  const table = sectionsOf(ctx.env);
  const profileOnDaemon = isDaemon("S2a", table);
  const viewsOnDaemon = isDaemon("S6a", table);
  // The first-launch screens and the tour save their progress on the daemon once S31a is switched.
  const onboardingOnDaemon = isDaemon("S31a", table);
  // The terminal view's own switch, section S9: a card is the daemon's only once its own `daemonId`
  // is set (S5a), so `setMode` still falls back to the mock's timeout-based switch for a card the
  // mock made, or when nothing is open at all, exactly the pattern every other split action here
  // uses, just decided per call instead of once per store (the mock's own switch has no card of its
  // own to check against `viewOnDaemon`, S.mode being global to whichever card is open).
  const viewOnDaemon = isDaemon("S9", table);
  const setMode = (c: Ctx, mode: Mode): void => {
    const card = c.S.openId ? q.card(c, c.S.openId) : undefined;
    if (viewOnDaemon && card?.daemonId) {
      void cardView.switchView(c, card.id, mode);
      return;
    }
    navigation.setMode(c, mode);
  };
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
    setMode,
    setViewport: navigation.setViewport,
    terminalSend: cardView.sendTerminalText,
    terminalKey: cardView.sendTerminalKey,
    finishOnboarding: onboardingOnDaemon
      ? onboardingWrites.finishOnboarding
      : onboarding.finishOnboarding,
    resetFirstLaunch: onboardingOnDaemon
      ? onboardingWrites.resetFirstLaunch
      : onboarding.resetFirstLaunch,
    startTour: onboardingOnDaemon ? onboardingWrites.startTour : onboarding.startTour,
    setOnboardingStep: onboardingOnDaemon
      ? onboardingWrites.setOnboardingStep
      : onboarding.setOnboardingStep,
    endTour: onboardingOnDaemon ? onboardingWrites.endTour : onboarding.endTour,
    setTheme: settings.setTheme,
    runChecks: settings.runChecks,
    simulateCiFailure: settings.simulateCiFailure,
    addFilter: filters.addFilter,
    removeFilter: filters.removeFilter,
    clearFilters: filters.clearFilters,
    applyView: filters.applyView,
    saveView: viewsOnDaemon ? savedViews.saveView : filters.saveView,
    saveProfile: profileOnDaemon ? profileWrites.saveProfile : settings.saveProfile,
    chooseAvatar: profileOnDaemon ? profileWrites.chooseAvatar : settings.chooseAvatar,
    addProject: projects.addProject,
    renameProject: projects.renameProject,
    saveProject: projects.saveProject,
    removeProject: projects.removeProject,
    signIn: connection.signIn,
    reconnect: connection.reconnect,
  });
}

function cardActions(ctx: Ctx) {
  // The cards are the daemon's once section S5a is switched, so these writes go to it; while the
  // section is still on the mock they are the mock's own. The phase that deletes the mock removes
  // the other branch. `newCard` (the New card dialog's draft) stays on the mock for good: it is a
  // draft before a card exists at all.
  const onDaemon = isDaemon("S5a", sectionsOf(ctx.env));
  // The session hold (pause, sleep, wake, pin) is its own section, S7c, switched on its own
  // schedule: a card can be the daemon's (S5a) before its hold controls are.
  const holdOnDaemon = isDaemon("S7c", sectionsOf(ctx.env));
  return bindActions(ctx, {
    dragStart,
    moveCard: onDaemon ? cardWrites.moveCard : cards.moveCard,
    fork: onDaemon ? cardWrites.fork : cards.fork,
    deleteCard: onDaemon ? cardWrites.deleteCard : cards.deleteCard,
    rename: onDaemon ? cardWrites.rename : cards.rename,
    setSetting: onDaemon ? cardWrites.setSetting : cards.setSetting,
    requestBypass: cards.requestBypass,
    turnOffBypass: cards.turnOffBypass,
    quickAdd: onDaemon ? cardWrites.quickAdd : cardCreate.quickAdd,
    newCard: cardCreate.newCard,
    createCard: onDaemon ? cardWrites.createCard : cardCreate.createCard,
    start: onDaemon ? cardWrites.start : sessions.start,
    pause: holdOnDaemon ? cardHold.pause : sessions.pause,
    sleep: holdOnDaemon ? cardHold.sleep : sessions.sleep,
    wake: holdOnDaemon ? cardHold.wake : sessions.wake,
    pin: holdOnDaemon ? cardHold.pin : sessions.pin,
    stopSession: holdOnDaemon ? cardHold.stopSession : sessions.stopSession,
    keepAwake: holdOnDaemon ? cardHold.keepAwake : sessions.keepAwake,
    sleepAll: holdOnDaemon ? cardHold.sleepAll : sessions.sleepAll,
    keepAllAwake: holdOnDaemon ? cardHold.keepAllAwake : sessions.keepAllAwake,
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
  // The project chats are their own section, S17: the daemon keeps them and their messages once it
  // is switched, and `openChat` (which chat the pane shows) stays the screen's own state.
  const onDaemon = isDaemon("S17", sectionsOf(ctx.env));
  return bindActions(ctx, {
    approve: approvals.approve,
    deny: approvals.deny,
    approvePlan: approvals.approvePlan,
    rejectPlan: approvals.rejectPlan,
    editPlan: approvals.editPlan,
    savePlan: approvals.savePlan,
    toggleTool: approvals.toggleTool,
    // The chat is its own section: a card that is the daemon's can still have the mock's chat, and
    // the other way round, so the send is chosen by the chat's section and not the card's.
    send: isDaemon("S8a", sectionsOf(ctx.env))
      ? (target: Ctx, id: CardKey, text: string) => {
          const api = target.env.data?.api;
          if (api) void sendToCard(target, api, id, text);
        }
      : messages.send,
    chatSend: onDaemon ? sendToChat : chats.chatSend,
    openChat: chats.openChat,
    newChat: onDaemon ? chatWrites.newChat : chats.newChat,
    renameChat: onDaemon ? chatWrites.renameChat : chats.renameChat,
    archiveChat: onDaemon ? chatWrites.archiveChat : chats.archiveChat,
    deleteChat: onDaemon ? chatWrites.deleteChat : chats.deleteChat,
    retryChat,
  });
}

/**
 * Creates one store: the state, the `M` API the views call, the simulation of what is still
 * mock (unless the hash turns it off), and the link with the daemon for what is not. The browser
 * app uses the single instance from `index.ts`; tests create their own.
 */
export function createMarshal(env: Env) {
  return createMarshalIn(createContext(env));
}

/** The store of a context that was made, and possibly filled, first. `createMarshal` is this with a fresh one. */
export function createMarshalIn(ctx: Ctx) {
  const M = {
    S: ctx.S,
    STATUS,
    COLUMNS,
    CI,
    THINK,
    PERMS,
    ROLE_NAMES,
    VIEWS,
    T0: ctx.today,
    D: DAY_MS,
    H: HOUR_MS,
    MIN: MINUTE_MS,
    colOf,
    tone,
    money,
    full,
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
    /** The agents by name, as the prototype's `AGENTS` table had them; now the daemon's catalog and the built-in agent. */
    get AGENTS(): Record<string, AgentInfo> {
      return agents.agentTable(ctx);
    },
    /** The models in the catalog that have no thinking setting. */
    get NO_THINK(): readonly string[] {
      return agents.noThink(ctx);
    },
    ...queries(ctx),
    ...appActions(ctx),
    ...cardActions(ctx),
    ...chatActions(ctx),
  };
  startSimulation(ctx, ctx.env.hash);
  startSync(ctx);
  return M;
}

export type Marshal = ReturnType<typeof createMarshal>;
