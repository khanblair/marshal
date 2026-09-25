/**
 * Chats view scenarios: each project's chats, a selected chat with card links and an approval, a
 * role and a card target, the New chat popover and sheet, the row menu, rename in place, the
 * archived section, search with and without matches, empty chats, empty projects, a long title,
 * hover, a draft, sending a message, Jump to latest, and the phone's list and chat screens.
 * Steps only use `window.M` and real interactions, so they run the same in both apps.
 */
import { settle } from "../lib.mjs";
import { project } from "../steps.mjs";

/** The runner pauses the clock, so Playwright's wait for a stable element would hang. */
const FORCE = { force: true };
/** Long enough for the scripted answer to start and finish streaming. */
const REPLY_MS = 3000;
const DESKTOP_ONLY = { sizes: ["desktop"], themes: ["light"] };
const WIDE_ONLY = { sizes: ["desktop", "tablet"] };
const LONG_TITLE =
  "Rework the per key rate limiting so two tabs that wake up at the same moment share one budget and never lock the person out";

/** Runs `steps` in order. */
const chain =
  (...steps) =>
  async (page, ctx) => {
    for (const step of steps) await step(page, ctx);
  };

/** Runs an action on `M`, then lets the paused clock flush the redraw. */
const act = (fn, arg) => async (page) => {
  await page.evaluate(fn, arg);
  await settle(page);
};

/** Opens a project's chats, then lets the paused clock flush the prototype's redraw. */
const chatsOf = (pid) => async (page, ctx) => {
  await project(pid, "chat")(page, ctx);
  await settle(page);
};

const openChat = (pid, title) =>
  act(
    ([p, t]) => window.M.openChat(p, window.M.S.chats[p].find((c) => c.title === t).id),
    [pid, title],
  );
const closeChat = (pid) => act((p) => window.M.openChat(p, null), pid);
const click = (locator) => async (page) => {
  await locator(page).click(FORCE);
  await settle(page);
};
const hover = (locator) => async (page) => {
  await locator(page).hover(FORCE);
  await settle(page);
};
const byName =
  (role, name, exact = true) =>
  (page) =>
    page.getByRole(role, { name, exact }).first();

const newChatButtons = (page) => page.getByRole("button", { name: "New chat", exact: true });
const newChatButton = (page) => newChatButtons(page).first();
const archivedToggle = (page) => page.getByRole("button", { name: /^Archived/ });
const moreActions = (title) => byName("button", `More actions for ${title}`);
const menuItem = (name) => byName("menuitem", name);
const searchBox = (page) => page.getByRole("textbox", { name: "Search chats", exact: true });
const composer = (page) => page.getByRole("textbox", { name: "Message", exact: true });
const rowButton = (title) => (page) =>
  page.getByRole("button", { name: new RegExp(`^${title}`) }).first();

const search = (text) => async (page) => {
  await searchBox(page).fill(text);
  await settle(page);
};
const typeDraft = (text) => async (page) => {
  await composer(page).fill(text);
  await settle(page);
};
const send = (page) => page.keyboard.press("Enter").then(() => settle(page));
const runFor = (ms) => async (page) => {
  await page.clock.runFor(ms);
  await settle(page);
};
const sendAs = (pid, title, text) =>
  act(
    ([p, t, body]) => window.M.chatSend(p, window.M.S.chats[p].find((c) => c.title === t).id, body),
    [pid, title, text],
  );

const openForm = click(newChatButton);
const startWith = (target) =>
  chain(openForm, async (page) => {
    await page.getByRole("combobox").selectOption(target);
    await settle(page);
  });
const emptyChat = act((pid) => window.M.newChat(pid), "api");
const OPEN_APPROVAL = "Upgrade grpc-go";
const OPEN_MESSAGES = "Rate limiting per key";

/** Scrolls the open chat's message area to the top, as a reader who is looking at older messages. */
const scrollThreadTop = act(() => {
  const box = document.querySelector("section[aria-label] > div:nth-child(2)");
  box.scrollTop = 0;
});

export default [
  { id: "chats-api", steps: chatsOf("api") },
  { id: "chats-web", steps: chatsOf("web") },
  { id: "chats-mobile", steps: chatsOf("mobile") },
  {
    id: "chats-selected",
    steps: chain(chatsOf("api"), openChat("api", OPEN_MESSAGES)),
  },
  {
    id: "chats-card-and-approval",
    steps: chain(chatsOf("api"), openChat("api", OPEN_APPROVAL)),
  },
  {
    id: "chats-role-target",
    steps: chain(chatsOf("api"), openChat("api", "Load test results")),
  },
  {
    id: "chats-card-target",
    steps: chain(chatsOf("web"), openChat("web", "Settings dark mode")),
  },
  { id: "chats-new-popover", steps: chain(chatsOf("api"), openForm) },
  {
    id: "chats-new-popover-role",
    steps: chain(chatsOf("api"), startWith("Tester")),
  },
  {
    id: "chats-start-role",
    steps: chain(chatsOf("api"), startWith("Tester"), click(byName("button", "Start chat"))),
  },
  {
    id: "chats-start-orchestrator",
    steps: chain(chatsOf("api"), openForm, click(byName("button", "Start chat"))),
  },
  {
    id: "chats-start-card",
    steps: chain(
      chatsOf("api"),
      act(() => {
        const card = window.M.cardsOf("api").find(window.M.isAwake);
        window.M.newChat("api", `#${card.id}`);
      }),
    ),
  },
  {
    id: "chats-row-menu",
    steps: chain(chatsOf("api"), click(moreActions("Rate limiting per key"))),
  },
  {
    id: "chats-row-menu-open-chat",
    steps: chain(chatsOf("api"), click(moreActions(OPEN_APPROVAL))),
  },
  {
    id: "chats-rename",
    steps: chain(
      chatsOf("api"),
      click(moreActions("Rate limiting per key")),
      click(menuItem("Rename")),
    ),
  },
  {
    id: "chats-rename-typed",
    steps: chain(
      chatsOf("api"),
      click(moreActions("Rate limiting per key")),
      click(menuItem("Rename")),
      async (page) => {
        await page.getByRole("textbox", { name: "Chat name" }).fill("Per key limits");
        await settle(page);
      },
    ),
  },
  {
    id: "chats-rename-committed",
    steps: chain(
      chatsOf("api"),
      click(moreActions("Rate limiting per key")),
      click(menuItem("Rename")),
      async (page) => {
        await page.getByRole("textbox", { name: "Chat name" }).fill("Per key limits");
        await page.keyboard.press("Enter");
        await settle(page);
      },
    ),
  },
  {
    id: "chats-rename-cancelled",
    steps: chain(
      chatsOf("api"),
      click(moreActions("Rate limiting per key")),
      click(menuItem("Rename")),
      async (page) => {
        await page.getByRole("textbox", { name: "Chat name" }).fill("Never kept");
        await page.keyboard.press("Escape");
        await settle(page);
      },
    ),
  },
  {
    id: "chats-archive-undo-toast",
    steps: chain(
      chatsOf("api"),
      click(moreActions("Rate limiting per key")),
      click(menuItem("Archive")),
    ),
  },
  {
    id: "chats-archive-open-chat",
    steps: chain(chatsOf("api"), click(moreActions(OPEN_APPROVAL)), click(menuItem("Archive"))),
  },
  {
    id: "chats-delete-dialog",
    steps: chain(
      chatsOf("api"),
      click(moreActions("Rate limiting per key")),
      click(menuItem("Delete")),
    ),
  },
  { id: "chats-archived-open", steps: chain(chatsOf("api"), click(archivedToggle)) },
  {
    id: "chats-archived-menu",
    steps: chain(
      chatsOf("api"),
      click(archivedToggle),
      click(moreActions("JWKS caching question")),
    ),
  },
  {
    id: "chats-archived-chat",
    steps: chain(chatsOf("api"), click(archivedToggle), click(rowButton("JWKS caching question"))),
  },
  {
    id: "chats-archived-restored",
    steps: chain(
      chatsOf("api"),
      click(archivedToggle),
      click(rowButton("JWKS caching question")),
      click(byName("button", "Restore")),
    ),
  },
  { id: "chats-search", steps: chain(chatsOf("api"), search("grpc")) },
  { id: "chats-search-message", steps: chain(chatsOf("api"), search("p99")) },
  { id: "chats-search-archived", steps: chain(chatsOf("api"), search("jwks")) },
  { id: "chats-search-empty", steps: chain(chatsOf("api"), search("Zzz")) },
  {
    id: "chats-search-empty-archived-open",
    steps: chain(chatsOf("api"), search("Zzz"), click(archivedToggle)),
  },
  { id: "chats-empty-chat", steps: chain(chatsOf("api"), emptyChat) },
  {
    id: "chats-none-open",
    ...WIDE_ONLY,
    steps: chain(chatsOf("api"), closeChat("api")),
  },
  {
    id: "chats-none-open-new",
    ...WIDE_ONLY,
    steps: chain(
      chatsOf("api"),
      closeChat("api"),
      click((page) => newChatButtons(page).nth(1)),
    ),
  },
  {
    id: "chats-no-chats",
    steps: chain(
      chatsOf("web"),
      // The prototype needs `emit()` after a direct write; the port ignores it.
      act(() => {
        const { S } = window.M;
        S.chats.web = [];
        S.chatOpen.web = null;
        window.M.emit();
      }),
    ),
  },
  { id: "chats-no-archived", steps: chain(chatsOf("web"), click(archivedToggle)) },
  {
    id: "chats-long-title",
    steps: chain(
      chatsOf("api"),
      act(
        ([p, t, title]) =>
          window.M.renameChat(p, window.M.S.chats[p].find((c) => c.title === t).id, title),
        ["api", OPEN_APPROVAL, LONG_TITLE],
      ),
    ),
  },
  {
    id: "chats-draft",
    steps: chain(
      chatsOf("api"),
      openChat("api", OPEN_APPROVAL),
      typeDraft("Please split the limiter work into two cards"),
    ),
  },
  {
    id: "chats-sent",
    steps: chain(
      chatsOf("api"),
      openChat("api", OPEN_APPROVAL),
      click(byName("button", "What is blocked?")),
      runFor(REPLY_MS),
    ),
  },
  {
    id: "chats-sent-cards",
    steps: chain(
      chatsOf("api"),
      openChat("api", OPEN_APPROVAL),
      typeDraft("Make cards for the export work"),
      send,
      runFor(REPLY_MS),
    ),
  },
  {
    id: "chats-jump-to-latest",
    steps: chain(
      chatsOf("api"),
      openChat("api", OPEN_APPROVAL),
      sendAs("api", OPEN_APPROVAL, "Make cards for the export work"),
      runFor(REPLY_MS),
      scrollThreadTop,
      sendAs("api", OPEN_APPROVAL, "What is blocked?"),
      runFor(REPLY_MS),
    ),
  },
  {
    id: "chats-row-hover",
    ...DESKTOP_ONLY,
    steps: chain(chatsOf("api"), hover(rowButton("Rate limiting per key"))),
  },
  {
    id: "chats-menu-button-hover",
    ...DESKTOP_ONLY,
    steps: chain(chatsOf("api"), hover(moreActions("Rate limiting per key"))),
  },
  {
    id: "chats-new-hover",
    ...DESKTOP_ONLY,
    steps: chain(chatsOf("api"), hover(newChatButton)),
  },
  {
    id: "chats-suggestion-hover",
    ...DESKTOP_ONLY,
    steps: chain(chatsOf("api"), hover(byName("button", "Merge the next ready card"))),
  },
  {
    id: "chats-phone-list-searched",
    sizes: ["phone"],
    steps: chain(chatsOf("api"), search("grpc")),
  },
];
