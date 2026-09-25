import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { KeyValueStore } from "~/data/storage";
import { createTestMarshal } from "~/testing/test-store";
import { FIXED_TIME, makeTwin, SLOW_TEST_MS, type Twin } from "../testing/twin";

describe("navigation", { timeout: SLOW_TEST_MS }, () => {
  let t: Twin;
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
    t = makeTwin();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it("goes to pages and views like the prototype", () => {
    t.run("go", "project", "web");
    expect(t.port.S.route).toEqual({ page: "project", pid: "web", view: "board" });
    t.run("setView", "list");
    t.run("go", "home");
    t.run("go", "project", "web");
    expect(t.port.S.route.view).toBe("list");
    t.run("go", "project", "api", "chat");
    t.run("go", "settings");
    t.run("go", "all");
    t.run("setView", "timeline");
    t.same();
    expect(t.port.S.route).toEqual({ page: "project", pid: "api", view: "timeline" });
  });

  it("uses the phone rules on narrow screens", () => {
    t.run("setViewport", 390, 844);
    expect(t.port.mobile).toBe(true);
    t.run("openCard", "api#41");
    t.run("go", "project", "api", "chat");
    expect(t.port.S).toMatchObject({ openId: null, mobileTab: "chat" });
    t.run("go", "project", "api", "board");
    expect(t.port.S.mobileTab).toBe("board");
    t.run("go", "home");
    expect(t.port.S.mobileTab).toBe("home");
    t.run("setViewport", 390, 844);
    t.same();
  });

  it("opens and closes cards, following the card's project", () => {
    t.run("go", "project", "api");
    t.run("openCard", "web#118", "diff");
    t.same();
    expect(t.port.S).toMatchObject({ openId: "web#118", focusId: "web#118", tab: "diff" });
    expect(t.port.S.route.pid).toBe("web");
    t.run("openCard", 9999);
    t.run("setTab", "activity");
    t.run("closeCard");
    t.same();
    expect(t.port.S.openId).toBeNull();
  });

  it("switches between chat and terminal after a short delay", () => {
    t.run("setMode", "chat");
    t.run("setMode", "terminal");
    t.run("setMode", "chat");
    expect(t.port.S.switching).toBe("terminal");
    t.play(1500, 100);
    expect(t.port.S).toMatchObject({ mode: "terminal", switching: false });
  });

  it("sets state with an object or a function", () => {
    t.run("set", { palette: true, menu: "filter" });
    t.port.set((S) => ({ dashRange: S.dashRange + 23 }));
    expect(t.port.S).toMatchObject({ palette: true, menu: "filter", dashRange: 30 });
    expect(t.port.set({ allKind: "ci" })).toBeUndefined();
    t.port.emit();
  });

  it("shows toasts for 4 s, or 8 s with an action, and keeps the last three", () => {
    t.run("toast", "One");
    t.run("toast", "Two", { label: "Open", run: () => {} });
    t.run("toast", "Three");
    t.run("toast", "Four");
    t.same();
    expect(t.port.S.toasts.map((x) => x.msg)).toEqual(["Two", "Three", "Four"]);
    vi.advanceTimersByTime(4000);
    expect(t.port.S.toasts.map((x) => x.msg)).toEqual(["Two"]);
    t.port.S.toasts[0]?.dismiss();
    expect(t.port.S.toasts).toEqual([]);
  });
});

describe("onboarding and tour", { timeout: SLOW_TEST_MS }, () => {
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it("finishes onboarding, remembers it, and resets to a first launch", () => {
    const t = makeTwin();
    expect(t.port.S.onboarding).toBe(true);
    t.run("openCard", "api#41");
    t.run("finishOnboarding");
    t.same();
    expect(t.port.S).toMatchObject({ onboarding: false, tour: { step: 0 }, openId: null });
    expect(window.localStorage.getItem("marshal-proto-onboarded")).toBe("1");
    t.run("set", { palette: true, menu: "more", obStep: 3 });
    t.run("resetFirstLaunch");
    t.same();
    expect(t.port.S).toMatchObject({ onboarding: true, obStep: 0, tour: null, palette: false });
    expect(window.localStorage.getItem("marshal-proto-onboarded")).toBeNull();
    t.run("go", "settings");
    t.run("startTour");
    t.same();
    expect(t.port.S).toMatchObject({ tour: { step: 0 }, mobileTab: "home" });
    expect(t.port.S.route.page).toBe("home");
  });

  const make = (storage: KeyValueStore | null) =>
    createTestMarshal({
      hash: "#nosim",
      storage,
      viewport: { w: 1440, h: 900 },
      applyTheme: () => {},
    });

  it("skips onboarding once it was finished before", () => {
    const saved = new Map([["marshal-proto-onboarded", "1"]]);
    const storage: KeyValueStore = {
      getItem: (k) => saved.get(k) ?? null,
      setItem: (k, v) => saved.set(k, v),
      removeItem: (k) => saved.delete(k),
    };
    const M = make(storage);
    expect(M.S.onboarding).toBe(false);
    M.resetFirstLaunch();
    expect(saved.has("marshal-proto-onboarded")).toBe(false);
  });

  it("keeps working when storage is missing or throws", () => {
    const broken: KeyValueStore = {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
      removeItem: () => {
        throw new Error("blocked");
      },
    };
    for (const storage of [broken, null]) {
      const M = make(storage);
      expect(M.S.onboarding).toBe(true);
      M.finishOnboarding();
      M.resetFirstLaunch();
      expect(M.S.onboarding).toBe(true);
    }
  });
});
