/**
 * Shell overlay scenarios (prefix `shellb-`): notices, command palette, confirm, New card,
 * New project, Remove project, toasts. Steps use `window.M` and real interactions in both apps.
 * The prototype throws on a project added at runtime, so New project is never submitted here.
 * The prototype is React and the clock is frozen, so it draws only when the clock runs: every step
 * that a later step depends on ends with `settle`.
 */
import { settle } from "../lib.mjs";
import { go, openCard, set } from "../steps.mjs";

const WIDE_AND_PHONE = ["desktop", "phone"];
const SEARCH = "Search actions, projects, cards, and settings";
const ARROWS_TO_SCROLL = 12;
const ARROWS_TO_MOVE = 3;
const LONG_QUERY = "zebra ".repeat(20).trim();
const LONG_TITLE =
  "Rework the token refresh flow so sessions survive a laptop sleep, a network change, and a clock jump";
const LONG_BODY = [
  "Refresh the access token a minute before it expires.",
  "Retry with backoff when the network is down.",
  "Keep the user signed in across a clock change.",
  "Add a test for each case.",
].join("\n");

const onProject =
  (pid, view = "board") =>
  (page) =>
    go(page, "project", pid, view);
const evaluate = (page, fn, arg) => page.evaluate(fn, arg);
/** Runs one interaction, then lets the prototype draw the result. */
const act = async (page, action) => {
  await action();
  await settle(page);
};

/* ---- notices ---- */

const SLEEP_COUNTDOWN_MS = 125_000;

/**
 * Opens the panel. The sleep countdown counts down from a deadline set when the store loads,
 * and the two apps load at different speeds, so the deadline is pinned to the frozen clock.
 */
const openNotices = async (page) => {
  await evaluate(
    page,
    (ms) => {
      const sleep = window.M.S.notices.find((n) => n.kind === "sleep");
      if (sleep) sleep.deadline = Date.now() + ms;
    },
    SLEEP_COUNTDOWN_MS,
  );
  await set(page, { noticesOpen: true });
};

const dismissMost = async (page) => {
  await openNotices(page);
  await evaluate(page, () => {
    window.M.dismissNotice("n2");
    window.M.dismissNotice("n3");
    window.M.keepAllAwake();
  });
};

/** No card needs you and no notice is left, so the panel shows its empty text. */
const emptyNotices = async (page) => {
  await evaluate(page, () => {
    const M = window.M;
    for (const c of M.S.cards) {
      if (c.state === "needs") {
        c.state = "working";
        c.reason = "";
      }
    }
    M.S.notices = [];
    M.emit();
  });
  await openNotices(page);
};

/** A plan notice, a CI failure on a card, and a notice with no card, on top of the seed. */
const moreKinds = async (page) => {
  await evaluate(page, () => {
    const M = window.M;
    const now = Date.now();
    M.S.notices.push(
      {
        id: "x1",
        kind: "plan",
        cardId: 43,
        text: "Plan ready for #43",
        sub: "Add rate limiting per API key",
        ts: now - 4 * 60000,
      },
      {
        id: "x2",
        kind: "ci",
        cardId: 40,
        pid: "api",
        text: "CI failed on #40",
        sub: "The unit workflow failed in TestBucket",
        ts: now - 2 * 3600000,
      },
      {
        id: "x3",
        kind: "ci-main",
        text: "Main is failing in web-dashboard",
        sub: "The lint workflow failed",
        ts: now - 26 * 3600000,
      },
    );
    M.emit();
  });
  await openNotices(page);
};

/** One idle card, so the sleep title uses the singular. */
const oneIdle = async (page) => {
  await evaluate(page, () => {
    const M = window.M;
    const now = Date.now();
    M.S.notices = [
      { id: "s1", kind: "sleep", cards: [39], deadline: now + 95000, ts: now - 30000 },
    ];
    M.emit();
  });
  await openNotices(page);
};

const notices = [
  { id: "shellb-notices-open", steps: openNotices },
  { id: "shellb-notices-dismissed", steps: dismissMost },
  { id: "shellb-notices-kinds", sizes: WIDE_AND_PHONE, steps: moreKinds },
  { id: "shellb-notices-sleep-one", sizes: WIDE_AND_PHONE, steps: oneIdle },
  { id: "shellb-notices-empty", steps: emptyNotices },
  {
    id: "shellb-notices-on-project",
    sizes: WIDE_AND_PHONE,
    steps: async (page) => {
      await onProject("web")(page);
      await openNotices(page);
    },
  },
];

/* ---- command palette ---- */

const openPalette = (page) => act(page, () => set(page, { palette: true }));
const searchBox = (page) => page.getByLabel(SEARCH);
const arrows = async (page, key, count) => {
  for (let i = 0; i < count; i++) await act(page, () => searchBox(page).press(key));
};
const typed = (text) => async (page) => {
  await openPalette(page);
  await act(page, () => searchBox(page).fill(text));
};

const palette = [
  { id: "shellb-palette-empty", steps: openPalette },
  { id: "shellb-palette-query", steps: typed("rate") },
  { id: "shellb-palette-query-switch", steps: typed("switch") },
  { id: "shellb-palette-query-project", sizes: WIDE_AND_PHONE, steps: typed("go") },
  { id: "shellb-palette-no-results", steps: typed("zzzz") },
  { id: "shellb-palette-long-query", sizes: WIDE_AND_PHONE, steps: typed(LONG_QUERY) },
  {
    id: "shellb-palette-keys",
    steps: async (page) => {
      await openPalette(page);
      await arrows(page, "ArrowDown", ARROWS_TO_MOVE);
    },
  },
  {
    id: "shellb-palette-scroll",
    steps: async (page) => {
      await openPalette(page);
      await arrows(page, "ArrowDown", ARROWS_TO_SCROLL);
    },
  },
  {
    id: "shellb-palette-scroll-back",
    sizes: WIDE_AND_PHONE,
    steps: async (page) => {
      await openPalette(page);
      await arrows(page, "ArrowDown", ARROWS_TO_SCROLL);
      await arrows(page, "ArrowUp", ARROWS_TO_MOVE);
    },
  },
  {
    id: "shellb-palette-card-open",
    sizes: WIDE_AND_PHONE,
    steps: async (page) => {
      await onProject("api")(page);
      await openCard(page, 44);
      await openPalette(page);
    },
  },
  {
    id: "shellb-palette-merge",
    sizes: WIDE_AND_PHONE,
    steps: async (page) => {
      await onProject("api")(page);
      await typed("merge")(page);
    },
  },
];

/* ---- confirm dialog ---- */

const confirmBypass = (checked) => async (page) => {
  await act(page, () => evaluate(page, () => window.M.requestBypass(41)));
  if (checked) await act(page, () => page.getByRole("checkbox").check());
};

const confirmPlain = (page) =>
  evaluate(page, () =>
    window.M.confirm({
      title: "Sleep all idle cards",
      message:
        "Every idle card sleeps and frees its memory. Each session is kept, and cards wake in a few seconds when needed.",
      action: "Sleep all",
      run: () => {},
    }),
  );

const confirmDelete = (withBranch) => (page) =>
  evaluate(
    page,
    (wanted) => {
      const c = window.M.S.cards.find((x) => (wanted ? x.branch : !x.branch));
      window.M.deleteCard(c.id);
    },
    withBranch,
  );

const confirms = [
  { id: "shellb-confirm-bypass", steps: confirmBypass(false) },
  { id: "shellb-confirm-bypass-checked", steps: confirmBypass(true) },
  { id: "shellb-confirm-plain", steps: confirmPlain },
  { id: "shellb-confirm-delete", steps: confirmDelete(true) },
  { id: "shellb-confirm-delete-no-branch", sizes: WIDE_AND_PHONE, steps: confirmDelete(false) },
];

/* ---- New card ---- */

const newCard = (opts) => async (page) => {
  await onProject("api")(page);
  await act(page, () => evaluate(page, (o) => window.M.newCard(o), opts));
};

const titleBox = (page) => page.getByPlaceholder("Fix token refresh on login");

const newCards = [
  { id: "shellb-newcard-empty", steps: newCard() },
  {
    id: "shellb-newcard-dupes",
    steps: async (page) => {
      await newCard()(page);
      await act(page, () => titleBox(page).fill("rate limiting API"));
    },
  },
  {
    id: "shellb-newcard-filled",
    steps: async (page) => {
      await onProject("web")(page);
      await evaluate(page, () =>
        window.M.newCard({
          title: "Fix token refresh on login",
          body: "Refresh the token before it expires.\nAdd a test.",
          template: "Plan first",
          role: "Reviewer",
          agent: Object.keys(window.M.AGENTS)[1],
          start: true,
        }),
      );
    },
  },
  {
    id: "shellb-newcard-long",
    sizes: WIDE_AND_PHONE,
    steps: newCard({ title: LONG_TITLE, body: LONG_BODY }),
  },
  {
    id: "shellb-newcard-typed",
    sizes: WIDE_AND_PHONE,
    steps: async (page) => {
      await newCard()(page);
      await act(page, () => titleBox(page).fill("Add retry to the webhook sender"));
      await act(page, () =>
        page
          .getByPlaceholder("What should the agent do, and what does done mean?")
          .fill("Retry three times, then give up."),
      );
    },
  },
];

/* ---- New project ---- */

const emptyProject = { source: "folder", path: "", url: "", name: "", branch: "main" };
const newProject = (page) => act(page, () => set(page, { newProject: emptyProject }));
const folderBox = (page) => page.getByPlaceholder("~/code/my-repo");

const withPath = (path) => async (page) => {
  await newProject(page);
  await act(page, () => folderBox(page).fill(path));
};

const projects = [
  { id: "shellb-newproject-empty", steps: newProject },
  { id: "shellb-newproject-go", steps: withPath("~/code/billing-service") },
  { id: "shellb-newproject-mono", sizes: WIDE_AND_PHONE, steps: withPath("~/code/acme-monorepo") },
  { id: "shellb-newproject-typescript", sizes: WIDE_AND_PHONE, steps: withPath("~/code/website") },
  {
    id: "shellb-newproject-choose",
    steps: async (page) => {
      await newProject(page);
      await act(page, () => page.getByRole("button", { name: "Choose folder" }).click());
    },
  },
  {
    id: "shellb-newproject-github",
    steps: async (page) => {
      await newProject(page);
      await act(page, () => page.getByRole("radio", { name: "Clone from GitHub" }).click());
      await act(page, () =>
        page
          .getByPlaceholder("https://github.com/owner/repo")
          .fill("https://github.com/acme/ledger.git"),
      );
    },
  },
  {
    id: "shellb-newproject-github-empty",
    sizes: WIDE_AND_PHONE,
    steps: async (page) => {
      await newProject(page);
      await act(page, () => page.getByRole("radio", { name: "Clone from GitHub" }).click());
    },
  },
  {
    id: "shellb-newproject-named",
    sizes: WIDE_AND_PHONE,
    steps: async (page) => {
      await withPath("~/code/billing-service")(page);
      await act(page, () =>
        page
          .getByLabel("Name", { exact: true })
          .fill("Payments API with a long name that keeps going"),
      );
      await act(page, () => page.getByLabel("Default branch").fill("develop"));
    },
  },
];

/* ---- Remove project ---- */

const remove =
  (id, keep = true) =>
  (page) =>
    set(page, { removeProject: { id, keepBranches: keep, keepMemory: keep } });

/** Every card of the project is merged or without a branch, so nothing is unmerged. */
const removeMerged = async (page) => {
  await evaluate(page, () => {
    const M = window.M;
    for (const c of M.S.cards) if (c.p === "web" && c.state !== "done") c.branch = null;
    M.emit();
  });
  await remove("web")(page);
};

const removals = [
  { id: "shellb-remove-web", steps: remove("web") },
  { id: "shellb-remove-web-unchecked", sizes: WIDE_AND_PHONE, steps: remove("web", false) },
  { id: "shellb-remove-web-merged", steps: removeMerged },
  { id: "shellb-remove-mobile", sizes: WIDE_AND_PHONE, steps: remove("mobile") },
  { id: "shellb-remove-api", sizes: WIDE_AND_PHONE, steps: remove("api") },
];

/* ---- toasts ---- */

const toastWithAction = (page) =>
  evaluate(page, () => window.M.toast("Card created", { label: "Open", run: () => {} }));

const toasts = [
  { id: "shellb-toast-action", steps: toastWithAction },
  {
    id: "shellb-toast-plain",
    steps: (page) => evaluate(page, () => window.M.toast("Light theme on")),
  },
  {
    id: "shellb-toast-stack",
    sizes: WIDE_AND_PHONE,
    steps: async (page) => {
      await evaluate(page, () => {
        const M = window.M;
        M.toast("Copied the branch name");
        M.toast("Chat archived", { label: "Undo", run: () => {} });
        M.toast(
          "Kept 3 cards awake and a very long message that has to wrap onto a second line in the toast",
        );
        M.set({ announce: "#43 moved to Review" });
      });
    },
  },
  {
    id: "shellb-toast-over-dialog",
    sizes: WIDE_AND_PHONE,
    steps: async (page) => {
      await confirmPlain(page);
      await toastWithAction(page);
    },
  },
];

/**
 * Every scenario starts on the web-dashboard board, a page without charts or clocks, so the
 * overlay is the only thing that can differ. A scenario that needs another page goes there itself.
 */
const onBoard = (scenario) => ({
  ...scenario,
  steps: async (page, context) => {
    await go(page, "project", "web", "board");
    await scenario.steps(page, context);
  },
});

export default [
  ...notices,
  ...palette,
  ...confirms,
  ...newCards,
  ...projects,
  ...removals,
  ...toasts,
].map(onBoard);
