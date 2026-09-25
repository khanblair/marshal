import { render } from "@solidjs/testing-library";
import { storeSource } from "./_design-source";
import { Icon, iconStrokeWidth } from "./Icon";
import { iconAliases } from "./icon-names";

const SVG_NS = "http://www.w3.org/2000/svg";

/** The prototype's own formula, copied from store.js, to check ours against. */
const prototypeStroke = (s: number) => ((((s <= 14 ? 1.75 : 1.5) * 24) / s) * 0.85).toFixed(2);

function svgOf(container: HTMLElement) {
  const svg = container.querySelector("svg");
  if (!svg) throw new Error("no svg rendered");
  return svg;
}

/** Parses a table like `const CUSTOM = { 'a': '...', ... };` out of store.js. */
function storeTable(name: string): Record<string, string> {
  const block = storeSource.match(new RegExp(`const ${name} = \\{([\\s\\S]*?)\\};`))?.[1] ?? "";
  return Object.fromEntries(
    [...block.matchAll(/'([^']+)':\s*'([^']*)'/g)].map((m) => [m[1], m[2]]),
  );
}

describe("Icon", () => {
  it("renders the m-icon wrapper: inline-flex box of the size, hidden from screen readers", () => {
    const { container } = render(() => <Icon name="bell" size={18} />);
    const span = container.firstElementChild as HTMLElement;
    expect(span.tagName).toBe("SPAN");
    expect(span).toHaveAttribute("aria-hidden", "true");
    expect(span).toHaveClass(
      "inline-flex",
      "flex-none",
      "items-center",
      "justify-center",
      "leading-[0]",
    );
    expect(span.style.width).toBe("18px");
    expect(span.style.height).toBe("18px");
  });

  it("defaults to 16 px", () => {
    const { container } = render(() => <Icon name="bell" />);
    expect(svgOf(container)).toHaveAttribute("width", "16");
    expect((container.firstElementChild as HTMLElement).style.width).toBe("16px");
  });

  it("draws Lucide icons with the design's svg attributes", () => {
    const { container } = render(() => <Icon name="file-search" size={14} />);
    const svg = svgOf(container);
    expect(svg).toHaveAttribute("viewBox", "0 0 24 24");
    expect(svg).toHaveAttribute("fill", "none");
    expect(svg).toHaveAttribute("stroke", "currentColor");
    expect(svg).toHaveAttribute("stroke-linecap", "round");
    expect(svg).toHaveAttribute("stroke-linejoin", "round");
    expect(svg).toHaveAttribute("width", "14");
    expect(svg).toHaveAttribute("height", "14");
    expect(svg).toHaveAttribute("stroke-width", prototypeStroke(14));
    expect(svg).toHaveClass("block");
    expect(svg.children.length).toBeGreaterThan(0);
  });

  it.each([10, 12, 14, 16, 18, 20, 24])("uses the prototype stroke formula at %i px", (size) => {
    expect(iconStrokeWidth(size)).toBe(prototypeStroke(size));
    const { container } = render(() => <Icon name="st-working" size={size} />);
    expect(svgOf(container)).toHaveAttribute("stroke-width", prototypeStroke(size));
  });

  it("passes class, style, and data attributes to the wrapper", () => {
    const { container } = render(() => (
      <Icon name="x" class="text-muted" data-testid="ic" style={{ color: "var(--color-ink)" }} />
    ));
    const span = container.firstElementChild as HTMLElement;
    expect(span).toHaveClass("text-muted", "inline-flex");
    expect(span).toHaveAttribute("data-testid", "ic");
    expect(span.style.color).toBe("var(--color-ink)");
  });

  it("draws the spinner as a ring three quarters of the box, spinning", () => {
    const { container } = render(() => <Icon name="spinner" size={14} />);
    expect(container.querySelector("svg")).toBeNull();
    const ring = container.querySelector("span > span") as HTMLElement;
    expect(ring.style.width).toBe("11px");
    expect(ring.style.height).toBe("11px");
    expect(ring).toHaveClass(
      "box-border",
      "rounded-full",
      "border-[1.5px]",
      "border-current",
      "border-r-transparent",
      "animate-spin-fast",
    );
  });

  it("draws the custom status flags exactly as the CUSTOM table in store.js", () => {
    const custom = storeTable("CUSTOM");
    expect(Object.keys(custom).sort()).toEqual([
      "st-backlog",
      "st-done",
      "st-planning",
      "st-working",
    ]);
    for (const [name, markup] of Object.entries(custom)) {
      const { container, unmount } = render(() => <Icon name={name} size={16} />);
      const svg = svgOf(container);
      const expected = document.createElementNS(SVG_NS, "svg");
      expected.innerHTML = markup;
      expect(svg.children.length).toBe(expected.children.length);
      Array.from(expected.children).forEach((child, i) => {
        expect(svg.children[i]?.isEqualNode(child)).toBe(true);
      });
      expect(svg).toHaveAttribute("stroke-width", prototypeStroke(16));
      unmount();
    }
  });

  it("maps the status aliases exactly as the ALIAS table in store.js", () => {
    expect(iconAliases).toEqual(storeTable("ALIAS"));
    const alias = render(() => <Icon name="st-needs" />);
    const target = render(() => <Icon name="hand" />);
    expect(svgOf(alias.container).outerHTML).toBe(svgOf(target.container).outerHTML);
  });

  it("draws an empty icon of the same size for an unknown name", () => {
    const { container } = render(() => <Icon name="not-an-icon" size={20} />);
    const svg = svgOf(container);
    expect(svg.children.length).toBe(0);
    expect(svg).toHaveAttribute("width", "20");
  });
});
