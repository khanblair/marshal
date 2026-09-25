import { render, screen } from "@solidjs/testing-library";
import { Callout } from "./Callout";

describe("Callout", () => {
  it("is a needs-you tinted status box with a 14 px icon", () => {
    render(() => (
      <Callout icon="copy">
        <strong>This looks like an existing card</strong>
      </Callout>
    ));
    const box = screen.getByRole("status");
    expect(box).toHaveClass(
      "bg-status-needs-you-subtle",
      "text-status-needs-you-text",
      "text-small",
      "leading-4.5",
      "rounded-md",
      "gap-2",
    );
    expect(box.querySelector("svg")).toHaveAttribute("width", "14");
  });

  it("has a neutral tone", () => {
    render(() => (
      <Callout tone="neutral" class="items-center">
        Detected a Go project.
      </Callout>
    ));
    const box = screen.getByRole("status");
    expect(box).toHaveClass("bg-surface-sunken", "text-secondary", "items-center");
    expect(box.querySelector("svg")).toBeNull();
  });
});
