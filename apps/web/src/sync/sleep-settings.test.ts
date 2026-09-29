import type { SleepSettings } from "@marshal/protocol";
import { afterEach, describe, expect, it } from "vitest";
import type { ApiClient } from "~/data/api-client";
import { toSleepChoice } from "~/data/mappers/sleep-settings";
import { golden } from "~/data/testing/golden";
import type { Marshal } from "~/mock";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createSyncedMarshal, createTestMarshal } from "~/testing/test-store";
import { applySleepChoice, sleepSettingsSyncer } from "./sleep-settings";

// Section S26a: the numbers and choices behind automatic sleep. The daemon holds them in the
// `settings` table and the idle timer runs on them, so the store mirrors what it says. There are no
// sleep-setting events: the route answers the whole record, and a load and a save both arrive
// through `applySleepChoice`.

const wire = golden<SleepSettings>("sleep-settings");
const choice = toSleepChoice(wire);

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

async function setup(): Promise<{ M: Marshal; d: FakeDaemon }> {
  const d = createFakeDaemon({ projects: PROTOTYPE_PROJECTS });
  daemon = d;
  return { M: await createSyncedMarshal(d), d };
}

const toasts = (M: Marshal): string[] => M.S.toasts.map((toast) => toast.msg);

describe("the sleep settings section", () => {
  it("is section S26a, follows no topic, and loads the record the daemon holds", async () => {
    expect(sleepSettingsSyncer.section).toBe("S26a");
    expect(sleepSettingsSyncer.topics).toEqual([]);
    const asked: string[] = [];
    const api = {
      sleepSettings: async () => {
        asked.push("sleepSettings");
        return wire;
      },
    } as unknown as ApiClient;
    expect(await sleepSettingsSyncer.load(api, contextOf(createTestMarshal()))).toEqual(choice);
    expect(asked).toEqual(["sleepSettings"]);
  });

  it("puts the daemon's record in the store, replacing what it had", () => {
    const ctx = contextOf(createTestMarshal());
    ctx.S.sleep = {
      idle: 60,
      warn: 5,
      keepAwake: 30,
      channel: "In app and Discord",
      restore: "manual",
    };
    applySleepChoice(ctx, choice);
    expect(ctx.S.sleep).toEqual(choice);
  });
});

describe("saving the sleep settings on the daemon", () => {
  it("sends the whole record as one write and keeps the daemon's own answer", async () => {
    const { M, d } = await setup();
    expect(await M.saveSleepSettings({ ...choice, idle: 30 })).toBe(true);
    expect(d.bodies("PUT /v1/settings/sleep")).toEqual([
      {
        idleMinutes: 30,
        warningMinutes: 2,
        keepAwakeMinutes: 15,
        restore: "auto",
        channel: "in-app",
      },
    ]);
    expect(d.routes()).toContain("PUT /v1/settings/sleep");
    expect(M.S.sleep.idle).toBe(30);
    expect(toasts(M)).toEqual(["Saved"]);
  });

  it("turns the two labels back into the wire values, so one save sends what the form shows", async () => {
    const { M, d } = await setup();
    await M.saveSleepSettings({ ...choice, channel: "In app and Discord", restore: "manual" });
    expect(d.bodies("PUT /v1/settings/sleep").at(-1)).toMatchObject({
      channel: "discord",
      restore: "manual",
    });
  });

  it("shows the daemon's own sentence and puts the store back when it refuses", async () => {
    const { M, d } = await setup();
    // The store starts on the daemon's own record, which is what a refusal must leave it on.
    M.S.sleep = { ...choice };
    d.refuseNext(
      "PUT /v1/settings/sleep",
      400,
      "invalid_argument",
      "Choose an idle time of 5, 15, 30, or 60 minutes.",
    );
    expect(await M.saveSleepSettings({ ...choice, idle: 7 })).toBe(false);
    expect(toasts(M)).toEqual(["Choose an idle time of 5, 15, 30, or 60 minutes."]);
    expect(M.S.sleep).toEqual(choice);
  });
});
