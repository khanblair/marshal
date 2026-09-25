import { render } from "@solidjs/testing-library";
import { StatusLabel } from "./StatusLabel";

describe("StatusLabel", () => {
  it("shows the state in its text color, bold, with the flag in its solid color", () => {
    const { container } = render(() => <StatusLabel state="needs">Needs you</StatusLabel>);
    const label = container.firstElementChild as HTMLElement;
    expect(label).toHaveTextContent("Needs you");
    expect(label).toHaveClass(
      "inline-flex",
      "gap-1",
      "font-semibold",
      "text-status-needs-you-text",
    );
    const icon = label.querySelector("svg");
    expect(icon).toHaveAttribute("width", "14");
    expect(icon?.parentElement).toHaveClass("text-status-needs-you-solid");
  });

  it("uses secondary text and a muted flag for backlog", () => {
    const { container } = render(() => (
      <StatusLabel state="backlog" iconSize={12}>
        Backlog
      </StatusLabel>
    ));
    expect(container.firstElementChild).toHaveClass("text-secondary");
    expect(container.querySelector("svg")?.parentElement).toHaveClass("text-muted");
    expect(container.querySelector("svg")).toHaveAttribute("width", "12");
  });

  it("can hide the flag", () => {
    const { container } = render(() => (
      <StatusLabel state="working" withIcon={false} class="text-caption">
        Working
      </StatusLabel>
    ));
    expect(container.querySelector("svg")).toBeNull();
    expect(container.firstElementChild).toHaveClass("text-status-working-text", "text-caption");
  });
});
