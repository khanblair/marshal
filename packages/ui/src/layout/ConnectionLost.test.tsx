import { fireEvent, render, screen } from "@solidjs/testing-library";
import { ConnectionLost } from "./ConnectionLost";

const SENTENCE =
  "Marshal runs as a background service on this computer. Check that it is running. This screen closes by itself when it answers.";

describe("ConnectionLost", () => {
  it("says the daemon cannot be reached and what to do, announced as an alert", () => {
    render(() => <ConnectionLost onRetry={() => {}} />);
    const alert = screen.getByRole("alert");
    expect(alert).toContainElement(screen.getByRole("heading", { name: "Can't reach the daemon" }));
    expect(alert).toHaveTextContent(SENTENCE);
  });

  it("retries when Try again is pressed", () => {
    const onRetry = vi.fn();
    render(() => <ConnectionLost onRetry={onRetry} />);
    const button = screen.getByRole("button", { name: "Try again" });
    expect(button).toHaveAttribute("type", "button");
    fireEvent.click(button);
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("shows the countdown only when it is set, outside the alert", () => {
    const { unmount } = render(() => <ConnectionLost onRetry={() => {}} />);
    expect(screen.queryByText(/Trying again/)).toBeNull();
    unmount();
    const { unmount: unmountNull } = render(() => (
      <ConnectionLost onRetry={() => {}} retryInSeconds={null} />
    ));
    expect(screen.queryByText(/Trying again/)).toBeNull();
    unmountNull();
    render(() => <ConnectionLost onRetry={() => {}} retryInSeconds={4} />);
    const line = screen.getByText("Trying again in 4 s");
    expect(line).toHaveClass("whitespace-nowrap");
    expect(screen.getByRole("alert")).not.toContainElement(line);
  });

  it("shows a countdown of zero, rounds up, and never goes below zero", () => {
    const { unmount } = render(() => <ConnectionLost onRetry={() => {}} retryInSeconds={0} />);
    expect(screen.getByText("Trying again in 0 s")).toBeInTheDocument();
    unmount();
    const { unmount: unmountFraction } = render(() => (
      <ConnectionLost onRetry={() => {}} retryInSeconds={3.2} />
    ));
    expect(screen.getByText("Trying again in 4 s")).toBeInTheDocument();
    unmountFraction();
    render(() => <ConnectionLost onRetry={() => {}} retryInSeconds={-2} />);
    expect(screen.getByText("Trying again in 0 s")).toBeInTheDocument();
  });

  it("keeps technical details inside a collapsed Details, and only when given", () => {
    const { container, unmount } = render(() => <ConnectionLost onRetry={() => {}} />);
    expect(container.querySelector("details")).toBeNull();
    expect(screen.queryByText("Details")).toBeNull();
    unmount();
    const view = render(() => (
      <ConnectionLost onRetry={() => {}} details="connection refused: 127.0.0.1:7421" />
    ));
    const details = view.container.querySelector("details") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    expect(screen.getByText("Details")).toBeInTheDocument();
    expect(screen.getByRole("alert")).not.toHaveTextContent("connection refused");
    fireEvent.click(screen.getByText("Details"));
    expect(screen.getByText("connection refused: 127.0.0.1:7421")).toBeVisible();
  });

  it("fills its container and centers the content without cutting it off", () => {
    const { container } = render(() => <ConnectionLost onRetry={() => {}} />);
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("flex-1", "w-full", "h-full", "min-h-0", "overflow-auto", "p-6");
    const column = root.firstElementChild as HTMLElement;
    expect(column).toHaveClass("m-auto", "items-center", "w-full", "max-w-[460px]", "text-center");
    expect(container.querySelector("svg")).toHaveAttribute("width", "20");
  });

  it("uses the phone layout: 16 px sides and room for the home bar", () => {
    const { container } = render(() => <ConnectionLost onRetry={() => {}} phone />);
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("px-4", "pt-4", "pb-[calc(16px+env(safe-area-inset-bottom))]");
    expect(root).not.toHaveClass("p-6");
  });

  it("adds class and passes other attributes to its root", () => {
    const { container } = render(() => (
      <ConnectionLost onRetry={() => {}} class="bg-canvas" data-testid="lost" />
    ));
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("bg-canvas", "flex-1");
    expect(root).toHaveAttribute("data-testid", "lost");
    expect(root).not.toHaveAttribute("onretry");
  });
});
