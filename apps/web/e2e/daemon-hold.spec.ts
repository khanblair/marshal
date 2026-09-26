import { type APIRequestContext, expect, type Page, test } from "@playwright/test";
import {
  BROWSER_NETWORK_NOISE,
  expectCleanScreen,
  openApp,
  SIZES,
  waitUntilOnline,
} from "./support/app";
import {
  agentsViaApi,
  boardViaApi,
  cardViaApi,
  createProjectViaApi,
  removeProjectViaApi,
} from "./support/daemon-api";
import { makeRepo } from "./support/git";

/**
 * The life of one card the daemon holds: a card made in the New card dialog with the stub agent, run
 * from the panel, held (pause, sleep, wake), and moved between columns. Every hold is checked on the
 * daemon and once again after a reload, because what the daemon has is what a second device sees.
 */
const DESKTOP = SIZES[2];
const RADIX = 36;
const RANDOM_START = 2;
const RANDOM_END = 8;
/** Starting a session makes a worktree and runs the agent, so the first answer can take a while. */
const START_MS = 30_000;

const uniqueName = (label: string): string =>
  `e2e-${label}-${Math.random().toString(RADIX).slice(RANDOM_START, RANDOM_END)}`;

const TITLE = "Hold the card while the agent works";
const newCardDialog = (page: Page) => page.getByRole("dialog", { name: /^New card in / });
const cardOnBoard = (page: Page, text: string) => page.locator("[data-card]", { hasText: text });
const action = (page: Page, name: string) => page.getByRole("button", { name, exact: true });

/** Opens the project's board, the way the shell does for a project this spec knows only by id. */
async function openBoard(page: Page, projectId: string): Promise<void> {
  await page.evaluate((pid) => window.M?.go("project", pid, "board"), projectId);
  await expect(page.locator("[data-col]").first()).toBeVisible();
}

/** Opens the card's panel from its board, after a reload or at the start. */
async function openPanel(page: Page, projectId: string, text: string): Promise<void> {
  await openBoard(page, projectId);
  await cardOnBoard(page, text).click();
  await expect(action(page, "More actions")).toBeVisible();
}

/** The card of this title as the daemon has it, or undefined while the daemon has not caught up. */
const cardOf = async (request: APIRequestContext, projectId: string, text: string) =>
  (await boardViaApi(request, projectId)).find((one) => one.title === text);

test.describe("a card the daemon runs", () => {
  test("starts a card, holds it, wakes it, and moves it, over a reload", async ({
    page,
    request,
  }) => {
    const repo = makeRepo("e2e-hold");
    const name = uniqueName("hold");
    let id = "";
    try {
      const project = await createProjectViaApi(request, repo.dir, name);
      id = project.id;
      const stub = (await agentsViaApi(request))[0];
      expect(stub).toBeDefined();

      const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
      await openBoard(page, id);

      // Make the card the way a person does, with the stub agent the daemon runs.
      await page.evaluate(() => window.M?.newCard());
      await newCardDialog(page)
        .getByRole("combobox", { name: "Agent" })
        .selectOption(stub?.name ?? "");
      await newCardDialog(page).getByRole("textbox", { name: "Title" }).fill(TITLE);
      await newCardDialog(page).getByRole("button", { name: "Create card" }).click();
      await expect.poll(async () => (await cardOf(request, id, TITLE))?.title ?? "").toBe(TITLE);
      const cardId = (await cardOf(request, id, TITLE))?.id ?? "";
      expect(cardId).not.toBe("");

      // Start it: the daemon makes a worktree, runs the stub agent, and the card is working.
      await cardOnBoard(page, TITLE).click();
      await action(page, "Start card").click();
      await expect
        .poll(async () => (await cardViaApi(request, id, cardId))?.state, { timeout: START_MS })
        .toBe("working");

      // Pause holds the card between turns, and the daemon remembers it.
      await action(page, "Pause").click();
      await expect.poll(async () => (await cardViaApi(request, id, cardId))?.paused).toBe(true);

      // A paused card can sleep, and the panel then offers to resume the session.
      await action(page, "Sleep").click();
      await expect
        .poll(async () => (await cardViaApi(request, id, cardId))?.session)
        .toBe("asleep");
      await expect(action(page, "Resume session")).toBeVisible();

      // The hold is the daemon's, so a reload opens the card asleep rather than awake.
      await page.reload();
      await waitUntilOnline(page);
      await openPanel(page, id, TITLE);
      await expect(action(page, "Resume session")).toBeVisible();
      await expect(action(page, "Pause")).toHaveCount(0);

      // Waking it puts the session back to work.
      await action(page, "Resume session").click();
      await expect.poll(async () => (await cardViaApi(request, id, cardId))?.session).toBe("awake");

      // An allowed move: Working to Planning changes only the state, and the daemon takes it.
      await action(page, "More actions").click();
      await page.getByRole("menuitem", { name: /Move to planning/i }).click();
      await expect
        .poll(async () => (await cardViaApi(request, id, cardId))?.state)
        .toBe("planning");
      await expect(cardOnBoard(page, TITLE)).toBeVisible();

      await expectCleanScreen(page, problems, "light");
    } finally {
      if (id) await removeProjectViaApi(request, id);
    }
  });
});
