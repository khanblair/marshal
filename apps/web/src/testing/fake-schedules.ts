/**
 * The schedule and calendar routes of the fake daemon (docs/backend-checklist.md B8.1, B8.4,
 * sections S30 and S25). `GET /v1/schedules` and the four writes are the Schedules screen's own
 * list-get-save-delete shape; `GET /v1/calendar` is the one call the calendar view and Home's
 * coming-up list read together (N21) - here it answers the same schedules, no due cards (a fake
 * daemon test that needs one seeds it through the card store instead), and no Google Calendar
 * events, since nothing here is ever connected.
 *
 * The list is seeded from the golden `schedule-list` the Go tests wrote, so a screen test reads the
 * shape the real daemon sends.
 */
import type {
  CalendarEvent,
  GoogleCalendarChoice,
  Schedule,
  ScheduleList,
  ScheduleRun,
} from "@marshal/protocol";
import { errorAnswer, type FakeRequest, jsonAnswer } from "~/data/testing/fake-fetch";
import { golden } from "~/data/testing/golden";

const STATUS = { badRequest: 400, notFound: 404 };

const LIST_PATH = "/v1/schedules";
const CALENDAR_PATH = "/v1/calendar";
const ONE_PATH = /^\/v1\/schedules\/([^/]+)$/;
const RUNS_PATH = /^\/v1\/schedules\/([^/]+)\/runs$/;

const KINDS = ["brief", "job"];
const TRIGGERS = ["Cron", "Interval", "One-time", "Event"];

/** The schedules the fake daemon holds. */
export interface ScheduleStore {
  rows: Schedule[];
  /** The number the next schedule's id is made from. Ids are opaque, so no screen reads them. */
  seq: number;
  /** Its clock, as the ISO string `serverTime` is written from. */
  now: () => string;
  /** What Google Calendar answers in `GET /v1/calendar`. Not connected, with no events, by default. */
  google: {
    events: CalendarEvent[];
    connected: boolean;
    error: string;
    stale: boolean;
    /** The calendars the owner has in Google, for the tick list. */
    calendars: GoogleCalendarChoice[];
    /** The owner has chosen calendars, so the ticks are theirs and no longer Google's. */
    chosen: boolean;
    /** A refusal sentence the calendar list answers with instead, as when Google is not connected. */
    refuse: string;
    /** Whether the build has Marshal's own Google client, and whether the person saved their own. */
    client: { bundled: boolean; own: boolean };
  };
}

export interface FakeScheduleOptions {
  /** The schedules it starts with. The golden list by default. */
  schedules?: readonly Schedule[];
  /** Its clock, as an ISO string. The wall clock by default. */
  now?: () => string;
}

export function createScheduleStore(options: FakeScheduleOptions = {}): ScheduleStore {
  return {
    rows: structuredClone([
      ...(options.schedules ?? golden<ScheduleList>("schedule-list").schedules),
    ]),
    seq: 0,
    now: options.now ?? (() => new Date().toISOString()),
    google: {
      events: [],
      connected: false,
      error: "",
      stale: false,
      calendars: [],
      chosen: false,
      refuse: "",
      client: { bundled: false, own: false },
    },
  };
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === "object" && value !== null && !Array.isArray(value);

const asText = (value: unknown): string => (typeof value === "string" ? value : "");

const asDays = (value: unknown): number[] =>
  Array.isArray(value) ? value.filter((item): item is number => typeof item === "number") : [];

function bodyOf(request: FakeRequest): Record<string, unknown> {
  try {
    return request.body ? (JSON.parse(request.body) as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

const refuse = (status: number, code: string, message: string): Response =>
  errorAnswer(status, code, message);

const notFoundSchedule = (): Response =>
  refuse(
    STATUS.notFound,
    "not_found",
    "Marshal cannot find that schedule. It may have been removed.",
  );

/** One request's schedule body, with the defaults the daemon's own decoding gives it, or the
 * refusal its validation would answer instead. */
function readSchedule(id: string, value: unknown): Schedule | Response {
  const body = isRecord(value) ? value : {};
  const name = asText(body.name).trim();
  if (!name) return refuse(STATUS.badRequest, "invalid_argument", "A schedule needs a name.");
  const kind = asText(body.kind);
  if (!KINDS.includes(kind)) {
    return refuse(
      STATUS.badRequest,
      "invalid_argument",
      "A schedule's kind is either brief or job.",
    );
  }
  const trigger = asText(body.trigger);
  if (!TRIGGERS.includes(trigger)) {
    return refuse(
      STATUS.badRequest,
      "invalid_argument",
      "A schedule's trigger is Cron, Interval, One-time, or Event.",
    );
  }
  return {
    id,
    name,
    kind,
    icon: asText(body.icon),
    trigger,
    when: asText(body.when),
    time: asText(body.time),
    days: asDays(body.days),
    action: asText(body.action),
    project: asText(body.project),
    enabled: body.enabled !== false,
    missed: asText(body.missed),
  };
}

function listAnswer(store: ScheduleStore, project: string): Response {
  const rows = project ? store.rows.filter((row) => row.project === project) : store.rows;
  return jsonAnswer({ schedules: rows, serverTime: store.now() });
}

function createSchedule(store: ScheduleStore, request: FakeRequest): Response {
  store.seq += 1;
  const schedule = readSchedule(`schedule-${store.seq}`, bodyOf(request));
  if (schedule instanceof Response) return schedule;
  store.rows.push(schedule);
  return jsonAnswer(schedule);
}

function saveSchedule(store: ScheduleStore, id: string, request: FakeRequest): Response {
  const at = store.rows.findIndex((row) => row.id === id);
  if (at < 0) return notFoundSchedule();
  const schedule = readSchedule(id, bodyOf(request));
  if (schedule instanceof Response) return schedule;
  store.rows[at] = schedule;
  return jsonAnswer(schedule);
}

function deleteSchedule(store: ScheduleStore, id: string): Response {
  const at = store.rows.findIndex((row) => row.id === id);
  if (at < 0) return notFoundSchedule();
  store.rows.splice(at, 1);
  return new Response(null, { status: 204 });
}

/** A schedule's run history: always empty, since nothing here ever fires one. */
function runsAnswer(store: ScheduleStore, id: string): Response {
  if (!store.rows.some((row) => row.id === id)) return notFoundSchedule();
  return jsonAnswer({ runs: [] as ScheduleRun[], serverTime: store.now() });
}

/** Answers one schedule route, or null when the request is not one. */
export function answerScheduleRoute(store: ScheduleStore, request: FakeRequest): Response | null {
  const url = new URL(request.url, "http://fake-daemon");
  const path = url.pathname;
  if (path === LIST_PATH) {
    if (request.method === "GET") return listAnswer(store, url.searchParams.get("project") ?? "");
    if (request.method === "POST") return createSchedule(store, request);
    return null;
  }
  const runs = RUNS_PATH.exec(path);
  if (runs && request.method === "GET") return runsAnswer(store, decodeURIComponent(runs[1] ?? ""));
  const one = ONE_PATH.exec(path);
  if (one) {
    const id = decodeURIComponent(one[1] ?? "");
    if (request.method === "PUT") return saveSchedule(store, id, request);
    if (request.method === "DELETE") return deleteSchedule(store, id);
  }
  return null;
}

const UNPROCESSABLE = 422;
const GOOGLE_CALENDARS_PATH = "/v1/integrations/gcal/calendars";
const GMAIL_AUTHORIZE_PATH = "/v1/integrations/gmail/authorize";
const GCAL_AUTHORIZE_PATH = "/v1/integrations/gcal/authorize";
const GOOGLE_CLIENT_PATH = "/v1/integrations/gcal/client";

/** The owner's Google calendars, as the tick list reads them. */
function calendarChoices(store: ScheduleStore): Response {
  const google = store.google;
  if (google.refuse) return refuse(UNPROCESSABLE, "refused", google.refuse);
  return jsonAnswer({ calendars: google.calendars, chosen: google.chosen });
}

/** Chooses which calendars are read, and answers the list as it now stands. */
function chooseCalendars(store: ScheduleStore, request: FakeRequest): Response {
  const body = bodyOf(request);
  const ids = Array.isArray(body.ids) ? body.ids.filter((id) => typeof id === "string") : [];
  store.google.calendars = store.google.calendars.map((calendar) => ({
    ...calendar,
    selected: ids.includes(calendar.id),
  }));
  store.google.chosen = true;
  return calendarChoices(store);
}

/**
 * Answers GET /v1/calendar, the Google calendar tick list, and Gmail's consent URL, or null when
 * the request is none of them.
 */
export function answerCalendarRoute(store: ScheduleStore, request: FakeRequest): Response | null {
  const path = new URL(request.url, "http://fake-daemon").pathname;
  if (path === GOOGLE_CALENDARS_PATH) {
    if (request.method === "GET") return calendarChoices(store);
    if (request.method === "PUT") return chooseCalendars(store, request);
    return null;
  }
  if (path === GMAIL_AUTHORIZE_PATH && request.method === "GET") {
    return jsonAnswer({ url: "https://accounts.google.com/o/oauth2/auth?fake=gmail" });
  }
  if (path === GCAL_AUTHORIZE_PATH && request.method === "GET") {
    return jsonAnswer({ url: "https://accounts.google.com/o/oauth2/auth?fake=calendar" });
  }
  if (path === GOOGLE_CLIENT_PATH && request.method === "GET") {
    return jsonAnswer(store.google.client);
  }
  if (request.method !== "GET" || path !== CALENDAR_PATH) return null;
  return jsonAnswer({
    schedules: store.rows,
    dueCards: [],
    events: store.google.events,
    googleConnected: store.google.connected,
    googleError: store.google.error,
    googleStale: store.google.stale,
    serverTime: store.now(),
  });
}
