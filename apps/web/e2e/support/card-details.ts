import { expect, type Page } from "@playwright/test";

/** Card metadata and actions start folded on phones; settings have a separate disclosure. */
export async function revealCardSettings(page: Page): Promise<void> {
  await expect(page.getByRole("tab", { name: "Chat", exact: true })).toBeVisible();
  const disclosure = page.getByRole("button", { name: /^(Show details|Show card details)$/ });
  if (await disclosure.count()) await disclosure.click();
  const settings = page.locator(
    'button[aria-controls="card-agent-settings"][aria-expanded="false"]',
  );
  if (await settings.count()) await settings.click();
  await expect(page.getByRole("combobox", { name: "Agent" })).toBeVisible();
}
