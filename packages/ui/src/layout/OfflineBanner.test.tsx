import { render, screen } from "@solidjs/testing-library";
import { OfflineBanner } from "./OfflineBanner";

const MESSAGE = "You're offline. Marshal reconnects on its own. Changes can't be made until then.";

describe("OfflineBanner", () => {
  it("is a polite status with the offline message", () => {
    render(() => <OfflineBanner />);
    const banner = screen.getByRole("status");
    expect(banner).toHaveAttribute("aria-live", "polite");
    expect(screen.getByText(MESSAGE)).toBeInTheDocument();
  });

  it("does not promise that changes are queued", () => {
    render(() => <OfflineBanner />);
    expect(screen.getByRole("status").textContent).not.toMatch(/queue|saved|sync/i);
  });

  it("shows the countdown only when it is set, and keeps its ticks out of the announcement", () => {
    const { unmount } = render(() => <OfflineBanner />);
    expect(screen.queryByText(/Trying again/)).toBeNull();
    unmount();
    const { unmount: unmountNull } = render(() => <OfflineBanner retryInSeconds={null} />);
    expect(screen.queryByText(/Trying again/)).toBeNull();
    unmountNull();
    render(() => <OfflineBanner retryInSeconds={4} />);
    const line = screen.getByText("Trying again in 4 s");
    expect(line).toHaveAttribute("aria-live", "off");
    expect(screen.getByRole("status")).toContainElement(line);
  });

  it("is a full-width bar with square corners and a 14 px icon", () => {
    render(() => <OfflineBanner />);
    const banner = screen.getByRole("status");
    expect(banner).toHaveClass("flex-none", "rounded-none!", "py-1.5!", "px-4!", "text-small");
    expect(banner.querySelector("svg")).toHaveAttribute("width", "14");
  });

  it("lets its text wrap inside the bar, so a phone never scrolls sideways", () => {
    render(() => <OfflineBanner />);
    const text = screen.getByText(MESSAGE);
    expect(text).toHaveClass("min-w-0");
    expect(screen.getByRole("status")).not.toHaveClass("whitespace-nowrap");
  });

  it("adds class and passes other attributes to its root", () => {
    render(() => <OfflineBanner class="border-b" data-testid="offline" />);
    const banner = screen.getByRole("status");
    expect(banner).toHaveClass("border-b", "flex-none");
    expect(banner).toHaveAttribute("data-testid", "offline");
  });
});
