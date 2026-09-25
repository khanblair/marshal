import { render } from "@solidjs/testing-library";
import { StatusDot } from "./StatusDot";

const dot = (el: HTMLElement) => el.firstElementChild as HTMLElement;

describe("StatusDot", () => {
  it("is an 8 px round dot by default", () => {
    const { container } = render(() => <StatusDot state="review" />);
    const el = dot(container);
    expect(el).toHaveClass("inline-block", "flex-none", "rounded-full", "bg-status-review-solid");
    expect(el.style.width).toBe("8px");
    expect(el.style.height).toBe("8px");
  });

  it("pulses only for working", () => {
    const working = render(() => <StatusDot state="working" />);
    expect(dot(working.container)).toHaveClass("animate-pulse-working", "bg-status-working-solid");
    for (const state of [
      "backlog",
      "planning",
      "needs",
      "review",
      "ready",
      "merging",
      "done",
      "danger",
    ]) {
      const { container, unmount } = render(() => <StatusDot state={state} />);
      expect(dot(container)).not.toHaveClass("animate-pulse-working");
      unmount();
    }
  });

  it.each([
    ["backlog", "bg-border-strong"],
    ["planning", "bg-status-planning-solid"],
    ["needs", "bg-status-needs-you-solid"],
    ["merging", "bg-status-ready-solid"],
    ["done", "bg-status-done-solid"],
    ["danger", "bg-status-danger-solid"],
    ["something-else", "bg-border-strong"],
  ])("colors %s with %s", (state, cls) => {
    const { container } = render(() => <StatusDot state={state} />);
    expect(dot(container)).toHaveClass(cls);
  });

  it("takes a size, class, and attributes", () => {
    const { container } = render(() => (
      <StatusDot state="done" size={6} class="mt-1" aria-hidden="true" />
    ));
    const el = dot(container);
    expect(el.style.width).toBe("6px");
    expect(el).toHaveClass("mt-1");
    expect(el).toHaveAttribute("aria-hidden", "true");
  });
});
