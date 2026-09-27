import type { LimitList } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "../testing/golden";
import {
  dollarsToMicros,
  KIND_OF,
  MICROS_PER_DOLLAR,
  microsToDollars,
  microsToWholeDollars,
  toLimitsByScope,
  wireValueOf,
} from "./limits";

/* The list the real daemon answers with, written by its own Go tests, so the two sides cannot drift. */
const list = golden<LimitList>("limit-list");

describe("toLimitsByScope", () => {
  it("maps the golden list, one entry per scope, and always keeps global", () => {
    expect(toLimitsByScope(list)).toEqual({
      global: { day: 25, month: 400, awake: 18 },
      "01JD7Q4M2X8K9V0P5T3RB6NHAE": { day: 5 },
    });
  });

  it("keeps only global, with no field at all, when the daemon has no ceilings", () => {
    expect(toLimitsByScope({ limits: [] })).toEqual({ global: {} });
  });

  it("leaves a kind the daemon did not send absent, rather than at zero", () => {
    const only = toLimitsByScope({ limits: [{ scope: "global", kind: "awake", value: 3 }] });
    expect(only.global).toEqual({ awake: 3 });
    expect(only.global.day).toBeUndefined();
    expect(only.global.month).toBeUndefined();
  });

  it("shows a cost ceiling in whole dollars, whatever the wire's own micro-dollars are", () => {
    const cents = toLimitsByScope({
      limits: [{ scope: "global", kind: "cost-day", value: 24_400_000 }],
    });
    expect(cents.global.day).toBe(24);
  });

  it("keeps an awake ceiling as the count it is", () => {
    expect(toLimitsByScope(list).global.awake).toBe(18);
  });
});

describe("the two units of money", () => {
  it("converts a whole dollar amount to micro-dollars, and back", () => {
    expect(dollarsToMicros(25)).toBe(25 * MICROS_PER_DOLLAR);
    expect(dollarsToMicros(0.5)).toBe(500_000);
    expect(microsToWholeDollars(25_000_000)).toBe(25);
    expect(microsToWholeDollars(24_400_000)).toBe(24);
  });

  it("keeps the cents for the spend a screen shows", () => {
    expect(microsToDollars(24_400_000)).toBe(24.4);
    expect(microsToDollars(0)).toBe(0);
  });
});

describe("one field as the wire wants it", () => {
  it("names the wire kind behind each field of the form", () => {
    expect(KIND_OF).toEqual({ day: "cost-day", month: "cost-month", awake: "awake" });
  });

  it("sends a cost field as micro-dollars and an awake field as a count", () => {
    expect(wireValueOf("day", 25)).toBe(25_000_000);
    expect(wireValueOf("month", 400)).toBe(400_000_000);
    expect(wireValueOf("awake", 18)).toBe(18);
  });
});
