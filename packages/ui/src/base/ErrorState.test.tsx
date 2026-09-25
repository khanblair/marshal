import { fireEvent, render, screen } from "@solidjs/testing-library";
import { ErrorState } from "./ErrorState";

const MESSAGE = "Couldn't load the activity. Check your connection and try again.";

describe("ErrorState", () => {
  it("shows the caller's message as plain text, not an alert, so failed sections do not each interrupt", () => {
    render(() => <ErrorState message={MESSAGE} />);
    expect(screen.getByText(MESSAGE)).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("offers Try again only when there is something to retry", () => {
    const onRetry = vi.fn();
    const { unmount } = render(() => <ErrorState message={MESSAGE} />);
    expect(screen.queryByRole("button")).toBeNull();
    unmount();
    render(() => <ErrorState message={MESSAGE} onRetry={onRetry} />);
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("has an expandable Details only when details are given", () => {
    const { container, unmount } = render(() => <ErrorState message={MESSAGE} />);
    expect(screen.queryByText("Details")).toBeNull();
    expect(container.querySelector("details")).toBeNull();
    unmount();
    const view = render(() => (
      <ErrorState message={MESSAGE} details="ERR_TIMEOUT: the daemon did not answer in 10 s" />
    ));
    const details = view.container.querySelector("details") as HTMLDetailsElement;
    expect(details.open).toBe(false);
    expect(details.querySelector("summary")).toHaveTextContent("Details");
    fireEvent.click(screen.getByText("Details"));
    expect(details.open).toBe(true);
    expect(screen.getByText("ERR_TIMEOUT: the daemon did not answer in 10 s")).toBeVisible();
  });

  it("keeps the technical text out of the message", () => {
    render(() => <ErrorState message={MESSAGE} details="ERR_TIMEOUT" />);
    expect(screen.getByText(MESSAGE)).not.toHaveTextContent("ERR_TIMEOUT");
  });

  it("wraps long details inside their own box, and lets touch screens tap the summary", () => {
    const { container } = render(() => <ErrorState message={MESSAGE} details={"x".repeat(400)} />);
    const box = container.querySelector("pre") as HTMLElement;
    expect(box).toHaveClass(
      "whitespace-pre-wrap",
      "[overflow-wrap:anywhere]",
      "overflow-auto",
      "m-0",
    );
    expect(container.querySelector("summary")).toHaveClass("[[data-touch='1']_&]:py-3");
  });

  it("is an empty state with a 20 px warning icon, and takes class and attributes", () => {
    const { container } = render(() => (
      <ErrorState message={MESSAGE} class="min-h-40" data-testid="err" />
    ));
    const root = container.firstElementChild as HTMLElement;
    expect(root).toHaveClass("flex-1", "items-center", "text-center", "min-h-40");
    expect(root).toHaveAttribute("data-testid", "err");
    expect(container.querySelector("svg")).toHaveAttribute("width", "20");
  });
});
