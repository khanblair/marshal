import { render, screen } from "@solidjs/testing-library";
import { PhoneRowTitle } from "./PhoneRowTitle";

describe("PhoneRowTitle", () => {
  it("shows a bold title and the muted number", () => {
    render(() => <PhoneRowTitle title="Fix token refresh" num="#41" />);
    expect(screen.getByText("Fix token refresh")).toHaveClass("font-semibold");
    expect(screen.getByText("#41")).toHaveClass("text-small", "text-muted");
  });

  it("colors the title from the store when given a color", () => {
    render(() => (
      <PhoneRowTitle title="Done card" num="#33" titleColor="var(--color-text-secondary)" />
    ));
    expect(screen.getByText("Done card")).toHaveStyle({ color: "var(--color-text-secondary)" });
  });
});
