import { expect, type Page, test } from "@playwright/test";
import {
  BROWSER_NETWORK_NOISE,
  expectCleanScreen,
  openApp,
  SIZES,
  THEMES,
  waitUntilOnline,
} from "./support/app";
import {
  boardViaApi,
  createCardViaApi,
  createProjectViaApi,
  homeViaApi,
  removeProjectViaApi,
} from "./support/daemon-api";
import { makeRepo } from "./support/git";

const DESKTOP = SIZES[2];
const RADIX = 36;
const RANDOM_START = 2;
const RANDOM_END = 8;

/** The daemon's own words for a move it refuses, from architecture.md 6.1 rule 4. */
const NEEDS_A_PULL_REQUEST =
  "In review needs an open pull request. The agent opens one when the work is ready.";

/** A name no other spec or run uses, so specs that run side by side never see each other's projects. */
const uniqueName = (label: string): string =>
  `e2e-${label}-${Math.random().toString(RADIX).slice(RANDOM_START, RANDOM_END)}`;

/** Opens the project's board and waits for it to be the page the shell shows. */
/**
 * Opens a project's board by its id. The route is taken the way the shell takes it, because a
 * project's name and its id are not the same and this spec knows the id from the API.
 */
async function openBoard(page: Page, projectId: string): Promise<void> {
  await page.evaluate((pid) => window.M?.go("project", pid, "board"), projectId);
  // The board is the page when a column is there - a phone shows one at a time - or when it says
  // it has no cards at all.
  await expect(
    page.locator("[data-col]").or(page.getByText("This board has no cards yet.")).first(),
  ).toBeVisible();
}

/** The card's own row on the board, found by its title. */
const cardOnBoard = (page: Page, title: string) => page.locator("[data-card]", { hasText: title });

/** The fixture's own card: the daemon the suite runs against loads `MARSHAL_FIXTURE=prototype`. */
const FIXTURE_CARD = "Fix token refresh on login";
const FIXTURE_PROJECT = "api";

/** Switches the open project's view by the tab on its view switcher. */
async function showView(page: Page, label: string): Promise<void> {
  await page.getByRole("tab", { name: label, exact: true }).click();
}

test.describe("the cards the daemon sends, in every view", () => {
  test("draws the fixture's card on the Board, the List, the Timeline, and the Agents view", async ({
    page,
  }) => {
    const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
    await openBoard(page, FIXTURE_PROJECT);
    await expect(cardOnBoard(page, FIXTURE_CARD)).toBeVisible();

    // The same card, from the same store, in each of the views the section names.
    for (const view of ["List", "Timeline", "Agents"]) {
      await showView(page, view);
      await expect(page.getByText(FIXTURE_CARD).first()).toBeVisible();
    }

    await expectCleanScreen(page, problems, "light");
  });

  test("Home's needs-you list shows every card the daemon says is waiting", async ({
    page,
    request,
  }) => {
    const home = await homeViaApi(request);
    // The fixture gives the daemon waiting cards, so this is a real comparison and not a zero.
    expect(home.needs.length).toBeGreaterThan(0);

    const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
    await waitUntilOnline(page);

    // The screen reads the same store the daemon's cards fill: a card that waits is on Home.
    for (const waiting of home.needs) {
      const number = waiting.key.split("#")[1] ?? "";
      await expect(page.getByText(new RegExp(`#${number}\\b`)).first()).toBeVisible();
    }

    await expectCleanScreen(page, problems, "light");
  });
});

test.describe("cards on the dev daemon", () => {
  test("shows a card the daemon has, and answers a refused move with the daemon's own sentence", async ({
    page,
    request,
  }) => {
    const repo = makeRepo("e2e-cards");
    const name = uniqueName("cards");
    let id = "";
    try {
      const project = await createProjectViaApi(request, repo.dir, name);
      id = project.id;
      const title = "Wire the card spec to the daemon";
      await createCardViaApi(request, id, title);

      const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
      await openBoard(page, id);

      // The card is the daemon's: it was never in the mock, so only the board call could bring it.
      const card = cardOnBoard(page, title);
      await expect(card).toBeVisible();
      await expect(card).toHaveAttribute("data-card", /.+#\d+$/);

      // The card panel's own move menu asks the daemon. Backlog to In review needs an open pull
      // request, so the daemon refuses, the card snaps back, and its own sentence is shown.
      await card.click();
      await page.getByRole("button", { name: "More actions", exact: true }).click();
      await page.getByRole("menuitem", { name: /Move to in review/i }).click();
      await expect(page.getByText(NEEDS_A_PULL_REQUEST)).toBeVisible();
      await expect(cardOnBoard(page, title)).toBeVisible();

      // Nothing moved on the daemon either, so a second device sees the card where it was.
      const cards = await boardViaApi(request, id);
      expect(cards.find((one) => one.title === title)?.state).toBe("backlog");

      await expectCleanScreen(page, problems, "light");
    } finally {
      if (id) await removeProjectViaApi(request, id);
    }
  });

  test("shows an honest empty board for a project with no cards", async ({ page, request }) => {
    const repo = makeRepo("e2e-cards-empty");
    const name = uniqueName("empty");
    let id = "";
    try {
      const project = await createProjectViaApi(request, repo.dir, name);
      id = project.id;

      const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
      await waitUntilOnline(page);
      await openBoard(page, id);

      // The board says it has no cards at all, rather than showing the mock's own 29.
      await expect(page.getByText("This board has no cards yet.")).toBeVisible();
      expect(await boardViaApi(request, id)).toEqual([]);

      await expectCleanScreen(page, problems, "light");
    } finally {
      if (id) await removeProjectViaApi(request, id);
    }
  });

  test("draws the daemon's card in both themes and at a phone width", async ({ page, request }) => {
    const repo = makeRepo("e2e-cards-themes");
    const name = uniqueName("themes");
    let id = "";
    try {
      const project = await createProjectViaApi(request, repo.dir, name);
      id = project.id;
      const title = "Read the card from the daemon";
      await createCardViaApi(request, id, title);

      for (const theme of THEMES) {
        const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE, theme });
        await openBoard(page, id);
        await expect(cardOnBoard(page, title)).toBeVisible();
        await expectCleanScreen(page, problems, theme);
      }

      const problems = await openApp(page, SIZES[0], { allow: BROWSER_NETWORK_NOISE });
      await openBoard(page, id);
      // A phone draws one column at a time, and the card the daemon sent is in the backlog.
      await page.evaluate(() => window.M?.set({ mobileCol: "backlog" }));
      await expect(cardOnBoard(page, title)).toBeVisible();
      await expectCleanScreen(page, problems, "light");
    } finally {
      if (id) await removeProjectViaApi(request, id);
    }
  });
});
