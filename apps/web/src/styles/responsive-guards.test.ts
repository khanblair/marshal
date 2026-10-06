import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const SRC_DIRS = [join(__dirname, ".."), join(__dirname, "../../../../packages/ui/src")];

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    return /\.(tsx?|css)$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

const files = SRC_DIRS.flatMap(sourceFiles);
const offenders = (pattern: RegExp, allow: (path: string) => boolean = () => false): string[] =>
  files.filter((path) => !allow(path) && pattern.test(readFileSync(path, "utf8")));

describe("responsive rules", () => {
  it("reads safe areas only through the --safe-* variables", () => {
    expect(offenders(/env\(safe-area-inset/, (path) => path.endsWith("app.css"))).toEqual([]);
  });

  it("has no fixed 44 px minimum on touch controls", () => {
    expect(offenders(/min-(height|width):\s*44px/)).toEqual([]);
  });

  it("sizes no control by a phone ternary: the phone: variant and the density variables do", () => {
    const phoneTernary = /(M\.mobile|isPhone\(\)|isTouch\(\))\s*\?\s*["'`][^"'`]*\b(min-)?h-\d/;
    expect(offenders(phoneTernary)).toEqual([]);
  });

  it("gates no size prop or constant on the layout: the CSS does it", () => {
    expect(offenders(/size=\{[^}]*\b(isTouch|isPhone)\(\)[^}]*\?/)).toEqual([]);
    expect(
      offenders(/\bTOUCH_[A-Z_]*PX\b/, (path) => /(Button|IconButton)\.tsx$/.test(path)),
    ).toEqual([]);
  });

  it("draws no raw hex color: colors come from the tokens", () => {
    expect(
      offenders(/["']#[0-9a-fA-F]{3,8}["']/, (path) => /\/(mock|testing)\//.test(path)),
    ).toEqual([]);
  });

  // The theme defines no size for these, so the class compiles to nothing.
  it("uses no size or container utility the theme does not define", () => {
    const undefinedSize = /\b(max-w|min-w|w)-(3xs|2xs|xs|sm|md|lg|xl|[2-7]xl|prose)\b/;
    const undefinedContainer = /@(3xs|2xs|xs|sm|md|lg|xl|[2-7]xl):/;
    expect(offenders(undefinedSize)).toEqual([]);
    expect(offenders(undefinedContainer)).toEqual([]);
  });
});
