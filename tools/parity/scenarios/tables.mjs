/**
 * Agents and List view scenarios: every project at all sizes, every sort column and direction,
 * focus, pinned, asleep, waking, bypass, and empty rows, column choices, filters, and both themes.
 * Sort scenarios run at desktop in light only; the sort logic does not depend on either.
 */
import { project, set } from "../steps.mjs";

const PROJECTS = ["api", "web", "mobile"];
const DESKTOP_ONLY = { sizes: ["desktop"] };
const SORT_ONLY = { sizes: ["desktop"], themes: ["light"] };
const SETTLE_MS = 200;
const DRAW_TRIES = 10;
const LONG_TITLE =
  "Rework the token refresh flow so two tabs that wake up at the same moment share one request and never log the person out";

const AGENT_SORT_KEYS = [
  "card",
  "role",
  "agent",
  "model",
  "think",
  "perm",
  "state",
  "sess",
  "activity",
  "cost",
];
const LIST_KEYS = [
  "id",
  "title",
  "state",
  "role",
  "agent",
  "model",
  "think",
  "pkg",
  "branch",
  "ci",
  "cost",
  "upd",
];

/**
 * Waits for the view to draw before a real interaction. The runner pauses the clock, and neither
 * app draws until it moves; the prototype also loads each view's file the first time it shows.
 */
const interact = (step) => async (page) => {
  for (let tries = 0; tries < DRAW_TRIES; tries++) {
    await page.clock.runFor(SETTLE_MS);
    const drawn = await page.evaluate(
      () => document.body.innerText.includes("Orchestrator for") || !!document.querySelector("th"),
    );
    if (drawn) break;
    await page.waitForTimeout(SETTLE_MS);
  }
  await step(page);
};

/** Runs `steps` in order, so a scenario can open a view and then change its state. */
const chain =
  (...steps) =>
  async (page, ctx) => {
    for (const step of steps) await step(page, ctx);
  };

const agentsOf = (pid) => project(pid, "agents");
const listOf = (pid) => project(pid, "list");

const sortAgents = (k, dir) => (page) =>
  page.evaluate(
    ([key, d]) => window.M.set({ sort: { ...window.M.S.sort, agents: { k: key, dir: d } } }),
    [k, dir],
  );
const sortList = (k, dir) => (page) =>
  page.evaluate(
    ([key, d]) => window.M.set({ sort: { ...window.M.S.sort, list: { k: key, dir: d } } }),
    [k, dir],
  );
const listCols = (cols) => (page) =>
  page.evaluate((c) => window.M.set({ listCols: { ...window.M.S.listCols, ...c } }), cols);
const focusCard = (id) => (page) => set(page, { focusId: id });
/** Calls an action, then drops the toast it raises: toasts belong to the shell, not to these views. */
const call =
  (name, ...args) =>
  async (page) => {
    await page.evaluate(([n, a]) => window.M[n](...a), [name, args]);
    await set(page, { toasts: [] });
  };
const searchList = (pid, text) => (page) =>
  page.evaluate(([p, q]) => window.M.set({ query: { ...window.M.S.query, [p]: q } }), [pid, text]);
/**
 * Moves every card of a project to the backlog, so it has no agent sessions. A project with no
 * cards at all breaks the prototype's shell.
 */
const allToBacklog = (pid) => (page) =>
  page.evaluate((p) => {
    for (const c of window.M.S.cards) if (c.p === p) c.state = "backlog";
    window.M.emit();
  }, pid);
const renameCard = (id, title) => (page) =>
  page.evaluate(([i, t]) => window.M.rename(i, t), [id, title]);

/** Scrolls the table sideways as far as it goes, to see the last columns. */
const scrollToEnd = interact((page) =>
  page.evaluate(() => {
    const box = document.querySelector("table")?.closest("div");
    if (box) box.scrollLeft = box.scrollWidth;
  }),
);

const direction = (dir) => (dir > 0 ? "asc" : "desc");

const listWithPackages = () => chain(listOf("mobile"), listCols({ pkg: true }));

const agentScenarios = [
  ...PROJECTS.map((pid) => ({ id: `tables-agents-${pid}`, steps: agentsOf(pid) })),
  { id: "tables-agents-focused", steps: chain(agentsOf("api"), focusCard(41)) },
  { id: "tables-agents-focused-bypass", steps: chain(agentsOf("mobile"), focusCard(209)) },
  { id: "tables-agents-focused-needs", steps: chain(agentsOf("api"), focusCard(44)) },
  { id: "tables-agents-asleep", steps: chain(agentsOf("api"), call("sleep", 39)) },
  { id: "tables-agents-pinned", steps: chain(agentsOf("api"), call("pin", 43)) },
  { id: "tables-agents-waking", steps: chain(agentsOf("web"), call("wake", 115)) },
  { id: "tables-agents-pinned-asleep", steps: chain(agentsOf("web"), call("pin", 117)) },
  { id: "tables-agents-empty", steps: chain(agentsOf("web"), allToBacklog("web")) },
  { id: "tables-agents-long-title", steps: chain(agentsOf("api"), renameCard(41, LONG_TITLE)) },
  {
    id: "tables-agents-row-hover",
    ...DESKTOP_ONLY,
    steps: chain(
      agentsOf("api"),
      interact((page) => page.locator('tr[data-card="43"] td').nth(1).hover()),
    ),
  },
  {
    id: "tables-agents-action-hover",
    ...DESKTOP_ONLY,
    steps: chain(
      agentsOf("api"),
      interact((page) => page.getByRole("button", { name: "Pin #43" }).hover()),
    ),
  },
  {
    id: "tables-agents-stop-dialog",
    sizes: ["desktop", "phone"],
    steps: chain(
      agentsOf("api"),
      interact((page) => page.getByRole("button", { name: "Stop session on #41" }).click()),
    ),
  },
  {
    id: "tables-agents-orchestrator-focus",
    ...DESKTOP_ONLY,
    steps: chain(
      agentsOf("api"),
      interact((page) => page.getByRole("row", { name: "Orchestrator session" }).focus()),
    ),
  },
  {
    id: "tables-agents-scrolled",
    sizes: ["desktop", "tablet"],
    steps: chain(agentsOf("api"), scrollToEnd),
  },
  {
    id: "tables-agents-scrolled-bypass",
    sizes: ["desktop", "tablet"],
    steps: chain(agentsOf("mobile"), focusCard(209), scrollToEnd),
  },
  ...AGENT_SORT_KEYS.flatMap((k) =>
    [1, -1].map((dir) => ({
      id: `tables-agents-sort-${k}-${direction(dir)}`,
      ...SORT_ONLY,
      steps: chain(agentsOf("api"), sortAgents(k, dir)),
    })),
  ),
];

const listScenarios = [
  ...PROJECTS.map((pid) => ({ id: `tables-list-${pid}`, steps: listOf(pid) })),
  {
    id: "tables-list-pkg-think",
    steps: chain(listOf("mobile"), listCols({ pkg: true, think: true })),
  },
  { id: "tables-list-think", steps: chain(listOf("api"), listCols({ think: true })) },
  {
    id: "tables-list-few-columns",
    steps: chain(
      listOf("web"),
      listCols({ role: false, agent: false, model: false, branch: false, upd: false }),
    ),
  },
  { id: "tables-list-focused", steps: chain(listOf("api"), focusCard(43)) },
  { id: "tables-list-long-title", steps: chain(listOf("api"), renameCard(41, LONG_TITLE)) },
  {
    id: "tables-list-filter-role",
    steps: chain(listOf("api"), call("addFilter", "role", "Worker")),
  },
  {
    id: "tables-list-filter-package",
    steps: chain(
      listWithPackages(),
      call("addFilter", "package", "apps/ios"),
      call("addFilter", "package", "apps/android"),
    ),
  },
  {
    id: "tables-list-filter-status-agent",
    steps: chain(
      listOf("api"),
      call("addFilter", "status", "review"),
      call("addFilter", "status", "needs"),
      call("addFilter", "agent", "Codex"),
    ),
  },
  { id: "tables-list-search", steps: chain(listOf("api"), searchList("api", "retry")) },
  {
    id: "tables-list-no-match",
    steps: chain(
      listOf("api"),
      call("addFilter", "role", "Tester"),
      call("addFilter", "status", "done"),
    ),
  },
  { id: "tables-list-no-match-query", steps: chain(listOf("web"), searchList("web", "zzz")) },
  {
    id: "tables-list-row-hover",
    ...DESKTOP_ONLY,
    steps: chain(
      listOf("api"),
      interact((page) => page.locator('tr[data-card="43"] td').nth(1).hover()),
    ),
  },
  {
    id: "tables-list-header-hover",
    ...DESKTOP_ONLY,
    steps: chain(
      listOf("api"),
      interact((page) => page.getByRole("button", { name: "Title" }).hover()),
    ),
  },
  {
    id: "tables-list-scrolled",
    sizes: ["desktop", "tablet"],
    steps: chain(listOf("api"), scrollToEnd),
  },
  {
    id: "tables-list-scrolled-pkg-think",
    sizes: ["desktop", "tablet"],
    steps: chain(listOf("mobile"), listCols({ pkg: true, think: true }), scrollToEnd),
  },
  ...LIST_KEYS.flatMap((k) =>
    [1, -1].map((dir) => ({
      id: `tables-list-sort-${k}-${direction(dir)}`,
      ...SORT_ONLY,
      steps: chain(listOf("mobile"), listCols({ pkg: true, think: true }), sortList(k, dir)),
    })),
  ),
];

export default [...agentScenarios, ...listScenarios];
