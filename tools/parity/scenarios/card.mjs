/**
 * Card panel scenarios: every card state across every tab, chat and terminal, the plan and
 * approval blocks, menus, forms, and the phone, tablet, and expanded layouts. The matrix at
 * the top runs at desktop in the light theme only; the representative cards and the
 * interaction scenarios below it also run at the other sizes and in the dark theme.
 */
import { settle } from "../lib.mjs";
import { openCard, project, set } from "../steps.mjs";

const TABS = ["chat", "comments", "activity", "diff", "checks", "preview", "notes"];
const DESKTOP_LIGHT = { sizes: ["desktop"], themes: ["light"] };
const ALL_SEEDED_CARDS = [
  45, 46, 41, 42, 43, 44, 119, 210, 39, 40, 36, 35, 33, 115, 209, 207, 118,
];
const REPRESENTATIVE_TABS_CARD = 41;
const REPRESENTATIVE_CHAT_CARDS = [43, 44, 209, 45, 119, 115, 207];
const MODE_SWITCH_MS = 1500;
const PREVIEW_START_MS = 1900;
const TOOL_RESULT_MS = 2600;

const run = async (page, fn, arg) => {
  await page.evaluate(fn, arg);
  await settle(page);
};

/** Opens the project the card belongs to first, because the phone closes any open card on navigation. */
const open =
  (id, tab, ...more) =>
  async (page, ctx) => {
    const pid = await page.evaluate((i) => window.M.card(i).p, id);
    await project(pid, "board")(page);
    await settle(page);
    await openCard(page, id, tab);
    await settle(page);
    for (const step of more) await step(page, ctx);
  };

const panel = (page) => page.locator('[data-no-nav="1"]');
const click = (role, name, extra = {}) => async (page) => {
  await panel(page).getByRole(role, { name, ...extra }).first().click();
  await settle(page);
};
const clickTitle = (title) => async (page) => {
  await panel(page).locator(`[title="${title}"]`).first().click();
  await settle(page);
};
const clickText = (text) => async (page) => {
  await panel(page).getByText(text, { exact: true }).first().click();
  await settle(page);
};
const advance = (ms) => async (page) => {
  await page.clock.runFor(ms);
  await settle(page);
};
const setMode = (mode) => (page) => run(page, (m) => window.M.setMode(m), mode);
const call = (name, ...args) => (page) =>
  run(page, ([fn, a]) => window.M[fn](...a), [name, args]);
const fill = (label, text) => async (page) => {
  await panel(page).getByLabel(label).first().fill(text);
  await settle(page);
};
const press = (label, key) => async (page) => {
  await panel(page).getByLabel(label).first().press(key);
  await settle(page);
};

const scenario = (id, steps, extra = {}) => ({ id: `card-${id}`, steps, ...extra });

/** Every seeded card on every tab, at desktop in the light theme. */
const matrix = ALL_SEEDED_CARDS.filter((id) => id !== REPRESENTATIVE_TABS_CARD).flatMap((id) =>
  TABS.map((tab) => scenario(`m${id}-${tab}`, open(id, tab), DESKTOP_LIGHT)),
);

/** Card 41 on every tab, and one chat per card state, at every size and in both themes. */
const representative = [
  ...TABS.map((tab) => scenario(`r41-${tab}`, open(REPRESENTATIVE_TABS_CARD, tab))),
  ...REPRESENTATIVE_CHAT_CARDS.map((id) => scenario(`r${id}-chat`, open(id, "chat"))),
];

const terminal = [
  scenario("terminal-41", open(41, "chat", setMode("terminal"), advance(MODE_SWITCH_MS))),
  scenario("terminal-209", open(209, "chat", setMode("terminal"), advance(MODE_SWITCH_MS)), {
    sizes: ["phone", "desktop"],
    themes: ["light"],
  }),
  scenario("switching-to-terminal", open(41, "chat", setMode("terminal")), {
    themes: ["light"],
  }),
  scenario(
    "switching-to-chat",
    open(41, "chat", setMode("terminal"), advance(MODE_SWITCH_MS), setMode("chat")),
    { themes: ["light"] },
  ),
  scenario(
    "terminal-keys",
    open(
      41,
      "chat",
      setMode("terminal"),
      advance(MODE_SWITCH_MS),
      async (page) => {
        const keys = panel(page).getByRole("toolbar", { name: "Terminal keys" });
        if (await keys.count()) {
          await keys.getByRole("button", { name: "Control key" }).click();
          await keys.getByRole("button", { name: "Escape key" }).click();
          await keys.getByRole("button", { name: "Control key" }).click();
        }
      },
    ),
    { sizes: ["phone"], themes: ["light"] },
  ),
  scenario(
    "terminal-command",
    open(41, "chat", setMode("terminal"), advance(MODE_SWITCH_MS), fill("Terminal input", "go test ./..."), press("Terminal input", "Enter")),
    { sizes: ["desktop"], themes: ["light"] },
  ),
];

const plan = [
  scenario("plan-edit", open(43, "chat", call("editPlan", 43, true)), { themes: ["light"] }),
  scenario(
    "plan-edit-saved",
    open(
      43,
      "chat",
      call("editPlan", 43, true),
      fill("Steps", "Add the limiter\nWrite the tests\nUpdate the docs"),
      click("button", "Save plan"),
    ),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario("plan-edit-cancel", open(43, "chat", call("editPlan", 43, true), click("button", "Cancel")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("plan-approved", open(43, "chat", click("button", "Approve plan"), advance(TOOL_RESULT_MS)), {
    sizes: ["desktop", "phone"],
    themes: ["light"],
  }),
  scenario("plan-rejected", open(43, "chat", click("button", "Reject")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("approval-approved", open(44, "chat", call("approve", 44), advance(TOOL_RESULT_MS)), {
    sizes: ["desktop", "phone"],
    themes: ["light"],
  }),
  scenario("approval-denied", open(44, "chat", call("deny", 44), advance(TOOL_RESULT_MS)), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario(
    "approval-keyboard",
    open(44, "chat", async (page) => {
      const group = panel(page).getByRole("group", { name: "Approval request" });
      await group.focus();
      await group.press("Enter");
      await settle(page);
    }),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario(
    "approval-escape",
    open(44, "chat", async (page) => {
      const group = panel(page).getByRole("group", { name: "Approval request" });
      await group.focus();
      await group.press("Escape");
      await settle(page);
    }),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario("approval-header-link", open(44, "chat"), { sizes: ["desktop"], themes: ["dark"] }),
  scenario(
    "tool-open",
    open(41, "chat", async (page) => {
      const rows = panel(page).locator('[role="tabpanel"] button[aria-expanded]');
      await rows.nth(0).click();
      await rows.nth(1).click();
      await settle(page);
    }),
    { themes: ["light"] },
  ),
];

const header = [
  scenario("members-menu", open(41, "chat", click("button", "Add or remove members")), {
    themes: ["light"],
  }),
  scenario(
    "members-toggle",
    open(41, "chat", click("button", "Add or remove members"), click("menuitemcheckbox", "Godana Emiru")),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario("more-menu", open(41, "chat", click("button", "More actions")), { themes: ["light"] }),
  scenario("more-menu-ready", open(36, "chat", click("button", "More actions")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("title-edit", open(41, "chat", clickTitle("Edit title")), {
    themes: ["light"],
  }),
  scenario(
    "title-rename",
    open(41, "chat", clickTitle("Edit title"), fill("Card title", "Fix the refresh race"), press("Card title", "Enter")),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario(
    "title-escape",
    open(41, "chat", clickTitle("Edit title"), fill("Card title", "Nope"), press("Card title", "Escape")),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario("settings-bypass-dialog", open(41, "chat", call("setSetting", 41, "perm", "Bypass permissions")), {
    sizes: ["desktop", "phone"],
    themes: ["light"],
  }),
  scenario(
    "settings-model",
    open(41, "chat", async (page) => {
      await panel(page).getByLabel("Model").selectOption("claude-opus-4-1");
      await settle(page);
    }),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario(
    "settings-agent",
    open(41, "chat", async (page) => {
      await panel(page).getByLabel("Agent", { exact: true }).selectOption("Codex");
      await settle(page);
    }),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario("bypass-off", open(209, "chat", click("button", "Turn off bypass")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("expanded", open(41, "chat", (page) => set(page, { detailExpanded: true })), {
    sizes: ["desktop"],
  }),
  scenario("expanded-diff", open(41, "diff", (page) => set(page, { detailExpanded: true })), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("action-start", open(45, "chat", click("button", "Start card")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("action-sleep", open(41, "chat", click("button", "Sleep")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("action-pause", open(41, "chat", click("button", "Pause")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("action-pin", open(41, "chat", click("button", "Pin")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("action-wake", open(115, "chat", click("button", "Resume session")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("action-fork", open(41, "chat", click("button", "Fork"), advance(400)), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("action-merge", open(36, "chat", click("button", "Merge")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
];

const chat = [
  scenario(
    "send-message",
    open(
      41,
      "chat",
      fill("Message the agent", "Please also add a regression test."),
      press("Message the agent", "Enter"),
      advance(TOOL_RESULT_MS),
    ),
    { sizes: ["desktop", "phone"], themes: ["light"] },
  ),
  scenario("draft-typed", open(41, "chat", fill("Message the agent", "Draft in progress")), {
    sizes: ["desktop", "phone"],
    themes: ["light"],
  }),
  scenario(
    "jump-to-latest",
    open(41, "chat", async (page) => {
      await panel(page).locator("[role=tabpanel] div.overflow-auto, [role=tabpanel] > div[style*=overflow]").first().evaluate((el) => {
        el.scrollTop = 0;
        el.dispatchEvent(new Event("scroll"));
      });
      await settle(page);
      await page.evaluate(() => window.M.send(41, "One more thing"));
      await settle(page);
      await page.clock.runFor(2600);
      await settle(page);
    }),
    { sizes: ["desktop"], themes: ["light"] },
  ),
];

const comments = [
  scenario("comment-draft", open(41, "comments", fill("Write a comment", "Looks good, @agent please retry")), {
    themes: ["light"],
  }),
  scenario(
    "comment-attachments",
    open(41, "comments", async (page) => {
      await panel(page)
        .locator('input[type="file"]')
        .first()
        .setInputFiles({ name: "notes.txt", mimeType: "text/plain", buffer: Buffer.from("hello") });
      await settle(page);
    }),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario("comment-link-open", open(41, "comments", click("button", "Add link")), {
    sizes: ["desktop", "phone"],
    themes: ["light"],
  }),
  scenario(
    "comment-link-added",
    open(41, "comments", click("button", "Add link"), fill("Link", "example.com/spec"), press("Link", "Enter")),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario(
    "comment-posted",
    open(41, "comments", fill("Write a comment", "Ship it"), click("button", "Post comment"), advance(TOOL_RESULT_MS)),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario("comment-delete", open(41, "comments", click("button", "Delete")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
];

const diff = [
  scenario("diff-collapse-all", open(41, "diff", clickText("Collapse all")), { themes: ["light"] }),
  scenario("diff-expand-all", open(41, "diff", clickText("Expand all")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("diff-large", open(41, "diff", click("button", "go.sum"), click("button", "Load diff")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("diff-large-open", open(41, "diff", click("button", "go.sum")), {
    sizes: ["desktop", "phone"],
    themes: ["light"],
  }),
];

const checks = [
  scenario("checks-add-item", open(41, "checks", click("button", "Add an item")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario(
    "checks-item-added",
    open(41, "checks", click("button", "Add an item"), fill("New checklist item", "Write the docs"), press("New checklist item", "Enter")),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario("checks-new-list", open(41, "checks", click("button", "Add checklist")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("checks-hide-checked", open(41, "checks", click("button", "Hide checked items")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("checks-toggle", open(41, "checks", click("checkbox", "Retry the request once after refresh")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("checks-run", open(41, "checks", click("button", "Run checks"), advance(3500)), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("checks-failed", open(119, "checks", click("button", "Run checks"), advance(3500)), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("checks-delete-dialog", open(41, "checks", click("button", "Delete")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("checks-empty", open(45, "checks"), { sizes: ["desktop", "phone"], themes: ["light"] }),
];

const preview = [
  scenario("preview-starting", open(118, "preview", click("button", "Start preview")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario(
    "preview-running-118",
    open(118, "preview", click("button", "Start preview"), advance(PREVIEW_START_MS)),
  ),
  scenario(
    "preview-running-119",
    open(119, "preview", click("button", "Start preview"), advance(PREVIEW_START_MS)),
    { sizes: ["desktop", "phone"] },
  ),
  scenario(
    "preview-running-mobile",
    open(209, "preview", click("button", "Start preview"), advance(PREVIEW_START_MS)),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario(
    "preview-stopped-again",
    open(118, "preview", click("button", "Start preview"), advance(PREVIEW_START_MS), click("button", "Stop preview")),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario("preview-no-dev-command", open(41, "preview", click("button", "Add dev command")), {
    sizes: ["desktop", "phone"],
    themes: ["light"],
  }),
];

const notes = [
  scenario("notes-edit", open(41, "notes", click("button", "Edit note")), { themes: ["light"] }),
  scenario(
    "notes-saved",
    open(41, "notes", click("button", "Edit note"), fill("Card note", "# Changed\n\nNew content"), click("button", "Save note")),
    { sizes: ["desktop"], themes: ["light"] },
  ),
  scenario(
    "notes-cancel",
    open(41, "notes", click("button", "Edit note"), fill("Card note", "Thrown away"), click("button", "Cancel")),
    { sizes: ["desktop"], themes: ["light"] },
  ),
];

const activity = [
  scenario("restore-dialog", open(41, "activity", click("button", "Restore")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("more-restore-checkpoint", open(41, "chat", click("button", "More actions"), click("menuitem", "Restore a checkpoint")), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("activity-live", open(41, "activity", call("approvePlan", 43), advance(TOOL_RESULT_MS)), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
];

const layout = [
  scenario("phone-back", open(41, "chat", click("button", "Back")), { sizes: ["phone"], themes: ["light"] }),
  scenario("tablet-overlay-comments", open(43, "comments"), { sizes: ["tablet"], themes: ["light"] }),
  scenario("ci-failure", open(40, "chat", call("simulateCiFailure", 40), advance(TOOL_RESULT_MS)), {
    sizes: ["desktop"],
    themes: ["light"],
  }),
  scenario("chat-unsupported-think", open(45, "chat"), { sizes: ["desktop"], themes: ["dark"] }),
];

export default [
  ...matrix,
  ...representative,
  ...terminal,
  ...plan,
  ...header,
  ...chat,
  ...comments,
  ...diff,
  ...checks,
  ...preview,
  ...notes,
  ...activity,
  ...layout,
];
