import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from "vitest";
import { wireCard } from "~/testing/fake-cards";
import { createFakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import type { Card, CardView, MsgView, State } from ".";
import { loadPrototype } from "./testing/prototype";

/** Every member of the prototype's `window.M`, minus `on` and `selRef`, the `_` helpers kept
 *  internal, and `_suppressClick`; plus `now` and `cardLabelOf`, which names a card with its project. */
const API = [
  "AGENTS",
  "CI",
  "COLUMNS",
  "D",
  "H",
  "MIN",
  "NO_THINK",
  "PERMS",
  "ROLE_NAMES",
  "S",
  "STATUS",
  "T0",
  "THINK",
  "VIEWS",
  "_framed",
  "_scale",
  "addChecklist",
  "addComment",
  "addFilter",
  "addItem",
  "addProject",
  "agentOptions",
  "reconnect",
  "applyView",
  "approve",
  "approvePlan",
  "archiveChat",
  "authorizeGoogleCalendar",
  "awake",
  "card",
  "cardLabelOf",
  "cardsOf",
  "chatById",
  "chatSend",
  "chatsOf",
  "chooseAvatar",
  "ciOnDaemon",
  "endTour",
  "clearFilters",
  "closeCard",
  "closeDialog",
  "colCards",
  "colOf",
  "commands",
  "confirm",
  "connectDiscord",
  "connectGitHub",
  "connectGmail",
  "connectGoogleCalendar",
  "connectTelegram",
  "pairingOnDaemon",
  "connectNtfy",
  "connectTrello",
  "connectionOnDaemon",
  "createPairingCode",
  "pairingCode",
  "removeDevice",
  "tailnetStatus",
  "tailnetPeers",
  "costTone",
  "costs",
  "createCard",
  "createRole",
  "createSchedule",
  "deco",
  "decoMsgs",
  "deleteCard",
  "deleteChat",
  "deleteChecklist",
  "deleteComment",
  "deleteRole",
  "deleteSchedule",
  "deny",
  "diffFor",
  "disconnectIntegration",
  "dismissNotice",
  "dragStart",
  "dupes",
  "duplicateRole",
  "editPlan",
  "emit",
  "filesFor",
  "filtered",
  "finishOnboarding",
  "fork",
  "full",
  "go",
  "importRoles",
  "isAwake",
  "keepAllAwake",
  "keepAwake",
  "limitsOnDaemon",
  "noticesOnDaemon",
  "loadActivityPage",
  "loadCardDiff",
  "loadFileHunks",
  "mobile",
  "money",
  "moveCard",
  "nav",
  "needs",
  "newCard",
  "newChat",
  "now",
  "openCard",
  "openChat",
  "pause",
  "pendingApproval",
  "person",
  "meId",
  "pin",
  "previewOnDaemon",
  "previewShotImage",
  "proj",
  "quickAdd",
  "refuse",
  "rejectPlan",
  "rel",
  "removeFilter",
  "removeItem",
  "removeProject",
  "rename",
  "renameChat",
  "renameProject",
  "saveProject",
  "saveProfile",
  "saveProviderKey",
  "saveRole",
  "saveSchedule",
  "setOnboardingStep",
  "requestBypass",
  "resetFirstLaunch",
  "resetRole",
  "restoreCheckpoint",
  "retryChat",
  "rolesOnDaemon",
  "runChecks",
  "saveCardNote",
  "saveLimits",
  "savePlan",
  "saveSleepSettings",
  "alerts",
  "saveAlertChannels",
  "saveView",
  "scheduleRuns",
  "send",
  "set",
  "setMode",
  "setSetting",
  "setTab",
  "setTheme",
  "setView",
  "setViewport",
  "signIn",
  "pairWithCode",
  "simulateCiFailure",
  "simulateCiFailureReal",
  "simulateOnDaemon",
  "sleep",
  "sleepAll",
  "sleepOnDaemon",
  "start",
  "startPreview",
  "startSearch",
  "startTour",
  "stopPreview",
  "stopSession",
  "takePreviewShot",
  "terminalKey",
  "terminalSend",
  "terminalText",
  "testIntegration",
  "testProviderKey",
  "thinkSupported",
  "toast",
  "toggleHideDone",
  "toggleItem",
  "toggleMember",
  "toggleTool",
  "tone",
  "turnOffBypass",
  "wake",
  "working",
];

describe("index.ts boot", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.useFakeTimers();
    Reflect.deleteProperty(window, "M");
    window.location.hash = "#nosim";
    // The boot builds the real data layer from the page's own `fetch` and `WebSocket`; a daemon in memory answers them.
    // One card on the daemon, so the boot has a card to draw whether S5a reads the mock or the
    // daemon: the store types and the view models are what this suite is about.
    const daemon = createFakeDaemon({
      projects: PROTOTYPE_PROJECTS,
      cards: [wireCard({ projectId: "api", number: 41, title: "Fix token refresh on login" })],
    });
    vi.stubGlobal("fetch", daemon.fetch);
    vi.stubGlobal("WebSocket", daemon.sockets.Impl);
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
    // The stubs stay: a boot that is still connecting when its test ends must not reach jsdom's real sockets.
    Reflect.deleteProperty(window, "M");
  });

  it("puts one instance on window.M, announces it, and is ready once the daemon has answered", async () => {
    const ready = vi.fn();
    window.addEventListener("marshal-ready", ready);
    const mod = await import(".");
    expect(window.M).toBe(mod.M);
    expect(mod.S).toBe(mod.M.S);
    expect(mod.M.S.ready).toBe(false);
    await vi.waitFor(() => expect(mod.M.S.ready).toBe(true));
    expect(mod.M.S.resolvedTheme).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    expect(ready).toHaveBeenCalledTimes(1);
    window.removeEventListener("marshal-ready", ready);
  });

  it("exposes exactly the prototype's API", async () => {
    const { M } = await import(".");
    expect(Object.keys(M).sort()).toEqual([...API].sort());
    expect(M.mobile).toBe(false);
    expect(M.nav).toBeNull();
  });

  it("types the store and view models with the exported types", async () => {
    const { M, S } = await import(".");
    await vi.waitFor(() => expect(S.ready).toBe(true));
    expectTypeOf(S).toEqualTypeOf<State>();
    expectTypeOf(M.card).returns.toEqualTypeOf<Card | undefined>();
    expectTypeOf(M.deco).returns.toEqualTypeOf<CardView>();
    expectTypeOf(M.decoMsgs).returns.toEqualTypeOf<MsgView[]>();
    expect(M.deco(M.S.cards[0] as Card).num).toBe("#41");
  });

  it("matches the prototype's own member list, minus the documented drops", () => {
    const dropped = ["on", "selRef", "_startSession", "_dropFromSleep", "_syncProjectApproval"];
    // The shell sets these on the prototype after load; the port declares them up front.
    const added = [
      "_framed",
      "_scale",
      "now",
      "cardLabelOf",
      "saveProject",
      "saveProfile",
      "saveProviderKey",
      "saveLimits",
      "limitsOnDaemon",
      "testProviderKey",
      // The roles screen's own actions. The prototype keeps them in its view's script rather than
      // in its store, and the port declares them all up front; the names differ because the port's
      // list is the daemon's (a save, a create, a duplicate, a reset, a delete, an import) and the
      // query beside them says whether the daemon is the one being edited.
      "saveRole",
      "createRole",
      "duplicateRole",
      "resetRole",
      "deleteRole",
      // A card's restore points are the daemon's own commits (B5.3), so the prototype has nothing
      // to restore from and no member for it.
      "restoreCheckpoint",
      // A card's note (S14) is saved through `card-note.ts`'s own `saveNote`, which calls this one
      // member only once the section is the daemon's; the prototype's `card-note.ts` writes its
      // store directly and has no member for a daemon write at all.
      "saveCardNote",
      "importRoles",
      "rolesOnDaemon",
      // The notices (S23) and the sleep settings (S26a): the prototype keeps the notice actions in
      // its store but has no sleep settings of its own to save, and each of these says whether the
      // daemon is the one behind that section.
      "noticesOnDaemon",
      "sleepOnDaemon",
      "saveSleepSettings",
      "alerts",
      "saveAlertChannels",
      "chooseAvatar",
      "endTour",
      "setOnboardingStep",
      "meId",
      "signIn",
      "pairWithCode",
      "reconnect",
      "agentOptions",
      "loadActivityPage",
      "loadCardDiff",
      "loadFileHunks",
      "startSearch",
      "stopSession",
      "retryChat",
      "terminalKey",
      "terminalSend",
      "terminalText",
      // "Simulate CI failure" is the prototype's, but only its one synthetic story: the daemon is
      // the one that runs both modes, and the second half of the pair is the real mode, which pushes
      // a failing change to GitHub. `simulateOnDaemon` is the query that says whether the daemon,
      // rather than the mock, is the one behind the item, and `ciOnDaemon` whether a daemon owns the
      // card at all (N28, B6.4).
      "ciOnDaemon",
      "simulateCiFailureReal",
      "simulateOnDaemon",
      // A card's live preview (S13, B6.6) is the daemon's own: it starts and stops a dev server on
      // the machine and takes the before and after screenshots of it, so the prototype has no member
      // for any of it, and the two queries beside the calls say whether the daemon is the one behind
      // the tab at all. A shot's image needs the token, so it is read through the client and drawn
      // from an object URL, the way an avatar is.
      "startPreview",
      "stopPreview",
      "takePreviewShot",
      "previewOnDaemon",
      "previewShotImage",
      // The GitHub connection (S29a, B6.1 and B6.7) is the daemon's own: it holds the App's key and
      // its webhook secret in the OS keychain, installs the App, and tests the connection, so the
      // prototype has no member for any of it. The prototype's own connection rows are a drawing,
      // and what a person could press there turned a state in the mock.
      "connectGitHub",
      "connectTelegram",
      "pairingOnDaemon",
      "connectNtfy",
      "connectDiscord",
      "disconnectIntegration",
      "testIntegration",
      // The paired devices (S2b, B9.1 and B9.2) are the daemon's own: it holds the token hash and
      // decides what a pairing code is worth, so the prototype's hardcoded code has no member in
      // either half. What a press makes, and what a revoke does, are reads and writes to it alone.
      "createPairingCode",
      "pairingCode",
      "removeDevice",
      "tailnetStatus",
      "tailnetPeers",
      // The query beside them says whether the daemon owns a connection's row, the way `rolesOnDaemon`
      // and the rest do, so a row of a later phase still reads as the mock's own.
      "connectionOnDaemon",
      // Trello, Google Calendar, and Gmail (B8.2-B8.4) are all the daemon's own: two-way card sync,
      // an OAuth client and consent, and a poller reading a label, none of which the prototype has.
      "connectTrello",
      "connectGoogleCalendar",
      "authorizeGoogleCalendar",
      "connectGmail",
      // Schedules (S30, B8.1): the daemon's own scheduler, so the prototype's array in the store has
      // no member for adding, editing, removing, or reading one's run history.
      "createSchedule",
      "saveSchedule",
      "deleteSchedule",
      "scheduleRuns",
    ];
    const proto = loadPrototype("#nosim").keys.filter((k) => !dropped.includes(k));
    expect([...proto, ...added].sort()).toEqual([...API].sort());
  });

  it("reuses an instance that is already on window.M", async () => {
    const first = await import(".");
    vi.resetModules();
    const second = await import(".");
    expect(second.M).toBe(first.M);
  });
});
