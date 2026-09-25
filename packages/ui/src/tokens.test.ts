import { themes } from "@marshal/tokens/themes";
import { staticColors, textSizes } from "@marshal/tokens/tokens";

/**
 * Every color class in the library must name a real token, so a typo such as
 * `bg-status-needs-subtle` fails here instead of silently drawing nothing.
 */
const sources = import.meta.glob(["./**/*.{ts,tsx}", "!./**/*.test.{ts,tsx}", "!./index.ts"], {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

/** Text color aliases that packages/tokens/scripts/build-css.ts adds. */
const TEXT_ALIASES = ["primary", "secondary", "muted", "inverse"];
/** Tailwind keywords that are colors without a token. */
const KEYWORDS = ["transparent", "current", "inherit"];

const colors = new Set([
  ...Object.keys(themes.light.colors).map((name) => name.replace(/^color-/, "")),
  ...Object.keys(themes.light.charts),
  ...Object.keys(staticColors),
  ...TEXT_ALIASES,
  ...KEYWORDS,
]);

/** Suffixes after a color prefix that are not colors: sizes, widths, styles, alignment. */
const NOT_COLORS = new Set([
  ...Object.keys(textSizes),
  ...["left", "right", "center", "none", "1", "2", "t", "b", "l", "r", "x", "y"],
]);
const COLOR_CLASS =
  /^(?:[a-z-]+:)*-?(?:bg|text|border(?!-separate\b|-collapse\b|-spacing)(?:-[trblxy])?|outline|accent)-([a-z0-9-]+)!?$/;

function classTokens(code: string): string[] {
  const literals = [...code.matchAll(/"([^"\n]*)"|`([^`]*)`/g)].map((m) => m[1] ?? m[2] ?? "");
  return literals.flatMap((text) => text.split(/\s+/)).filter(Boolean);
}

describe("color classes", () => {
  it("name tokens from @marshal/tokens", () => {
    const unknown: string[] = [];
    let checked = 0;
    for (const [file, code] of Object.entries(sources)) {
      for (const token of classTokens(code)) {
        const suffix = token.match(COLOR_CLASS)?.[1];
        if (!suffix || NOT_COLORS.has(suffix) || suffix.startsWith("offset")) continue;
        checked++;
        if (!colors.has(suffix)) unknown.push(`${file}: ${token}`);
      }
    }
    expect(unknown).toEqual([]);
    expect(checked).toBeGreaterThan(100);
  });

  it("catches a misspelled color", () => {
    expect(colors.has("status-needs-you-subtle")).toBe(true);
    expect(colors.has("status-needs-subtle")).toBe(false);
  });
});
