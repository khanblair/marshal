import { render, screen } from "@solidjs/testing-library";
import { createSignal } from "solid-js";
import { BarChart } from "./BarChart";

const TICKS = [
  { index: 0, label: "Sep 18" },
  { index: 2, label: "Sep 20" },
];

function renderChart() {
  return render(() => (
    <BarChart
      width={300}
      values={[2, 0, 5]}
      tips={["Sep 18: 2 cards", "Sep 19: 0 cards", "Sep 20: 5 cards"]}
      ticks={TICKS}
      label="7 cards finished in the last 3 days"
    />
  ));
}

describe("BarChart", () => {
  it("is an image with the summary as its name and the design's size", () => {
    renderChart();
    const chart = screen.getByRole("img", { name: "7 cards finished in the last 3 days" });
    expect(chart).toHaveAttribute("width", "300");
    expect(chart).toHaveAttribute("height", "180");
    expect(chart).toHaveClass("block", "overflow-visible");
  });

  it("draws one path per day with its hover text, empty for a zero day", () => {
    const { container } = renderChart();
    const paths = [...container.querySelectorAll("path")];
    expect(paths.map((p) => p.querySelector("title")?.textContent)).toEqual([
      "Sep 18: 2 cards",
      "Sep 19: 0 cards",
      "Sep 20: 5 cards",
    ]);
    expect(paths[1]).toHaveAttribute("d", "");
    expect(paths[2]?.getAttribute("d")).toMatch(/^M\S+ 150 V/);
    expect(paths[0]).toHaveClass("fill-chart-1");
  });

  it("labels the grid 0, half, and max, and the chosen dates", () => {
    const { container } = renderChart();
    const texts = [...container.querySelectorAll("text")].map((t) => t.textContent);
    expect(texts).toEqual(["0", "3", "5", "Sep 18", "Sep 20"]);
    const label = container.querySelector("text");
    expect(label).toHaveAttribute("font-size", "12");
    expect(label).toHaveAttribute("text-anchor", "end");
  });

  it("follows a new width", () => {
    const [width, setWidth] = createSignal(300);
    const { container } = render(() => (
      <BarChart width={width()} values={[2, 4]} tips={["a", "b"]} ticks={[]} label="Bars" />
    ));
    const before = container.querySelector("path")?.getAttribute("d");
    setWidth(600);
    expect(screen.getByRole("img", { name: "Bars" })).toHaveAttribute("width", "600");
    expect(container.querySelector("path")?.getAttribute("d")).not.toBe(before);
  });
});
