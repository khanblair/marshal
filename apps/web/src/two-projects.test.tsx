import { cleanup, render, screen } from "@solidjs/testing-library";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { buildNotices } from "~/app/notices/notice-list";
import { paletteResults } from "~/app/palette/palette-results";
import { type Card, M } from "~/mock";
import { cardKey, cardLabel } from "~/mock/card-key";
import { checksFor } from "~/mock/seed/checks";
import { addTwoProjects, removeTwoProjects, type TwoProjects } from "~/mock/testing/two-projects";
import { BoardView } from "~/views/board/BoardView";
import { defaultNote, noteText, saveNote } from "~/views/card/card-note";
import { newChatTargets, targetLabel } from "~/views/chats/chat-model";
import { awakeCards } from "~/views/home/awake";
import { feedProjectName, openFeedItem } from "~/views/home/feed-row";
import { todayItems } from "~/views/home/today";
import { ListView } from "~/views/list/ListView";
import { dependencyLines, spanOf } from "~/views/timeline/timeline-geometry";
import { barTip, planMove } from "~/views/timeline/timeline-model";

/*
 * The store and the fake daemon it follows are built here, before any import runs, because the
 * views read the one store `~/mock` hands out: it has to be made with the section table that puts
 * the cards on the daemon, and with a daemon to answer the writes the fixture makes. The daemon
 * holds the prototype's own api cards, so a card added to api still continues that project's
 * numbering.
 */
const host = await vi.hoisted(async () => {
  window.location.hash = "#nosim";
  const { createFakeDaemon } = await import("~/testing/fake-daemon");
  const { wireCard } = await import("~/testing/fake-cards");
  const { prototypeCards } = await import("~/testing/prototype-cards");
  const { PROTOTYPE_PROJECTS } = await import("~/testing/projects");
  const { createTestMarshal, DAEMON_CARDS, MOCK_HISTORY } = await import("~/testing/test-store");
  const cards = prototypeCards()
    .filter((card) => card.p === "api")
    .map((card) => wireCard({ projectId: card.p, number: card.n, title: card.title }));
  const daemon = createFakeDaemon({ projects: PROTOTYPE_PROJECTS, cards });
  // The cards are the daemon's and their chat and activity, and the project chats, are still the
  // mock's, which is a state the register really has while the sections switch one at a time.
  window.M = createTestMarshal({
    data: daemon.data,
    sections: { ...DAEMON_CARDS, ...MOCK_HISTORY, S17: "mock" },
  });
  return { daemon };
});

/*
 * Card 12 exists in two projects. Every test here would show the wrong card, or two cards, if
 * something still told cards apart by their number alone.
 */

const DESKTOP_PX = 1440;
const HEIGHT_PX = 900;
const ALPHA_TITLE = "Alpha login form";
const BETA_TITLE = "Beta billing export";

let two: TwoProjects;
let savedNotices: typeof M.S.notices;
let savedFeed: typeof M.S.feed;

const alpha = (): Card => M.card(two.alphaCard) as Card;
const beta = (): Card => M.card(two.betaCard) as Card;

/**
 * Waits until the app has stopped asking for boards: the stream's first Resync makes it load once
 * more after `ready`, and that snapshot landing on top of a card a test just made would put the
 * card back the way the daemon had it before the change.
 */
async function settled(): Promise<void> {
  let last = -1;
  await vi.waitFor(() => {
    const loads = host.daemon.routes().filter((route) => route.endsWith("/board")).length;
    const steady = loads > 0 && loads === last;
    last = loads;
    expect(steady).toBe(true);
  });
}

beforeAll(async () => {
  await host.daemon.connect();
  await vi.waitFor(() => expect(M.S.ready).toBe(true));
  await settled();
});

afterAll(() => {
  host.daemon.data.stop();
});

beforeEach(async () => {
  vi.useFakeTimers();
  savedNotices = M.S.notices;
  savedFeed = M.S.feed;
  M.setViewport(DESKTOP_PX, HEIGHT_PX);
  two = await addTwoProjects(M, host.daemon);
  M.set({ openId: null, focusId: null, dialog: null, toasts: [], notices: [], feed: [] });
});

afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  removeTwoProjects(M, host.daemon, two);
  M.set({ openId: null, focusId: null, dragId: null, notices: savedNotices, feed: savedFeed });
  vi.useRealTimers();
});

describe("card numbers and keys", () => {
  it("gives each project its own numbers, so both have a card 12", () => {
    expect(alpha().n).toBe(12);
    expect(beta().n).toBe(12);
    expect(two.alphaCard).not.toBe(two.betaCard);
    const twelves = M.S.cards.filter((c) => c.n === 12).map((c) => c.id);
    expect(twelves.sort()).toEqual([two.alphaCard, two.betaCard].sort());
  });

  it("continues the numbering of a seeded project, not a global counter", async () => {
    // The daemon answers the key it numbered the card with. The store keeps its cards sorted, so
    // the newest one is not the last of `S.cards` the way the mock's own quick add left it.
    expect(await M.quickAdd("api", "backlog", "Next api card")).toBe("api#47");
    expect(await M.quickAdd(two.alpha, "backlog", "Next alpha card")).toBe(cardKey(two.alpha, 13));
  });

  it("finds each card by its own key and no card by a key of another project", () => {
    expect(alpha().title).toBe(ALPHA_TITLE);
    expect(beta().title).toBe(BETA_TITLE);
    expect(M.card(cardKey("api", 12))).toBeUndefined();
    expect(M.card(undefined)).toBeUndefined();
  });

  it("gives two cards numbered 12 in two projects different view keys", () => {
    // The view model's key is the card's key, not its number: the prototype's `c<number>` made
    // alpha#12 and beta#12 the same key.
    expect(M.deco(alpha()).key).toBe(two.alphaCard);
    expect(M.deco(beta()).key).toBe(two.betaCard);
    expect(M.deco(alpha()).key).not.toBe(M.deco(beta()).key);
  });
});

describe("the views of each project", () => {
  it("draws only its own card 12 on the board of each project", () => {
    M.go("project", two.alpha, "board");
    render(() => <BoardView />);
    expect(screen.getByText(ALPHA_TITLE)).toBeInTheDocument();
    expect(screen.queryByText(BETA_TITLE)).toBeNull();
    const card = document.querySelector<HTMLElement>(`[data-card="${two.alphaCard}"]`);
    expect(card).toHaveTextContent("#12");
    expect(document.querySelector(`[data-card="${two.betaCard}"]`)).toBeNull();
    cleanup();
    M.go("project", two.beta, "board");
    render(() => <BoardView />);
    expect(screen.getByText(BETA_TITLE)).toBeInTheDocument();
    expect(screen.queryByText(ALPHA_TITLE)).toBeNull();
    expect(document.querySelector(`[data-card="${two.betaCard}"]`)).toHaveTextContent("#12");
  });

  it("lists only its own card 12 in the list of each project", () => {
    M.go("project", two.alpha, "list");
    render(() => <ListView />);
    const row = document.querySelector<HTMLElement>(`tr[data-card="${two.alphaCard}"]`);
    expect(row).toHaveTextContent("#12");
    expect(row).toHaveTextContent(ALPHA_TITLE);
    expect(document.querySelector(`tr[data-card="${two.betaCard}"]`)).toBeNull();
    cleanup();
    M.go("project", two.beta, "list");
    render(() => <ListView />);
    expect(document.querySelector(`tr[data-card="${two.betaCard}"]`)).toHaveTextContent(BETA_TITLE);
    expect(document.querySelector(`tr[data-card="${two.alphaCard}"]`)).toBeNull();
  });

  it("opens one card and leaves the other closed, switching to its project", () => {
    M.go("project", two.alpha, "board");
    M.openCard(two.betaCard);
    expect(M.S.openId).toBe(two.betaCard);
    expect(M.S.route.pid).toBe(two.beta);
    expect(M.deco(beta()).selected).toBe(true);
    expect(M.deco(alpha()).selected).toBe(false);
  });
});

describe("what a card owns", () => {
  it("keeps chat, activity, checks, and notes apart per key", () => {
    M.send(two.alphaCard, "Ping alpha");
    vi.advanceTimersByTime(5000);
    expect(M.S.chat[two.alphaCard]?.some((m) => m.k === "user" && m.text === "Ping alpha")).toBe(
      true,
    );
    expect(M.S.chat[two.betaCard]?.some((m) => m.k === "user") ?? false).toBe(false);
    expect((M.S.act[two.alphaCard] ?? []).length).toBeGreaterThan(0);
    expect(M.S.act[two.betaCard] ?? []).toHaveLength(0);
    expect(beta().state).toBe("backlog");

    // Acceptance checks are still the mock's own: a card the daemon made has none in the store, so
    // each card is given the checks the mock's own create path would have written for it.
    for (const card of [alpha(), beta()]) M.S.checks[card.id] = checksFor(card);
    M.runChecks(two.alphaCard);
    expect(M.S.checks[two.alphaCard]?.some((k) => k.st === "running")).toBe(true);
    expect(M.S.checks[two.betaCard]?.every((k) => k.st === "pending")).toBe(true);

    saveNote(alpha(), "Alpha note");
    expect(noteText(alpha())).toBe("Alpha note");
    expect(noteText(beta())).toBe(defaultNote(beta()));
    expect(M.S.notes?.[two.betaCard]).toBeUndefined();
  });

  it("moves the dragged card and no other, and dims only that card", async () => {
    M.set({ dragId: two.alphaCard });
    expect(M.deco(alpha()).dragging).toBe(true);
    expect(M.deco(beta()).dragging).toBe(false);
    await M.moveCard(two.alphaCard, "working");
    expect(alpha().state).toBe("working");
    expect(beta().state).toBe("backlog");
    await M.moveCard(two.betaCard, "planning");
    expect(alpha().state).toBe("working");
    expect(beta().state).toBe("planning");
  });

  it("deletes one card and leaves the other with everything it has", async () => {
    M.send(two.betaCard, "Ping beta");
    M.openCard(two.betaCard);
    await M.deleteCard(two.alphaCard);
    M.S.dialog?.run();
    await vi.waitFor(() => expect(M.card(two.alphaCard)).toBeUndefined());
    expect(M.S.chat[two.alphaCard]).toBeUndefined();
    expect(beta().title).toBe(BETA_TITLE);
    expect(M.S.chat[two.betaCard]?.some((m) => m.k === "user")).toBe(true);
    expect(M.S.openId).toBe(two.betaCard);
    await M.deleteCard(two.betaCard);
    M.S.dialog?.run();
    await vi.waitFor(() => expect(M.S.openId).toBeNull());
  });
});

describe("search and the palette", () => {
  it("searches the visible label in one project and accepts the project name with it", () => {
    M.S.query[two.alpha] = "#12";
    M.S.query[two.beta] = "#12";
    expect(M.filtered(two.alpha).map((c) => c.id)).toEqual([two.alphaCard]);
    expect(M.filtered(two.beta).map((c) => c.id)).toEqual([two.betaCard]);
    M.S.query[two.alpha] = "alpha-service #12";
    expect(M.filtered(two.alpha).map((c) => c.id)).toEqual([two.alphaCard]);
    M.S.query[two.alpha] = "beta-service #12";
    expect(M.filtered(two.alpha)).toEqual([]);
  });

  it("finds both cards for #12 and shows each with its project name", () => {
    const found = paletteResults(M, "#12").filter((x) => x.group === "Cards");
    expect(found.find((x) => x.card === two.alphaCard)?.label).toBe(
      `alpha-service #12 ${ALPHA_TITLE}`,
    );
    expect(found.find((x) => x.card === two.betaCard)?.label).toBe(
      `beta-service #12 ${BETA_TITLE}`,
    );
    const named = paletteResults(M, "beta-service #12");
    expect(named.map((x) => x.card)).toEqual([two.betaCard]);
  });

  it("tells two cards with the same title apart, and opens the one that was picked", () => {
    alpha().title = "Same title";
    beta().title = "Same title";
    const found = paletteResults(M, "same title");
    expect(found.map((x) => x.label)).toEqual([
      "alpha-service #12 Same title",
      "beta-service #12 Same title",
    ]);
    found[1]?.run();
    expect(M.S.openId).toBe(two.betaCard);
    found[0]?.run();
    expect(M.S.openId).toBe(two.alphaCard);
  });

  it("lists both cards that need you, each once", () => {
    for (const c of [alpha(), beta()]) {
      c.state = "needs";
      c.reason = "Plan ready for review";
    }
    const needs = paletteResults(M, "").filter((x) => x.group === "Cards that need you");
    const keys = needs.map((x) => x.card);
    expect(keys.filter((k) => k === two.alphaCard)).toHaveLength(1);
    expect(keys.filter((k) => k === two.betaCard)).toHaveLength(1);
  });
});

describe("notices and the feed", () => {
  it("names the project of a notice about each card and opens the right card", () => {
    const at = Date.now();
    M.S.notices = [
      { id: "na", kind: "plan", cardId: two.alphaCard, text: "Plan ready", sub: "", ts: at },
      { id: "nb", kind: "plan", cardId: two.betaCard, text: "Plan ready", sub: "", ts: at },
    ];
    const [first, second] = buildNotices(M, false).filter((n) => n.key === "na" || n.key === "nb");
    expect([first?.project, second?.project]).toEqual(["alpha-service", "beta-service"]);
    second?.actions[0]?.run();
    expect(M.S.openId).toBe(two.betaCard);
    first?.actions[0]?.run();
    expect(M.S.openId).toBe(two.alphaCard);
  });

  it("lists both waiting cards with their projects and opens the right one from its row", () => {
    for (const c of [alpha(), beta()]) {
      c.state = "needs";
      c.reason = "Plan ready for review";
    }
    const group = buildNotices(M, false).find((n) => n.key === "needs");
    const rows = group?.rows.filter((r) => r.title.startsWith("#12 ")) ?? [];
    expect(rows.map((r) => r.project).sort()).toEqual(["alpha-service", "beta-service"]);
    rows.find((r) => r.project === "beta-service")?.open();
    expect(M.S.openId).toBe(two.betaCard);
  });

  it("opens the right card from a feed item about each and names its project", () => {
    const at = Date.now();
    const items = [
      {
        id: "fa",
        kind: "plan",
        text: "Plan ready for review on #12",
        pid: two.alpha,
        cardId: two.alphaCard,
        ts: at,
      },
      {
        id: "fb",
        kind: "plan",
        text: "Plan ready for review on #12",
        pid: two.beta,
        cardId: two.betaCard,
        ts: at,
      },
    ] as const;
    expect(items.map((item) => feedProjectName({ ...item }))).toEqual([
      "alpha-service",
      "beta-service",
    ]);
    openFeedItem({ ...items[1] });
    expect(M.S.openId).toBe(two.betaCard);
    openFeedItem({ ...items[0] });
    expect(M.S.openId).toBe(two.alphaCard);
  });
});

describe("Home", () => {
  it("names the project of cards that are due today and opens the right one", () => {
    for (const c of [alpha(), beta()]) c.due = 0;
    const weekday = new Date(M.now()).getDay();
    const due = todayItems(weekday).filter((item) => item.kind === "Due");
    const labels = due.map((item) => item.label);
    expect(labels).toContain(`alpha-service #12 ${ALPHA_TITLE} is due`);
    expect(labels).toContain(`beta-service #12 ${BETA_TITLE} is due`);
    due.find((item) => item.label.startsWith("beta-service"))?.open();
    expect(M.S.openId).toBe(two.betaCard);
  });

  it("names the project in the label of each awake card", () => {
    // A daemon card is awake when its session runs an agent, whatever column it is in.
    for (const c of [alpha(), beta()]) Object.assign(c, { state: "working", session: "working" });
    const awake = awakeCards();
    expect(awake.map((c) => c.id)).toEqual(expect.arrayContaining([two.alphaCard, two.betaCard]));
    expect(M.cardLabelOf(alpha())).toBe("alpha-service #12");
    expect(M.cardLabelOf(beta())).toBe("beta-service #12");
    expect(cardLabel(alpha())).toBe("#12");
  });
});

describe("dependencies and the timeline", () => {
  const span = (c: Card) => spanOf(c, null);

  function chainInBothProjects(): { a11: Card; b11: Card } {
    const a11 = M.card(cardKey(two.alpha, 11)) as Card;
    const b11 = M.card(cardKey(two.beta, 11)) as Card;
    Object.assign(a11, { s: 0, e: 2 });
    Object.assign(b11, { s: 0, e: 2 });
    Object.assign(alpha(), { s: 3, e: 4, deps: [a11.id] });
    Object.assign(beta(), { s: 3, e: 4, deps: [b11.id] });
    return { a11, b11 };
  }

  it("treats deps as keys: moving card 11 breaks only its own project's card 12", () => {
    const { a11 } = chainInBothProjects();
    const plan = planMove(a11, 3, M.S.cards, (id) => M.card(id));
    expect(plan.broken).toEqual(["#12 would start before #11 finishes"]);
    const blocks = M.S.cards.filter((c) => c.deps.includes(a11.id));
    expect(blocks.map((c) => c.id)).toEqual([two.alphaCard]);
    expect(barTip(a11, "Backlog", blocks)).toBe("#11 alpha-service filler 11. Backlog. Blocks #12");
  });

  it("draws one line per dependency and none between the projects", () => {
    const { a11, b11 } = chainInBothProjects();
    const rows = [a11, alpha(), b11, beta()];
    expect(dependencyLines(rows, span)).toHaveLength(2);
    expect(dependencyLines([alpha(), beta()], span)).toHaveLength(0);
  });
});

describe("chats", () => {
  it("points a chat at one card and lists only the cards of its own project", async () => {
    for (const c of [alpha(), beta()]) Object.assign(c, { state: "working", session: "working" });
    const targets = newChatTargets(two.alpha).map((option) => option.value);
    expect(targets).toContain(two.alphaCard);
    expect(targets).not.toContain(two.betaCard);
    const chat = await M.newChat(two.beta, two.betaCard);
    expect(chat?.target).toBe(two.betaCard);
    expect(targetLabel(chat?.target ?? "")).toBe(`#12 ${beta().agent}`);
  });
});
