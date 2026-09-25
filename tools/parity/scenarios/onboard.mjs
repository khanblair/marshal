/**
 * Onboarding and the guided tour. Onboarding scenarios start from a first launch
 * (`onboarded: false`) and reach each of the five screens two ways: by setting `obStep`
 * and by clicking Continue. Tour scenarios start the tour on Home and set the step, or
 * click through it.
 */
import { set } from "../steps.mjs";

/** Long enough for the prototype's next animation frame and the 250 ms tour measurement. */
const FRAME_MS = 100;
const MEASURE_MS = 600;
const REAL_WAIT_MS = 60;
const PROJECT_STEP = 3;

/**
 * Runs the fake clock, then lets real time pass so React (the prototype) has drawn the frame and
 * web fonts used for the first time have loaded before the next click or the screenshot.
 */
async function flush(page, ms = FRAME_MS) {
  await page.clock.runFor(ms);
  await page.waitForTimeout(REAL_WAIT_MS);
  await page.evaluate(() => document.fonts.ready);
  await page.clock.runFor(FRAME_MS);
}

const button = (page, name) => page.getByRole("button", { name, exact: true });
const nameField = (page) => page.locator('input[autocomplete="name"]');

async function press(page, name) {
  await button(page, name).click();
  await flush(page);
}

async function pressRadio(page, name) {
  await page.getByRole("radio", { name }).click();
  await flush(page);
}

async function fillName(page, name = "Grace Hopper") {
  await nameField(page).fill(name);
  await flush(page);
}

/**
 * Continue from the welcome screen, with a name typed, until `step` is showing. The prototype
 * throws when Continue adds a project (its store only knows three projects' files), so it cannot
 * leave the project screen that way. Skip does, and the port does the same when Continue is used.
 */
async function continueTo(page, step) {
  for (let at = 0; at < step; at++) {
    if (at === 1) await fillName(page);
    await press(page, at === PROJECT_STEP ? "Skip" : "Continue");
  }
}

const onScreen = (step) => async (page) => {
  await set(page, { obStep: step });
  await flush(page);
};

const viaContinue = (step) => (page) => continueTo(page, step);

async function startTour(page) {
  await page.evaluate(() => window.M.startTour());
  await flush(page, MEASURE_MS);
}

const onTourStep = (step) => async (page) => {
  await startTour(page);
  await set(page, { tour: { step } });
  await flush(page, MEASURE_MS);
};

async function clickNext(page, times) {
  for (let i = 0; i < times; i++) {
    await button(page, "Next").click();
    await flush(page, MEASURE_MS);
  }
}

const SCREENS = ["welcome", "profile", "agents", "project", "control"];

const screens = SCREENS.map((name, step) => ({
  id: `onboard-step-${name}`,
  onboarded: false,
  steps: onScreen(step),
}));

const clicked = SCREENS.slice(1).map((name, index) => ({
  id: `onboard-click-${name}`,
  onboarded: false,
  steps: viaContinue(index + 1),
}));

const profile = [
  {
    id: "onboard-name-error",
    onboarded: false,
    steps: async (page) => {
      await onScreen(1)(page);
      await press(page, "Continue");
    },
  },
  {
    id: "onboard-name-typed",
    onboarded: false,
    steps: async (page) => {
      await onScreen(1)(page);
      await fillName(page);
      await page.getByPlaceholder("Optional").fill("grace@navy.mil");
      await page.locator("select").selectOption("Asia/Singapore");
      await flush(page);
    },
  },
  {
    id: "onboard-name-long",
    onboarded: false,
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await onScreen(1)(page);
      await fillName(page, "Alexandria Catherine Montgomery-Fitzwilliam");
    },
  },
  {
    id: "onboard-avatar-upload",
    onboarded: false,
    steps: async (page) => {
      await onScreen(1)(page);
      await press(page, "Upload image");
    },
  },
  ...["Light", "Dark", "System"].map((theme) => ({
    id: `onboard-theme-${theme.toLowerCase()}`,
    onboarded: false,
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await onScreen(1)(page);
      await pressRadio(page, theme);
    },
  })),
];

const agents = [
  {
    id: "onboard-keys-typed",
    onboarded: false,
    steps: async (page) => {
      await onScreen(2)(page);
      const fields = page.locator('input[type="password"]');
      await fields.nth(0).fill("sk-ant-api03-abcdefgh1234");
      await fields.nth(1).fill("sk-proj-short");
      await flush(page);
    },
  },
];

const project = [
  {
    id: "onboard-source-folder",
    onboarded: false,
    steps: async (page) => {
      await onScreen(3)(page);
      await pressRadio(page, /Pick a folder/);
    },
  },
  {
    id: "onboard-source-folder-typed",
    onboarded: false,
    steps: async (page) => {
      await onScreen(3)(page);
      await pressRadio(page, /Pick a folder/);
      await page.getByPlaceholder("~/code/my-repo").fill("~/code/acme-monorepo");
      await flush(page);
    },
  },
  {
    id: "onboard-source-github",
    onboarded: false,
    steps: async (page) => {
      await onScreen(3)(page);
      await pressRadio(page, /Clone from GitHub/);
    },
  },
  {
    id: "onboard-source-github-typed",
    onboarded: false,
    steps: async (page) => {
      await onScreen(3)(page);
      await pressRadio(page, /Clone from GitHub/);
      await page
        .getByPlaceholder("https://github.com/owner/repo")
        .fill("https://github.com/acme/web.git");
      await flush(page);
    },
  },
  {
    id: "onboard-source-sample",
    onboarded: false,
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await onScreen(3)(page);
      await pressRadio(page, /Pick a folder/);
      await pressRadio(page, /Use a sample project/);
    },
  },
];

const control = ["Telegram", "Discord"].map((app) => ({
  id: `onboard-chat-${app.toLowerCase()}`,
  onboarded: false,
  steps: async (page) => {
    await onScreen(4)(page);
    await press(page, `Connect ${app}`);
  },
}));

control.push({
  id: "onboard-chat-both",
  onboarded: false,
  steps: async (page) => {
    await onScreen(4)(page);
    await press(page, "Connect Telegram");
    await press(page, "Connect Discord");
  },
});

const finish = [
  {
    id: "onboard-finish",
    onboarded: false,
    steps: async (page) => {
      await continueTo(page, 4);
      await press(page, "Open Marshal");
      await flush(page, MEASURE_MS);
    },
  },
  {
    id: "onboard-skip-all",
    onboarded: false,
    steps: async (page) => {
      for (let step = 0; step < 5; step++) await press(page, "Skip");
      await flush(page, MEASURE_MS);
    },
  },
];

const interaction = [
  {
    id: "onboard-back",
    onboarded: false,
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await continueTo(page, 3);
      await press(page, "Back");
    },
  },
  ...["Skip", "Continue"].map((name) => ({
    id: `onboard-hover-${name.toLowerCase()}`,
    onboarded: false,
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await button(page, name).hover();
      await flush(page);
    },
  })),
  {
    id: "onboard-focus-tab",
    onboarded: false,
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await onScreen(1)(page);
      await page.keyboard.press("Tab");
      await flush(page);
    },
  },
];

const TOUR_STEP_COUNT = 9;

const tourSteps = Array.from({ length: TOUR_STEP_COUNT }, (_, step) => ({
  id: `onboard-tour-${step + 1}`,
  steps: onTourStep(step),
}));

const tourInteraction = [
  {
    id: "onboard-tour-next-3",
    steps: async (page) => {
      await startTour(page);
      await clickNext(page, 3);
    },
  },
  {
    id: "onboard-tour-back",
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await startTour(page);
      await clickNext(page, 2);
      await button(page, "Back").click();
      await flush(page, MEASURE_MS);
    },
  },
  {
    id: "onboard-tour-enter",
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await startTour(page);
      await page.keyboard.press("Enter");
      await flush(page, MEASURE_MS);
      await page.keyboard.press("Enter");
      await flush(page, MEASURE_MS);
    },
  },
  {
    id: "onboard-tour-finish",
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await startTour(page);
      await clickNext(page, TOUR_STEP_COUNT - 1);
      await button(page, "Finish tour").click();
      await flush(page, MEASURE_MS);
    },
  },
  {
    id: "onboard-tour-escape",
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await startTour(page);
      await page.keyboard.press("Escape");
      await flush(page, MEASURE_MS);
    },
  },
  {
    id: "onboard-tour-skip",
    sizes: ["desktop", "phone"],
    steps: async (page) => {
      await startTour(page);
      await button(page, "Skip tour").click();
      await flush(page, MEASURE_MS);
    },
  },
];

export default [
  ...screens,
  ...clicked,
  ...profile,
  ...agents,
  ...project,
  ...control,
  ...finish,
  ...interaction,
  ...tourSteps,
  ...tourInteraction,
];
