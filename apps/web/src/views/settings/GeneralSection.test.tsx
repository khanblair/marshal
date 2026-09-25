import { cleanup, fireEvent, screen, within } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { M } from "~/mock";
import { lastToast, showSettings } from "./test-support";

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

const themeGroup = () => screen.getByRole("radiogroup", { name: "Theme" });

describe("General section: theme", () => {
  it("shows three theme cards with the current one checked", () => {
    showSettings("general", { theme: "dark" });
    const cards = within(themeGroup()).getAllByRole("radio");
    expect(cards.map((c) => c.getAttribute("aria-checked"))).toEqual(["false", "true", "false"]);
    expect(within(themeGroup()).getByRole("radio", { name: /Dark/ })).toHaveClass("border-2");
    expect(screen.getByText("Chalk neutrals")).toBeInTheDocument();
    expect(screen.getByText("Asphalt neutrals")).toBeInTheDocument();
    expect(screen.getByText("Follows your OS setting")).toBeInTheDocument();
  });

  it("changes the theme when a card is pressed", () => {
    showSettings("general", { theme: "light" });
    fireEvent.click(within(themeGroup()).getByRole("radio", { name: /System/ }));
    expect(M.S.theme).toBe("system");
    expect(within(themeGroup()).getByRole("radio", { name: /System/ })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    fireEvent.click(within(themeGroup()).getByRole("radio", { name: /Dark/ }));
    expect(M.S.theme).toBe("dark");
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
  });

  it("previews light and dark boards, split for System", () => {
    showSettings("general");
    const previews = within(themeGroup())
      .getAllByRole("radio")
      .map((card) =>
        Array.from(card.querySelectorAll("span.flex-1")).map((half) => half.className),
      );
    expect(previews[0]?.every((c) => c.includes("bg-theme-preview-light-bg"))).toBe(true);
    expect(previews[1]?.every((c) => c.includes("bg-theme-preview-dark-bg"))).toBe(true);
    expect(previews[2]?.[0]).toContain("bg-theme-preview-light-bg");
    expect(previews[2]?.[1]).toContain("bg-theme-preview-dark-bg");
  });

  it("toasts for the custom theme editor", () => {
    showSettings("general");
    fireEvent.click(screen.getByRole("button", { name: "Create custom theme" }));
    expect(lastToast()).toBe("Custom theme editor opened");
  });
});

describe("General section: sessions", () => {
  it("shows the session options from the store", () => {
    showSettings("general");
    expect(screen.getByRole("heading", { name: "Sessions", level: 2 })).toBeInTheDocument();
    expect(screen.getByLabelText("After a restart", { exact: false })).toHaveValue(
      "Auto-restore on startup",
    );
    expect(screen.getByLabelText("Sleep idle cards after", { exact: false })).toHaveValue("15");
    expect(screen.getByLabelText("Sleep warnings go to")).toHaveValue("In app only");
    expect(
      screen.getByText("You get a notice 2 minutes before a card sleeps."),
    ).toBeInTheDocument();
  });

  it("saves each choice and toasts", () => {
    showSettings("general");
    fireEvent.change(screen.getByLabelText("Sleep idle cards after", { exact: false }), {
      target: { value: "60" },
    });
    expect(M.S.sleep.idle).toBe(60);
    fireEvent.change(screen.getByLabelText("After a restart", { exact: false }), {
      target: { value: "Show a resume button on each card" },
    });
    expect(M.S.sleep.restore).toBe("Show a resume button on each card");
    fireEvent.change(screen.getByLabelText("Sleep warnings go to"), {
      target: { value: "In app and Discord" },
    });
    expect(M.S.sleep.channel).toBe("In app and Discord");
    expect(lastToast()).toBe("Saved");
  });
});
