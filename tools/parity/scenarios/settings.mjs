/**
 * Settings scenarios: every section, each role, every edit state of the forms (profile, project,
 * role, provider key, limits, schedule), integration states, the confirm dialogs, the theme cards,
 * and drafts that survive a section switch. Sections run at all three sizes in both themes; the
 * edit states run at desktop and phone in light, because their logic does not depend on either.
 */
import { settle } from "../lib.mjs";
import { go, set } from "../steps.mjs";

const EDIT_SIZES = ["desktop", "phone"];
const EDIT = { sizes: EDIT_SIZES, themes: ["light"] };
const DESKTOP = { sizes: ["desktop"], themes: ["light"] };
const SECTIONS = [
  "profile",
  "general",
  "project",
  "roles",
  "providers",
  "limits",
  "schedules",
  "integrations",
  "shortcuts",
  "help",
];
const ROLES = [
  "Orchestrator",
  "Worker",
  "Reviewer",
  "Integrator",
  "Tester",
  "Docs writer",
  "Security checker",
  "UI checker",
];
const NEAR_LIMIT_SHARE = 0.85;
const OVER_LIMIT_SHARE = 0.9;
const LONG_TEXT =
  "Review every change against the acceptance checks, leave a specific comment on each problem, and keep going until the whole diff has been read once from top to bottom.";

/**
 * Runs the steps in order, so a scenario can open a section and then change its state. The
 * prototype draws on a timer, so each step waits for the frozen clock to flush before the next.
 */
const chain =
  (...steps) =>
  async (page, ctx) => {
    for (const step of steps) {
      await step(page, ctx);
      await settle(page);
    }
  };

/** Opens a settings section. `extra` sets more store fields first (a role, a project). */
const open = (section, extra = {}) => async (page) => {
  await set(page, { settingsSection: section, ...extra });
  await go(page, "settings");
};

/** The Settings view's own root: the section list's parent, so the shell's controls do not match. */
const view = (page) => page.getByRole("navigation", { name: "Settings sections" }).locator("..");
const button = (page, name, nth = 0) =>
  view(page).getByRole("button", { name, exact: true }).nth(nth);
const press = (name, nth = 0) => (page) => button(page, name, nth).click();
const fill = (label, text, nth = 0) => (page) => view(page).getByLabel(label).nth(nth).fill(text);
const choose = (label, value, nth = 0) => (page) =>
  view(page).getByLabel(label, { exact: true }).nth(nth).selectOption(value);
const tick = (label) => (page) => view(page).getByLabel(label).check();
const goSection = (name) => (page) =>
  page.getByRole("navigation", { name: "Settings sections" }).getByRole("button", { name }).click();
const clearToasts = (page) => set(page, { toasts: [] });

/** Sets a store field from the page, for states no click reaches. */
const evaluate = (fn, arg) => (page) => page.evaluate(fn, arg);

/** Fills a limit with a share of what the scope uses now, to draw the near and over states. */
const limitFromUsage = (label, nth, share, read) => async (page) => {
  const used = await page.evaluate(read);
  await view(page).getByLabel(label).nth(nth).fill((used / share).toFixed(2));
};

const base = SECTIONS.map((section) => ({ id: `settings-${section}`, steps: open(section) }));

const profile = [
  {
    id: "settings-profile-name-empty",
    ...EDIT,
    steps: chain(open("profile"), fill("Name", "")),
  },
  {
    id: "settings-profile-edited",
    ...EDIT,
    steps: chain(
      open("profile"),
      fill("Name", "Ada Lovelace Byron"),
      fill("Email", "ada@kolaborate.co"),
      choose("Time zone", "Asia/Singapore"),
    ),
  },
  {
    id: "settings-profile-saved",
    ...EDIT,
    steps: chain(open("profile"), fill("Name", "Grace Hopper"), press("Save profile")),
  },
  { id: "settings-profile-pairing", ...EDIT, steps: chain(open("profile"), press("Pair a device")) },
  {
    id: "settings-profile-no-devices",
    ...EDIT,
    steps: chain(
      open("profile"),
      evaluate(() => window.M.set({ profile: { ...window.M.S.profile, devices: [] } })),
    ),
  },
  {
    id: "settings-profile-remove-device-dialog",
    ...EDIT,
    steps: chain(open("profile"), press("Remove device")),
  },
  {
    id: "settings-profile-upload",
    ...DESKTOP,
    steps: chain(open("profile"), press("Upload image")),
  },
];

const general = [
  { id: "settings-general-dark", ...DESKTOP, steps: chain(open("general"), pressRadio("Dark")) },
  { id: "settings-general-light", ...DESKTOP, steps: chain(open("general"), pressRadio("Light")) },
  {
    id: "settings-general-system",
    ...DESKTOP,
    steps: chain(open("general"), pressRadio("Dark"), pressRadio("System")),
  },
  {
    id: "settings-general-sessions",
    ...EDIT,
    steps: chain(
      open("general"),
      choose("Sleep idle cards after", "60"),
      choose("After a restart", "Show a resume button on each card"),
      choose("Sleep warnings go to", "In app and Discord"),
    ),
  },
  {
    id: "settings-general-custom-theme",
    ...DESKTOP,
    steps: chain(open("general"), press("Create custom theme")),
  },
];

function pressRadio(name) {
  return (page) => view(page).getByRole("radio", { name }).click();
}

const project = [
  { id: "settings-project-web", steps: open("project", { settingsPid: "web" }) },
  {
    id: "settings-project-mobile",
    sizes: ["desktop", "tablet", "phone"],
    steps: open("project", { settingsPid: "mobile" }),
  },
  {
    id: "settings-project-pick",
    ...EDIT,
    steps: chain(open("project"), choose("Project", "web")),
  },
  {
    id: "settings-project-edited",
    ...EDIT,
    steps: chain(
      open("project"),
      fill("Name", "gateway"),
      fill("Default branch", "develop"),
      fill("Dev command", "go run ./cmd/dev"),
      tick("Lock bypass permissions"),
    ),
  },
  {
    id: "settings-project-saved",
    ...EDIT,
    steps: chain(open("project"), fill("Default branch", "develop"), press("Save project")),
  },
  {
    id: "settings-project-empty-name",
    ...EDIT,
    steps: chain(open("project"), fill("Name", ""), press("Save project")),
  },
  {
    id: "settings-project-remove-dialog",
    ...EDIT,
    steps: chain(open("project", { settingsPid: "web" }), press("Remove project")),
  },
];

const roles = [
  ...ROLES.map((name) => ({
    id: `settings-roles-${name.toLowerCase().replace(/\s+/g, "-")}`,
    ...(name === "Worker" ? {} : DESKTOP),
    steps: open("roles", { roleSel: name }),
  })),
  {
    id: "settings-roles-edited",
    ...EDIT,
    steps: chain(
      open("roles"),
      fill("Name", "Builder"),
      fill("Description", "Builds features end to end"),
      fill("Instructions", LONG_TEXT),
      fill("Time limit", "90"),
      fill("Cost limit", "7.5"),
      fill("Round limit", "20"),
    ),
  },
  {
    id: "settings-roles-saved",
    ...EDIT,
    steps: chain(open("roles"), fill("Description", "Builds features"), press("Save role")),
  },
  {
    id: "settings-roles-agent-change",
    ...EDIT,
    steps: chain(open("roles", { roleSel: "Tester" }), choose("Agent", "Gemini CLI")),
  },
  {
    id: "settings-roles-weak-model",
    ...EDIT,
    steps: chain(
      open("roles", { roleSel: "Reviewer" }),
      choose("Agent", "Built-in agent"),
      choose("Model", "gpt-5-mini"),
    ),
  },
  {
    id: "settings-roles-weak-integrator",
    ...DESKTOP,
    steps: chain(
      open("roles", { roleSel: "Integrator" }),
      choose("Agent", "Claude Code"),
      choose("Model", "claude-haiku-4-5"),
    ),
  },
  { id: "settings-roles-new", ...EDIT, steps: chain(open("roles"), press("New role")) },
  {
    id: "settings-roles-duplicate",
    ...EDIT,
    steps: chain(open("roles", { roleSel: "Reviewer" }), press("Duplicate")),
  },
  {
    id: "settings-roles-reset-dialog",
    ...EDIT,
    steps: chain(open("roles"), press("Reset to starter")),
  },
  {
    id: "settings-roles-delete-dialog",
    ...EDIT,
    steps: chain(open("roles"), press("New role"), press("Delete role")),
  },
  {
    id: "settings-roles-delete-done",
    ...DESKTOP,
    steps: chain(open("roles"), press("New role"), press("Delete role"), confirmDialog),
  },
  {
    id: "settings-roles-draft-kept",
    ...DESKTOP,
    steps: chain(open("roles"), fill("Name", "Builder"), goSection("Profile"), goSection("Roles")),
  },
  {
    id: "settings-roles-select-clears-draft",
    ...DESKTOP,
    steps: chain(
      open("roles"),
      fill("Name", "Builder"),
      (page) => view(page).getByRole("option", { name: /^Reviewer/ }).click(),
    ),
  },
];

/** Runs the confirm dialog's action, as its button does. */
async function confirmDialog(page) {
  await page.evaluate(() => window.M.S.dialog?.run());
  await set(page, { dialog: null });
}

const providers = [
  { id: "settings-providers-edit", ...EDIT, steps: chain(open("providers"), press("Change key")) },
  {
    id: "settings-providers-add",
    ...EDIT,
    steps: chain(open("providers"), press("Add key")),
  },
  {
    id: "settings-providers-local",
    ...EDIT,
    steps: chain(open("providers"), press("Change URL")),
  },
  {
    id: "settings-providers-invalid-edit",
    ...EDIT,
    steps: chain(open("providers"), press("Change key", 3)),
  },
  {
    id: "settings-providers-short-key",
    ...EDIT,
    steps: chain(open("providers"), press("Change key"), fill("Anthropic API key", "sk-short"), enter),
  },
  {
    id: "settings-providers-short-url",
    ...DESKTOP,
    steps: chain(open("providers"), press("Change URL"), fill("Server URL", "http://x"), enter),
  },
  {
    id: "settings-providers-saved",
    ...EDIT,
    steps: chain(
      open("providers"),
      press("Add key"),
      fill("DeepSeek API key", "sk-deepseek-1234567890"),
      press("Save key"),
    ),
  },
  {
    id: "settings-providers-fixed",
    ...DESKTOP,
    steps: chain(
      open("providers"),
      press("Change key", 3),
      fill("OpenRouter API key", "sk-or-v1-abcdefghijklmnop"),
      press("Save key"),
    ),
  },
  {
    id: "settings-providers-cancel",
    ...DESKTOP,
    steps: chain(open("providers"), press("Add key"), press("Cancel")),
  },
  {
    id: "settings-providers-two-open",
    ...DESKTOP,
    steps: chain(open("providers"), press("Add key"), press("Change URL")),
  },
];

function enter(page) {
  return page.keyboard.press("Enter");
}

const usedToday = () => window.M.costs(null).today;
const usedApiMonth = () => window.M.costs("api").month;
const awakeApi = () => window.M.awake("api").length;

const limits = [
  {
    id: "settings-limits-edited",
    ...EDIT,
    steps: chain(open("limits"), fill("Daily cost limit", "30"), fill("Awake card limit", "12", 1)),
  },
  {
    id: "settings-limits-zero",
    ...EDIT,
    steps: chain(open("limits"), fill("Daily cost limit", "0")),
  },
  {
    id: "settings-limits-near",
    ...DESKTOP,
    steps: chain(open("limits"), limitFromUsage("Daily cost limit", 0, NEAR_LIMIT_SHARE, usedToday)),
  },
  {
    id: "settings-limits-over",
    ...DESKTOP,
    steps: chain(open("limits"), limitFromUsage("Daily cost limit", 0, OVER_LIMIT_SHARE, usedToday)),
  },
  {
    id: "settings-limits-month-over",
    ...DESKTOP,
    steps: chain(
      open("limits"),
      limitFromUsage("Monthly cost limit", 1, OVER_LIMIT_SHARE, usedApiMonth),
    ),
  },
  {
    id: "settings-limits-awake-over",
    ...DESKTOP,
    steps: chain(open("limits"), limitFromUsage("Awake card limit", 1, OVER_LIMIT_SHARE, awakeApi)),
  },
  {
    id: "settings-limits-discard",
    ...DESKTOP,
    steps: chain(open("limits"), fill("Daily cost limit", "30"), press("Discard changes")),
  },
  {
    id: "settings-limits-saved",
    ...EDIT,
    steps: chain(open("limits"), fill("Daily cost limit", "20"), press("Save limits")),
  },
  {
    id: "settings-limits-blocked",
    ...EDIT,
    steps: chain(open("limits"), fill("Daily cost limit", "0"), press("Save limits")),
  },
  {
    id: "settings-limits-raise-dialog",
    ...EDIT,
    steps: chain(
      open("limits"),
      fill("Daily cost limit", "40"),
      fill("Monthly cost limit", "500", 2),
      press("Save limits"),
    ),
  },
  {
    id: "settings-limits-raise-done",
    ...DESKTOP,
    steps: chain(
      open("limits"),
      fill("Daily cost limit", "40"),
      press("Save limits"),
      confirmDialog,
    ),
  },
];

const schedules = [
  {
    id: "settings-schedules-deeplink",
    ...EDIT,
    steps: open("schedules", { schedEdit: "s3" }),
  },
  { id: "settings-schedules-edit", ...EDIT, steps: chain(open("schedules"), press("Edit")) },
  {
    id: "settings-schedules-edit-interval",
    ...DESKTOP,
    steps: chain(open("schedules"), press("Edit", 4)),
  },
  {
    id: "settings-schedules-edit-close",
    ...DESKTOP,
    steps: chain(open("schedules"), press("Edit"), press("Close")),
  },
  {
    id: "settings-schedules-when-error",
    ...EDIT,
    steps: chain(
      open("schedules"),
      press("Edit"),
      fill("When", "tomorrow at noon"),
      press("Save schedule"),
    ),
  },
  {
    id: "settings-schedules-saved",
    ...EDIT,
    steps: chain(
      open("schedules"),
      press("Edit", 2),
      fill("Name", "Check issues daily"),
      fill("When", "On weekdays at 10:00"),
      choose("Trigger", "Event"),
      choose("If the machine was asleep", "Skip"),
      fill("Action", "Send a message to the Reviewer"),
      press("Save schedule"),
    ),
  },
  {
    id: "settings-schedules-cancel",
    ...DESKTOP,
    steps: chain(open("schedules"), press("Edit"), fill("Name", "Changed"), press("Cancel")),
  },
  {
    id: "settings-schedules-new",
    ...EDIT,
    steps: chain(open("schedules"), press("New schedule")),
  },
  {
    id: "settings-schedules-toggle-off",
    ...EDIT,
    steps: chain(open("schedules"), (page) =>
      view(page).getByRole("switch", { name: "Turn off Morning brief" }).click(),
    ),
  },
  {
    id: "settings-schedules-toggle-on",
    ...DESKTOP,
    steps: chain(open("schedules"), press("New schedule"), press("Close"), (page) =>
      view(page).getByRole("switch", { name: "Turn on New schedule" }).click(),
    ),
  },
  {
    id: "settings-schedules-two-errors",
    ...DESKTOP,
    steps: chain(
      open("schedules"),
      press("Edit"),
      fill("When", "never"),
      press("Save schedule"),
      press("New schedule"),
    ),
  },
];

const integrations = [
  {
    id: "settings-integrations-connect",
    ...EDIT,
    steps: chain(open("integrations"), press("Connect")),
  },
  {
    id: "settings-integrations-reconnect",
    ...EDIT,
    steps: chain(open("integrations"), press("Reconnect")),
  },
  {
    id: "settings-integrations-manage",
    ...DESKTOP,
    steps: chain(open("integrations"), press("Manage")),
  },
  {
    id: "settings-integrations-all-connected",
    ...DESKTOP,
    steps: chain(open("integrations"), press("Connect"), press("Reconnect"), clearToasts),
  },
];

const help = [
  {
    id: "settings-help-shortcuts",
    ...DESKTOP,
    steps: chain(open("help"), press("Open shortcuts")),
  },
  { id: "settings-help-search", ...DESKTOP, steps: chain(open("help"), press("Open search")) },
  { id: "settings-help-tour", ...DESKTOP, steps: chain(open("help"), press("Replay tour")) },
];

const navigation = [
  {
    id: "settings-nav-switch",
    ...EDIT,
    steps: chain(open("profile"), goSection("Keyboard shortcuts")),
  },
  {
    id: "settings-profile-draft-kept",
    ...DESKTOP,
    steps: chain(open("profile"), fill("Name", "Ada Byron"), goSection("Help"), goSection("Profile")),
  },
  {
    id: "settings-unknown-section",
    ...EDIT,
    steps: open("nothing"),
  },
];

export default [
  ...base,
  ...profile,
  ...general,
  ...project,
  ...roles,
  ...providers,
  ...limits,
  ...schedules,
  ...integrations,
  ...help,
  ...navigation,
];
