import { existsSync } from "node:fs";
import { join } from "node:path";
import { expect, type Page, test } from "@playwright/test";
import {
  BROWSER_NETWORK_NOISE,
  expectCleanScreen,
  openApp,
  SIZES,
  THEMES,
  waitUntilOnline,
} from "./support/app";
import { createProjectViaApi, getProjectViaApi, removeProjectViaApi } from "./support/daemon-api";
import { makeFolder, makeRepo } from "./support/git";

const DESKTOP = SIZES[2];
const NOT_A_REPOSITORY =
  "That folder is not a Git repository. Choose the top folder of a repository.";
const ALREADY_ADDED = "That repository is already a project in Marshal.";

const RADIX = 36;
const RANDOM_START = 2;
const RANDOM_END = 8;

/** A name no other spec or run uses, so specs that run side by side never see each other's projects. */
const uniqueName = (label: string): string =>
  `e2e-${label}-${Math.random().toString(RADIX).slice(RANDOM_START, RANDOM_END)}`;

const sidebarRow = (page: Page, name: string) =>
  page.getByRole("button", { name: new RegExp(`^${name}: `) });

async function openNewProject(page: Page) {
  await page.getByRole("button", { name: "New project" }).click();
  await expect(page.getByRole("dialog", { name: "New project" })).toBeVisible();
}

async function fillNewProject(page: Page, folder: string, name: string) {
  await page.getByRole("textbox", { name: /Repository folder/ }).fill(folder);
  await page.getByRole("textbox", { name: "Name" }).fill(name);
}

async function openRowMenu(page: Page, name: string) {
  await page.getByRole("button", { name: `More actions for ${name}` }).click();
}

test.describe("adding a project on the dev daemon", () => {
  test("adds a project from a real folder, shows it in the sidebar, and opens its empty board", async ({
    page,
    request,
  }) => {
    const repo = makeRepo("e2e-add");
    const name = uniqueName("add");
    let id = "";
    try {
      const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
      await openNewProject(page);
      await fillNewProject(page, repo.dir, name);
      await page.getByRole("button", { name: "Add project" }).click();

      await expect(page.getByRole("dialog", { name: "New project" })).toHaveCount(0);
      await expect(page.getByText("Project added")).toBeVisible();
      await expect(sidebarRow(page, name)).toBeVisible();
      id = await page.evaluate(
        (projectName) => window.M?.S.projects.find((p) => p.name === projectName)?.id ?? "",
        name,
      );
      expect(id).not.toBe("");
      // Its board is open, and it is empty: no mock card belongs to a project that is new.
      await expect(page.locator("[data-app-root]")).toHaveAttribute("data-connection", "online");
      expect(await page.evaluate(() => window.M?.S.route.pid)).toBe(id);
      await expect(page.getByText("This board has no cards yet.")).toBeVisible();
      expect(await page.evaluate((pid) => window.M?.cardsOf(pid).length, id)).toBe(0);
      expect((await getProjectViaApi(request, id))?.name).toBe(name);
      await expectCleanScreen(page, problems, "light");
    } finally {
      if (id) await removeProjectViaApi(request, id);
      repo.remove();
    }
  });

  test("refuses the same folder a second time in plain words, and keeps the dialog and its fields", async ({
    page,
    request,
  }) => {
    const repo = makeRepo("e2e-dup");
    const name = uniqueName("dup");
    const project = await createProjectViaApi(request, repo.dir, name);
    try {
      const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
      await expect(sidebarRow(page, name)).toBeVisible();
      await openNewProject(page);
      await fillNewProject(page, repo.dir, "again");
      await page.getByRole("button", { name: "Add project" }).click();

      await expect(page.getByRole("alert")).toHaveText(ALREADY_ADDED);
      await expect(page.getByRole("dialog", { name: "New project" })).toBeVisible();
      await expect(page.getByRole("textbox", { name: /Repository folder/ })).toHaveValue(repo.dir);
      await expect(page.getByRole("textbox", { name: "Name" })).toHaveValue("again");
      await expect(page.getByRole("button", { name: "Add project" })).toBeEnabled();
      expect(
        await page.evaluate(() => window.M?.S.projects.filter((p) => p.name === "again").length),
      ).toBe(0);
      await expectCleanScreen(page, problems, "light");
    } finally {
      await removeProjectViaApi(request, project.id);
      repo.remove();
    }
  });
});

test.describe("renaming a project on the dev daemon", () => {
  test("renames a project from the sidebar and the daemon keeps the name", async ({
    page,
    request,
  }) => {
    const repo = makeRepo("e2e-rename");
    const name = uniqueName("rename");
    const renamed = `${name}-renamed`;
    const project = await createProjectViaApi(request, repo.dir, name);
    try {
      await openApp(page, DESKTOP);
      await expect(sidebarRow(page, name)).toBeVisible();
      await openRowMenu(page, name);
      await page.getByRole("menuitem", { name: "Rename" }).click();
      const field = page.getByRole("textbox", { name: "Project name" });
      await field.fill(renamed);
      await field.press("Enter");

      await expect(sidebarRow(page, renamed)).toBeVisible();
      await expect
        .poll(async () => (await getProjectViaApi(request, project.id))?.name)
        .toBe(renamed);
      await page.reload();
      await waitUntilOnline(page);
      await expect(sidebarRow(page, renamed)).toBeVisible();
    } finally {
      await removeProjectViaApi(request, project.id);
      repo.remove();
    }
  });
});

test.describe("changing a project's settings on the dev daemon", () => {
  test("changes the dev command in Project settings and it stays after a reload", async ({
    page,
    request,
  }) => {
    const repo = makeRepo("e2e-dev");
    const name = uniqueName("dev");
    const project = await createProjectViaApi(request, repo.dir, name);
    try {
      await openApp(page, DESKTOP);
      await expect(sidebarRow(page, name)).toBeVisible();
      await openRowMenu(page, name);
      await page.getByRole("menuitem", { name: "Project settings" }).click();
      const dev = page.getByLabel("Dev command", { exact: false });
      await dev.fill("go run ./cmd/web");
      await page.getByRole("button", { name: "Save project" }).click();
      await expect(page.getByText("Project saved")).toBeVisible();
      await expect
        .poll(async () => (await getProjectViaApi(request, project.id))?.devCommand)
        .toBe("go run ./cmd/web");

      await page.reload();
      await waitUntilOnline(page);
      await page.evaluate(
        (id) => window.M?.set({ settingsSection: "project", settingsPid: id }),
        project.id,
      );
      await page.evaluate(() => window.M?.go("settings"));
      await expect(page.getByLabel("Dev command", { exact: false })).toHaveValue(
        "go run ./cmd/web",
      );
      await expect(page.getByRole("button", { name: "Save project" })).toBeDisabled();
    } finally {
      await removeProjectViaApi(request, project.id);
      repo.remove();
    }
  });
});

test.describe("removing and watching projects on the dev daemon", () => {
  test("removes a project keeping its branches, and never touches the repository folder", async ({
    page,
    request,
  }) => {
    const repo = makeRepo("e2e-remove");
    const name = uniqueName("remove");
    const project = await createProjectViaApi(request, repo.dir, name);
    const before = repo.entries();
    try {
      await openApp(page, DESKTOP);
      await expect(sidebarRow(page, name)).toBeVisible();
      await openRowMenu(page, name);
      await page.getByRole("menuitem", { name: "Remove" }).click();
      const dialog = page.getByRole("alertdialog", { name: `Remove ${name}` });
      await expect(dialog).toBeVisible();
      await expect(dialog).toContainText("The repository on disk is never deleted.");

      // The dialog always sends both choices; with no body the daemon would keep nothing.
      const sent = page.waitForRequest(
        (req) => req.method() === "DELETE" && req.url().endsWith(`/v1/projects/${project.id}`),
      );
      await dialog.getByRole("button", { name: "Remove project" }).click();
      expect((await sent).postDataJSON()).toEqual({ keepBranches: true, keepMemory: true });

      await expect(sidebarRow(page, name)).toHaveCount(0);
      await expect(page.getByText("Project removed")).toBeVisible();
      await expect.poll(() => getProjectViaApi(request, project.id)).toBeNull();
      // The folder, its files, and its Git repository are as they were.
      expect(existsSync(join(repo.dir, ".git"))).toBe(true);
      expect(existsSync(join(repo.dir, "README.md"))).toBe(true);
      expect(existsSync(join(repo.dir, "src", "main.go"))).toBe(true);
      expect(repo.entries()).toEqual(before);
      expect(repo.status()).toBe("");
    } finally {
      await removeProjectViaApi(request, project.id);
      repo.remove();
    }
  });
});

test.describe("the fixture and other devices", () => {
  test("the mobile fixture project is a monorepo, and its packages are on disk", async ({
    page,
  }) => {
    await openApp(page, DESKTOP);
    const mobile = await page.evaluate(() => {
      const project = window.M?.proj("mobile");
      return project
        ? { name: project.name, path: project.path, packages: project.packages ?? [] }
        : null;
    });
    expect(mobile?.name).toBe("mobile-app");
    expect(mobile?.packages.length).toBeGreaterThan(0);
    for (const folder of mobile?.packages ?? []) {
      expect(existsSync(join(mobile?.path ?? "", folder))).toBe(true);
    }
  });

  test("a project added in one page appears in a second page with no reload", async ({
    browser,
    request,
  }) => {
    const repo = makeRepo("e2e-live");
    const name = uniqueName("live");
    const context = await browser.newContext({ baseURL: `http://localhost:5299` });
    let id = "";
    try {
      const first = await context.newPage();
      const second = await context.newPage();
      await openApp(first, DESKTOP);
      await openApp(second, DESKTOP);
      await second.evaluate(() => Reflect.set(window, "e2eSurvivor", 1));

      await openNewProject(first);
      await fillNewProject(first, repo.dir, name);
      await first.getByRole("button", { name: "Add project" }).click();
      await expect(sidebarRow(first, name)).toBeVisible();

      await expect(sidebarRow(second, name)).toBeVisible();
      id = await second.evaluate(
        (projectName) => window.M?.S.projects.find((p) => p.name === projectName)?.id ?? "",
        name,
      );
      expect(await second.evaluate(() => Reflect.get(window, "e2eSurvivor"))).toBe(1);
      // Its screen state was made for it in the page that only heard about it.
      expect(await second.evaluate((pid) => window.M?.S.filters[pid], id)).toEqual([]);

      // Removed in the second page's daemon, it disappears from the first without a reload as well.
      await removeProjectViaApi(request, id);
      await expect(sidebarRow(first, name)).toHaveCount(0);
      await expect(sidebarRow(second, name)).toHaveCount(0);
    } finally {
      if (id) await removeProjectViaApi(request, id);
      await context.close();
      repo.remove();
    }
  });
});

for (const size of [SIZES[0], SIZES[2]]) {
  test(`draws Home with no project, and a way to add one, when the daemon has none (${size.name} ${size.width})`, async ({
    page,
  }) => {
    // Only the list is answered empty; every other call goes to the real daemon.
    await page.route("**/v1/projects", (route) =>
      route.request().method() === "GET"
        ? route.fulfill({
            contentType: "application/json",
            body: JSON.stringify({ projects: [], serverTime: new Date().toISOString() }),
          })
        : route.fallback(),
    );
    const problems = await openApp(page, size, { online: false });
    await expect(page.locator("[data-app-root]")).toHaveAttribute("data-connection", "online");
    expect(await page.evaluate(() => window.M?.S.projects.length)).toBe(0);
    expect(await page.evaluate(() => window.M?.S.cards.length)).toBe(0);
    await expect(page.getByRole("banner")).toBeVisible();
    if (size.name === "desktop") {
      await expect(page.getByRole("button", { name: "New project" })).toBeVisible();
    }
    await expectCleanScreen(page, problems, "light");
  });
}

for (const size of SIZES) {
  for (const theme of THEMES) {
    test(`shows the daemon's refusal in the New project dialog and stays clean (${size.name} ${size.width}, ${theme})`, async ({
      page,
    }) => {
      const folder = makeFolder("e2e-plain");
      try {
        const problems = await openApp(page, size, { theme, allow: BROWSER_NETWORK_NOISE });
        await page.evaluate(() =>
          window.M?.set({
            newProject: { source: "folder", path: "", url: "", name: "", branch: "" },
          }),
        );
        await expect(page.getByRole("dialog", { name: "New project" })).toBeVisible();
        await fillNewProject(page, folder.dir, "plain-folder");
        await page.getByRole("button", { name: "Add project" }).click();
        await expect(page.getByRole("alert")).toHaveText(NOT_A_REPOSITORY);
        await expect(page.getByRole("dialog", { name: "New project" })).toBeVisible();
        await expectCleanScreen(page, problems, theme);
      } finally {
        folder.remove();
      }
    });
  }
}
