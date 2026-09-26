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
  "awake",
  "card",
  "cardLabelOf",
  "cardsOf",
  "chatById",
  "chatSend",
  "chatsOf",
  "chooseAvatar",
  "endTour",
  "clearFilters",
  "closeCard",
  "closeDialog",
  "colCards",
  "colOf",
  "commands",
  "confirm",
  "costTone",
  "costs",
  "createCard",
  "deco",
  "decoMsgs",
  "deleteCard",
  "deleteChat",
  "deleteChecklist",
  "deleteComment",
  "deny",
  "diffFor",
  "dismissNotice",
  "dragStart",
  "dupes",
  "editPlan",
  "emit",
  "filesFor",
  "filtered",
  "finishOnboarding",
  "fork",
  "full",
  "go",
  "isAwake",
  "keepAllAwake",
  "keepAwake",
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
  "setOnboardingStep",
  "requestBypass",
  "resetFirstLaunch",
  "retryChat",
  "runChecks",
  "savePlan",
  "saveView",
  "send",
  "set",
  "setMode",
  "setSetting",
  "setTab",
  "setTheme",
  "setView",
  "setViewport",
  "signIn",
  "simulateCiFailure",
  "sleep",
  "sleepAll",
  "start",
  "startSearch",
  "startTour",
  "stopSession",
  "terminalKey",
  "terminalSend",
  "terminalText",
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
      "chooseAvatar",
      "endTour",
      "setOnboardingStep",
      "meId",
      "signIn",
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
