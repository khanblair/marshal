/**
 * The alert settings routes of the fake daemon (docs/backend-checklist.md B9.4, section S26c). It
 * answers the way the daemon does: one read, and a write that changes only the alerts it names,
 * refuses an alert or a channel the screen does not offer, and answers the whole settings.
 *
 * It is seeded from the golden `alert-settings` the Go tests wrote, so a screen test reads the shape
 * the real daemon sends.
 */
import type { AlertSettings, SaveAlertSettingsRequest } from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";

const PATH = "/v1/settings/alerts";
const CHANNELS = ["telegram", "discord", "ntfy"];

export interface AlertsStore {
  settings: AlertSettings;
}

export function createAlertsStore(options: { settings?: AlertSettings } = {}): AlertsStore {
  return { settings: structuredClone(options.settings ?? golden<AlertSettings>("alert-settings")) };
}

function bodyOf(request: FakeRequest): SaveAlertSettingsRequest {
  try {
    return JSON.parse(request.body ?? "{}") as SaveAlertSettingsRequest;
  } catch {
    return { routes: [] };
  }
}

const BAD_REQUEST = 400;
const refuse = (message: string): Response => errorAnswer(BAD_REQUEST, "invalid_argument", message);

function save(store: AlertsStore, request: FakeRequest): Response {
  const choices = bodyOf(request).routes ?? [];
  for (const choice of choices) {
    if (!store.settings.routes.some((route) => route.event === choice.event)) {
      return refuse("Marshal has no alert of that kind.");
    }
    if ((choice.channels ?? []).some((channel) => !CHANNELS.includes(channel))) {
      return refuse("Choose Telegram, Discord, or ntfy.");
    }
  }
  for (const choice of choices) {
    const route = store.settings.routes.find((one) => one.event === choice.event);
    if (route) route.channels = [...new Set(choice.channels ?? [])];
  }
  const quiet = bodyOf(request).quietDuringEvents;
  if (typeof quiet === "boolean") store.settings.quietDuringEvents = quiet;
  return jsonAnswer(store.settings);
}

/** Answers one alert settings route, or null when the request is not one. */
export function answerAlertsRoute(store: AlertsStore, request: FakeRequest): Response | null {
  if (new URL(request.url, "http://fake-daemon").pathname !== PATH) return null;
  if (request.method === "GET") return jsonAnswer(store.settings);
  if (request.method === "PUT") return save(store, request);
  return null;
}
