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
  ScheduleCatalog,
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
const RUN_PATH = /^\/v1\/schedules\/([^/]+)\/run$/;
const PREVIEW_PATH = /^\/v1\/schedules\/([^/]+)\/preview$/;
const CATALOG_PATH = "/v1/schedules/catalog";

const KINDS = ["brief", "job"];
const TRIGGERS = ["Cron", "Interval", "One-time", "Event"];

const MONDAY = 1;
const FRIDAY = 5;
const WEEKDAYS = Array.from({ length: FRIDAY - MONDAY + 1 }, (_, i) => MONDAY + i);

/**
 * What the real daemon's catalog lists, trimmed to three starters. The parts and chats are the real
 * ones, so an editor test sees what a person sees.
 */
export const FAKE_CATALOG: ScheduleCatalog = {
  templates: [
    {
      key: "morning",
      name: "Morning brief",
      summary: "Today's calendar and what waits on you.",
      icon: "sunrise",
      trigger: "Cron",
      when: "Every weekday at 8:00",
      time: "08:00",
      days: WEEKDAYS,
      missed: "Run once on wake",
      sections: ["calendar", "needs-you", "working"],
      quietWhenEmpty: false,
    },
    {
      key: "wind-down",
      name: "Evening wind-down",
      summary: "What finished and what waits tomorrow.",
      icon: "sunset",
      trigger: "Cron",
      when: "Every weekday at 18:00",
      time: "18:00",
      days: WEEKDAYS,
      missed: "Skip",
      sections: ["finished", "needs-you", "calendar"],
      quietWhenEmpty: false,
    },
    {
      key: "stale",
      name: "Stale cards",
      summary: "Cards that have not moved in a week.",
      icon: "archive",
      trigger: "Cron",
      when: "Every Monday at 9:00",
      time: "09:00",
      days: [MONDAY],
      missed: "Skip",
      sections: ["stale"],
      quietWhenEmpty: true,
    },
  ],
  sections: [
    { id: "calendar", label: "Calendar", hint: "Your Google Calendar events." },
    { id: "needs-you", label: "Needs you", hint: "Cards waiting on you." },
    { id: "working", label: "Still working", hint: "Cards an agent is working on." },
    { id: "in-review", label: "In review", hint: "Cards in review, with their pull requests." },
    { id: "finished", label: "Finished", hint: "Cards that finished since the last brief." },
    { id: "card-ci", label: "Card CI failures", hint: "Cards whose CI failed." },
    {
      id: "main-ci",
      label: "Main branch CI",
      hint: "Whether each project's main branch is passing.",
    },
    { id: "stale", label: "Stale cards", hint: "Cards that have not moved in a week." },
  ],
  channels: [
    { id: "telegram", label: "Telegram" },
    { id: "discord", label: "Discord" },
    { id: "ntfy", label: "ntfy" },
  ],
  serverTime: "2026-09-28T12:00:00.000Z",
};

/** The schedules the fake daemon holds. */
export interface ScheduleStore {
  rows: Schedule[];
  /** What the editor offers. */
  catalog: ScheduleCatalog;
  /** The runs of each schedule, newest first. Only a run made by hand puts one here. */
  runs: Record<string, ScheduleRun[]>;
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
  /** What the catalog route answers. {@link FAKE_CATALOG} by default. */
  catalog?: ScheduleCatalog;
  /** Its clock, as an ISO string. The wall clock by default. */
  now?: () => string;
}

export function createScheduleStore(options: FakeScheduleOptions = {}): ScheduleStore {
  return {
    rows: structuredClone([
      ...(options.schedules ?? golden<ScheduleList>("schedule-list").schedules),
    ]),
    catalog: options.catalog ?? FAKE_CATALOG,
    runs: {},
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

const asTexts = (value: unknown): string[] =>
  Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];

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
function readSchedule(store: ScheduleStore, id: string, value: unknown): Schedule | Response {
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
  const refused = refuseUnknownParts(store, body);
  if (refused) return refused;
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
    template: asText(body.template),
    sections: asTexts(body.sections),
    deliver: asTexts(body.deliver),
    quietWhenEmpty: body.quietWhenEmpty === true,
  };
}

/** The daemon's own checks of the starter, the parts, and the chats a schedule names. */
function refuseUnknownParts(store: ScheduleStore, body: Record<string, unknown>): Response | null {
  const { templates, sections, channels } = store.catalog;
  const template = asText(body.template);
  if (template && !templates.some((one) => one.key === template)) {
    return refuse(STATUS.badRequest, "invalid_argument", "That starter schedule does not exist.");
  }
  if (!asTexts(body.sections).every((id) => sections.some((one) => one.id === id))) {
    return refuse(STATUS.badRequest, "invalid_argument", "A brief has no such part.");
  }
  if (!asTexts(body.deliver).every((id) => channels.some((one) => one.id === id))) {
    return refuse(STATUS.badRequest, "invalid_argument", "A brief cannot be sent to that chat.");
  }
  return null;
}

function listAnswer(store: ScheduleStore, project: string): Response {
  const rows = project ? store.rows.filter((row) => row.project === project) : store.rows;
  return jsonAnswer({ schedules: rows, serverTime: store.now() });
}

function createSchedule(store: ScheduleStore, request: FakeRequest): Response {
  store.seq += 1;
  const schedule = readSchedule(store, `schedule-${store.seq}`, bodyOf(request));
  if (schedule instanceof Response) return schedule;
  store.rows.push(schedule);
  return jsonAnswer(schedule);
}

function saveSchedule(store: ScheduleStore, id: string, request: FakeRequest): Response {
  const at = store.rows.findIndex((row) => row.id === id);
  if (at < 0) return notFoundSchedule();
  const schedule = readSchedule(store, id, bodyOf(request));
  if (schedule instanceof Response) return schedule;
  // The starter a schedule began as does not change when it is edited.
  schedule.template = store.rows[at]?.template ?? "";
  store.rows[at] = schedule;
  return jsonAnswer(schedule);
}

function deleteSchedule(store: ScheduleStore, id: string): Response {
  const at = store.rows.findIndex((row) => row.id === id);
  if (at < 0) return notFoundSchedule();
  store.rows.splice(at, 1);
  return new Response(null, { status: 204 });
}

/** A schedule's run history: empty until one is run by hand, since nothing here fires one on its own. */
function runsAnswer(store: ScheduleStore, id: string): Response {
  if (!store.rows.some((row) => row.id === id)) return notFoundSchedule();
  return jsonAnswer({ runs: store.runs[id] ?? [], serverTime: store.now() });
}

/** What a chat would get from a brief: its name, a blank line, and a line for each part it names. */
function previewAnswer(store: ScheduleStore, id: string): Response {
  const row = store.rows.find((one) => one.id === id);
  if (!row) return notFoundSchedule();
  if (row.kind !== "brief") {
    return refuse(
      UNPROCESSABLE,
      "refused",
      "Only a brief can be previewed. This kind of schedule cannot run yet.",
    );
  }
  const parts = row.sections.map(
    (part) => store.catalog.sections.find((one) => one.id === part)?.label ?? part,
  );
  return jsonAnswer({ text: [row.name, "", ...parts].join("\n"), serverTime: store.now() });
}

/** Runs a schedule now: the run is a success that says it was run by hand. */
function runAnswer(store: ScheduleStore, id: string): Response {
  if (!store.rows.some((row) => row.id === id)) return notFoundSchedule();
  const kept = store.runs[id] ?? [];
  const run: ScheduleRun = {
    id: `run-${id}-${kept.length + 1}`,
    runAt: store.now(),
    status: "success",
    details:
      "Run by hand.\n# Morning brief\n\n## Finished\nNothing finished.\n\n---\nSent to Telegram.",
  };
  store.runs[id] = [run, ...kept];
  return jsonAnswer(run);
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
  if (path === CATALOG_PATH && request.method === "GET") return jsonAnswer(store.catalog);
  const runs = RUNS_PATH.exec(path);
  if (runs && request.method === "GET") return runsAnswer(store, decodeURIComponent(runs[1] ?? ""));
  const preview = PREVIEW_PATH.exec(path);
  if (preview && request.method === "GET") {
    return previewAnswer(store, decodeURIComponent(preview[1] ?? ""));
  }
  const runNow = RUN_PATH.exec(path);
  if (runNow && request.method === "POST")
    return runAnswer(store, decodeURIComponent(runNow[1] ?? ""));
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
