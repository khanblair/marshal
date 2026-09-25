import { render, screen } from "@solidjs/testing-library";
import { ChartFigure } from "./ChartFigure";

describe("ChartFigure", () => {
  it("captions its children with a bold title and a secondary summary", () => {
    const { container } = render(() => (
      <ChartFigure title="Cards finished per day" summary="12 cards finished in the last 7 days">
        <p>chart goes here</p>
      </ChartFigure>
    ));
    const figure = container.querySelector("figure");
    expect(figure).toHaveClass("m-0", "flex", "flex-col", "gap-1.5", "min-w-0");
    expect(screen.getByText("Cards finished per day")).toHaveClass("font-semibold");
    expect(screen.getByText("12 cards finished in the last 7 days")).toHaveClass(
      "text-small",
      "text-secondary",
    );
    expect(container.querySelector("figcaption")?.nextElementSibling?.textContent).toBe(
      "chart goes here",
    );
  });
});
