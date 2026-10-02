import type { AlertSettings } from "@marshal/protocol";
import { createSignal } from "solid-js";
import type { ApiClient } from "~/data/api-client";
import type { Ctx } from "~/mock/context";
import { toast } from "~/mock/engine";
import type { Syncer } from "./syncer";

/*
 * Section S26c: which channel each kind of alert goes to (B9.4). The daemon's notification router
 * holds the choices and follows them at once, so the store only mirrors what it says. The answer is
 * one value that a load and every save both replace, so it is held whole beside the store, the way
 * the pairing code is, rather than spread over its fields.
 */
const [settings, setSettings] = createSignal<AlertSettings | null>(null);

/** The alert settings the daemon last answered, or null before the first answer. */
export function currentAlerts(_ctx: Ctx): AlertSettings | null {
  return settings();
}

export const alertsSyncer: Syncer<AlertSettings> = {
  section: "S26c",
  topics: [],
  async load(api: ApiClient) {
    return api.alertSettings();
  },
  apply(_ctx, answer) {
    setSettings(answer);
  },
};

/** The settings with one alert's channels replaced, for drawing a change before the daemon answers. */
function withChannels(before: AlertSettings, event: string, channels: string[]): AlertSettings {
  return {
    ...before,
    routes: before.routes.map((route) => (route.event === event ? { ...route, channels } : route)),
  };
}

/**
 * Changes where one kind of alert goes. The change is drawn at once and put back, with the daemon's
 * own sentence, if it refuses. What the daemon answers replaces the whole settings, so a screen that
 * missed a change made elsewhere catches up.
 */
export async function saveAlertChannels(
  c: Ctx,
  event: string,
  channels: string[],
): Promise<boolean> {
  const api = c.env.data?.api;
  const before = settings();
  if (!api || !before) return false;
  try {
    const saved = await c.optimistic({
      key: `alert:${event}`,
      apply: () => setSettings(withChannels(before, event, channels)),
      request: () => api.saveAlertSettings({ routes: [{ event, channels }] }),
      rollback: () => setSettings(before),
    });
    setSettings(saved);
    toast(c, "Saved");
    return true;
  } catch {
    return false;
  }
}

/**
 * Turns "stay quiet during calendar events" on or off. While a Google Calendar event is on, alerts
 * that are not asking for an answer wait and go out together when it ends. The change is drawn at
 * once and put back, with the daemon's own sentence, if it refuses.
 */
export async function saveQuietDuringEvents(c: Ctx, on: boolean): Promise<boolean> {
  const api = c.env.data?.api;
  const before = settings();
  if (!api || !before) return false;
  try {
    const saved = await c.optimistic({
      key: "alert:quiet-during-events",
      apply: () => setSettings({ ...before, quietDuringEvents: on }),
      request: () => api.saveAlertSettings({ routes: [], quietDuringEvents: on }),
      rollback: () => setSettings(before),
    });
    setSettings(saved);
    toast(c, "Saved");
    return true;
  } catch {
    return false;
  }
}
