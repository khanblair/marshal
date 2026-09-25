import {
  type AxisTick,
  barLayout,
  LINE_STYLES,
  lineLayout,
  lineStyle,
  roundedBarPath,
  spreadLabels,
  yScale,
} from "./chart-geometry";

/* The oracle below is HomeView's script from the design, copied with only its
   inputs turned into parameters. The port must print the same strings. */

const rnd = (i: number) => {
  const x = Math.sin(i * 12.9898 + 78.233) * 43758.5453;
  return x - Math.floor(x);
};

const DAY = 86_400_000;
const TODAY = Math.floor(Date.UTC(2026, 8, 24, 0, 0, 0) / DAY) * DAY;

function tickIndexes(n: number): number[] {
  if (n === 7) return [0, 3, 6];
  return n === 30 ? [0, 10, 20, 29] : [0, 30, 60, 89];
}

function inputs(N: number) {
  const dayIdx = (i: number) => Math.floor(TODAY / DAY) - (N - 1 - i);
  const fin = Array.from({ length: N }, (_, i) =>
    i === N - 1 ? 3 : Math.round(1 + rnd(dayIdx(i)) * 5 + (i % 3 === 0 ? -1 : 1)),
  );
  const base: Record<string, number> = { api: 3.6, web: 2.8, mobile: 5.4 };
  const ids = ["api", "web", "mobile"];
  const series = ids.map((id) =>
    Array.from({ length: N }, (_, i) =>
      i === N - 1
        ? 6.4
        : Math.max(0.2, (base[id] || 2) * (0.55 + rnd(dayIdx(i) * 3 + id.length) * 0.9)),
    ),
  );
  const total = Array.from({ length: N }, (_, i) => series.reduce((a, s) => a + (s[i] ?? 0), 0));
  return { fin, series: [total, ...series], tickIdx: tickIndexes(N) };
}

function oracleBars(fin: number[], cw: number, tickIdx: number[]) {
  const N = fin.length;
  const top = 8;
  const bot = 150;
  const maxB = Math.max(4, ...fin);
  const x0 = 32;
  const bw = (cw - x0) / N;
  const yB = (v: number) => bot - (v / maxB) * (bot - top);
  const barW = Math.max(2, bw * 0.64);
  const r = Math.min(3, barW / 2);
  const bars = fin.map((v, i) => {
    const x = x0 + i * bw + (bw - barW) / 2;
    const y = yB(v);
    const h = bot - y;
    const rr = Math.min(r, h);
    return h <= 0
      ? ""
      : `M${x} ${bot} V${y + rr} Q${x} ${y} ${x + rr} ${y} H${x + barW - rr} Q${x + barW} ${y} ${x + barW} ${y + rr} V${bot} Z`;
  });
  return {
    bars,
    xLabels: tickIdx.map((i) => x0 + i * bw + bw / 2),
    yGrid: [0, Math.round(maxB / 2), maxB].map((v) => ({ y: yB(v), ty: yB(v) + 4, l: v })),
  };
}

function oracleLines(arrs: number[][], limit: number, cw: number, tickIdx: number[]) {
  const first = arrs[0] ?? [];
  const N = first.length;
  const top = 8;
  const bot = 150;
  const xl0 = 40;
  const plotR = cw - 96;
  const lw = (plotR - xl0) / Math.max(1, N - 1);
  const maxL = Math.max(limit * 1.1, ...first);
  const yL = (v: number) => bot - (v / maxL) * (bot - top);
  const path = (arr: number[]) =>
    arr.map((v, i) => `${i ? "L" : "M"}${(xl0 + i * lw).toFixed(1)} ${yL(v).toFixed(1)}`).join(" ");
  const lys = arrs.map((s) => yL(s[N - 1] ?? 0) + 4);
  const order: [number, number][] = lys.map((y, i) => [y, i]);
  order.sort((a, b) => a[0] - b[0]);
  for (let k = 1; k < order.length; k++) {
    const cur = order[k];
    const prev = order[k - 1];
    if (cur && prev && cur[0] - prev[0] < 14) cur[0] = prev[0] + 14;
  }
  for (const [y, i] of order) lys[i] = y;
  return {
    paths: arrs.map(path),
    lys,
    xLabels: tickIdx.map((i) => xl0 + i * lw),
    yGrid: [0, maxL / 2, maxL].map((v) => ({ y: yL(v), ty: yL(v) + 4, l: `$${Math.round(v)}` })),
    plotR,
    limitY: yL(limit),
    limitTY: yL(limit) - 6,
  };
}

const RANGES = [7, 30, 90, 45];
const WIDTHS = [240, 358, 520, 1100];
const LIMIT = 25;

const asTicks = (idx: number[]): AxisTick[] => idx.map((index) => ({ index, label: `d${index}` }));

describe("barLayout", () => {
  for (const N of RANGES) {
    for (const width of WIDTHS) {
      it(`matches the design for ${N} days at ${width} px`, () => {
        const { fin, tickIdx } = inputs(N);
        const want = oracleBars(fin, width, tickIdx);
        const got = barLayout(fin, width, asTicks(tickIdx));
        expect(got.paths).toEqual(want.bars);
        expect(got.ticks.map((t) => t.x)).toEqual(want.xLabels);
        expect(got.grid.map((g) => ({ y: g.y, ty: g.labelY, l: Number(g.label) }))).toEqual(
          want.yGrid,
        );
      });
    }
  }

  it("keeps a floor of four cards on the axis", () => {
    const layout = barLayout([1, 0, 2], 300, []);
    expect(layout.grid.map((g) => g.label)).toEqual(["0", "2", "4"]);
    expect(layout.paths[1]).toBe("");
  });
});

describe("lineLayout", () => {
  for (const N of RANGES) {
    for (const width of WIDTHS) {
      it(`matches the design for ${N} days at ${width} px`, () => {
        const { series, tickIdx } = inputs(N);
        const want = oracleLines(series, LIMIT, width, tickIdx);
        const got = lineLayout({
          series,
          limit: LIMIT,
          width,
          ticks: asTicks(tickIdx),
          format: (v) => `$${Math.round(v)}`,
        });
        expect(got.paths).toEqual(want.paths);
        expect(got.labelYs).toEqual(want.lys);
        expect(got.ticks.map((t) => t.x)).toEqual(want.xLabels);
        expect(got.grid.map((g) => ({ y: g.y, ty: g.labelY, l: g.label }))).toEqual(want.yGrid);
        expect(got.plotRight).toBe(want.plotR);
        expect(got.labelX).toBe(want.plotR + 6);
        expect(got.limitY).toBe(want.limitY);
        expect(got.limitLabelY).toBe(want.limitTY);
      });
    }
  }

  it("handles a single day without dividing by zero", () => {
    const layout = lineLayout({
      series: [[2]],
      limit: 4,
      width: 300,
      ticks: [],
      format: String,
    });
    expect(layout.paths[0]).toBe("M40.0 85.5");
  });
});

describe("roundedBarPath", () => {
  it("shrinks the corner radius on a short bar", () => {
    const d = roundedBarPath({ x: 10, y: 149, width: 8, radius: 3 });
    expect(d).toBe("M10 150 V150 Q10 149 11 149 H17 Q18 149 18 150 V150 Z");
  });

  it("draws nothing for an empty bar", () => {
    expect(roundedBarPath({ x: 0, y: 150, width: 4, radius: 2 })).toBe("");
  });
});

describe("yScale", () => {
  it("puts zero on the plot bottom and the maximum on its top", () => {
    const y = yScale(10);
    expect(y(0)).toBe(150);
    expect(y(10)).toBe(8);
    expect(y(5)).toBe(79);
  });
});

describe("spreadLabels", () => {
  it("pushes close labels apart and keeps the input order", () => {
    expect(spreadLabels([50, 10, 52, 90], 14)).toEqual([50, 10, 64, 90]);
  });

  it("leaves distant labels alone", () => {
    expect(spreadLabels([10, 40, 80], 14)).toEqual([10, 40, 80]);
  });
});

describe("lineStyle", () => {
  it("gives each position its style and reuses the last for extra series", () => {
    expect(lineStyle(0)).toBe(LINE_STYLES[0]);
    expect(lineStyle(3)).toBe(LINE_STYLES[3]);
    expect(lineStyle(9)).toBe(LINE_STYLES[3]);
  });
});
