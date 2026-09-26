import { expect, test } from "@playwright/test";
import { BROWSER_NETWORK_NOISE, expectCleanScreen, openApp, SIZES } from "./support/app";
import { createProjectViaApi, removeProjectViaApi } from "./support/daemon-api";
import { makeRepo } from "./support/git";

/**
 * The command palette over the daemon's own answers: what is typed is sent to the daemon, and what it
 * finds opens from the row. The project is made for this spec, so the query can only match it.
 */
const DESKTOP = SIZES[2];
const RADIX = 36;
const RANDOM_START = 2;
const RANDOM_END = 8;

const uniqueName = (label: string): string =>
  `e2e-${label}-${Math.random().toString(RADIX).slice(RANDOM_START, RANDOM_END)}`;

test.describe("the command palette", () => {
  test("finds a project by name and opens it from the row", async ({ page, request }) => {
    const repo = makeRepo("e2e-palette");
    const name = uniqueName("palette");
    let id = "";
    try {
      const project = await createProjectViaApi(request, repo.dir, name);
      id = project.id;

      const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });

      // The top bar's search box opens the palette, as Cmd or Ctrl and K does.
      await page.getByRole("button", { name: "Search", exact: true }).click();
      const search = page.getByRole("textbox", {
        name: "Search actions, projects, cards, and settings",
      });
      await expect(search).toBeVisible();
      await search.fill(name);

      // The daemon matched the project, and its row opens the project's board.
      const row = page.getByRole("option").filter({ hasText: name }).first();
      await expect(row).toBeVisible();
      await row.click();
      await expect(page.getByRole("tab", { name: "Board", exact: true })).toHaveAttribute(
        "aria-selected",
        "true",
      );
      // The board of the project the row named: this one has no cards at all.
      await expect(page.getByText("This board has no cards yet.")).toBeVisible();

      await expectCleanScreen(page, problems, "light");
    } finally {
      if (id) await removeProjectViaApi(request, id);
    }
  });
});
