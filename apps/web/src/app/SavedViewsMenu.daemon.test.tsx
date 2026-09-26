// biome-ignore-all assist/source/organizeImports: the fake daemon's store has to be imported first, so the store `~/mock` builds is the one that follows it (the person's sections are the daemon's).
import { daemon } from "~/testing/daemon-person-store";
import { cleanup, fireEvent, render, screen, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { SavedViewsMenu } from "./SavedViewsMenu";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => {
  // One daemon and one store serve the whole file, so each test starts from the same menu.
  daemon.me.savedViews.splice(1);
  daemon.emit("project:api", "saved_view.updated", {
    projectId: "api",
    views: daemon.me.savedViews,
  });
  M.go("project", "api", "board");
  M.applyView("All cards");
  M.set({ menu: null, toasts: [] });
});
afterEach(cleanup);

const menuButton = () =>
  screen.getByRole("button", { name: /Unsaved view|All cards|Needs me|Testers/ });
const items = () => screen.getAllByRole("menuitemradio").map((item) => item.textContent);
const nameInput = () => screen.getByLabelText<HTMLInputElement>("Name this view");
const patches = () => daemon.bodies("PATCH /v1/me/preferences");

describe("the saved views menu on the daemon", () => {
  it("lists the client's All cards and the daemon's views, with All cards in use", () => {
    render(() => <SavedViewsMenu />);
    fireEvent.click(menuButton());
    expect(items()).toEqual(["All cards", "Needs me1 filter"]);
    expect(screen.getByRole("menuitemradio", { name: /All cards/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  });

  it("applies a view: its filters, and the daemon is told the view in use", async () => {
    render(() => <SavedViewsMenu />);
    fireEvent.click(menuButton());
    fireEvent.click(screen.getByRole("menuitemradio", { name: /Needs me/ }));
    expect(M.S.filters.api).toEqual([{ k: "status", v: "needs" }]);
    expect(M.S.savedView.api).toBe("Needs me");
    await waitFor(() =>
      expect(daemon.me.preferences.projects.api?.savedViewId).toBe("01M3C107JB041061050R3GG2V1"),
    );
    expect(patches().at(-1)).toMatchObject({
      projects: { api: { savedViewId: "01M3C107JB041061050R3GG2V1" } },
    });
  });

  it("saves the filters on screen under a name, and lists the new view in use", async () => {
    M.addFilter("role", "Tester");
    render(() => <SavedViewsMenu />);
    fireEvent.click(menuButton());
    fireEvent.input(nameInput(), { target: { value: "  Testers  " } });
    fireEvent.click(screen.getByRole("button", { name: "Save view" }));
    await waitFor(() => expect(M.S.savedView.api).toBe("Testers"));
    expect(daemon.bodies("POST /v1/projects/api/saved-views").at(-1)).toEqual({
      name: "Testers",
      filters: [{ key: "role", value: "Tester" }],
      swimlane: "none",
    });
    expect(M.S.toasts.map((toast) => toast.msg)).toContain("View saved");
    expect(screen.queryByRole("menuitemradio")).toBeNull();
    expect(menuButton()).toHaveTextContent("Testers");
  });

  it("shows the daemon's sentence when it refuses the name, and leaves the menu open", async () => {
    render(() => <SavedViewsMenu />);
    fireEvent.click(menuButton());
    fireEvent.input(nameInput(), { target: { value: "x".repeat(70) } });
    fireEvent.click(screen.getByRole("button", { name: "Save view" }));
    await waitFor(() =>
      expect(M.S.toasts.map((toast) => toast.msg)).toContain(
        "Saved view names can have at most 60 characters.",
      ),
    );
    expect(screen.getAllByRole("menuitemradio").length).toBeGreaterThan(0);
  });
});
