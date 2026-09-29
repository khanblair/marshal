import { isDaemon } from "~/data/sections";
import { startSync } from "~/sync";
import * as cardWrites from "~/sync/card-actions";
import * as cardHold from "~/sync/card-hold";
import { sendToCard } from "~/sync/card-session";
import * as cardView from "~/sync/card-view";
import * as chatWrites from "~/sync/chat-actions";
import { retryChat, sendToChat } from "~/sync/chat-session";
import * as ciWrites from "~/sync/ci-actions";
import * as connection from "~/sync/connection-actions";
import * as devices from "~/sync/devices";
import { loadCardDiff, loadFileHunks } from "~/sync/diff";
import { homeActivityPage } from "~/sync/home-feed";
import * as integrationWrites from "~/sync/integration-actions";
import * as integrations from "~/sync/integrations";
import * as limitWrites from "~/sync/limit-actions";
import { saveCardNote } from "~/sync/notes";
import * as noticeWrites from "~/sync/notice-actions";
import * as onboardingWrites from "~/sync/onboarding-actions";
import * as profileWrites from "~/sync/profile-actions";
import * as projects from "~/sync/project-actions";
import * as providerWrites from "~/sync/provider-actions";
import * as previewWrites from "~/sync/preview";
import * as roleWrites from "~/sync/role-actions";
import { roleNames } from "~/sync/roles";
import * as savedViews from "~/sync/saved-views";
import * as scheduleWrites from "~/sync/schedule-actions";
import { createPaletteSearch } from "~/sync/search";
import * as alertWrites from "~/sync/alerts";
import * as sleepWrites from "~/sync/sleep-actions";
import * as approvals from "./actions/approvals";
import * as cardCreate from "./actions/card-create";
import * as cards from "./actions/cards";
import * as chats from "./actions/chats";
import * as checklists from "./actions/checklists";
import * as checkpoints from "./actions/checkpoints";
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
    // True while the cost and awake limits (S26b) are the daemon's: the limits form then saves
    // through `saveLimits` rather than writing the store itself.
    limitsOnDaemon: (c: Ctx) => isDaemon("S26b", sectionsOf(c.env)),
    // True while the roles (S27) are the daemon's: the role editor then saves, renames, duplicates,
    // resets, deletes, and imports through it rather than writing the store itself.
    rolesOnDaemon: (c: Ctx) => isDaemon("S27", sectionsOf(c.env)),
    // True while the notices (S23) are the daemon's, and while the sleep settings (S26a) are: the
    // notice's buttons and the Sessions panel of Settings then go through the daemon.
    noticesOnDaemon: (c: Ctx) => isDaemon("S23", sectionsOf(c.env)),
    sleepOnDaemon: (c: Ctx) => isDaemon("S26a", sectionsOf(c.env)),
    // True while onboarding's pairing and alert-apps step (S31b) is the daemon's: it makes a real
    // pairing code and saves real connections. It needs a daemon that answers, so a store with none,
    // or one that is not online, shows the design's own picture of the step.
    pairingOnDaemon: (c: Ctx) =>
      isDaemon("S31b", sectionsOf(c.env)) && c.S.connection?.state === "online",
    // True while "Simulate CI failure" is the daemon's for this card (N28, B6.4): the card is the
    // daemon's own and that daemon runs in dev mode, which is the only mode that has the routes. The
    // card menu reads it to decide which items to draw, so the rule lives beside the action that
    // uses it (`sync/ci-actions.ts`). `ciOnDaemon` is the first half of the same pair - whether a
    // daemon owns the card at all - which is what tells a card the mock made from one it did not.
    ciOnDaemon: ciWrites.ciOnDaemon,
    simulateOnDaemon: ciWrites.simulateOnDaemon,
    // True while a card's live preview (S13) is the daemon's: the Preview tab then starts, stops, and
    // shoots through it rather than drawing the mock's own story, which is gone with the section.
    previewOnDaemon: previewWrites.previewOnDaemon,
    // The bytes of a screenshot, fetched with the token and shown as an object URL, the way an
    // avatar is (`sync/avatar.ts`). Null when there is nothing to show.
    previewShotImage: previewWrites.readPreviewShotImage,
    // True while a connection's row in Settings (S29a for GitHub) is the daemon's: the row then
    // reads its status, its sentence, and its last test from the daemon, and its buttons call the
    // daemon's routes rather than the mock's own "connect" story.
    connectionOnDaemon: integrations.connectionOnDaemon,
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
    // "Simulate CI failure" (N28, B6.4) is the mock's own story until the card is the daemon's and
    // that daemon runs in dev mode, where the two modes are the daemon's own work. It is decided per
    // call rather than once per store, because one store holds both a card the mock made and one the
    // daemon did, exactly like `setMode` above.
    simulateCiFailure: (c: Ctx, id?: CardKey) => {
      const card = id === undefined ? undefined : q.card(c, id);
      if (id !== undefined && ciWrites.simulateOnDaemon(c, card))
        void ciWrites.simulateSynthetic(c, id);
      else settings.simulateCiFailure(c, id);
    },
    // The real mode (B6.4) has no mock story at all: it pushes a failing change to the card's own
    // branch on GitHub, which only a dev daemon can do, so nothing happens without one.
    simulateCiFailureReal: (c: Ctx, id: CardKey) => {
      if (ciWrites.simulateOnDaemon(c, q.card(c, id))) ciWrites.simulateReal(c, id);
    },
    addFilter: filters.addFilter,
    removeFilter: filters.removeFilter,
    clearFilters: filters.clearFilters,
    applyView: filters.applyView,
    saveView: viewsOnDaemon ? savedViews.saveView : filters.saveView,
    saveProfile: profileOnDaemon ? profileWrites.saveProfile : settings.saveProfile,
    chooseAvatar: profileOnDaemon ? profileWrites.chooseAvatar : settings.chooseAvatar,
    // The provider keys (S28) are the daemon's: the key lives in the OS keychain, only the masked
    // form ever comes back, and the test runs in the daemon, so there is no mock behaviour to fall
    // back to. A store with no data layer answers "not connected" the way every other daemon
    // action does.
    saveProviderKey: providerWrites.saveProviderKey,
    testProviderKey: providerWrites.testProviderKey,
    // The connections (S29a for GitHub) are the daemon's once their section is switched: the
    // settings row saves the App's whole setup, forgets it, and runs its test through the daemon,
    // which owns the keychain entry and the last test's result. The mock's own "connect" story
    // stays in the screen for the rows whose sections are still the mock's.
    connectGitHub: integrationWrites.connectGitHub,
    connectTrello: integrationWrites.connectTrello,
    connectGoogleCalendar: integrationWrites.connectGoogleCalendar,
    connectGmail: integrationWrites.connectGmail,
    connectTelegram: integrationWrites.connectTelegram,
    connectNtfy: integrationWrites.connectNtfy,
    connectDiscord: integrationWrites.connectDiscord,
    // The paired devices (S2b): the list is the daemon's once its section is switched, and the
    // pairing code and a revoke are its own calls - the prototype's hardcoded code is gone.
    pairingCode: devices.currentPairingCode,
    createPairingCode: devices.requestPairingCode,
    removeDevice: devices.removeDevice,
    tailnetStatus: devices.readTailnetStatus,
    tailnetPeers: devices.readTailnetPeers,
    authorizeGoogleCalendar: integrationWrites.authorizeGoogleCalendar,
    disconnectIntegration: integrationWrites.disconnectIntegration,
    testIntegration: integrationWrites.testIntegration,
    // Schedules (S30) and Google Calendar's events (S25/S22): the daemon's once their sections are
    // switched. A schedule's own save/delete already exist through the daemon; the screen's local
    // push/splice fallback stays for a store still on the mock.
    createSchedule: scheduleWrites.createSchedule,
    saveSchedule: scheduleWrites.saveSchedule,
    deleteSchedule: scheduleWrites.deleteSchedule,
    scheduleRuns: scheduleWrites.scheduleRuns,
    // The limits (S26b) are the daemon's once the section is switched: the save compares the form
    // with the store and PUTs or DELETEs one ceiling at a time. The mock's own save lives in the
    // form itself, which picks this or that by `limitsOnDaemon`.
    saveLimits: limitWrites.saveLimits,
    // The roles (S27) are the daemon's once the section is switched: the editor saves, renames,
    // duplicates, resets, deletes, and imports through it. The mock's own writes stay in the
    // screen's `role-actions.ts`, which picks this or that by `rolesOnDaemon`.
    saveRole: roleWrites.saveRole,
    createRole: roleWrites.createRole,
    duplicateRole: roleWrites.duplicateRole,
    resetRole: roleWrites.resetRole,
    deleteRole: roleWrites.deleteRole,
    importRoles: roleWrites.importRoles,
    // The sleep settings (S26a) are the daemon's once the section is switched. The mock's own write
    // stays in the screen's `sleep-actions.ts`, which picks this or that by `sleepOnDaemon`.
    saveSleepSettings: sleepWrites.saveSleepSettings,
    alerts: alertWrites.currentAlerts,
    saveAlertChannels: alertWrites.saveAlertChannels,
    addProject: projects.addProject,
    renameProject: projects.renameProject,
    saveProject: projects.saveProject,
    removeProject: projects.removeProject,
    signIn: connection.signIn,
    pairWithCode: connection.pairWithCode,
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
  // The notices are their own section too (S23). While the hold controls are the daemon's but the
  // notices are still the mock's, the notice's own buttons change nothing and say so
  // (`sync/card-hold.ts`); once S23 is switched they are the daemon's, and they are the ones that
  // win, because a notice is what those buttons are about.
  const noticesOnDaemon = isDaemon("S23", sectionsOf(ctx.env));
  // Bypass is its own section too (S7b): the confirmation is the screen's either way, and the grant
  // is the daemon's once the section is switched, because it is the daemon that holds the project
  // lock and writes the audit row.
  const bypassOnDaemon = isDaemon("S7b", sectionsOf(ctx.env));
  return bindActions(ctx, {
    dragStart,
    moveCard: onDaemon ? cardWrites.moveCard : cards.moveCard,
    fork: onDaemon ? cardWrites.fork : cards.fork,
    deleteCard: onDaemon ? cardWrites.deleteCard : cards.deleteCard,
    rename: onDaemon ? cardWrites.rename : cards.rename,
    setSetting: onDaemon ? cardWrites.setSetting : cards.setSetting,
    requestBypass: bypassOnDaemon ? cardWrites.requestBypass : cards.requestBypass,
    turnOffBypass: bypassOnDaemon ? cardWrites.turnOffBypass : cards.turnOffBypass,
    quickAdd: onDaemon ? cardWrites.quickAdd : cardCreate.quickAdd,
    newCard: cardCreate.newCard,
    createCard: onDaemon ? cardWrites.createCard : cardCreate.createCard,
    start: onDaemon ? cardWrites.start : sessions.start,
    pause: holdOnDaemon ? cardHold.pause : sessions.pause,
    sleep: holdOnDaemon ? cardHold.sleep : sessions.sleep,
    wake: holdOnDaemon ? cardHold.wake : sessions.wake,
    pin: holdOnDaemon ? cardHold.pin : sessions.pin,
    stopSession: holdOnDaemon ? cardHold.stopSession : sessions.stopSession,
    keepAwake: noticesOnDaemon
      ? noticeWrites.keepAwake
      : holdOnDaemon
        ? cardHold.keepAwake
        : sessions.keepAwake,
    sleepAll: noticesOnDaemon
      ? noticeWrites.sleepAll
      : holdOnDaemon
        ? cardHold.sleepAll
        : sessions.sleepAll,
    keepAllAwake: noticesOnDaemon
      ? noticeWrites.keepAllAwake
      : holdOnDaemon
        ? cardHold.keepAllAwake
        : sessions.keepAllAwake,
    dismissNotice: noticesOnDaemon ? noticeWrites.dismissNotice : sessions.dismissNotice,
    toggleItem: checklists.toggleItem,
    addItem: checklists.addItem,
    removeItem: checklists.removeItem,
    addChecklist: checklists.addChecklist,
    deleteChecklist: checklists.deleteChecklist,
    toggleHideDone: checklists.toggleHideDone,
    restoreCheckpoint: checkpoints.restoreCheckpoint,
    // A card's note (S14): `card-note.ts`'s own `saveNote` decides whether to call this (the
    // section is the daemon's) or write the mock's store directly, so this is never reached while
    // S14 is still the mock's.
    saveCardNote,
    // A card's live preview (S13): the daemon owns whether a dev server runs and which screenshots
    // exist, so these ask it and the tab draws whatever it last answered.
    startPreview: previewWrites.startCardPreview,
    stopPreview: previewWrites.stopCardPreview,
    takePreviewShot: previewWrites.takeCardPreviewShot,
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
    /**
     * The role names every picker lists (N18). Once the roles (S27) are the daemon's, they are its
     * own roles, so a person's own roles appear in the New card dialog and the chat target picker;
     * until then they are the prototype's eight.
     */
    get ROLE_NAMES(): readonly string[] {
      return isDaemon("S27", sectionsOf(ctx.env)) ? roleNames(ctx) : ROLE_NAMES;
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
