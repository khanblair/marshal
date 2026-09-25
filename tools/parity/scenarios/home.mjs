/**
 * Home scenarios: the dashboard at every range, its states (nothing needs you,
 * cost tones, long lists, empty lists, long text), and the two view-all pages.
 * Steps edit the store through `window.M` in both apps; the prototype is React,
 * so every direct write is followed by `M.emit()`.
 */
import { settle } from "../lib.mjs";
import { go, set } from "../steps.mjs";

const SIZES_WIDE_AND_PHONE = ["desktop", "phone"];
const LIGHT = ["light"];

/** Adds three more projects and a calendar event for every weekday, so every Home list passes five rows. */
const longLists = (page) =>
  page.evaluate(() => {
    const M = window.M;
    const extra = [
      ["p4", "billing-service", "passed", 5],
      ["p5", "search-indexer", "cancelled", 70],
      ["p6", "docs-site", "queued", 1500],
    ];
    for (const [id, name, ci, ciAgo] of extra) {
      M.S.projects.push({ id, name, lang: "Go", path: `~/code/${name}`, ci, ciAgo, monthBase: 20, runs: [{ wf: "test", st: ci, ago: ciAgo }] });
    }
    const days = [0, 1, 2, 3, 4, 5, 6];
    M.S.calEvents.push({ id: "ev-x1", title: "Design review", time: "15:00", days });
    M.S.calEvents.push({ id: "ev-x2", title: "Release sync", time: "16:30", days });
    M.S.calEvents.push({ id: "ev-x3", title: "Retro", time: "17:45", days });
    M.emit();
  });

/** Presses every "Show all N" button; each turns into "Show less". */
const expandAll = async (page) => {
  const toggle = page.getByRole("button", { name: /^Show all \d+$/ });
  while ((await toggle.count()) > 0) await toggle.first().click();
};

/** Sets the daily cost limit so today's cost is `ratio` of it. */
const costLimit = (page, divisor) =>
  page.evaluate((d) => {
    const M = window.M;
    const today = M.costs().today;
    M.S.limits.global.day = d > 1 ? Math.floor(today / d) : Math.ceil(today / 0.9);
    M.emit();
  }, divisor);

/** Scrolls Home's own scroll container so the heading `#id` is at the top; without an id it scrolls to the end. */
const scrollHome = (page, id) =>
  page.evaluate((headingId) => {
    const start = document.querySelector('[data-tour="tiles"]');
    let box = start;
    while (box && !(box.scrollHeight > box.clientHeight && /auto|scroll/.test(getComputedStyle(box).overflowY))) {
      box = box.parentElement;
    }
    if (!box) return;
    const heading = headingId && document.getElementById(headingId);
    box.scrollTop = heading ? heading.getBoundingClientRect().top - box.getBoundingClientRect().top + box.scrollTop - 16 : box.scrollHeight;
  }, id);

/** Opens a view-all page. The steps that follow need it drawn, and the clock is paused, so it settles first. */
const viewAll = (kind) => async (page) => {
  await set(page, { allKind: kind });
  await go(page, "all");
  await settle(page);
};

const projectSelect = (page) => page.getByRole("combobox", { name: "Project", exact: true });

const pickKind = (label) => async (page) => {
  await viewAll("activity")(page);
  await page.getByRole("button", { name: label, exact: true }).click();
};

/*
 * The prototype's template engine wraps every interpolation in an HTML span, and an HTML
 * span inside an SVG text element never renders, so the prototype's charts have no labels.
 * The port draws them (the design's markup asks for them; the geometry is covered by the
 * oracle tests in chart-geometry.test.ts). Both apps hide chart text so the rest compares.
 */
const HIDE_CHART_TEXT = 'svg[role="img"] text { display: none !important; }';

const scenarios = [
  { id: "home-default", steps: async () => {} },
  { id: "home-range-30", steps: (page) => set(page, { dashRange: 30 }) },
  { id: "home-range-90", steps: (page) => set(page, { dashRange: 90 }) },
  ...[
    ["needs", "h-needs"],
    ["activity", "h-act"],
    ["today", "h-today"],
    ["awake", "h-awake"],
    ["ci", "h-ci"],
    ["end", ""],
  ].map(([name, id]) => ({ id: `home-scroll-${name}`, steps: (page) => scrollHome(page, id) })),
  {
    id: "home-long-lists",
    steps: async (page) => {
      await longLists(page);
      await scrollHome(page, "h-act");
    },
  },
  {
    id: "home-long-lists-end",
    steps: async (page) => {
      await longLists(page);
      await scrollHome(page, "");
    },
  },
  {
    id: "home-expanded",
    steps: async (page) => {
      await longLists(page);
      await expandAll(page);
      await scrollHome(page, "h-act");
    },
  },
  {
    id: "home-expanded-end",
    steps: async (page) => {
      await longLists(page);
      await expandAll(page);
      await scrollHome(page, "");
    },
  },
  {
    id: "home-needs-none",
    steps: (page) =>
      page.evaluate(() => {
        for (const c of window.M.S.cards) if (c.state === "needs") c.state = "working";
        window.M.emit();
      }),
  },
  { id: "home-cost-near", steps: (page) => costLimit(page, 1) },
  { id: "home-cost-over", steps: (page) => costLimit(page, 2) },
  {
    id: "home-awake-full",
    steps: async (page) => {
      await page.evaluate(() => {
        window.M.S.limits.global.awake = 3;
        window.M.emit();
      });
      await scrollHome(page, "h-awake");
    },
  },
  {
    id: "home-empty-lists",
    steps: async (page) => {
      await page.evaluate(() => {
        const M = window.M;
        M.S.feed = [];
        M.S.calEvents = [];
        for (const s of M.S.schedules) s.enabled = false;
        for (const c of M.S.cards) if (c.due === 0) c.due = null;
        M.emit();
      });
      await scrollHome(page, "h-act");
    },
  },
  {
    id: "home-long-text",
    steps: async (page) => {
      await page.evaluate(() => {
        const M = window.M;
        const needs = M.S.cards.find((c) => c.state === "needs");
        needs.title = "Migrate the whole authentication layer to the new session token format and update every dependent service";
        needs.reason = "Approval needed to run a long command that touches the whole workspace";
        M.emit();
      });
      await scrollHome(page, "h-needs");
    },
  },
  {
    id: "home-long-text-activity",
    steps: async (page) => {
      await page.evaluate(() => {
        const M = window.M;
        M.S.feed[0].text = "Deploy preview ready at https://preview.example.com/marshal/a-very-long-path-without-any-breaks/index.html";
        M.S.feed[1].text = "Nightly dependency audit finished with a summary long enough to wrap onto a second line on narrow screens";
        M.emit();
      });
      await scrollHome(page, "h-act");
    },
  },
  {
    id: "home-hover-tile",
    sizes: ["desktop"],
    steps: (page) => page.getByRole("button", { name: /Working now/ }).hover(),
  },
  {
    id: "home-hover-row",
    sizes: ["desktop"],
    steps: async (page) => {
      await page.getByRole("button", { name: /Sleep #/ }).first().hover();
    },
  },
  {
    id: "home-hover-feed",
    sizes: ["desktop"],
    themes: LIGHT,
    steps: (page) => page.getByRole("listitem").filter({ hasText: "Plan ready for review" }).getByRole("button").hover(),
  },
  { id: "home-all-activity", steps: viewAll("activity") },
  { id: "home-all-ci", steps: viewAll("ci") },
  {
    id: "home-all-activity-long",
    sizes: SIZES_WIDE_AND_PHONE,
    themes: LIGHT,
    steps: async (page) => {
      await longLists(page);
      await viewAll("activity")(page);
    },
  },
  {
    id: "home-all-ci-long",
    sizes: SIZES_WIDE_AND_PHONE,
    themes: LIGHT,
    steps: async (page) => {
      await longLists(page);
      await viewAll("ci")(page);
    },
  },
  ...[
    ["merge", "Merges"],
    ["ci", "CI results"],
    ["approval", "Approvals"],
    ["plan", "Plans"],
    ["schedule", "Schedule runs"],
    ["brief", "Briefs"],
  ].map(([kind, label]) => ({
    id: `home-all-kind-${kind}`,
    sizes: SIZES_WIDE_AND_PHONE,
    themes: LIGHT,
    steps: pickKind(label),
  })),
  ...["api", "web", "mobile"].flatMap((pid) => [
    {
      id: `home-all-activity-${pid}`,
      sizes: ["desktop"],
      themes: LIGHT,
      steps: async (page) => {
        await viewAll("activity")(page);
        await projectSelect(page).selectOption(pid);
      },
    },
    {
      id: `home-all-ci-${pid}`,
      sizes: ["desktop"],
      themes: LIGHT,
      steps: async (page) => {
        await viewAll("ci")(page);
        await projectSelect(page).selectOption(pid);
      },
    },
  ]),
  {
    id: "home-all-activity-empty",
    sizes: ["desktop"],
    themes: LIGHT,
    steps: async (page) => {
      await viewAll("activity")(page);
      await projectSelect(page).selectOption("api");
      await page.getByRole("button", { name: "Briefs", exact: true }).click();
    },
  },
];

export default scenarios.map((scenario) => ({
  ...scenario,
  steps: async (page, context) => {
    await page.addStyleTag({ content: HIDE_CHART_TEXT });
    await scenario.steps(page, context);
  },
}));
