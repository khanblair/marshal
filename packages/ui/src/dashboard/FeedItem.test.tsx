import { fireEvent, render, screen } from "@solidjs/testing-library";
import { FeedItem } from "./FeedItem";

describe("FeedItem", () => {
  it("shows a colored icon, the text, its project, and the time", () => {
    const onClick = vi.fn();
    const { container } = render(() => (
      <ol>
        <FeedItem
          icon="git-merge"
          iconColor="var(--color-status-ready-solid)"
          project="mobile-app"
          when="3 days ago"
          whenTitle="Sep 21, 2026, 9:00 AM"
          onClick={onClick}
        >
          #205 Refresh tokens in secure storage merged into main
        </FeedItem>
      </ol>
    ));
    expect(screen.getByRole("listitem")).toHaveClass("border-t", "border-border");
    const row = screen.getByRole("button");
    expect(row).toHaveClass("w-full", "items-start", "gap-2.5", "py-2.5", "hover:bg-surface-hover");
    const icon = container.querySelector("svg")?.parentElement as HTMLElement;
    expect(icon.style.color).toBe("var(--color-status-ready-solid)");
    expect(icon).toHaveClass("mt-0.5");
    expect(screen.getByText("mobile-app")).toHaveClass("text-caption", "text-secondary");
    expect(screen.getByText("3 days ago")).toHaveAttribute("title", "Sep 21, 2026, 9:00 AM");
    fireEvent.click(row);
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("leaves out the project line when there is none", () => {
    render(() => (
      <ol>
        <FeedItem icon="info" when="Just now">
          Morning brief is ready
        </FeedItem>
      </ol>
    ));
    expect(screen.getByRole("button").querySelectorAll(".text-caption")).toHaveLength(1);
    expect(() => fireEvent.click(screen.getByRole("button"))).not.toThrow();
  });
});
