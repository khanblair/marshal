/**
 * The sleep-settings routes of the fake daemon (docs/backend-checklist.md B5.6, section S26a). Both
 * routes answer the whole record, because the Settings screen and the daemon both hold it as one
 * value: `GET /v1/settings/sleep` is what a fresh install sleeps with or what was last saved, and
 * `PUT /v1/settings/sleep` checks the record, stores it, and answers what it stored.
 *
 * The record is seeded from the golden `sleep-settings` the Go tests wrote, so a screen test reads
 * the numbers the real daemon sends: idle after 15 minutes, a 2-minute warning, 15 minutes of Keep
 * awake, automatic restore, and warnings in the app.
 *
 * A value the screen would not offer is refused with the sentence the form shows, word for word the
 * way `settings.checkSleep` refuses it, and nothing is stored: a caller that read it back would
 * otherwise see a number nobody chose.
 */
import {
  type SleepSettings,
  SleepChannelInApp,
  SleepRestoreAuto,
  SleepRestoreManual,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";

const STATUS = { badRequest: 400 };

const SLEEP_PATH = "/v1/settings/sleep";

/** The idle times the daemon accepts, in minutes (`settings.idleChoices`). */
const IDLE_CHOICES: readonly number[] = [5, 15, 30, 60];
/** The two answers to "After a restart" (`settings.restoreChoices`). */
const RESTORE_CHOICES: readonly string[] = [SleepRestoreAuto, SleepRestoreManual];
/** Where a sleep warning can go, in the daemon's order (`settings.channelChoices`). */
const CHANNEL_CHOICES: readonly string[] = [SleepChannelInApp, "telegram", "discord"];

/** The sleep settings, as the fake daemon holds them. */
export interface SleepStore {
  /** The whole record. One save replaces it. */
  settings: SleepSettings;
}

export interface FakeSleepOptions {
  /** The record it starts with. The golden `sleep-settings` by default. */
  settings?: SleepSettings;
}

export function createSleepStore(options: FakeSleepOptions = {}): SleepStore {
  return { settings: structuredClone(options.settings ?? golden<SleepSettings>("sleep-settings")) };
}

/** Answers one sleep-settings route, or null when the request is not one. */
export function answerSleepRoute(store: SleepStore, request: FakeRequest): Response | null {
  const path = request.url.replace(/^https?:\/\/[^/]+/, "");
  if (path !== SLEEP_PATH) return null;
  if (request.method === "GET") return jsonAnswer(store.settings);
  if (request.method === "PUT") return save(store, request);
  return null;
}

function bodyOf(request: FakeRequest): Record<string, unknown> {
  try {
    return request.body ? (JSON.parse(request.body) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

const asCount = (value: unknown): number =>
  typeof value === "number" && Number.isFinite(value) ? Math.trunc(value) : 0;

const asText = (value: unknown): string => (typeof value === "string" ? value : "");

/** The daemon's own sentence for a record it will not store (`settings.checkSleep`). */
function refusal(incoming: SleepSettings): Response | null {
  if (!IDLE_CHOICES.includes(incoming.idleMinutes)) {
    return refuse("Choose an idle time of 5, 15, 30, or 60 minutes.");
  }
  if (incoming.warningMinutes < 1) return refuse("The sleep warning must be at least a minute.");
  if (incoming.warningMinutes >= incoming.idleMinutes) {
    return refuse("The sleep warning must be shorter than the idle time.");
  }
  if (incoming.keepAwakeMinutes < 1) return refuse("Keep awake must be at least a minute.");
  if (!RESTORE_CHOICES.includes(incoming.restore)) {
    return refuse("Choose whether cards are restored on startup or resumed by hand.");
  }
  if (!CHANNEL_CHOICES.includes(incoming.channel)) return refuse("Choose where sleep warnings go.");
  return null;
}

const refuse = (message: string): Response =>
  errorAnswer(STATUS.badRequest, "invalid_argument", message);

function save(store: SleepStore, request: FakeRequest): Response {
  const body = bodyOf(request);
  const incoming: SleepSettings = {
    idleMinutes: asCount(body.idleMinutes),
    warningMinutes: asCount(body.warningMinutes),
    keepAwakeMinutes: asCount(body.keepAwakeMinutes),
    restore: asText(body.restore),
    channel: asText(body.channel),
  };
  const bad = refusal(incoming);
  if (bad) return bad;
  store.settings = incoming;
  return jsonAnswer(store.settings);
}
