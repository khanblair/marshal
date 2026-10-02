import type { CalendarEvent } from "@marshal/protocol";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sectionStatus } from "~/data/sections";
import { createFakeDaemon, type FakeDaemon } from "~/testing/fake-daemon";
import { PROTOTYPE_PROJECTS } from "~/testing/projects";
import { contextOf, createTestMarshal } from "~/testing/test-store";
import { calendarSyncer } from "./calendar";

const SECTIONS = { ...sectionStatus, S25: "daemon" as const };

let daemon: FakeDaemon | null = null;
afterEach(() => {
  daemon?.data.stop();
  daemon = null;
});

const event = (over: Partial<CalendarEvent> = {}): CalendarEvent => ({
  id: "e1",
  title: "Standup",
  start: "2026-09-30T09:00:00.000Z",
  end: "2026-09-30T09:30:00.000Z",
  allDay: false,
  location: "Room 2",
  url: "https://calendar.google.com/e1",
  joinUrl: "",
  calendar: "Work",
  ...over,
});

async function store() {
  daemon = createFakeDaemon({ projects: PROTOTYPE_PROJECTS });
  const M = createTestMarshal({ data: daemon.data, sections: SECTIONS });
  await daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  return { M, ctx: contextOf(M), d: daemon };
}

describe("the calendar syncer", () => {
  it("says Google is not connected, and shows no events, until it is", async () => {
    const { ctx, d } = await store();
    calendarSyncer.apply(ctx, await calendarSyncer.load(d.data.api, ctx));
    expect(ctx.S.calEvents).toEqual([]);
    expect(ctx.S.calGoogle).toEqual({ known: true, connected: false, error: "", stale: false });
  });

  it("carries an event's end, place, calendar, and link", async () => {
    const { ctx, d } = await store();
    d.schedules.google = {
      ...d.schedules.google,
      events: [event(), event({ id: "e2", title: "Holiday", allDay: true, end: null })],
      connected: true,
      error: "",
      stale: false,
      calendars: [],
      chosen: false,
      refuse: "",
    };
    calendarSyncer.apply(ctx, await calendarSyncer.load(d.data.api, ctx));
    const [standup, holiday] = ctx.S.calEvents;
    expect(standup).toMatchObject({
      title: "Standup",
      location: "Room 2",
      calendar: "Work",
      url: "https://calendar.google.com/e1",
    });
    expect(standup?.endTime).toMatch(/^\d\d:\d\d$/);
    expect(holiday).toMatchObject({ title: "Holiday", allDay: true, time: "" });
    expect(ctx.S.calGoogle).toMatchObject({ known: true, connected: true });
  });

  it("keeps the reason Google's events are old or missing", async () => {
    const { ctx, d } = await store();
    d.schedules.google = {
      ...d.schedules.google,
      events: [event()],
      connected: true,
      error: "Google Calendar could not be reached just now.",
      stale: true,
      calendars: [],
      chosen: false,
      refuse: "",
    };
    calendarSyncer.apply(ctx, await calendarSyncer.load(d.data.api, ctx));
    expect(ctx.S.calGoogle).toEqual({
      known: true,
      connected: true,
      error: "Google Calendar could not be reached just now.",
      stale: true,
    });
    expect(ctx.S.calEvents).toHaveLength(1);
  });
});
