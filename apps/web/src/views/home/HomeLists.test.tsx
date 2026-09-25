import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { HomeView } from "./HomeView";
import { type HomeSnapshot, homeSnapshot, resetHome } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const snapshot: HomeSnapshot = homeSnapshot();
/** Rendering all of Home in jsdom is slow. */
const SLOW_TEST_MS = 30_000;

beforeEach(() => {
  vi.useFakeTimers();
  resetHome(snapshot);
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

const section = (name: string) => screen.getByRole("region", { name });
const weekday = () => new Date(M.now()).getDay();
const feedRows = () => within(section("Recent activity")).getAllByRole("listitem");

describe("Recent activity", { timeout: SLOW_TEST_MS }, () => {
  it("shows the five newest entries, then all of them", () => {
    render(() => <HomeView />);
    expect(feedRows()).toHaveLength(5);
    const toggle = screen.getByRole("button", { name: `Show all ${M.S.feed.length}` });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    expect(feedRows()).toHaveLength(M.S.feed.length);
    const less = screen.getByRole("button", { name: "Show less" });
    expect(less).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(less);
    expect(feedRows()).toHaveLength(5);
  });

  it("shows each entry with its project and a time that carries the full date", () => {
    render(() => <HomeView />);
    const first = M.S.feed[0];
    if (!first) throw new Error("seed has no feed");
    const row = screen.getByText(first.text).closest("li");
    expect(row).not.toBeNull();
    const time = within(row as HTMLElement).getByTitle(M.full(first.ts));
    expect(time).toHaveTextContent(M.rel(first.ts));
    const project = M.proj(first.pid)?.name;
    if (project) expect(within(row as HTMLElement).getByText(project)).toBeVisible();
  });

  it("opens the card, the schedule, or the project an entry is about", () => {
    render(() => <HomeView />);
    fireEvent.click(screen.getByRole("button", { name: /Plan ready for review on #43/ }));
    expect(M.S.openId).toBe("api#43");
    M.S.feed.unshift({
      id: "f-job",
      kind: "schedule",
      text: "Job ran",
      pid: null,
      ts: M.now(),
      job: "s3",
    });
    fireEvent.click(screen.getByRole("button", { name: /Job ran/ }));
    expect(M.S).toMatchObject({ settingsSection: "schedules", schedEdit: "s3" });
    expect(M.S.route.page).toBe("settings");
    M.go("home");
    M.S.feed.unshift({ id: "f-proj", kind: "tool", text: "Project note", pid: "web", ts: M.now() });
    fireEvent.click(screen.getByRole("button", { name: /Project note/ }));
    expect(M.S.route).toMatchObject({ page: "project", pid: "web" });
  });

  it("links to the full page", () => {
    render(() => <HomeView />);
    fireEvent.click(screen.getByRole("button", { name: "View all activity" }));
    expect(M.S.route.page).toBe("all");
    expect(M.S.allKind).toBe("activity");
  });

  it("shows a new entry at the top as soon as it arrives", () => {
    render(() => <HomeView />);
    M.S.feed.unshift({ id: "f-new", kind: "tool", text: "Fresh entry", pid: null, ts: M.now() });
    expect(feedRows()).toHaveLength(5);
    expect(within(feedRows()[0] as HTMLElement).getByText("Fresh entry")).toBeVisible();
  });

  it("marks the list as live", () => {
    render(() => <HomeView />);
    expect(within(section("Recent activity")).getByRole("list")).toHaveAttribute(
      "aria-live",
      "polite",
    );
  });
});

describe("Coming up today", { timeout: SLOW_TEST_MS }, () => {
  const isolate = () => {
    M.S.schedules = [];
    M.S.calEvents = [];
    for (const card of M.S.cards) card.due = null;
  };
  const schedule = (patch: Record<string, unknown>) => {
    const [first] = snapshot.schedules;
    if (!first) throw new Error("seed has no schedules");
    return { ...first, enabled: true, ...patch } as (typeof M.S.schedules)[number];
  };
  const openCard = () => {
    const card = M.S.cards.find((c) => c.state === "review");
    if (!card) throw new Error("seed has no review card");
    card.due = 0;
    return card;
  };

  it("says when nothing is scheduled", () => {
    isolate();
    render(() => <HomeView />);
    expect(screen.getByText("Nothing else is scheduled today.")).toBeVisible();
  });

  it("lists schedules, events, and due cards by time", () => {
    isolate();
    M.S.schedules = [
      schedule({ id: "s-x", name: "Nightly", kind: "job", time: "23:00", days: [weekday()] }),
    ];
    M.S.calEvents = [{ id: "e-x", title: "Standup", time: "09:15", days: [weekday()] }];
    const due = openCard();
    render(() => <HomeView />);
    expect(screen.queryByText("Nothing else is scheduled today.")).not.toBeInTheDocument();
    const today = within(section("Coming up today"));
    const rows = [
      today.getByRole("button", { name: /Standup/ }),
      today.getByRole("button", { name: /Nightly/ }),
      today.getByRole("button", { name: new RegExp(`${M.cardLabelOf(due)} .* is due`) }),
    ];
    expect(rows.map((r) => r.querySelector("span")?.textContent)).toEqual([
      "09:15",
      "23:00",
      "Today",
    ]);
    expect(rows.map((r) => r.lastElementChild?.textContent)).toEqual(["Event", "Job", "Due"]);
    expect(rows[0]?.compareDocumentPosition(rows[1] as Node)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
  });

  it("leaves out disabled schedules and other weekdays", () => {
    isolate();
    M.S.schedules = [
      schedule({ id: "s-off", name: "Disabled job", enabled: false, days: [weekday()] }),
      schedule({ id: "s-other", name: "Other day", days: [(weekday() + 1) % 7] }),
    ];
    render(() => <HomeView />);
    expect(screen.queryByText("Disabled job")).not.toBeInTheDocument();
    expect(screen.queryByText("Other day")).not.toBeInTheDocument();
  });

  it("opens the schedule, says an event comes from Google Calendar, and opens a due card", () => {
    isolate();
    M.S.schedules = [schedule({ id: "s-open", name: "Open me", time: "07:00", days: [weekday()] })];
    M.S.calEvents = [{ id: "e-open", title: "Planning", time: "08:00", days: [weekday()] }];
    const due = openCard();
    render(() => <HomeView />);
    fireEvent.click(screen.getByRole("button", { name: /Open me/ }));
    expect(M.S).toMatchObject({ schedEdit: "s-open", settingsSection: "schedules" });
    M.go("home");
    fireEvent.click(screen.getByRole("button", { name: /Planning/ }));
    expect(M.S.toasts.at(-1)?.msg).toBe("Planning is from Google Calendar");
    fireEvent.click(
      screen.getByRole("button", { name: new RegExp(`${M.cardLabelOf(due)} .* is due`) }),
    );
    expect(M.S.openId).toBe(due.id);
  });

  it("opens the calendar of the busiest project when Home has no project", () => {
    render(() => <HomeView />);
    fireEvent.click(screen.getByRole("button", { name: "Open calendar" }));
    expect(M.S.route).toMatchObject({ page: "project", view: "calendar" });
  });

  it("offers Show all past five rows", () => {
    isolate();
    const days = [0, 1, 2, 3, 4, 5, 6];
    M.S.calEvents = Array.from({ length: 7 }, (_, i) => ({
      id: `e${i}`,
      title: `Event ${i}`,
      time: `1${i}:00`,
      days,
    }));
    render(() => <HomeView />);
    const buttons = () =>
      within(section("Coming up today")).getAllByRole("button", { name: /Event/ });
    expect(buttons()).toHaveLength(5);
    fireEvent.click(screen.getByRole("button", { name: "Show all 7" }));
    expect(buttons()).toHaveLength(7);
  });
});

describe("Agents awake", { timeout: SLOW_TEST_MS }, () => {
  const sleepButtons = () =>
    within(section("Agents awake")).getAllByRole("button", { name: /^Sleep / });

  it("counts the awake cards against the limit, amber once the limit is reached", () => {
    render(() => <HomeView />);
    const count = M.awake().length;
    const label = screen.getByText(`${count} of ${M.S.limits.global.awake}`);
    expect(label).toHaveClass("text-secondary");
    M.S.limits.global.awake = count;
    expect(label).toHaveClass("text-status-needs-you-text");
  });

  it("lists working cards first, five at first", () => {
    render(() => <HomeView />);
    expect(sleepButtons()).toHaveLength(5);
    const states = within(section("Agents awake")).getAllByText(
      /^(Working|Needs you|Planning|Merging|In review|Ready to merge)$/,
    );
    const order = ["Working", "Needs you", "Planning", "Merging", "In review", "Ready to merge"];
    const ranks = states.map((s) => order.indexOf(s.textContent ?? ""));
    expect(ranks).toEqual([...ranks].sort((a, b) => a - b));
    expect(ranks[0]).toBe(0);
  });

  it("shows each card's state, project, and agent", () => {
    render(() => <HomeView />);
    const card = M.awake().find((c) => c.state === "working");
    if (!card) throw new Error("seed has no working card");
    const view = M.deco(card);
    const row = screen.getByRole("button", { name: `Sleep ${M.cardLabelOf(card)}` })
      .parentElement as HTMLElement;
    expect(within(row).getByText(view.title)).toHaveClass("font-semibold");
    expect(within(row).getByText(view.stateLabel)).toBeVisible();
    expect(within(row).getByText(view.projectName)).toBeVisible();
    expect(within(row).getByText(view.agent)).toBeVisible();
  });

  it("titles each Sleep button and puts the card to sleep", () => {
    render(() => <HomeView />);
    fireEvent.click(within(section("Agents awake")).getByRole("button", { name: /^Show all/ }));
    const card = M.awake().find((c) => c.state === "review");
    if (!card) throw new Error("seed has no card in review");
    const button = screen.getByRole("button", { name: `Sleep ${M.cardLabelOf(card)}` });
    expect(button).toHaveAttribute("title", `Sleep ${M.cardLabelOf(card)}`);
    fireEvent.click(button);
    expect(M.card(card.id)?.asleep).toBe(true);
    expect(
      screen.queryByRole("button", { name: `Sleep ${M.cardLabelOf(card)}` }),
    ).not.toBeInTheDocument();
  });

  it("shows every card on request, and opens the busiest project's agents", () => {
    render(() => <HomeView />);
    const total = M.awake().length;
    fireEvent.click(screen.getByRole("button", { name: `Show all ${total}` }));
    expect(sleepButtons()).toHaveLength(total);
    fireEvent.click(screen.getByRole("button", { name: "Open agents" }));
    expect(M.S.route).toMatchObject({ page: "project", view: "agents" });
  });

  it("opens a card from its row", () => {
    render(() => <HomeView />);
    const row = sleepButtons()[0]?.parentElement as HTMLElement;
    fireEvent.click(row.querySelector("button") as HTMLElement);
    expect(M.S.openId).not.toBeNull();
  });
});

describe("CI health", { timeout: SLOW_TEST_MS }, () => {
  const rows = () => within(section("CI health")).getAllByRole("button", { name: /Main/ });

  it("shows each project's main branch state and how long ago", () => {
    render(() => <HomeView />);
    expect(rows().map((r) => r.textContent)).toEqual([
      "api-gatewayMain passed38 min ago",
      "web-dashboardMain running2 min ago",
      "mobile-appMain failed22 min ago",
    ]);
  });

  it("colors the state and opens the project", () => {
    render(() => <HomeView />);
    expect(within(section("CI health")).getByText("failed").parentElement).toHaveClass(
      "text-status-danger-text",
      "text-small",
      "font-semibold",
    );
    fireEvent.click(rows()[2] as HTMLElement);
    expect(M.S.route).toMatchObject({ page: "project", pid: "mobile" });
  });

  it("says hours for old runs and nothing for a project without a run time", () => {
    const [api, web] = M.S.projects;
    if (!api || !web) throw new Error("seed has too few projects");
    api.ciAgo = 150;
    web.ciAgo = 0;
    render(() => <HomeView />);
    expect(rows().map((r) => r.textContent)).toEqual([
      "api-gatewayMain passed3 h ago",
      "web-dashboardMain running",
      "mobile-appMain failed22 min ago",
    ]);
  });

  const withoutCi = (project: (typeof M.S.projects)[number]) => {
    delete project.ci;
    delete project.ciAgo;
    delete project.monthBase;
    delete project.runs;
  };

  it("lists only the projects that have CI data, and draws nothing made up for the rest", () => {
    const web = M.proj("web");
    if (!web) throw new Error("seed has no web project");
    withoutCi(web);
    render(() => <HomeView />);
    expect(rows().map((r) => r.textContent)).toEqual([
      "api-gatewayMain passed38 min ago",
      "mobile-appMain failed22 min ago",
    ]);
    expect(within(section("CI health")).queryByText(/queued/i)).toBeNull();
  });

  it("says GitHub is not connected, and shows no row, when no project has CI data", () => {
    for (const project of M.S.projects) withoutCi(project);
    render(() => <HomeView />);
    const ci = section("CI health");
    expect(within(ci).queryAllByRole("button", { name: /Main/ })).toHaveLength(0);
    expect(
      within(ci).getByText(
        "GitHub is not connected. CI runs appear here once GitHub is connected.",
      ),
    ).toBeInTheDocument();
    expect(within(ci).queryByRole("button", { name: "View all CI runs" })).toBeNull();
    fireEvent.click(within(ci).getByRole("button", { name: "Connect GitHub" }));
    expect(M.S).toMatchObject({ settingsSection: "integrations", route: { page: "settings" } });
  });

  it("offers Show all beyond five projects and links to the full CI page", () => {
    for (const id of ["p4", "p5", "p6"]) {
      M.S.projects.push({
        id,
        name: `extra-${id}`,
        lang: "Go",
        path: "~/x",
        ci: "passed",
        ciAgo: 5,
        monthBase: 1,
        runs: [],
      });
    }
    render(() => <HomeView />);
    expect(rows()).toHaveLength(5);
    fireEvent.click(screen.getByRole("button", { name: "Show all 6" }));
    expect(rows()).toHaveLength(6);
    fireEvent.click(screen.getByRole("button", { name: "View all CI runs" }));
    expect(M.S).toMatchObject({ allKind: "ci", route: { page: "all" } });
  });
});
