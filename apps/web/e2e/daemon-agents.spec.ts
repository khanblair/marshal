import { expect, type Page, test } from "@playwright/test";
import { expectCleanScreen, openApp, SIZES, type Size, THEMES, type Theme } from "./support/app";
import { agentsViaApi, type WireAgent } from "./support/daemon-api";

const BUILT_IN = "Built-in agent";
/** The built-in agent is Marshal's own until the daemon lists it: five models, two of them without thinking. */
const BUILT_IN_MODELS = [
  "claude-sonnet-4-5",
  "gpt-5-mini",
  "deepseek-chat",
  "gemini-2.5-flash",
  "qwen2.5-coder:32b",
];
const BUILT_IN_WITHOUT_THINKING = ["deepseek-chat", "qwen2.5-coder:32b"];
const NOT_INSTALLED = "Codex is not installed. Install it with: npm install -g @openai/codex";
const UNTESTED = "Marshal has not been tested with Gemini CLI 9.9.9. It usually works.";

interface Case {
  page: Page;
  size: Size;
  theme: Theme;
}

const dialog = (page: Page) => page.getByRole("dialog", { name: /^New card in / });
const agentPicker = (page: Page) => dialog(page).getByRole("combobox", { name: "Agent" });
const optionTexts = (page: Page, name: string) =>
  page.getByRole("combobox", { name }).locator("option").allTextContents();

async function openNewCard(page: Page): Promise<void> {
  await page.evaluate(() => {
    window.M?.go("project", "api", "board");
    window.M?.newCard();
  });
  await expect(dialog(page)).toBeVisible();
}

/** Creates a card with this agent through the dialog, and opens it. Returns the card's key. */
async function createCardWith(page: Page, agent: string, title: string): Promise<string> {
  await agentPicker(page).selectOption(agent);
  await dialog(page).getByRole("textbox", { name: "Title" }).fill(title);
  await dialog(page).getByRole("button", { name: "Create card" }).click();
  await expect(dialog(page)).toHaveCount(0);
  const key = await page.evaluate(
    (text) => window.M?.S.cards.find((card) => card.title === text)?.id ?? "",
    title,
  );
  expect(key).not.toBe("");
  await page.evaluate((id) => window.M?.openCard(id), key);
  return key;
}

/** What the Thinking mode picker must do for a model, from the daemon's own flags. */
const offersThinking = (agent: WireAgent, model: string): boolean =>
  agent.capabilities.thinking && (agent.models.find((m) => m.id === model)?.thinking ?? false);

async function expectThinking(page: Page, offered: boolean): Promise<void> {
  await expect(page.getByRole("combobox", { name: "Thinking mode" })).toHaveCount(offered ? 1 : 0);
}

async function checkPickerListsCatalog({ page, size, theme }: Case, agents: WireAgent[]) {
  const problems = await openApp(page, size, { theme });
  await openNewCard(page);
  expect(await optionTexts(page, "Agent")).toEqual([...agents.map((a) => a.name), BUILT_IN]);
  // The stub reports every agent as installed, so nothing is disabled and no note is drawn.
  await expect(agentPicker(page).locator("option:disabled")).toHaveCount(0);
  await expect(agentPicker(page)).toHaveValue(agents[0]?.name ?? "");
  await expectCleanScreen(page, problems, theme);
}

async function checkModelsFollowAgent({ page, size, theme }: Case, agents: WireAgent[]) {
  const problems = await openApp(page, size, { theme });
  const [first, second] = agents;
  if (!first || !second) throw new Error("the daemon reports fewer than two agents");
  await openNewCard(page);
  const key = await createCardWith(page, second.name, `Agents e2e ${size.width} ${theme}`);
  // A card made with an agent starts on that agent's first model, and lists that agent's models.
  expect(await page.evaluate((id) => window.M?.card(id)?.model, key)).toBe(second.models[0]?.id);
  expect(await optionTexts(page, "Model")).toEqual(second.models.map((m) => m.id));
  await expectThinking(page, offersThinking(second, second.models[0]?.id ?? ""));

  for (const agent of agents) {
    await page.getByRole("combobox", { name: "Agent" }).selectOption(agent.name);
    expect(await optionTexts(page, "Model")).toEqual(agent.models.map((m) => m.id));
    await expect(page.getByRole("combobox", { name: "Model" })).toHaveValue(
      agent.models[0]?.id ?? "",
    );
    for (const model of agent.models) {
      await page.getByRole("combobox", { name: "Model" }).selectOption(model.id);
      await expectThinking(page, offersThinking(agent, model.id));
    }
  }
  await expectCleanScreen(page, problems, theme);
}

async function checkThinkingFollowsModel({ page, size, theme }: Case) {
  const problems = await openApp(page, size, { theme });
  await openNewCard(page);
  await createCardWith(page, BUILT_IN, `Thinking e2e ${size.width} ${theme}`);
  expect(await optionTexts(page, "Model")).toEqual(BUILT_IN_MODELS);
  for (const model of BUILT_IN_MODELS) {
    await page.getByRole("combobox", { name: "Model" }).selectOption(model);
    await expectThinking(page, !BUILT_IN_WITHOUT_THINKING.includes(model));
  }
  await expectCleanScreen(page, problems, theme);
}

/** Answers `/v1/agents` with the daemon's catalog, changed to have a missing and an untested agent. */
async function reportMissingAndUntested(page: Page): Promise<void> {
  await page.route("**/v1/agents", async (route) => {
    const response = await route.fetch();
    const catalog = (await response.json()) as { agents: WireAgent[] };
    catalog.agents = catalog.agents.map((agent) => {
      if (agent.kind === "codex") {
        return { ...agent, status: "missing", version: "", installHint: NOT_INSTALLED };
      }
      if (agent.kind === "gemini") return { ...agent, status: "untested", warning: UNTESTED };
      return agent;
    });
    await route.fulfill({ response, json: catalog });
  });
}

async function checkMissingAndUntested({ page, size, theme }: Case) {
  await reportMissingAndUntested(page);
  const problems = await openApp(page, size, { theme });
  await openNewCard(page);
  // A missing agent stays in the list, disabled, and its install command is on the screen.
  const codex = agentPicker(page).locator("option", { hasText: "Codex (not installed)" });
  await expect(codex).toHaveJSProperty("disabled", true);
  await expect(dialog(page).getByText(NOT_INSTALLED)).toBeVisible();
  // An untested agent can be picked, and its warning shows while it is.
  await expect(dialog(page).getByText(UNTESTED)).toHaveCount(0);
  await agentPicker(page).selectOption("Gemini CLI");
  await expect(dialog(page).getByText(UNTESTED)).toBeVisible();
  await expectCleanScreen(page, problems, theme);

  const key = await createCardWith(page, "Gemini CLI", `Untested e2e ${size.width} ${theme}`);
  expect(await page.evaluate((id) => window.M?.card(id)?.agent, key)).toBe("Gemini CLI");
  await expect(page.getByText(UNTESTED)).toBeVisible();
  await expect(page.getByText(NOT_INSTALLED)).toBeVisible();
  await expectCleanScreen(page, problems, theme);
}

async function checkOnboardingStatuses({ page, size, theme }: Case, agents: WireAgent[]) {
  await reportMissingAndUntested(page);
  const problems = await openApp(page, size, { theme });
  await page.evaluate(() => window.M?.set({ onboarding: true, obStep: 2 }));
  const rows = page.getByRole("dialog").getByRole("listitem");
  await expect(rows).toHaveCount(agents.length);
  await expect(rows.filter({ hasText: "Not installed" })).toContainText(NOT_INSTALLED);
  await expect(rows.filter({ hasText: "Gemini CLI" })).toContainText(UNTESTED);
  await expect(rows.getByText(BUILT_IN, { exact: true })).toHaveCount(0);
  await expectCleanScreen(page, problems, theme);
}

for (const size of SIZES) {
  for (const theme of THEMES) {
    test.describe(`${size.name} (${size.width} by ${size.height}), ${theme} theme`, () => {
      test("the New card dialog lists the agents the daemon reports, then the built-in agent", async ({
        page,
        request,
      }) => checkPickerListsCatalog({ page, size, theme }, await agentsViaApi(request)));
      test("the models follow the agent, and thinking follows the model, for every agent", async ({
        page,
        request,
      }) => checkModelsFollowAgent({ page, size, theme }, await agentsViaApi(request)));
      test("the built-in agent offers thinking except for the two models without it", ({ page }) =>
        checkThinkingFollowsModel({ page, size, theme }));
      test("an agent that is not installed is disabled with its install command, and an untested one shows its warning", ({
        page,
      }) => checkMissingAndUntested({ page, size, theme }));
      test("the onboarding agents screen shows each agent as found, untested, or not installed", async ({
        page,
        request,
      }) => checkOnboardingStatuses({ page, size, theme }, await agentsViaApi(request)));
    });
  }
}
