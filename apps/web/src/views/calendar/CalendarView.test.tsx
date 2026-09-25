import { fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type CalEvent, type Card, M, type Schedule } from "~/mock";
import { cardLabel } from "~/mock/card-key";
import { CalendarView } from "./CalendarView";
import { fullDate, monthLabel, shiftCursor, weekdayLabels } from "./calendar-dates";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const seed = {
  schedules: structuredClone(JSON.parse(JSON.stringify(M.S.schedules))) as Schedule[],
  events: structuredClone(JSON.parse(JSON.stringify(M.S.calEvents))) as CalEvent[],
  cards: structuredClone(JSON.parse(JSON.stringify(M.S.cards))) as Card[],
};
const DESKTOP_W = 1440;
const PHONE_W = 390;
const EXTRA_EVENTS = 5;

const heading = () => screen.getByRole("heading", { level: 2 }).textContent;
const dayNumbers = () => [...document.querySelectorAll<HTMLElement>("span[title]")];
const dayOf = (t: number) => {
  const el = dayNumbers().find((s) => s.title === fullDate(t));
  if (!el) throw new Error("day not on the grid");
  return el;
};
const cellOf = (t: number) => dayOf(t).parentElement as HTMLElement;
/** Puts more events on today than a month cell shows. */
const crowdToday = () => {
  for (let i = 0; i < EXTRA_EVENTS; i++) {
    M.S.calEvents.push({ id: `x${i}`, title: `Extra ${i}`, time: `0${i}:00`, dayOffset: 0 });
  }
};

beforeEach(() => {
  M.S.schedules = structuredClone(seed.schedules);
  M.S.calEvents = structuredClone(seed.events);
  M.S.cards = structuredClone(seed.cards);
  M.clearFilters();
  M.go("project", "web", "calendar");
  M.set({
    vw: DESKTOP_W,
    calMode: "month",
    calCursor: M.T0,
    calExpand: null,
    dialog: null,
    openId: null,
  });
});
afterEach(() => {
  vi.clearAllTimers();
});

describe("CalendarView toolbar", () => {
  it("names the month and offers the layout tabs, Month selected", () => {
    render(() => <CalendarView />);
    expect(heading()).toBe(monthLabel(M.T0));
    expect(screen.getByRole("tablist", { name: "Calendar layout" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Month" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Week" })).toHaveAttribute("aria-selected", "false");
  });

  it("shows the legend", () => {
    render(() => <CalendarView />);
    for (const label of ["Scheduled job", "Brief", "Due card", "Calendar event"]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
  });

  it("moves to the next and previous month and back to today", () => {
    render(() => <CalendarView />);
    fireEvent.click(screen.getByRole("button", { name: "Next month" }));
    expect(heading()).toBe(monthLabel(shiftCursor("month", M.T0, 1)));
    fireEvent.click(screen.getByRole("button", { name: "Previous month" }));
    fireEvent.click(screen.getByRole("button", { name: "Previous month" }));
    expect(heading()).toBe(monthLabel(shiftCursor("month", M.T0, -1)));
    fireEvent.click(screen.getByRole("button", { name: "Today" }));
    expect(heading()).toBe(monthLabel(M.T0));
  });

  it("switches to the week layout, with week arrows and a Week of heading", () => {
    render(() => <CalendarView />);
    fireEvent.click(screen.getByRole("tab", { name: "Week" }));
    expect(M.S.calMode).toBe("week");
    expect(heading()).toMatch(/^Week of /);
    expect(screen.getByRole("tab", { name: "Week" })).toHaveAttribute("aria-selected", "true");
    fireEvent.click(screen.getByRole("button", { name: "Next week" }));
    expect(M.S.calCursor).toBe(shiftCursor("week", M.T0, 1));
    expect(screen.getByRole("button", { name: "Previous week" })).toBeInTheDocument();
  });

  it("forgets an expanded day when the month changes", () => {
    render(() => <CalendarView />);
    M.set({ calExpand: String(M.T0) });
    fireEvent.click(screen.getByRole("button", { name: "Next month" }));
    expect(M.S.calExpand).toBeNull();
  });
});

describe("CalendarView month grid", () => {
  it("draws six weeks under the seven weekday headings", () => {
    render(() => <CalendarView />);
    expect(dayNumbers()).toHaveLength(42);
    for (const w of weekdayLabels("month", M.T0, false)) {
      expect(screen.getAllByText(w).length).toBeGreaterThan(0);
    }
  });

  it("marks today with the ink circle and dims days of other months", () => {
    render(() => <CalendarView />);
    expect(dayOf(M.T0)).toHaveClass("bg-ink", "font-bold");
    const outside = dayNumbers().find((s) =>
      s.parentElement?.classList.contains("bg-surface-sunken"),
    );
    expect(outside).toHaveClass("text-muted");
    expect(cellOf(M.T0)).toHaveClass("min-h-28");
  });

  it("shows a Google Calendar event, and asks before closing it", () => {
    render(() => <CalendarView />);
    const button = screen.getByTitle("Design review at 14:00, from Google Calendar");
    expect(within(button).getByText("14:00")).toBeInTheDocument();
    fireEvent.click(button);
    expect(M.S.dialog?.title).toBe("Design review");
    expect(M.S.dialog?.action).toBe("Close");
    expect(M.S.dialog?.message).toContain("Synced from Google Calendar.");
  });

  it("opens a schedule's dialog, and Edit schedule goes to Settings", () => {
    render(() => <CalendarView />);
    fireEvent.click(screen.getAllByTitle(/^Morning brief\. /)[0] as HTMLElement);
    expect(M.S.dialog?.title).toBe("Morning brief");
    expect(M.S.dialog?.action).toBe("Edit schedule");
    expect(M.S.dialog?.message).toContain("Missed runs: run once on wake.");
    M.S.dialog?.run();
    expect(M.S.route.page).toBe("settings");
    expect(M.S.settingsSection).toBe("schedules");
    expect(M.S.schedEdit).toBe("s1");
  });

  it("shows a due card on its day and opens it", () => {
    const due = M.cardsOf("web").find((c) => c.due === 0) as Card;
    M.set({ calExpand: String(M.T0) });
    render(() => <CalendarView />);
    const button = within(cellOf(M.T0)).getByTitle(`${cardLabel(due)} ${due.title} is due`);
    expect(button).toHaveClass("bg-surface-sunken");
    expect(within(button).queryByText(/^\d\d:\d\d$/)).toBeNull();
    fireEvent.click(button);
    expect(M.S.openId).toBe(due.id);
  });

  it("shows only the open project's schedules", () => {
    render(() => <CalendarView />);
    expect(screen.queryByTitle(/^Dependency updates\. /)).toBeNull();
    expect(screen.getAllByTitle(/^Check open issues\. /).length).toBeGreaterThan(0);
  });

  it("hides a schedule that is turned off", () => {
    (M.S.schedules.find((s) => s.id === "s1") as Schedule).enabled = false;
    render(() => <CalendarView />);
    expect(screen.queryByTitle(/^Morning brief\. /)).toBeNull();
  });

  it("shows an interval job on the three days from today", () => {
    M.go("project", "mobile", "calendar");
    M.S.schedules = M.S.schedules.filter((s) => s.id === "s5");
    M.S.calEvents = [];
    render(() => <CalendarView />);
    const loops = screen.getAllByTitle(/^Fix until e2e passes\. /);
    expect(loops).toHaveLength(3);
    expect(
      within(loops[0] as HTMLElement).getByText("Fix until e2e passes, every 30 min"),
    ).toBeInTheDocument();
  });

  it("cuts a crowded day at four items and expands it from the N more button", () => {
    crowdToday();
    render(() => <CalendarView />);
    const cell = cellOf(M.T0);
    const shown = () => within(cell).getAllByRole("button").length;
    const more = within(cell).getByRole("button", { name: /^\d+ more$/ });
    expect(shown() - 1).toBe(4);
    const hidden = Number(more.textContent?.split(" ")[0]);
    fireEvent.click(more);
    expect(M.S.calExpand).toBe(String(M.T0));
    expect(within(cell).queryByRole("button", { name: /more$/ })).toBeNull();
    expect(shown()).toBe(4 + hidden);
  });
});

describe("CalendarView week grid", () => {
  beforeEach(() => {
    M.set({ calMode: "week" });
  });

  it("draws seven tall days with the date in each heading", () => {
    render(() => <CalendarView />);
    expect(dayNumbers()).toHaveLength(7);
    expect(cellOf(M.T0)).toHaveClass("min-h-105");
    for (const w of weekdayLabels("week", M.T0, false)) {
      expect(screen.getByText(w)).toBeInTheDocument();
    }
  });

  it("shows every item of a crowded day, with no N more button", () => {
    crowdToday();
    render(() => <CalendarView />);
    expect(screen.queryByRole("button", { name: /more$/ })).toBeNull();
    expect(within(cellOf(M.T0)).getAllByTitle(/^Extra \d at /)).toHaveLength(EXTRA_EVENTS);
  });
});

describe("CalendarView on a phone", () => {
  beforeEach(() => {
    M.set({ vw: PHONE_W });
  });

  it("shows the month agenda instead of the grid, without layout tabs", () => {
    render(() => <CalendarView />);
    expect(screen.queryByRole("tablist")).toBeNull();
    expect(dayNumbers()).toHaveLength(0);
    expect(heading()).toBe(monthLabel(M.T0));
    expect(screen.getAllByRole("heading", { level: 3 }).length).toBeGreaterThan(5);
  });

  it("puts Today in front of today's heading and shows each time, or Due", () => {
    crowdToday();
    render(() => <CalendarView />);
    const today = screen.getByRole("heading", { level: 3, name: /^Today, / });
    const section = today.parentElement as HTMLElement;
    expect(within(section).getByText("Extra 0")).toBeInTheDocument();
    expect(within(section).getByText("01:00")).toBeInTheDocument();
    const due = M.cardsOf("web").find((c) => c.due === 0) as Card;
    expect(within(section).getByText("Due")).toBeInTheDocument();
    fireEvent.click(within(section).getByText(`${cardLabel(due)} ${due.title}`));
    expect(M.S.openId).toBe(due.id);
  });

  it("opens an event's dialog from the agenda", () => {
    M.S.calEvents.push({ id: "solo", title: "Solo event", time: "10:00", dayOffset: 0 });
    render(() => <CalendarView />);
    fireEvent.click(screen.getByText("Solo event"));
    expect(M.S.dialog?.title).toBe("Solo event");
  });

  it("says when nothing is scheduled", () => {
    M.S.schedules = [];
    M.S.calEvents = [];
    M.set({ calCursor: new Date(new Date(M.T0).getFullYear() + 1, 0, 1).getTime() });
    render(() => <CalendarView />);
    expect(screen.getByText("Nothing is scheduled this month.")).toBeInTheDocument();
    expect(screen.queryAllByRole("heading", { level: 3 })).toHaveLength(0);
  });
});
