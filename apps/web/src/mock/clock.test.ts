import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createClock } from "./clock";

describe("createClock", () => {
  beforeEach(() => vi.useFakeTimers({ now: 1_000_000 }));
  afterEach(() => vi.useRealTimers());

  it("is the local clock when no offset is given", () => {
    expect(createClock().now()).toBe(1_000_000);
  });

  it("counts from the daemon's clock, following the offset as it is learned", () => {
    let offset = 0;
    const clock = createClock(() => offset);
    expect(clock.now()).toBe(1_000_000);
    offset = 5000;
    expect(clock.now()).toBe(1_005_000);
    offset = -2000;
    expect(clock.now()).toBe(998_000);
  });
});
