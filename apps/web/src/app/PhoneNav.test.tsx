import { cleanup, fireEvent, render, screen } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { PhoneNav } from "./PhoneNav";
import { PHONE_PX, resetShell } from "./shell-test-utils";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => {
  resetShell(PHONE_PX);
});
afterEach(cleanup);

const tab = (name: string) => screen.getByRole("button", { name: new RegExp(`^${name}`) });

describe("PhoneNav", () => {
  it("draws nothing except on phones", () => {
    M.setViewport(1440, 900);
    render(() => <PhoneNav />);
    expect(screen.queryByRole("navigation")).toBeNull();
  });

  it("has five tabs in a labeled navigation", () => {
    render(() => <PhoneNav />);
    const nav = screen.getByRole("navigation", { name: "Main" });
    expect(nav).toHaveAttribute("data-tour", "views-phone");
    expect(screen.getAllByRole("button").map((b) => b.textContent?.replace(/\d+$/, ""))).toEqual([
      "Home",
      "Board",
      "Chats",
      "Agents",
      "More",
    ]);
  });

  it("tags only the More tab for the tour", () => {
    render(() => <PhoneNav />);
    expect(tab("More")).toHaveAttribute("data-tour", "more-phone");
    expect(tab("Board")).toHaveAttribute("data-tour", "");
  });

  it("marks Home current on Home, with the needs-you count over it", () => {
    render(() => <PhoneNav />);
    expect(tab("Home")).toHaveAttribute("aria-current", "page");
    expect(tab("Home")).toHaveTextContent(String(M.needs().length));
    expect(tab("Board")).not.toHaveAttribute("aria-current");
    expect(tab("Board")).toHaveClass("text-secondary", "font-normal");
  });

  it.each([
    ["board", "Board"],
    ["chat", "Chats"],
    ["agents", "Agents"],
  ] as const)("marks %s current on that view", (view, name) => {
    M.go("project", "api", view);
    render(() => <PhoneNav />);
    expect(tab(name)).toHaveAttribute("aria-current", "page");
    expect(tab(name)).toHaveClass("text-primary", "font-semibold");
    expect(
      screen.getAllByRole("button").filter((b) => b.hasAttribute("aria-current")),
    ).toHaveLength(1);
  });

  it.each(["list", "timeline", "calendar"] as const)(
    "marks More current on the %s view",
    (view) => {
      M.go("project", "api", view);
      render(() => <PhoneNav />);
      expect(tab("More")).toHaveAttribute("aria-current", "page");
    },
  );

  it("marks More current on Settings and while the More sheet is open", () => {
    M.go("settings");
    render(() => <PhoneNav />);
    expect(tab("More")).toHaveAttribute("aria-current", "page");
    M.go("home");
    expect(tab("More")).not.toHaveAttribute("aria-current");
    M.set({ menu: "more" });
    expect(tab("More")).toHaveAttribute("aria-current", "page");
  });

  it("goes to the tab's page and closes the card and notices", () => {
    M.go("project", "web", "board");
    M.openCard(41);
    M.set({ noticesOpen: true });
    const pid = M.S.route.pid;
    render(() => <PhoneNav />);
    fireEvent.click(tab("Agents"));
    expect(M.S.route).toMatchObject({ page: "project", pid, view: "agents" });
    expect(M.S.openId).toBeNull();
    expect(M.S.noticesOpen).toBe(false);
    fireEvent.click(tab("Home"));
    expect(M.S.route.page).toBe("home");
  });

  it("More opens its sheet", () => {
    render(() => <PhoneNav />);
    fireEvent.click(tab("More"));
    expect(M.S.menu).toBe("more");
  });
});
