import { expect, type Page, test } from "@playwright/test";
import {
  BROWSER_NETWORK_NOISE,
  expectCleanScreen,
  openApp,
  SIZES,
  waitUntilOnline,
} from "./support/app";
import {
  chatMessagesViaApi,
  chatsViaApi,
  createProjectViaApi,
  removeProjectViaApi,
} from "./support/daemon-api";
import { makeRepo } from "./support/git";

/**
 * A project chat: made from the Chats view, spoken to from its composer, and read again from the
 * daemon after a reload. The chat's session runs the stub agent, so nothing here depends on a real
 * model, and every answer is read back from the daemon rather than from the screen alone.
 */
const DESKTOP = SIZES[2];
const RADIX = 36;
const RANDOM_START = 2;
const RANDOM_END = 8;

const uniqueName = (label: string): string =>
  `e2e-${label}-${Math.random().toString(RADIX).slice(RANDOM_START, RANDOM_END)}`;

const MESSAGE = "Add a health check endpoint to the sample service";

/** Opens a project's Chats view, the way the view switcher does. */
async function openChats(page: Page, projectId: string): Promise<void> {
  await page.evaluate((pid) => window.M?.go("project", pid, "chat"), projectId);
  await expect(page.getByRole("tab", { name: "Chats", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
}

test.describe("a project chat", () => {
  test("starts a chat, sends a message, and the daemon keeps it over a reload", async ({
    page,
    request,
  }) => {
    const repo = makeRepo("e2e-chats");
    const name = uniqueName("chats");
    let id = "";
    try {
      const project = await createProjectViaApi(request, repo.dir, name);
      id = project.id;

      const problems = await openApp(page, DESKTOP, { allow: BROWSER_NETWORK_NOISE });
      await openChats(page, id);

      // A new chat with the Orchestrator, which is what the form starts on.
      await page.getByRole("button", { name: "New chat" }).click();
      await page.getByRole("button", { name: "Start chat" }).click();

      // The message goes to the chat's own session, and stands in the thread at once.
      await page.getByRole("textbox", { name: "Message" }).fill(MESSAGE);
      await page.getByRole("button", { name: "Send message" }).click();
      await expect(page.getByText(MESSAGE).first()).toBeVisible();

      // The daemon has the chat and the message, so a second device reads the same.
      await expect.poll(async () => (await chatsViaApi(request, id)).length).toBe(1);
      const chatId = (await chatsViaApi(request, id))[0]?.id ?? "";
      expect(chatId).not.toBe("");
      await expect
        .poll(async () =>
          (await chatMessagesViaApi(request, chatId)).some((m) => m.text === MESSAGE),
        )
        .toBe(true);

      // A reload opens the chat again from the daemon, with what was said still in it.
      await page.reload();
      await waitUntilOnline(page);
      await openChats(page, id);
      const title = (await chatsViaApi(request, id))[0]?.title ?? "";
      await page.getByRole("button", { name: new RegExp(`^${title}`) }).click();
      await expect(page.getByText(MESSAGE).first()).toBeVisible();

      await expectCleanScreen(page, problems, "light");
    } finally {
      if (id) await removeProjectViaApi(request, id);
    }
  });
});
