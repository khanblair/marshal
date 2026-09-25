/** Board scenarios: columns, swimlanes, filters, empty states, add a card, Done limit, phone tabs, drag. */
import { settle } from "../lib.mjs";
import { openCard, project, set } from "../steps.mjs";

const COLUMNS = ["backlog", "planning", "working", "needs", "review", "ready", "done"];
const DONE_CARD_ID = 33;
const FIRST_CLONE_ID = 400;
const EXTRA_DONE_CARDS = 25;
const NO_PHONE = ["desktop", "tablet"];

/** Evaluates `fn` in the page with one argument, then lets the frozen clock flush timers and frames. */
const run = async (page, fn, arg) => {
  await page.evaluate(fn, arg);
  await settle(page);
};

const setSwim = (pid, swim) => (page) =>
  run(page, ([id, mode]) => window.M.set({ swim: { ...window.M.S.swim, [id]: mode } }), [
    pid,
    swim,
  ]);

const collapseLane = (page, key) =>
  run(
    page,
    (k) => window.M.set({ laneCollapsed: { ...window.M.S.laneCollapsed, [k]: true } }),
    key,
  );

/** Opens a project's board, with optional extra steps. */
const board =
  (pid, ...more) =>
  async (page, ctx) => {
    await project(pid, "board")(page);
    await settle(page);
    for (const step of more) await step(page, ctx);
  };

/** Adds cards to Done so it passes the 20 card limit. Clones of a seed card, newest first. */
const addDoneCards = (page) =>
  run(
    page,
    ([id, first, count]) => {
      const M = window.M;
      const template = M.card(id);
      const clones = Array.from({ length: count }, (_, i) => ({
        ...JSON.parse(JSON.stringify(template)),
        id: first + i,
        title: `Older finished card ${i + 1}`,
        upd: template.upd - (i + 1) * 60000,
      }));
      M.set({ cards: [...M.S.cards, ...clones] });
    },
    [DONE_CARD_ID, FIRST_CLONE_ID, EXTRA_DONE_CARDS],
  );

/** Scrolls the board's scroll area (the nearest scrollable ancestor of a column). */
const scrollBoard = (page, { left = 0, bottom = false } = {}) =>
  run(
    page,
    ([x, toBottom]) => {
      let el = document.querySelector("[data-col]");
      while (
        el &&
        !/(auto|scroll)/.test(getComputedStyle(el).overflowY + getComputedStyle(el).overflowX)
      )
        el = el.parentElement;
      if (!el) return;
      el.scrollLeft = x;
      if (toBottom) el.scrollTop = el.scrollHeight;
    },
    [left, bottom],
  );

const addFilter = (kind, value) => (page) =>
  run(page, ([k, v]) => window.M.addFilter(k, v), [kind, value]);
/** On a phone the board shows one column; other sizes ignore this. */
const showColumn = (col) => (page) => run(page, (c) => window.M.set({ mobileCol: c }), col);
const openAdd = (key) => (page) => run(page, (k) => window.M.set({ quickAddAt: k }), key);
const setQuery = (pid, text) => (page) =>
  run(page, ([id, q]) => window.M.set({ query: { ...window.M.S.query, [id]: q } }), [pid, text]);

/** Mutates cards the way the daemon would, then redraws. */
const patchCards = (page, patches) =>
  run(
    page,
    (list) => {
      for (const [id, patch] of list) Object.assign(window.M.card(id), patch);
      window.M.emit();
    },
    patches,
  );

const project3 = ["api", "web", "mobile"];

export default [
  ...project3.map((pid) => ({ id: `board-${pid}`, steps: board(pid) })),
  { id: "board-api-swim-role", sizes: NO_PHONE, steps: board("api", setSwim("api", "role")) },
  { id: "board-api-swim-agent", sizes: NO_PHONE, steps: board("api", setSwim("api", "agent")) },
  { id: "board-api-swim-label", sizes: NO_PHONE, steps: board("api", setSwim("api", "label")) },
  {
    id: "board-web-swim-package-none",
    sizes: NO_PHONE,
    steps: board("web", setSwim("web", "package")),
  },
  {
    id: "board-api-swim-role-collapsed",
    sizes: NO_PHONE,
    steps: board("api", setSwim("api", "role"), (page) => collapseLane(page, "api:role:Worker")),
  },
  {
    id: "board-mobile-package-collapsed",
    sizes: NO_PHONE,
    steps: board(
      "mobile",
      (page) => collapseLane(page, "mobile:package:apps/ios"),
      (page) => collapseLane(page, "mobile:package:packages/ui"),
    ),
  },
  {
    id: "board-mobile-swim-none",
    sizes: NO_PHONE,
    steps: board("mobile", setSwim("mobile", "none")),
  },
  {
    id: "board-mobile-swim-role",
    sizes: ["desktop"],
    steps: board("mobile", setSwim("mobile", "role")),
  },
  { id: "board-filter-status", steps: board("api", addFilter("status", "needs")) },
  {
    id: "board-filter-two",
    sizes: ["desktop"],
    steps: board("api", addFilter("agent", "Claude Code"), addFilter("role", "Worker")),
  },
  { id: "board-search-match", sizes: ["desktop"], steps: board("api", setQuery("api", "auth")) },
  { id: "board-search-none", steps: board("api", setQuery("api", "zzzz")) },
  {
    id: "board-filter-none",
    steps: board(
      "web",
      addFilter("role", "Tester"),
      addFilter("agent", "Codex"),
      addFilter("label", "perf"),
    ),
  },
  {
    id: "board-empty",
    steps: board("web", (page) =>
      run(page, () => window.M.set({ cards: window.M.S.cards.filter((c) => c.p !== "web") })),
    ),
  },
  {
    id: "board-done-limited",
    steps: board("api", addDoneCards, (page) => scrollBoard(page, { bottom: true })),
  },
  {
    id: "board-done-show-all",
    sizes: ["desktop"],
    steps: board(
      "api",
      addDoneCards,
      (page) => scrollBoard(page, { bottom: true }),
      async (page) => {
        await page.getByRole("button", { name: `Show all ${EXTRA_DONE_CARDS + 1}` }).click();
        await settle(page);
      },
      (page) => scrollBoard(page, { bottom: true }),
    ),
  },
  { id: "board-add-backlog", steps: board("api", openAdd("all:backlog")) },
  { id: "board-add-planning", steps: board("api", openAdd("all:planning")) },
  { id: "board-add-working", steps: board("api", openAdd("all:working")) },
  {
    id: "board-add-typed",
    sizes: ["desktop", "phone"],
    steps: board("api", showColumn("backlog"), openAdd("all:backlog"), async (page) => {
      await page
        .getByRole("textbox", { name: "Card title" })
        .fill("Write the release notes for the beta");
      await settle(page);
    }),
  },
  {
    id: "board-add-in-lane",
    sizes: ["desktop"],
    steps: board("api", setSwim("api", "role"), openAdd("Worker:working")),
  },
  {
    id: "board-add-in-package-lane",
    sizes: ["desktop"],
    steps: board("mobile", openAdd("packages/ui:backlog")),
  },
  {
    id: "board-card-selected",
    sizes: NO_PHONE,
    steps: board("api", (page) => openCard(page, 41)),
  },
  {
    id: "board-card-focused",
    sizes: NO_PHONE,
    steps: board("api", (page) => set(page, { focusId: 44 })),
  },
  {
    id: "board-card-hover",
    sizes: ["desktop"],
    steps: board("api", async (page) => {
      await page.hover('[data-card="41"]');
      await settle(page);
    }),
  },
  {
    id: "board-add-row-hover",
    sizes: ["desktop"],
    steps: board("api", async (page) => {
      await page.getByRole("button", { name: "Add a card" }).first().hover();
      await settle(page);
    }),
  },
  {
    id: "board-lane-header-hover",
    sizes: ["desktop"],
    steps: board("api", setSwim("api", "role"), async (page) => {
      await page.getByRole("button", { name: /^Worker/ }).hover();
      await settle(page);
    }),
  },
  {
    id: "board-card-variants",
    sizes: ["desktop", "phone"],
    steps: board(
      "api",
      (page) =>
        patchCards(page, [
          [
            41,
            {
              bypass: true,
              pinned: true,
              cost: 3.4,
              ci: "failed",
              think: "High",
              pkg: "services/api",
              members: ["ada", "grace"],
            },
          ],
          [42, { asleep: true, ci: "passed", cost: 0.12 }],
          [
            43,
            {
              reason:
                "Which of the two migration strategies should the agent follow for the existing tokens? It needs your decision before it can go on.",
            },
          ],
        ]),
      (page) =>
        page.viewportSize().width < 640 ? set(page, { mobileCol: "working" }) : Promise.resolve(),
    ),
  },
  {
    id: "board-card-paused-long",
    sizes: ["desktop"],
    steps: board("api", (page) =>
      patchCards(page, [
        [
          41,
          {
            paused: true,
            title:
              "Rework the authentication middleware so session refresh survives a server restart and a rolling deploy",
            branch: "feature/auth-middleware-session-refresh-after-restart-and-rolling-deploys",
          },
        ],
      ]),
    ),
  },
  {
    id: "board-dragging",
    steps: board("api", (page) => set(page, { dragId: 41, dropCol: "review" })),
  },
  {
    id: "board-dragging-lanes",
    sizes: ["desktop"],
    steps: board("api", setSwim("api", "role"), (page) =>
      set(page, { dragId: 41, dropCol: "needs" }),
    ),
  },
  {
    id: "board-drag-real",
    sizes: ["desktop"],
    steps: board("api", async (page) => {
      const box = await page.locator('[data-card="41"]').boundingBox();
      const x = box.x + box.width / 2;
      const y = box.y + box.height / 2;
      await page.mouse.move(x, y);
      await page.mouse.down();
      await page.mouse.move(x + 60, y + 20, { steps: 4 });
      await page.mouse.move(x + 600, y + 40, { steps: 8 });
      await settle(page);
    }),
  },
  ...COLUMNS.map((col) => ({
    id: `board-phone-${col}`,
    sizes: ["phone"],
    steps: board("api", (page) => set(page, { mobileCol: col })),
  })),
  {
    id: "board-phone-mobile-working",
    sizes: ["phone"],
    steps: board("mobile", (page) => set(page, { mobileCol: "working" })),
  },
  {
    id: "board-phone-empty-column",
    sizes: ["phone"],
    steps: board("mobile", (page) => set(page, { mobileCol: "planning" })),
  },
  {
    id: "board-phone-filtered",
    sizes: ["phone"],
    steps: board("api", addFilter("status", "needs")),
  },
  {
    id: "board-phone-add",
    sizes: ["phone"],
    steps: board("api", (page) => set(page, { mobileCol: "backlog" }), openAdd("all:backlog")),
  },
  {
    id: "board-tablet-scrolled",
    sizes: ["tablet"],
    steps: board("api", (page) => scrollBoard(page, { left: 420 })),
  },
  {
    id: "board-desktop-scrolled",
    sizes: ["desktop"],
    steps: board("api", (page) => scrollBoard(page, { left: 200 })),
  },
  {
    id: "board-web-selected-lane",
    sizes: ["desktop"],
    steps: board("web", setSwim("web", "agent"), (page) => openCard(page, 118)),
  },
];
