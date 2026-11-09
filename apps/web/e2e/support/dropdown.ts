import { expect, type Locator } from "@playwright/test";

/** Operates the public combobox/menu interaction, including the current custom Select. */
export async function pickDropdown(trigger: Locator, value: string): Promise<void> {
  if (await trigger.evaluate((node) => node.tagName === "SELECT")) {
    await trigger.selectOption(value);
    return;
  }
  await trigger.click();
  const option = trigger
    .locator("..")
    .locator(`[role="menuitemradio"][data-value=${JSON.stringify(value)}]`);
  await expect(option).toBeEnabled();
  await option.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
}

/** Reads all displayed rows while the menu is open, then closes it through its trigger. */
export async function dropdownOptions(trigger: Locator): Promise<string[]> {
  if (await trigger.evaluate((node) => node.tagName === "SELECT")) {
    return trigger.locator("option").allTextContents();
  }
  await trigger.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  const labels = await trigger.locator("..").getByRole("menuitemradio").allTextContents();
  await trigger.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
  return labels.map((label) => label.trim());
}

export async function dropdownValue(trigger: Locator): Promise<string> {
  return (await trigger.evaluate((node) => node.tagName === "SELECT"))
    ? trigger.inputValue()
    : trigger.locator("..").locator('input[type="hidden"]').inputValue();
}
