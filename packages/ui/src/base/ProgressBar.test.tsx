import { render, screen } from "@solidjs/testing-library";
import { ProgressBar } from "./ProgressBar";

const fill = (bar: HTMLElement) => bar.firstElementChild as HTMLElement;

describe("ProgressBar", () => {
  it("is a labelled 6 px progressbar with a neutral fill", () => {
    render(() => <ProgressBar label="Sub-tasks progress" value={75} class="flex-1" />);
    const bar = screen.getByRole("progressbar", { name: "Sub-tasks progress" });
    expect(bar).toHaveAttribute("aria-valuenow", "75");
    expect(bar).toHaveAttribute("aria-valuemin", "0");
    expect(bar).toHaveAttribute("aria-valuemax", "100");
    expect(bar).toHaveClass(
      "h-1.5",
      "rounded-full",
      "overflow-hidden",
      "bg-surface-sunken",
      "flex-1",
    );
    expect(fill(bar).style.width).toBe("75%");
    expect(fill(bar)).toHaveClass("bg-text-secondary", "transition-[width]", "duration-base");
  });

  it("draws the 4 px merge bar in teal on a teal tint", () => {
    render(() => <ProgressBar label="Merge progress" value={55} tone="ready" size={4} />);
    const bar = screen.getByRole("progressbar");
    expect(bar).toHaveClass("h-1", "bg-status-ready-subtle");
    expect(fill(bar)).toHaveClass("bg-status-ready-solid");
  });

  it("is a bordered meter for the context window, and clamps the value", () => {
    render(() => (
      <ProgressBar label="Context window used" value={130} tone="needs-you" meter class="w-14" />
    ));
    const meter = screen.getByRole("meter", { name: "Context window used" });
    expect(meter).toHaveClass("border", "border-border", "w-14");
    expect(meter).toHaveAttribute("aria-valuenow", "100");
    expect(fill(meter)).toHaveClass("bg-status-needs-you-solid");
  });

  it("uses green for a finished checklist", () => {
    render(() => <ProgressBar label="Done" value={-5} tone="working" />);
    const bar = screen.getByRole("progressbar");
    expect(fill(bar)).toHaveClass("bg-status-working-solid");
    expect(fill(bar).style.width).toBe("0%");
  });
});
