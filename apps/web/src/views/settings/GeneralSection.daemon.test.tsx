// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the sleep settings are the daemon's).
import { ctx, daemon, resetSleep } from "~/testing/daemon-sleep-store";
import type { SleepSettings } from "@marshal/protocol";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toSleepChoice } from "~/data/mappers/sleep-settings";
import { M } from "~/mock";
import { applySleepChoice } from "~/sync/sleep-settings";
import { SettingsView } from "./SettingsView";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

/*
 * Section S26a: the Sessions panel of Settings against the fake daemon. The daemon holds the sleep
 * settings and the idle timer runs on them, so the panel mirrors what it says rather than the other
 * way round: each of the three selects is one field of the whole record, and a change is one PUT
 * that answers the record the daemon now holds.
 */

const RESTORE = "After a restart";
const IDLE = "Sleep idle cards after";
const CHANNEL = "Sleep warnings go to";
const PUT = "PUT /v1/settings/sleep";

/** Puts one record on the daemon, mirrors it into the store, and draws the panel. */
function showDaemon(sleep: Partial<SleepSettings>): void {
  daemon.sleep.settings = { ...daemon.sleep.settings, ...sleep };
  applySleepChoice(ctx, toSleepChoice(daemon.sleep.settings));
  render(() => <SettingsView />);
}

const change = (label: string, value: string): void => {
  fireEvent.change(screen.getByLabelText(label, { exact: false }), { target: { value } });
};

const toasts = (): string[] => M.S.toasts.map((toast) => toast.msg);

beforeEach(() => {
  resetSleep();
  M.set({ settingsSection: "general", toasts: [], dialog: null });
});
afterEach(cleanup);

describe("the sleep settings on the daemon", () => {
  it("draws the daemon's own record, not the mock's", () => {
    showDaemon({ idleMinutes: 30, restore: "manual", channel: "discord" });
    expect(screen.getByLabelText(RESTORE, { exact: false })).toHaveValue(
      "Show a resume button on each card",
    );
    expect(screen.getByLabelText(IDLE, { exact: false })).toHaveValue("30");
    expect(screen.getByLabelText(CHANNEL)).toHaveValue("In app and Discord");
  });

  it("saves one change as the whole record, and keeps the daemon's answer", async () => {
    showDaemon({});
    change(IDLE, "60");
    await waitFor(() => expect(toasts()).toContain("Saved"));
    expect(daemon.bodies(PUT)).toEqual([
      {
        idleMinutes: 60,
        warningMinutes: 2,
        keepAwakeMinutes: 15,
        restore: "auto",
        channel: "in-app",
      },
    ]);
    expect(M.S.sleep.idle).toBe(60);
    expect(screen.getByLabelText(IDLE, { exact: false })).toHaveValue("60");
  });

  it("sends the two labels as the wire values behind them", async () => {
    showDaemon({});
    change(CHANNEL, "In app and Discord");
    await waitFor(() => expect(toasts()).toContain("Saved"));
    expect(daemon.bodies(PUT).at(-1)).toMatchObject({ channel: "discord" });

    change(RESTORE, "Show a resume button on each card");
    await waitFor(() => expect(daemon.bodies(PUT)).toHaveLength(2));
    expect(daemon.bodies(PUT).at(-1)).toMatchObject({ restore: "manual" });
  });

  it("shows the daemon's own sentence when it refuses, and puts the record back", async () => {
    showDaemon({});
    daemon.refuseNext(
      PUT,
      400,
      "invalid_argument",
      "Choose an idle time of 5, 15, 30, or 60 minutes.",
    );
    change(IDLE, "60");
    await waitFor(() =>
      expect(toasts()).toContain("Choose an idle time of 5, 15, 30, or 60 minutes."),
    );
    // The daemon still holds 15 minutes, so the store and the panel read 15 again.
    expect(M.S.sleep.idle).toBe(15);
    expect(screen.getByLabelText(IDLE, { exact: false })).toHaveValue("15");
  });
});
