import { designFiles } from "./_design-source";
import { iconNames, isIconName, resolveIcon } from "./icon-names";
import { lucideIcons } from "./lucide-icons";

/** Every icon file in the pinned lucide-solid, by kebab-case name. */
const lucideFiles = new Set(
  Object.keys(import.meta.glob("../../node_modules/lucide-solid/dist/types/icons/*.d.ts")).map(
    (path) => (path.split("/").pop() ?? "").replace(/\.d\.ts$/, ""),
  ),
);

/**
 * Words in the design that happen to be Lucide icon names but are data, not
 * icons: message, activity, and feed kinds, menu and size keys, a card label,
 * and HTML attribute values (role, type, name).
 */
const DATA_WORDS = new Set([
  "bug",
  "command",
  "diff",
  "file",
  "filter",
  "folder",
  "group",
  "key",
  "menu",
  "merge",
  "option",
  "phone",
  "radio",
  "target",
  "text",
  "user",
]);

const CUSTOM_NAMES = /^st-|^spinner$/;
const KEBAB = /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/;
const texts = Object.values(designFiles);

/** Names in the places the design always puts icons: static m-icon names, `icon:` values, and tool calls. */
function strictNames(): Set<string> {
  const patterns = [
    /<m-icon name="([^"{]+)"/g,
    /\bicon:\s*'([^']+)'/g,
    /\bT\('([^']+)'/g,
    /\brunTool\([^,]+,\s*'([^']+)'/g,
  ];
  return new Set(
    texts.flatMap((t) => patterns.flatMap((p) => [...t.matchAll(p)].map((m) => m[1] ?? ""))),
  );
}

/** Every quoted kebab-case string in the design that names an icon. */
function designIconNames(): Set<string> {
  const quoted = texts.flatMap((t) =>
    [...t.matchAll(/'([a-z][a-z0-9-]*)'|"([a-z][a-z0-9-]*)"/g)].map((m) => m[1] ?? m[2] ?? ""),
  );
  return new Set(
    quoted.filter(
      (s) => KEBAB.test(s) && !DATA_WORDS.has(s) && (lucideFiles.has(s) || CUSTOM_NAMES.test(s)),
    ),
  );
}

describe("icon registry", () => {
  it("reads the design files", () => {
    expect(Object.keys(designFiles)).toContain("store.js");
    expect(Object.keys(designFiles)).toContain("Marshal.dc.html");
    expect(Object.keys(designFiles)).not.toContain("Marshal Showcase.dc.html");
    expect(lucideFiles.size).toBeGreaterThan(1000);
  });

  it("covers every icon name the design uses in an icon slot", () => {
    const missing = [...strictNames()].filter((name) => !isIconName(name));
    expect(missing).toEqual([]);
  });

  it("matches the full set of icon names found anywhere in the design", () => {
    expect([...iconNames].sort()).toEqual([...designIconNames()].sort());
  });

  it("only registers icons that exist in the pinned lucide-solid", () => {
    const absent = Object.keys(lucideIcons).filter((name) => !lucideFiles.has(name));
    expect(absent).toEqual([]);
  });

  it("resolves names by kind", () => {
    expect(resolveIcon("spinner")).toEqual({ kind: "spinner" });
    expect(resolveIcon("st-done")).toEqual({ kind: "glyph", name: "st-done" });
    expect(resolveIcon("st-ready")).toEqual({ kind: "lucide", name: "git-merge" });
    expect(resolveIcon("folder-git-2")).toEqual({ kind: "lucide", name: "folder-git-2" });
    expect(resolveIcon("toString")).toEqual({ kind: "unknown" });
    expect(isIconName("nope")).toBe(false);
  });
});
