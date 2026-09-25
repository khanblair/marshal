import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from "vitest";
import type { Card, CardView, MsgView, State } from ".";
import { loadPrototype } from "./testing/prototype";

/** Every member of the prototype's `window.M`, minus `on` and `selRef`, the `_` helpers kept
 *  internal, and `_suppressClick`; plus `now`. */
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
  "applyView",
  "approve",
  "approvePlan",
  "archiveChat",
  "awake",
  "card",
  "cardsOf",
  "chatById",
  "chatSend",
  "chatsOf",
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
  "requestBypass",
  "resetFirstLaunch",
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
  "simulateCiFailure",
  "sleep",
  "sleepAll",
  "start",
  "startTour",
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
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
    Reflect.deleteProperty(window, "M");
  });

  it("puts one ready instance on window.M and announces it", async () => {
    const ready = vi.fn();
    window.addEventListener("marshal-ready", ready);
    const mod = await import(".");
    expect(window.M).toBe(mod.M);
    expect(mod.S).toBe(mod.M.S);
    expect(mod.M.S.ready).toBe(true);
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
    expectTypeOf(S).toEqualTypeOf<State>();
    expectTypeOf(M.card).returns.toEqualTypeOf<Card | undefined>();
    expectTypeOf(M.deco).returns.toEqualTypeOf<CardView>();
    expectTypeOf(M.decoMsgs).returns.toEqualTypeOf<MsgView[]>();
    expect(M.deco(M.S.cards[0] as Card).num).toBe("#41");
  });

  it("matches the prototype's own member list, minus the documented drops", () => {
    const dropped = ["on", "selRef", "_startSession", "_dropFromSleep", "_syncProjectApproval"];
    // The shell sets these on the prototype after load; the port declares them up front.
    const added = ["_framed", "_scale", "now"];
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
