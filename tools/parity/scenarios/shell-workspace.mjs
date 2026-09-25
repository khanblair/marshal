/**
 * Shell workspace scenarios: the view header, the filter bar and its menus, the split
 * panes, and the card detail panel. The views inside are other agents' work, so
 * differences inside the main area are not this file's to fix.
 */
import { settle } from "../lib.mjs";
import { go, openCard, project, set } from "../steps.mjs";

/**
 * Opens a project view and lets both apps draw it. The prototype redraws on a
 * frame, and the clock is paused, so a step that looks up an element needs this first.
 */
const open = (pid, view) => async (page) => {
  await project(pid, view)(page);
  await settle(page);
};

/** Board on the api project with a project view, then a few store writes. */
const onApi =
  (view, ...after) =>
  async (page) => {
    await project("api", view)(page);
    for (const step of after) await step(page);
  };

// The prototype redraws on `emit()`, so direct store writes call it.
const filters = (page) =>
  page.evaluate(() => {
    window.M.addFilter("status", "needs");
    window.M.addFilter("agent", "Claude Code");
    window.M.addFilter("label", "auth");
    window.M.emit();
  });
const query = (text) => (page) =>
  page.evaluate((t) => {
    window.M.S.query.api = t;
    window.M.emit();
  }, text);
const menu = (name) => (page) => set(page, { menu: name });
const swim = (kind) => (page) =>
  page.evaluate((k) => {
    window.M.S.swim.api = k;
    window.M.S.savedView.api = null;
    window.M.emit();
  }, kind);

const splitOf = (views) => (page) => set(page, { split: views });
const openApiCard = (page) => openCard(page, 41);
const desktopTablet = ["desktop", "tablet"];

const viewScenarios = ["chat", "agents", "board", "list", "timeline", "calendar"].map((view) => ({
  id: `shellw-view-${view}`,
  steps: project("api", view),
}));

export default [
  ...viewScenarios,
  {
    id: "shellw-view-tab-hover",
    sizes: desktopTablet,
    steps: async (page) => {
      await open("api", "board")(page);
      await page.getByRole("tab", { name: "List" }).hover();
    },
  },
  {
    id: "shellw-view-new-card-hover",
    sizes: ["desktop"],
    steps: async (page) => {
      await open("api", "board")(page);
      await page.getByRole("button", { name: "New card" }).hover();
    },
  },

  { id: "shellw-filterbar-chips", steps: onApi("board", filters) },
  { id: "shellw-filterbar-query", steps: onApi("list", query("config")) },
  { id: "shellw-filterbar-timeline", steps: onApi("timeline", filters, query("auth")) },
  {
    id: "shellw-filterbar-focus",
    steps: async (page) => {
      await open("api", "board")(page);
      await page.locator("[data-search]").focus();
    },
  },
  {
    id: "shellw-filterbar-hover-add",
    sizes: ["desktop"],
    steps: async (page) => {
      await open("api", "board")(page);
      await page.getByRole("button", { name: "Add filter" }).hover();
    },
  },
  {
    id: "shellw-filterbar-package-chip",
    steps: async (page) => {
      await project("mobile", "board")(page);
      await page.evaluate(() => {
        window.M.addFilter("package", "packages/api-client");
        window.M.addFilter("status", "review");
        window.M.emit();
      });
    },
  },
  { id: "shellw-filter-menu", steps: onApi("board", menu("filter")) },
  {
    id: "shellw-filter-menu-packages",
    steps: async (page) => {
      await project("mobile", "board")(page);
      await set(page, { menu: "filter" });
    },
  },
  {
    id: "shellw-filter-menu-open-click",
    sizes: ["desktop"],
    steps: async (page) => {
      await open("api", "list")(page);
      await page.getByRole("button", { name: "Add filter" }).click();
      await settle(page);
      await page.getByRole("menuitem", { name: /Working/ }).hover();
    },
  },
  { id: "shellw-views-menu", steps: onApi("board", menu("views")) },
  {
    id: "shellw-views-menu-mobile-project",
    steps: async (page) => {
      await project("mobile", "board")(page);
      await set(page, { menu: "views" });
    },
  },
  {
    id: "shellw-views-menu-typed",
    sizes: ["desktop"],
    steps: async (page) => {
      await open("api", "board")(page);
      await set(page, { menu: "views" });
      await settle(page);
      await page.getByPlaceholder("Name this view").fill("My review queue");
    },
  },
  { id: "shellw-cols-menu", sizes: desktopTablet, steps: onApi("list", menu("cols")) },
  {
    id: "shellw-cols-menu-hover",
    sizes: ["desktop"],
    steps: async (page) => {
      await open("api", "list")(page);
      await set(page, { menu: "cols" });
      await settle(page);
      await page.getByText("Thinking mode").hover();
    },
  },
  {
    id: "shellw-cols-toggled",
    sizes: ["desktop"],
    steps: async (page) => {
      await open("api", "list")(page);
      await set(page, { menu: "cols" });
      await settle(page);
      await page.getByLabel("Package", { exact: true }).check();
      await page.getByLabel("Cost", { exact: true }).uncheck();
    },
  },
  { id: "shellw-swimlane-role", sizes: desktopTablet, steps: onApi("board", swim("role")) },
  {
    id: "shellw-swimlane-package",
    sizes: desktopTablet,
    steps: async (page) => {
      await project("mobile", "board")(page);
    },
  },

  {
    id: "shellw-split-add-click",
    sizes: ["desktop"],
    steps: async (page) => {
      await open("api", "chat")(page);
      await page.getByRole("button", { name: "Split view" }).click();
    },
  },
  { id: "shellw-split-one", sizes: desktopTablet, steps: onApi("board", splitOf(["list"])) },
  { id: "shellw-split-two", steps: onApi("chat", splitOf(["board", "list"])) },
  {
    id: "shellw-split-three",
    sizes: desktopTablet,
    steps: onApi("board", splitOf(["chat", "list", "timeline"])),
  },
  {
    id: "shellw-split-calendar-agents",
    sizes: ["desktop"],
    steps: onApi("list", splitOf(["calendar", "agents"])),
  },

  { id: "shellw-detail-open", steps: onApi("board", openApiCard) },
  { id: "shellw-detail-open-list", sizes: desktopTablet, steps: onApi("list", openApiCard) },
  {
    id: "shellw-detail-expanded",
    sizes: desktopTablet,
    steps: onApi("board", openApiCard, (page) => set(page, { detailExpanded: true })),
  },
  {
    id: "shellw-detail-wide",
    sizes: ["desktop"],
    steps: onApi("board", openApiCard, (page) => set(page, { detailW: 720 })),
  },
  {
    id: "shellw-detail-narrow",
    sizes: ["desktop"],
    steps: onApi("board", openApiCard, (page) => set(page, { detailW: 300 })),
  },
  {
    id: "shellw-detail-with-split",
    sizes: desktopTablet,
    steps: onApi("board", splitOf(["list"]), openApiCard),
  },
  {
    id: "shellw-detail-handle-hover",
    sizes: ["desktop"],
    steps: async (page) => {
      await open("api", "board")(page);
      await openApiCard(page);
      await settle(page);
      await page.getByRole("separator", { name: "Resize card panel" }).hover();
    },
  },
  {
    id: "shellw-detail-handle-key",
    sizes: ["desktop"],
    steps: async (page) => {
      await open("api", "board")(page);
      await openApiCard(page);
      await settle(page);
      await page.getByRole("separator", { name: "Resize card panel" }).focus();
      await page.keyboard.press("ArrowLeft");
      await page.keyboard.press("ArrowLeft");
    },
  },
  {
    id: "shellw-detail-mobile-project",
    steps: async (page) => {
      await project("mobile", "board")(page);
      await openCard(page, 209);
    },
  },

  { id: "shellw-route-all-activity", steps: async (page) => go(page, "all") },
  {
    id: "shellw-route-all-ci",
    steps: async (page) => {
      await set(page, { allKind: "ci" });
      await go(page, "all");
    },
  },
];
