import { createRoot } from "solid-js";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import type { DiffList } from "./diff-list";
import { createLiveDiff } from "./live-diff";
import { cardOf, resetStore } from "./test-helpers";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => {
  resetStore();
  vi.useFakeTimers();
});
afterEach(() => vi.useRealTimers());

const stub = (): DiffList & { reads: number } => {
  const list = {
    reads: 0,
    files: () => [],
    loading: () => false,
    error: () => undefined,
    ready: () => true,
    refetch: () => {
      list.reads += 1;
    },
  };
  return list;
};

describe("the live diff", () => {
  it("reads the diff again once the card's chat has been quiet, and only once for a burst", () => {
    const card = cardOf("api#41");
    const diff = stub();
    const dispose = createRoot((stop) => {
      createLiveDiff(() => card, diff);
      return stop;
    });
    // Opening the card is the panel's own read, so nothing is read for it here.
    vi.advanceTimersByTime(5000);
    expect(diff.reads).toBe(0);

    const chat = M.S.chat[card.id] ?? [];
    M.S.chat[card.id] = chat;
    chat.push({ id: "t1", k: "system", text: "an edit" });
    vi.advanceTimersByTime(500);
    chat.push({ id: "t2", k: "system", text: "another edit" });
    vi.advanceTimersByTime(1400);
    expect(diff.reads).toBe(0);
    vi.advanceTimersByTime(200);
    expect(diff.reads).toBe(1);
    dispose();
  });
});
