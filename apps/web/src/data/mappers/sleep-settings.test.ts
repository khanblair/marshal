import type { SleepSettings } from "@marshal/protocol";
import { describe, expect, it } from "vitest";
import { golden } from "../testing/golden";
import {
  CHANNEL_LABELS,
  IDLE_CHOICE_MINUTES,
  RESTORE_LABELS,
  toSleepChoice,
  toWireSleepSettings,
} from "./sleep-settings";

/* The record the real daemon answers with, written by its own Go tests, so the two sides cannot drift. */
const wire = golden<SleepSettings>("sleep-settings");

describe("toSleepChoice", () => {
  it("maps the golden record, with the two closed sets as the labels the form shows", () => {
    expect(toSleepChoice(wire)).toEqual({
      idle: 15,
      warn: 2,
      keepAwake: 15,
      channel: "In app only",
      restore: "Auto-restore on startup",
    });
  });

  it("keeps a channel or a restore answer this build does not know as it came", () => {
    const odd = { ...wire, channel: "matrix", restore: "later" } as SleepSettings;
    expect(toSleepChoice(odd)).toMatchObject({ channel: "matrix", restore: "later" });
  });
});

describe("toWireSleepSettings", () => {
  it("turns a choice back into the record the daemon sent", () => {
    expect(toWireSleepSettings(toSleepChoice(wire))).toEqual(wire);
  });

  it("sends a label this build does not know straight back as its own value", () => {
    expect(toWireSleepSettings({ ...toSleepChoice(wire), channel: "matrix" })).toMatchObject({
      channel: "matrix",
    });
  });

  it("carries a field the form never shows, so one save does not drop it", () => {
    const choice = { ...toSleepChoice(wire), keepAwake: 30 };
    expect(toWireSleepSettings(choice).keepAwakeMinutes).toBe(30);
  });
});

describe("the two closed sets the form draws", () => {
  it("offers the idle times the daemon accepts, in order", () => {
    expect(IDLE_CHOICE_MINUTES).toEqual([5, 15, 30, 60]);
  });

  it("names a label for each wire value, and nothing the daemon would refuse", () => {
    expect(RESTORE_LABELS).toEqual({
      auto: "Auto-restore on startup",
      manual: "Show a resume button on each card",
    });
    expect(CHANNEL_LABELS).toEqual({
      "in-app": "In app only",
      telegram: "In app and Telegram",
      discord: "In app and Discord",
    });
  });
});
