import { createEffect, createMemo, createRoot } from "solid-js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createTestMarshal } from "~/testing/test-store";
import type { Marshal } from "./marshal";

const make = (hash = "#nosim"): Marshal =>
  createTestMarshal({ hash, storage: null, viewport: { w: 1440, h: 900 }, applyTheme: () => {} });

/** Runs `read` in an effect and records every value it produced. */
function track<T>(read: () => T): { values: T[]; dispose: () => void } {
  const values: T[] = [];
  let dispose = (): void => {};
  createRoot((d) => {
    dispose = d;
    createEffect(() => {
      values.push(read());
    });
  });
  return { values, dispose };
}

describe("reactivity contract", () => {
  let M: Marshal;
  beforeEach(() => {
    vi.useFakeTimers({ now: new Date("2026-09-24T10:00:00") });
    M = make();
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it("re-runs an effect reading M.S.cards[0].doing when an action changes it", () => {
    const seen = track(() => M.S.cards[0]?.doing);
    expect(seen.values).toEqual(["Running auth tests"]);
    M.moveCard("api#41", "backlog");
    expect(seen.values).toEqual(["Running auth tests", ""]);
    seen.dispose();
  });

  it("applies one action as one update", () => {
    const seen = track(() => `${M.S.openId}:${M.S.tab}:${M.S.focusId}`);
    M.openCard("web#118", "diff");
    expect(seen.values).toEqual(["null:chat:null", "web#118:diff:web#118"]);
    seen.dispose();
  });

  it("updates a deco memo when the card changes", () => {
    createRoot((dispose) => {
      const card = M.card("api#42");
      if (!card) throw new Error("no card");
      const view = createMemo(() => M.deco(card));
      expect(view().showDoing).toBe(true);
      M.pause("api#42");
      expect(view()).toMatchObject({ showDoing: false, paused: true });
      dispose();
    });
  });

  it("notifies through objects kept by timers: tool results, streams, comments, new cards", () => {
    const tool = track(
      () => M.S.chat["api#44"]?.at(-1)?.k === "tool" && JSON.stringify(M.S.chat["api#44"]?.at(-1)),
    );
    const streamed = track(
      () => M.S.chat["web#119"]?.at(-1)?.k === "agent" && M.S.chat["web#119"]?.at(-1),
    );
    const comment = track(() => M.card("api#41")?.comments.at(-1)?.read);
    M.approve("api#44");
    M.send("web#119", "Use the typed helper");
    M.addComment("api#41", "Done?", []);
    M.quickAdd("api", "working", "New work");
    const fresh = track(() => M.card("api#47")?.doing);
    vi.advanceTimersByTime(1199);
    expect(comment.values.at(-1)).toBe(false);
    vi.advanceTimersByTime(1);
    expect(comment.values.at(-1)).toBe(true);
    vi.advanceTimersByTime(2800);
    expect(tool.values.length).toBeGreaterThan(2);
    expect(streamed.values.length).toBeGreaterThan(2);
    expect(fresh.values).toEqual(["Setting up the worktree", "Editing internal/proxy/handler.go"]);
    for (const s of [tool, streamed, comment, fresh]) s.dispose();
  });

  it("keeps time-based text live while the simulation ticks, frozen under #nosim", () => {
    const ts = Date.now() - 30_000;
    const frozen = track(() => M.rel(ts));
    vi.advanceTimersByTime(60_000);
    expect(frozen.values).toEqual(["Just now"]);
    frozen.dispose();

    const live = make("#n41-n46-nm-nci");
    const text = track(() => live.rel(Date.now() - 30_000 - 0));
    const countdown = track(() => {
      const n = live.S.notices.find((x) => x.kind === "sleep");
      return n && "deadline" in n ? Math.ceil((n.deadline - live.now()) / 1000) : null;
    });
    vi.advanceTimersByTime(3000);
    expect(countdown.values).toEqual([125, 124, 123, 122]);
    expect(text.values.length).toBe(4);
    text.dispose();
    countdown.dispose();
  });

  it("exposes the viewport and reports phone layouts", () => {
    const layout = track(() => M.mobile);
    M.setViewport(390, 844);
    M.setViewport(390, 844);
    expect(layout.values).toEqual([false, true]);
    expect(M.S.vh).toBe(844);
    layout.dispose();
  });
});
