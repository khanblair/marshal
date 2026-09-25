import { render, screen } from "@solidjs/testing-library";
import { ChartLegend } from "./ChartLegend";

describe("ChartLegend", () => {
  it("lists each name then the limit, with a swatch styled like its line", () => {
    const { container } = render(() => (
      <ChartLegend names={["All projects", "Api", "Web", "Mobile"]} limitName="Daily limit" />
    ));
    const entries = [...container.querySelectorAll("span")].map((s) => s.textContent);
    expect(entries).toEqual(["All projects", "Api", "Web", "Mobile", "Daily limit"]);
    const lines = [...container.querySelectorAll("line")];
    expect(lines).toHaveLength(5);
    expect(lines[0]).toHaveClass("stroke-chart-1");
    expect(lines[2]).toHaveAttribute("stroke-dasharray", "6 3");
    expect(lines[3]).toHaveAttribute("stroke-dasharray", "1 4");
    expect(lines[3]).toHaveAttribute("stroke-linecap", "round");
    expect(lines[4]).toHaveClass("stroke-chart-limit");
    expect(lines[4]).not.toHaveAttribute("stroke-linecap");
  });

  it("hides the swatches from assistive tech and passes attributes through", () => {
    const { container } = render(() => (
      <ChartLegend names={["A"]} limitName="Daily limit" data-testid="legend" class="mt-1" />
    ));
    expect(screen.getByTestId("legend")).toHaveClass("mt-1", "text-caption", "text-secondary");
    expect(container.querySelectorAll("svg[aria-hidden='true']")).toHaveLength(2);
  });
});
