import { cleanup, fireEvent, render, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { HomeAllView } from "./HomeAllView";
import { type HomeSnapshot, homeSnapshot, PHONE_WIDTH_PX, resetHome } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

const snapshot: HomeSnapshot = homeSnapshot();

beforeEach(() => {
  vi.useFakeTimers();
  resetHome(snapshot);
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

const kindButton = (label: string) => screen.getByRole("button", { name: label });
const rowTexts = () => screen.getAllByRole("listitem").map((li) => li.textContent);

describe("HomeAllView frame", () => {
  it("has a way back, a project select, and a heading", () => {
    const { container } = render(() => <HomeAllView />);
    expect(container.firstElementChild).toHaveClass("absolute", "inset-0", "overflow-y-auto");
    expect(screen.getByRole("heading", { level: 2, name: "Recent activity" })).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "Back to Home" }));
    expect(M.S.route.page).toBe("home");
  });

  it("styles the back button as a quiet button with a left chevron", () => {
    render(() => <HomeAllView />);
    const back = screen.getByRole("button", { name: "Back to Home" });
    expect(back).toHaveClass("border-none", "text-secondary", "pl-1.5!", "pr-2.5!");
    expect(back.querySelector("svg")).not.toBeNull();
  });

  it("lists every project after All projects", () => {
    render(() => <HomeAllView />);
    const select = screen.getByRole("combobox", { name: "Project" });
    expect([...select.querySelectorAll("option")].map((o) => o.textContent)).toEqual([
      "All projects",
      "api-gateway",
      "web-dashboard",
      "mobile-app",
    ]);
    expect(select).toHaveValue("all");
  });

  it("pads 16 px on phones and 24 px elsewhere", () => {
    const wide = render(() => <HomeAllView />);
    expect(wide.container.querySelector(".max-w-\\[880px\\]")).toHaveClass("px-6", "pt-6", "pb-12");
    wide.unmount();
    M.setViewport(PHONE_WIDTH_PX, 800);
    const phone = render(() => <HomeAllView />);
    expect(phone.container.querySelector(".max-w-\\[880px\\]")).toHaveClass("px-4", "pt-4", "pb-8");
  });
});

describe("Recent activity page", () => {
  it("lists every entry, newest first", () => {
    render(() => <HomeAllView />);
    expect(screen.getAllByRole("listitem")).toHaveLength(M.S.feed.length);
    expect(rowTexts()[0]).toContain(M.S.feed[0]?.text);
  });

  it("offers the seven kinds, with All pressed first", () => {
    render(() => <HomeAllView />);
    const group = screen.getByRole("group", { name: "Kinds of activity" });
    const pills = within(group).getAllByRole("button");
    expect(pills.map((p) => p.textContent)).toEqual([
      "All",
      "Merges",
      "CI results",
      "Approvals",
      "Plans",
      "Schedule runs",
      "Briefs",
    ]);
    expect(pills[0]).toHaveAttribute("aria-pressed", "true");
    expect(pills[0]).toHaveClass("bg-ink", "text-on-ink");
    expect(pills[1]).toHaveAttribute("aria-pressed", "false");
    expect(pills[1]).toHaveClass("bg-surface", "border-border-strong");
    for (const pill of pills) expect(pill).toHaveClass("rounded-full", "h-8", "px-3");
  });

  it("filters by kind", () => {
    render(() => <HomeAllView />);
    fireEvent.click(kindButton("Merges"));
    expect(kindButton("Merges")).toHaveAttribute("aria-pressed", "true");
    expect(kindButton("All")).toHaveAttribute("aria-pressed", "false");
    const merges = M.S.feed.filter((f) => f.kind === "merge");
    expect(screen.getAllByRole("listitem")).toHaveLength(merges.length);
    fireEvent.click(kindButton("CI results"));
    expect(screen.getAllByRole("listitem")).toHaveLength(
      M.S.feed.filter((f) => f.kind === "ci").length,
    );
    fireEvent.click(kindButton("All"));
    expect(screen.getAllByRole("listitem")).toHaveLength(M.S.feed.length);
  });

  it("filters by project, and by both", () => {
    render(() => <HomeAllView />);
    const select = screen.getByRole("combobox", { name: "Project" });
    fireEvent.change(select, { target: { value: "web" } });
    expect(screen.getAllByRole("listitem")).toHaveLength(
      M.S.feed.filter((f) => f.pid === "web").length,
    );
    fireEvent.click(kindButton("Merges"));
    expect(screen.getAllByRole("listitem")).toHaveLength(
      M.S.feed.filter((f) => f.pid === "web" && f.kind === "merge").length,
    );
  });

  it("says when nothing matches", () => {
    render(() => <HomeAllView />);
    fireEvent.change(screen.getByRole("combobox", { name: "Project" }), {
      target: { value: "api" },
    });
    fireEvent.click(kindButton("Briefs"));
    expect(screen.getByText("No activity matches these filters.")).toBeVisible();
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });

  it("opens what an entry is about", () => {
    render(() => <HomeAllView />);
    fireEvent.click(screen.getByRole("button", { name: /Plan ready for review on #43/ }));
    expect(M.S.openId).toBe(43);
  });

  it("shows entries added while the page is open", () => {
    render(() => <HomeAllView />);
    const before = screen.getAllByRole("listitem").length;
    M.S.feed.unshift({ id: "f-new", kind: "merge", text: "#1 merged", pid: "api", ts: M.now() });
    expect(screen.getAllByRole("listitem")).toHaveLength(before + 1);
  });
});

describe("CI health page", () => {
  beforeEach(() => {
    M.set({ allKind: "ci" });
  });

  it("swaps the heading and the content", () => {
    render(() => <HomeAllView />);
    expect(screen.getByRole("heading", { level: 2, name: "CI health" })).toBeVisible();
    expect(screen.queryByRole("group", { name: "Kinds of activity" })).not.toBeInTheDocument();
    expect(screen.queryAllByRole("listitem")).toHaveLength(0);
  });

  it("has a section per project with its main state and an Open board button", () => {
    render(() => <HomeAllView />);
    const headings = screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent);
    expect(headings).toEqual(["api-gateway", "web-dashboard", "mobile-app"]);
    const api = screen.getByRole("heading", { level: 3, name: "api-gateway" })
      .parentElement as HTMLElement;
    expect(within(api).getByText("passed").parentElement).toHaveClass(
      "text-status-working-text",
      "text-small",
    );
    fireEvent.click(within(api).getByRole("button", { name: "Open board" }));
    expect(M.S.route).toMatchObject({ page: "project", pid: "api", view: "board" });
  });

  it("lists workflow runs on main with their package, newest first", () => {
    render(() => <HomeAllView />);
    const mobile = screen
      .getByRole("heading", { level: 3, name: "mobile-app" })
      .closest("section") as HTMLElement;
    for (const name of ["android apps/android", "ios apps/ios", "packages packages/*"]) {
      expect(within(mobile).getByText(name)).toHaveClass("font-mono");
    }
    expect(within(mobile).getAllByText("main")).toHaveLength(3);
    // "ago" text is rounded to minutes, hours, or days, so parse it back to minutes
    // before checking order: "4 h ago" (240 min) must not sort before "44 min ago".
    const ages = [...mobile.querySelectorAll("button > span:last-child")].map(
      (s) => s.textContent ?? "",
    );
    const minutes = ages.map((age) => {
      const n = Number.parseInt(age, 10);
      if (age.endsWith("days ago")) return n * 24 * 60;
      if (age.endsWith("h ago")) return n * 60;
      return n;
    });
    expect(minutes).toEqual([...minutes].sort((x, y) => x - y));
    // The android workflow and card #213 both failed, so scope to the android row.
    const androidRow = within(mobile)
      .getByText("android apps/android")
      .closest("button") as HTMLElement;
    expect(within(androidRow).getByText("Failed")).toHaveClass(
      "text-status-danger-text",
      "font-semibold",
    );
    expect(within(mobile).getAllByText(/min ago$/)[0]).toHaveClass("w-21", "text-right");
  });

  it("lists the runs of cards under the project with the card as their place", () => {
    render(() => <HomeAllView />);
    const card = M.cardsOf("api").find((c) => c.ci);
    if (!card) throw new Error("seed has no api card with CI");
    const api = screen
      .getByRole("heading", { level: 3, name: "api-gateway" })
      .closest("section") as HTMLElement;
    const row = within(api).getByText(`#${card.id} ${card.title}`).closest("button") as HTMLElement;
    expect(within(row).getByText(card.branch ?? "")).toHaveClass("font-mono");
    fireEvent.click(row);
    expect(M.S.openId).toBe(card.id);
  });

  it("says days for old runs", () => {
    const api = M.S.projects[0];
    if (!api) throw new Error("seed has no project");
    api.runs = [{ wf: "release", st: "cancelled", ago: 4320 }];
    render(() => <HomeAllView />);
    const release = screen.getByText("release").closest("button") as HTMLElement;
    expect(within(release).getByText("3 days ago")).toBeVisible();
    expect(within(release).getByText("Cancelled")).toHaveClass("text-muted");
  });

  it("narrows to one project with the select", () => {
    render(() => <HomeAllView />);
    fireEvent.change(screen.getByRole("combobox", { name: "Project" }), {
      target: { value: "web" },
    });
    expect(screen.getAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual([
      "web-dashboard",
    ]);
  });
});
