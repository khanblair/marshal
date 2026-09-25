import { describe, expect, it } from "vitest";
import { backoffDelay } from "./backoff";

const POLICY = { baseMs: 500, capMs: 10_000, jitter: 0.2 };

describe("backoffDelay", () => {
  it("doubles from the base and stops at the cap", () => {
    const waits = [0, 1, 2, 3, 4, 5, 6, 40].map((n) => backoffDelay(n, POLICY, () => 0.5));
    expect(waits).toEqual([500, 1000, 2000, 4000, 8000, 10_000, 10_000, 10_000]);
  });

  it("moves the wait by up to 20 percent either way", () => {
    expect(backoffDelay(1, POLICY, () => 0)).toBe(800);
    expect(backoffDelay(1, POLICY, () => 1)).toBe(1200);
    expect(backoffDelay(1, POLICY, () => 0.75)).toBe(1100);
  });

  it("adds no jitter when the policy has none", () => {
    const plain = { baseMs: 1000, capMs: 10_000, jitter: 0 };
    expect([0, 1, 2, 3, 4].map((n) => backoffDelay(n, plain, () => 0.9))).toEqual([
      1000, 2000, 4000, 8000, 10_000,
    ]);
  });

  it("uses Math.random by default", () => {
    const wait = backoffDelay(0, POLICY);
    expect(wait).toBeGreaterThanOrEqual(400);
    expect(wait).toBeLessThanOrEqual(600);
  });
});
