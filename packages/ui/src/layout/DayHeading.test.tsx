import { render, screen } from "@solidjs/testing-library";
import { DayHeading } from "./DayHeading";

describe("DayHeading", () => {
  it("draws a level 3 heading with a line under it in the secondary color", () => {
    render(() => <DayHeading class="py-2">Fri, Sep 25</DayHeading>);
    const heading = screen.getByRole("heading", { level: 3, name: "Fri, Sep 25" });
    expect(heading).toHaveClass(
      "m-0",
      "text-body",
      "leading-5",
      "font-semibold",
      "border-b",
      "border-border",
      "text-secondary",
      "py-2",
    );
    expect(heading).not.toHaveClass("text-primary");
  });

  it("uses the primary color for today and passes attributes through", () => {
    render(() => (
      <DayHeading today id="day-today">
        Today, Thu, Sep 24
      </DayHeading>
    ));
    const heading = screen.getByRole("heading", { name: "Today, Thu, Sep 24" });
    expect(heading).toHaveClass("text-primary");
    expect(heading).not.toHaveClass("text-secondary");
    expect(heading).toHaveAttribute("id", "day-today");
  });
});
