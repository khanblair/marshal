import { render, screen } from "@solidjs/testing-library";
import { ItemText } from "./ItemText";

describe("ItemText", () => {
  it("shows a bold title over secondary lines", () => {
    const { container } = render(() => (
      <ItemText
        basis={220}
        tight
        title="Morning brief"
        description="Every weekday at 8:00"
        detail="Send the brief"
      />
    ));
    expect(container.firstElementChild).toHaveClass("flex", "flex-col", "flex-[1_1_220px]");
    expect(screen.getByText("Morning brief")).toHaveClass("font-semibold");
    for (const text of ["Every weekday at 8:00", "Send the brief"]) {
      expect(screen.getByText(text)).toHaveClass("text-small", "text-secondary", "leading-4.5");
    }
  });

  it("inherits the line height unless tight, and skips missing lines", () => {
    const { container } = render(() => (
      <ItemText basis={160} title="Pixel 8" description="Last seen 40 min ago" />
    ));
    expect(container.firstElementChild).toHaveClass("flex-[1_1_160px]");
    expect(screen.getByText("Last seen 40 min ago")).not.toHaveClass("leading-4.5");
    expect(container.firstElementChild?.childElementCount).toBe(2);
  });
});
