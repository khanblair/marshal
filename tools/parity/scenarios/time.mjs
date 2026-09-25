/** Timeline and Calendar scenarios (prefix `time-`). */
import { settle } from "../lib.mjs";
import { go, project, set } from "../steps.mjs";

const PROJECTS = ["api", "web", "mobile"];
/** Days the pointer moves for a drag: 3 days is 120 px at 40 px a day. */
const THREE_DAYS_PX = 120;
const TWO_DAYS_PX = 80;
const DRAG_STEPS = 6;
const SCROLL_LEFT_PX = 300;
const SCROLL_TOP_PX = 60;

/** Opens a project view and lets the prototype draw it (its clock is paused). */
async function open(page, pid, view) {
  await go(page, "project", pid, view);
  await settle(page);
}

const barOf = (page, id) => page.locator(`[data-card="${id}"] + div [role="button"]`);

/** Presses on a bar and moves the pointer `dx` px; releases only when asked. */
async function dragBar(page, id, dx, { release }) {
  const bar = barOf(page, id);
  await bar.scrollIntoViewIfNeeded();
  const box = await bar.boundingBox();
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.mouse.move(x + dx, y, { steps: DRAG_STEPS });
  if (release) await page.mouse.up();
}

/**
 * Scrolls the nearest scrolling ancestor of a row, as a user would in a wide grid. The
 * prototype may draw its views in a shadow root, so the row is found with a locator.
 */
const scrollGrid = (page, left, top) =>
  page
    .locator("[data-card]")
    .first()
    .evaluate(
      (row, [l, t]) => {
        let el = row;
        while (el && !/(auto|scroll)/.test(getComputedStyle(el).overflowX)) el = el.parentElement;
        if (el) el.scrollTo(l, t);
      },
      [left, top],
    );

const calendar = (pid, mode) => async (page) => {
  await open(page, pid, "calendar");
  if (mode) {
    await set(page, { calMode: mode });
    await settle(page);
  }
};

const timelineScenarios = PROJECTS.map((pid) => ({
  id: `time-timeline-${pid}`,
  steps: project(pid, "timeline"),
}));

const calendarScenarios = PROJECTS.flatMap((pid) => [
  { id: `time-calendar-month-${pid}`, steps: calendar(pid) },
  { id: `time-calendar-week-${pid}`, steps: calendar(pid, "week") },
]);

export default [
  ...timelineScenarios,
  {
    id: "time-timeline-filtered",
    steps: async (page) => {
      await open(page, "api", "timeline");
      await page.evaluate(() => window.M.addFilter("status", "working"));
    },
  },
  {
    id: "time-timeline-empty",
    steps: async (page) => {
      await open(page, "api", "timeline");
      await page.evaluate(() => window.M.addFilter("label", "no-such-label"));
    },
  },
  {
    // Card 45 waits for #36; starting it 3 days before today breaks that line and the phone chip.
    id: "time-timeline-broken",
    steps: async (page) => {
      await open(page, "api", "timeline");
      await page.evaluate(() => {
        const c = window.M.card(45);
        c.s = -3;
        window.M.emit();
      });
    },
  },
  {
    id: "time-timeline-scrolled",
    sizes: ["desktop", "tablet"],
    steps: async (page) => {
      await open(page, "api", "timeline");
      await scrollGrid(page, SCROLL_LEFT_PX, SCROLL_TOP_PX);
    },
  },
  {
    id: "time-timeline-row-hover",
    sizes: ["desktop"],
    steps: async (page) => {
      await open(page, "api", "timeline");
      await page.locator('[data-card="46"]').hover();
    },
  },
  {
    id: "time-timeline-row-focus",
    sizes: ["desktop"],
    steps: async (page) => {
      await open(page, "api", "timeline");
      await page.keyboard.press("Shift");
      await page.locator('[data-card="46"]').focus();
    },
  },
  {
    // Bar in flight: shadow, shifted position, and the line follows it.
    id: "time-timeline-drag-hold",
    sizes: ["desktop"],
    steps: async (page) => {
      await open(page, "api", "timeline");
      await dragBar(page, 46, -THREE_DAYS_PX, { release: false });
    },
  },
  {
    // Card 46 waits for #39: moving it 3 days earlier asks before breaking that dependency.
    id: "time-timeline-drag-break",
    sizes: ["desktop"],
    steps: async (page) => {
      await open(page, "api", "timeline");
      await dragBar(page, 46, -THREE_DAYS_PX, { release: true });
    },
  },
  {
    id: "time-timeline-drag-break-confirm",
    sizes: ["desktop"],
    steps: async (page) => {
      await open(page, "api", "timeline");
      await dragBar(page, 46, -THREE_DAYS_PX, { release: true });
      await settle(page);
      await page.getByRole("button", { name: "Move anyway" }).click();
    },
  },
  {
    // Card 45 has nothing else waiting on it after #36: a plain move, then the toast.
    id: "time-timeline-drag-drop",
    sizes: ["desktop"],
    steps: async (page) => {
      await open(page, "api", "timeline");
      await dragBar(page, 45, TWO_DAYS_PX, { release: true });
    },
  },
  {
    id: "time-timeline-bar-click",
    sizes: ["desktop"],
    steps: async (page) => {
      await open(page, "api", "timeline");
      await barOf(page, 46).click();
    },
  },
  ...calendarScenarios,
  {
    id: "time-calendar-next",
    steps: async (page) => {
      await calendar("web")(page);
      await page.getByRole("button", { name: "Next month" }).click();
    },
  },
  {
    id: "time-calendar-prev-today",
    steps: async (page) => {
      await calendar("web")(page);
      await page.getByRole("button", { name: "Previous month" }).click();
      await page.getByRole("button", { name: "Previous month" }).click();
      await page.getByRole("button", { name: "Today" }).click();
    },
  },
  {
    id: "time-calendar-week-next",
    sizes: ["desktop", "tablet"],
    steps: async (page) => {
      await calendar("web", "week")(page);
      await page.getByRole("button", { name: "Next week" }).click();
    },
  },
  {
    id: "time-calendar-tab-week",
    sizes: ["desktop", "tablet"],
    steps: async (page) => {
      await calendar("api")(page);
      await page.getByRole("tab", { name: "Week" }).click();
    },
  },
  {
    id: "time-calendar-expanded",
    sizes: ["desktop", "tablet"],
    steps: async (page) => {
      await calendar("web")(page);
      await page
        .getByRole("button", { name: /^\d+ more$/ })
        .first()
        .click();
    },
  },
  {
    id: "time-calendar-expanded-mobile",
    sizes: ["desktop", "tablet"],
    steps: async (page) => {
      await calendar("mobile")(page);
      await page
        .getByRole("button", { name: /^\d+ more$/ })
        .first()
        .click();
    },
  },
  {
    id: "time-calendar-item-hover",
    sizes: ["desktop"],
    steps: async (page) => {
      await calendar("web")(page);
      await page.getByTitle("Design review at 14:00, from Google Calendar").hover();
    },
  },
  {
    id: "time-calendar-dialog-event",
    steps: async (page) => {
      await calendar("web")(page);
      await page.getByText("Design review").first().click();
    },
  },
  {
    id: "time-calendar-dialog-schedule",
    steps: async (page) => {
      await calendar("web")(page);
      await page.getByText("Morning brief").first().click();
    },
  },
  {
    id: "time-calendar-dialog-schedule-edit",
    steps: async (page) => {
      await calendar("web")(page);
      await page.getByText("Morning brief").first().click();
      await settle(page);
      await page.getByRole("button", { name: "Edit schedule" }).click();
    },
  },
  {
    id: "time-calendar-due-card",
    steps: async (page) => {
      await calendar("api")(page);
      await page
        .getByText(/^#\d+ .*/)
        .first()
        .click();
    },
  },
  {
    id: "time-calendar-empty",
    steps: async (page) => {
      await calendar("api")(page);
      await page.evaluate(() => {
        const t = new Date(window.M.T0);
        t.setMonth(t.getMonth() + 8);
        window.M.set({ schedules: [], calEvents: [], calCursor: t.getTime() });
      });
    },
  },
];
