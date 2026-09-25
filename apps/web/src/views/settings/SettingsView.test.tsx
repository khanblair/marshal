import { cleanup, fireEvent, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { SECTIONS } from "./sections";
import { DESKTOP_WIDTH_PX, PHONE_WIDTH_PX, showSettings } from "./test-support";

vi.hoisted(() => {
  window.location.hash = "#nosim";
});

beforeEach(() => {
  vi.useFakeTimers();
});
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

const nav = () => screen.getByRole("navigation", { name: "Settings sections" });

describe("SettingsView shell", () => {
  it("is a full-size root the shell can recognise, with a section list", () => {
    const { container } = showSettings("general");
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveAttribute("data-no-nav", "1");
    expect(root).toHaveClass("absolute", "inset-0", "flex", "bg-canvas");
    expect(root).not.toHaveClass("flex-col");
    const labels = within(nav())
      .getAllByRole("button")
      .map((b) => b.textContent);
    expect(labels).toEqual(SECTIONS.map((section) => section.label));
  });

  it("marks the chosen section as the current page", () => {
    showSettings("roles");
    const buttons = within(nav()).getAllByRole("button");
    const current = buttons.filter((b) => b.getAttribute("aria-current") === "page");
    expect(current.map((b) => b.textContent)).toEqual(["Roles"]);
    expect(current[0]).toHaveClass("bg-surface-selected", "font-semibold");
    expect(buttons[0]).toHaveClass("bg-transparent", "font-medium");
  });

  it("switches section when a button is pressed", () => {
    showSettings("general");
    expect(screen.getByRole("heading", { name: "Theme" })).toBeInTheDocument();
    fireEvent.click(within(nav()).getByRole("button", { name: "Keyboard shortcuts" }));
    expect(M.S.settingsSection).toBe("shortcuts");
    expect(
      screen.getByRole("heading", { name: "Keyboard shortcuts", level: 2 }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Theme" })).toBeNull();
  });

  it("shows nothing but the list for a section it does not know", () => {
    showSettings("nothing");
    expect(screen.queryByRole("heading", { level: 2 })).toBeNull();
    expect(within(nav()).getAllByRole("button")).toHaveLength(SECTIONS.length);
  });

  it("puts the list on top as a row on phones, with taller buttons", () => {
    const { container } = showSettings("general", {}, PHONE_WIDTH_PX);
    expect(container.firstElementChild).toHaveClass("flex-col");
    expect(nav()).toHaveClass("flex-row", "border-b");
    expect(within(nav()).getByRole("button", { name: "General" })).toHaveClass("min-h-11");
  });

  it("puts the list in a 232 px rail beside the page on larger screens", () => {
    showSettings("general", {}, DESKTOP_WIDTH_PX);
    expect(nav()).toHaveClass("flex-col", "w-sidebar", "border-r");
    expect(within(nav()).getByRole("button", { name: "General" })).toHaveClass("min-h-8");
  });

  it("opens the schedule editor a caller asked for, once", () => {
    showSettings("schedules", { schedEdit: "s3" });
    expect(screen.getByRole("button", { name: "Close" })).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toHaveValue("Check open issues");
    expect(M.S.schedEdit).toBeNull();
  });

  it("keeps unsaved edits while another section is shown", () => {
    showSettings("profile");
    fireEvent.input(screen.getByLabelText("Name"), { target: { value: "Ada Byron" } });
    fireEvent.click(within(nav()).getByRole("button", { name: "Help" }));
    fireEvent.click(within(nav()).getByRole("button", { name: "Profile" }));
    expect(screen.getByLabelText("Name")).toHaveValue("Ada Byron");
  });
});
