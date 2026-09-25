import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FIXED_TIME, makeTwin, SLOW_TEST_MS } from "../testing/twin";

describe("simulation parity with the prototype", { timeout: SLOW_TEST_MS }, () => {
  beforeEach(() => {
    vi.useFakeTimers({ now: FIXED_TIME });
  });
  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it("plays the whole scripted daemon the same, second by second", () => {
    const t = makeTwin("");
    t.same();
    // Past #41's pull request, #46's plan, the #35 merge, three CI failures on #40,
    // and the 125 s sleep notice.
    t.play(135_000, 1000);
    const S = t.port.S;
    expect(S.cards.find((c) => c.id === "api#41")?.state).toBe("review");
    expect(S.cards.find((c) => c.id === "api#46")?.state).toBe("needs");
    expect(S.cards.find((c) => c.id === "api#35")?.state).toBe("done");
    expect(S.cards.find((c) => c.id === "api#40")?.reason).toBe(
      "CI failed 3 times: TestRetryBackoff",
    );
    expect(S.notices.some((n) => n.kind === "sleep")).toBe(false);
  });

  it("matches in fine steps while messages stream", () => {
    const t = makeTwin("#n46-nm-nci-nt");
    t.play(12_000, 45);
  });

  it("runs only the tick with #n41-n46-nm-nci", () => {
    const t = makeTwin("#n41-n46-nm-nci");
    t.play(41_000, 1000);
    expect(t.port.S.feed[0]?.text).toBe("Fix until e2e passes ran round 3 on #213");
  });

  it("does nothing under #nosim", () => {
    const t = makeTwin("#nosim");
    const before = JSON.stringify(t.port.S);
    vi.advanceTimersByTime(60_000);
    expect(JSON.stringify(t.port.S)).toBe(before);
    t.same();
  });

  it.each([
    ["#n41", "api#41", "working"],
    ["#n46", "api#46", "planning"],
    ["#nm", "api#35", "merging"],
    ["#nci", "api#40", "review"],
  ] as const)("%s keeps card %s out of the script", (hash, id, state) => {
    const t = makeTwin(hash);
    t.play(30_000, 1000);
    expect(t.port.S.cards.find((c) => c.id === id)?.state).toBe(state);
  });

  it("stops the tick with #nt", () => {
    const t = makeTwin("#n41-n46-nm-nci-nt");
    const cost = t.port.S.cards.find((c) => c.id === "api#42")?.cost;
    vi.advanceTimersByTime(10_000);
    expect(t.port.S.cards.find((c) => c.id === "api#42")?.cost).toBe(cost);
  });
});
