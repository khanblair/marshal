/** Shell scenarios: navigation, top bar, menus, palette, notices, dialogs. */
import { go, project, set } from "../steps.mjs";

const DESKTOP = ["desktop"];
const TABLET = ["tablet"];
const PHONE = ["phone"];
const DESKTOP_TABLET = ["desktop", "tablet"];
const LONG_NAME = "A very long project name that cannot fit in the sidebar";

const button = (page, name, options = {}) => page.getByRole("button", { name, ...options });
const click = (name, options) => async (page) => button(page, name, options).click();
const themeToggle = (page) => button(page, /Switch to (dark|light) theme/);
const menuItem = (page, name) => page.getByRole("menuitem", { name, exact: true });

/** Opens the tablet sidebar overlay, which is where a tablet's project rows live. */
const openSide = (page) => set(page, { sideOpen: true });
const onApi = (view) => project("api", view);
const inOrder =
  (...steps) =>
  async (page) => {
    for (const step of steps) await step(page);
  };

const projectMenu = (name) => click(`More actions for ${name}`);
const startRename = async (page) => {
  await projectMenu("api-gateway")(page);
  await menuItem(page, "Rename").click();
};
const renameField = (page) => page.getByLabel("Project name");

const navTab = (name) => async (page) =>
  page
    .locator("[data-tour=views-phone]")
    .getByRole("button", { name: new RegExp(`^${name}`) })
    .click();
const pickerButton = async (page) => page.locator("[data-tour=projects-phone]").click();

export default [
  { id: "shell-home", steps: async () => {} },
  { id: "shell-settings", steps: async (page) => go(page, "settings") },
  { id: "shell-project-board", steps: project("api", "board") },
  {
    id: "shell-notices-open",
    steps: async (page) => set(page, { noticesOpen: true }),
  },
  {
    id: "shell-palette-open",
    steps: async (page) => set(page, { palette: true }),
  },
  {
    id: "shell-avatar-menu",
    steps: async (page) => set(page, { menu: "avatar" }),
  },
  {
    id: "shell-new-project-dialog",
    steps: async (page) =>
      set(page, { newProject: { source: "folder", path: "", url: "", name: "", branch: "main" } }),
  },
  {
    id: "shell-remove-project-dialog",
    sizes: ["desktop", "phone"],
    steps: async (page) =>
      set(page, { removeProject: { id: "mobile", keepBranches: true, keepMemory: true } }),
  },
  {
    id: "shell-first-launch",
    onboarded: false,
    steps: async () => {},
  },

  // Sidebar: collapsed on desktop, the tablet overlay, and its toggle buttons.
  {
    id: "shell-sidebar-collapsed",
    sizes: DESKTOP,
    steps: async (page) => set(page, { sidebarCollapsed: true }),
  },
  {
    id: "shell-sidebar-collapsed-project",
    sizes: DESKTOP,
    steps: inOrder((page) => set(page, { sidebarCollapsed: true }), onApi("board")),
  },
  { id: "shell-sidebar-collapse-click", sizes: DESKTOP, steps: click("Collapse sidebar") },
  {
    id: "shell-sidebar-expand-click",
    sizes: DESKTOP,
    steps: inOrder((page) => set(page, { sidebarCollapsed: true }), click("Expand sidebar")),
  },
  { id: "shell-sidebar-tablet-overlay", sizes: TABLET, steps: openSide },
  {
    id: "shell-sidebar-tablet-overlay-project",
    sizes: TABLET,
    steps: inOrder(onApi("board"), openSide),
  },
  { id: "shell-sidebar-tablet-expand-click", sizes: TABLET, steps: click("Expand sidebar") },
  {
    id: "shell-sidebar-tablet-scrim-click",
    sizes: TABLET,
    steps: inOrder(openSide, (page) => page.mouse.click(700, 600)),
  },
  {
    id: "shell-sidebar-hover-project",
    sizes: DESKTOP,
    steps: async (page) => button(page, /^api-gateway: /).hover(),
  },
  {
    id: "shell-sidebar-hover-new-project",
    sizes: DESKTOP,
    steps: async (page) => button(page, "New project").hover(),
  },
  {
    id: "shell-sidebar-hover-settings",
    sizes: DESKTOP,
    steps: async (page) => button(page, "Settings").hover(),
  },
  {
    id: "shell-sidebar-focus-project",
    sizes: DESKTOP,
    steps: async (page) => {
      await page.keyboard.press("Tab");
      await button(page, /^api-gateway: /).focus();
      await page.keyboard.press("Shift+Tab");
      await page.keyboard.press("Tab");
    },
  },
  {
    id: "shell-sidebar-long-name",
    sizes: DESKTOP_TABLET,
    steps: inOrder(
      (page) => page.evaluate((name) => window.M.renameProject("api", name), LONG_NAME),
      (page) => (page.viewportSize().width < 1200 ? openSide(page) : Promise.resolve()),
    ),
  },
  {
    id: "shell-sidebar-settings-current",
    sizes: DESKTOP_TABLET,
    steps: async (page) => go(page, "settings"),
  },

  // Project row menu and rename mode.
  {
    id: "shell-project-menu",
    sizes: DESKTOP,
    steps: async (page) => set(page, { menu: "proj:api" }),
  },
  {
    id: "shell-project-menu-tablet",
    sizes: TABLET,
    steps: inOrder(openSide, (page) => set(page, { menu: "proj:web" })),
  },
  { id: "shell-project-menu-click", sizes: DESKTOP, steps: projectMenu("api-gateway") },
  {
    id: "shell-project-menu-hover-remove",
    sizes: DESKTOP,
    steps: inOrder(projectMenu("web-dashboard"), async (page) => menuItem(page, "Remove").hover()),
  },
  {
    id: "shell-project-menu-hover-rename",
    sizes: DESKTOP,
    steps: inOrder(projectMenu("mobile-app"), async (page) => menuItem(page, "Rename").hover()),
  },
  {
    id: "shell-project-rename",
    sizes: DESKTOP,
    steps: async (page) => set(page, { renaming: "api" }),
  },
  { id: "shell-project-rename-click", sizes: DESKTOP, steps: startRename },
  {
    id: "shell-project-rename-tablet",
    sizes: TABLET,
    steps: inOrder(openSide, (page) => set(page, { renaming: "web" })),
  },
  {
    id: "shell-project-rename-typed",
    sizes: DESKTOP,
    steps: inOrder(startRename, async (page) => {
      await renameField(page).fill("billing-service");
      await renameField(page).press("Enter");
    }),
  },
  {
    id: "shell-project-rename-typing",
    sizes: DESKTOP,
    steps: inOrder(startRename, async (page) => renameField(page).fill("billing-service")),
  },
  {
    id: "shell-project-rename-escape",
    sizes: DESKTOP,
    steps: inOrder(startRename, async (page) => {
      await renameField(page).fill("scratch");
      await renameField(page).press("Escape");
    }),
  },
  {
    id: "shell-project-rename-empty",
    sizes: DESKTOP,
    steps: inOrder(startRename, async (page) => {
      await renameField(page).fill("   ");
      await renameField(page).press("Enter");
    }),
  },
  {
    id: "shell-project-rename-blur",
    sizes: DESKTOP,
    steps: inOrder(startRename, async (page) => {
      await renameField(page).fill("gateway-v2");
      await page.mouse.click(800, 500);
    }),
  },
  {
    id: "shell-project-rename-long",
    sizes: DESKTOP,
    steps: inOrder(startRename, async (page) => renameField(page).fill(LONG_NAME)),
  },
  {
    id: "shell-project-settings-click",
    sizes: DESKTOP,
    steps: inOrder(projectMenu("web-dashboard"), async (page) =>
      menuItem(page, "Project settings").click(),
    ),
  },
  {
    id: "shell-project-remove-click",
    sizes: DESKTOP,
    steps: inOrder(projectMenu("mobile-app"), async (page) => menuItem(page, "Remove").click()),
  },
  {
    id: "shell-project-go-click",
    sizes: DESKTOP_TABLET,
    steps: click(/^web-dashboard: /),
  },

  // Phone: project picker and More sheets, and the bottom navigation.
  { id: "shell-phone-picker", sizes: PHONE, steps: async (page) => set(page, { menu: "picker" }) },
  {
    id: "shell-phone-picker-project",
    sizes: PHONE,
    steps: inOrder(onApi("board"), (page) => set(page, { menu: "picker" })),
  },
  { id: "shell-phone-picker-click", sizes: PHONE, steps: pickerButton },
  { id: "shell-phone-more", sizes: PHONE, steps: async (page) => set(page, { menu: "more" }) },
  {
    id: "shell-phone-more-project",
    sizes: PHONE,
    steps: inOrder(onApi("list"), (page) => set(page, { menu: "more" })),
  },
  { id: "shell-phone-more-click", sizes: PHONE, steps: navTab("More") },
  { id: "shell-phone-more-settings", sizes: PHONE, steps: async (page) => go(page, "settings") },
  { id: "shell-phone-more-calendar", sizes: PHONE, steps: onApi("calendar") },
  {
    id: "shell-phone-more-item-click",
    sizes: PHONE,
    steps: inOrder(onApi("board"), navTab("More"), async (page) =>
      page.getByRole("button", { name: "Timeline view" }).click(),
    ),
  },
  {
    id: "shell-phone-scrim-click",
    sizes: PHONE,
    steps: inOrder(
      (page) => set(page, { menu: "more" }),
      (page) => page.mouse.click(195, 100),
    ),
  },
  {
    id: "shell-phone-nav-chats",
    sizes: PHONE,
    steps: inOrder(onApi("board"), navTab("Chats")),
  },
  {
    id: "shell-phone-nav-agents",
    sizes: PHONE,
    steps: inOrder(onApi("board"), navTab("Agents")),
  },
  {
    id: "shell-phone-nav-board",
    sizes: PHONE,
    steps: inOrder(onApi("chat"), navTab("Board")),
  },
  {
    id: "shell-phone-nav-home",
    sizes: PHONE,
    steps: inOrder(onApi("board"), navTab("Home")),
  },
  {
    id: "shell-phone-new-project-row",
    sizes: PHONE,
    steps: inOrder((page) => set(page, { menu: "picker" }), click("New project", { exact: true })),
  },
  {
    id: "shell-phone-long-project",
    sizes: PHONE,
    steps: inOrder(
      (page) => page.evaluate((name) => window.M.renameProject("api", name), LONG_NAME),
      onApi("board"),
      (page) => set(page, { menu: "picker" }),
    ),
  },

  // Top bar: search, theme, notices, and the avatar menu.
  { id: "shell-topbar-project-list", steps: onApi("list") },
  {
    id: "shell-topbar-hover-search",
    sizes: DESKTOP_TABLET,
    steps: async (page) => button(page, "Search", { exact: true }).hover(),
  },
  {
    id: "shell-topbar-hover-theme",
    sizes: DESKTOP,
    steps: async (page) => themeToggle(page).hover(),
  },
  {
    id: "shell-topbar-hover-notices",
    sizes: DESKTOP,
    steps: async (page) => button(page, /^Notices/).hover(),
  },
  {
    id: "shell-topbar-hover-avatar",
    sizes: DESKTOP,
    steps: async (page) => button(page, "Profile and settings").hover(),
  },
  { id: "shell-topbar-theme-toggle", steps: async (page) => themeToggle(page).click() },
  { id: "shell-topbar-notices-click", steps: async (page) => button(page, /^Notices/).click() },
  {
    id: "shell-topbar-search-click",
    steps: async (page) => button(page, "Search", { exact: true }).click(),
  },
  { id: "shell-avatar-click", steps: async (page) => button(page, "Profile and settings").click() },
  {
    id: "shell-avatar-hover-item",
    sizes: DESKTOP,
    steps: inOrder(click("Profile and settings"), async (page) =>
      menuItem(page, "Keyboard shortcuts").hover(),
    ),
  },
  {
    id: "shell-avatar-scrim-click",
    sizes: DESKTOP,
    steps: inOrder(click("Profile and settings"), (page) => page.mouse.click(700, 500)),
  },
  {
    id: "shell-avatar-item-settings",
    steps: inOrder(click("Profile and settings"), async (page) =>
      menuItem(page, "Settings").click(),
    ),
  },
  {
    id: "shell-avatar-item-shortcuts",
    steps: inOrder(click("Profile and settings"), async (page) =>
      menuItem(page, "Keyboard shortcuts").click(),
    ),
  },
  {
    id: "shell-avatar-item-tour",
    steps: inOrder(click("Profile and settings"), async (page) =>
      menuItem(page, "Replay tour").click(),
    ),
  },
  {
    id: "shell-avatar-menu-project",
    sizes: DESKTOP_TABLET,
    steps: inOrder(onApi("board"), (page) => set(page, { menu: "avatar" })),
  },
  {
    id: "shell-avatar-long-name",
    steps: inOrder(
      (page) =>
        page.evaluate(() => {
          window.M.S.profile.name = "Alexandria Katherine Ojo-Fernandez";
          window.M.emit();
        }),
      (page) => set(page, { menu: "avatar" }),
    ),
  },
  {
    id: "shell-topbar-long-title",
    steps: inOrder(
      (page) => page.evaluate((name) => window.M.renameProject("api", name), LONG_NAME),
      onApi("board"),
    ),
  },
  {
    id: "shell-topbar-all-ci",
    sizes: DESKTOP_TABLET,
    steps: async (page) =>
      page.evaluate(() => {
        window.M.S.allKind = "ci";
        window.M.go("all");
      }),
  },
];
