import { render, screen } from "@solidjs/testing-library";
import { LineChart } from "./LineChart";

const SERIES = [
  { name: "All projects", values: [4, 6, 8] },
  { name: "Api", values: [2, 3, 4] },
  { name: "Web", values: [2, 3, 4] },
];

function renderChart() {
  return render(() => (
    <LineChart
      width={400}
      series={SERIES}
      limit={10}
      limitLabel="Limit $10.00"
      ticks={[
        { index: 0, label: "Sep 22" },
        { index: 2, label: "Sep 24" },
      ]}
      format={(v) => `$${Math.round(v)}`}
      label="All projects today $8.00 of the $10.00 daily limit"
    />
  ));
}

describe("LineChart", () => {
  it("is an image with the summary as its name", () => {
    renderChart();
    const chart = screen.getByRole("img", {
      name: "All projects today $8.00 of the $10.00 daily limit",
    });
    expect(chart).toHaveAttribute("width", "400");
    expect(chart).toHaveAttribute("height", "180");
  });

  it("draws the limit line dashed with its red label", () => {
    const { container } = renderChart();
    const limit = container.querySelector("line.stroke-chart-limit");
    expect(limit).toHaveAttribute("stroke-dasharray", "5 4");
    expect(limit).toHaveAttribute("stroke-width", "1.5");
    const label = screen.getByText("Limit $10.00");
    expect(label).toHaveClass("fill-status-danger-text");
    expect(label).toHaveAttribute("x", "310");
  });

  it("draws each series with its own style and name at its last point", () => {
    const { container } = renderChart();
    const paths = [...container.querySelectorAll("path")];
    expect(paths).toHaveLength(3);
    expect(paths[0]).toHaveClass("stroke-chart-1");
    expect(paths[1]).toHaveClass("stroke-chart-2");
    expect(paths[2]).toHaveAttribute("stroke-dasharray", "6 3");
    expect(paths[0]?.getAttribute("d")).toMatch(/^M40\.0 \S+ L/);
    expect(screen.getByText("All projects")).toHaveClass("fill-chart-axis");
    const ys = ["All projects", "Api", "Web"].map((n) =>
      Number(screen.getByText(n).getAttribute("y")),
    );
    expect(ys[2]).toBeGreaterThanOrEqual((ys[1] ?? 0) + 14);
  });

  it("labels the grid with the formatter and the dates", () => {
    const { container } = renderChart();
    const texts = [...container.querySelectorAll("text")].map((t) => t.textContent);
    expect(texts).toEqual(
      expect.arrayContaining(["$0", "$6", "$11", "Sep 22", "Sep 24", "Api", "Web"]),
    );
  });
});
